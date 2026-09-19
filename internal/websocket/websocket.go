package websocket

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RFC 6455 WebSocket Opcodes.
const (
	OpcodeContinuation = 0x0
	OpcodeText         = 0x1
	OpcodeBinary       = 0x2
	OpcodeClose        = 0x8
	OpcodePing         = 0x9
	OpcodePong         = 0xA
)

const (
	wsGUID          = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	maxMessageBytes = 32 * 1024 * 1024 // 32 MB sanity limit
)

var (
	ErrNotWebSocket     = errors.New("request is not a valid WebSocket upgrade")
	ErrConnectionClosed = errors.New("websocket connection closed")
	ErrMessageTooLarge  = errors.New("websocket frame payload exceeds maximum allowed size")
)

// Conn represents an active RFC 6455 WebSocket connection.
type Conn struct {
	rwc          net.Conn
	br           *bufio.Reader
	bw           *bufio.Writer
	mu           sync.Mutex // guards writes and closed state
	closed       bool
	subprotocol  string
}

// Subprotocol returns the negotiated WebSocket subprotocol, if any.
func (c *Conn) Subprotocol() string {
	return c.subprotocol
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.rwc.RemoteAddr()
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.rwc.LocalAddr()
}

// Upgrade upgrades an incoming HTTP connection to a WebSocket connection.
func Upgrade(w http.ResponseWriter, r *http.Request, supportedProtocols []string) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, ErrNotWebSocket
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, ErrNotWebSocket
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, fmt.Errorf("%w: missing Sec-WebSocket-Key", ErrNotWebSocket)
	}

	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, fmt.Errorf("%w: unsupported Sec-WebSocket-Version", ErrNotWebSocket)
	}

	// Negotiate subprotocol if requested (e.g. "binary" for noVNC)
	var selectedProto string
	if clientProtos := r.Header.Get("Sec-WebSocket-Protocol"); clientProtos != "" {
		for _, cp := range strings.Split(clientProtos, ",") {
			trimmed := strings.TrimSpace(cp)
			for _, sp := range supportedProtocols {
				if strings.EqualFold(trimmed, sp) {
					selectedProto = sp
					break
				}
			}
			if selectedProto != "" {
				break
			}
		}
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("http server does not support connection hijacking")
	}

	netConn, brw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("failed to hijack connection: %w", err)
	}

	// Disable HTTP server read/write deadlines for persistent WebSocket stream
	_ = netConn.SetDeadline(time.Time{})

	// Calculate Sec-WebSocket-Accept token
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Write HTTP 101 Switching Protocols response
	res := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n",
		accept,
	)
	if selectedProto != "" {
		res += fmt.Sprintf("Sec-WebSocket-Protocol: %s\r\n", selectedProto)
	}
	res += "\r\n"

	if _, err := brw.WriteString(res); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("failed to write 101 handshake: %w", err)
	}
	if err := brw.Flush(); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("failed to flush handshake: %w", err)
	}

	return &Conn{
		rwc:         netConn,
		br:          brw.Reader,
		bw:          brw.Writer,
		subprotocol: selectedProto,
	}, nil
}

// ReadMessage reads the next data message (Text or Binary) from the connection.
// Control frames (Ping/Pong/Close) are handled automatically.
func (c *Conn) ReadMessage() (int, []byte, error) {
	var messageBuf []byte
	var messageOpcode int

	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(c.br, header); err != nil {
			return 0, nil, err
		}

		fin := (header[0] & 0x80) != 0
		opcode := int(header[0] & 0x0F)
		masked := (header[1] & 0x80) != 0
		lenField := uint64(header[1] & 0x7F)

		var payloadLen uint64
		switch lenField {
		case 126:
			lenBuf := make([]byte, 2)
			if _, err := io.ReadFull(c.br, lenBuf); err != nil {
				return 0, nil, err
			}
			payloadLen = uint64(binary.BigEndian.Uint16(lenBuf))
		case 127:
			lenBuf := make([]byte, 8)
			if _, err := io.ReadFull(c.br, lenBuf); err != nil {
				return 0, nil, err
			}
			payloadLen = binary.BigEndian.Uint64(lenBuf)
		default:
			payloadLen = lenField
		}

		if payloadLen > maxMessageBytes {
			_ = c.Close()
			return 0, nil, ErrMessageTooLarge
		}

		var maskKey [4]byte
		if masked {
			if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
				return 0, nil, err
			}
		}

		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(c.br, payload); err != nil {
				return 0, nil, err
			}
			if masked {
				for i := range payload {
					payload[i] ^= maskKey[i%4]
				}
			}
		}

		// Handle control frames
		switch opcode {
		case OpcodePing:
			_ = c.writeFrame(OpcodePong, payload)
			continue
		case OpcodePong:
			continue
		case OpcodeClose:
			_ = c.writeFrame(OpcodeClose, payload)
			_ = c.Close()
			return OpcodeClose, payload, io.EOF
		case OpcodeContinuation:
			if messageOpcode == 0 {
				return 0, nil, errors.New("unexpected continuation frame")
			}
			messageBuf = append(messageBuf, payload...)
			if fin {
				return messageOpcode, messageBuf, nil
			}
		case OpcodeText, OpcodeBinary:
			if !fin {
				messageOpcode = opcode
				messageBuf = append(messageBuf, payload...)
				continue
			}
			return opcode, payload, nil
		default:
			return 0, nil, fmt.Errorf("unknown websocket opcode: 0x%x", opcode)
		}
	}
}

// WriteMessage writes an unfragmented data frame (Text or Binary) to the connection.
func (c *Conn) WriteMessage(opcode int, payload []byte) error {
	return c.writeFrame(opcode, payload)
}

func (c *Conn) writeFrame(opcode int, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrConnectionClosed
	}

	length := len(payload)
	var header []byte

	b0 := byte(0x80 | (opcode & 0x0F)) // FIN=1

	if length < 126 {
		header = []byte{b0, byte(length)}
	} else if length <= 65535 {
		header = []byte{b0, 126, byte(length >> 8), byte(length & 0xFF)}
	} else {
		header = make([]byte, 10)
		header[0] = b0
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(length))
	}

	if _, err := c.bw.Write(header); err != nil {
		return err
	}
	if length > 0 {
		if _, err := c.bw.Write(payload); err != nil {
			return err
		}
	}
	return c.bw.Flush()
}

// Close gracefully closes the WebSocket connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Best-effort attempt to write empty close frame
	_, _ = c.bw.Write([]byte{0x88, 0x00})
	_ = c.bw.Flush()
	return c.rwc.Close()
}
