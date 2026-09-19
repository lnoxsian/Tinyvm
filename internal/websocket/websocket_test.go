package websocket

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// clientSendFrame is a helper for unit testing to send a masked client-to-server frame.
func clientSendFrame(w io.Writer, opcode int, payload []byte) error {
	var header []byte
	b0 := byte(0x80 | (opcode & 0x0F))
	l := len(payload)

	maskKey := []byte{0x12, 0x34, 0x56, 0x78}

	if l < 126 {
		header = append(header, b0, byte(0x80|l))
	} else if l <= 65535 {
		header = append(header, b0, byte(0x80|126), byte(l>>8), byte(l&0xFF))
	} else {
		header = append(header, b0, byte(0x80|127))
		var lenBuf [8]byte
		binary.BigEndian.PutUint64(lenBuf[:], uint64(l))
		header = append(header, lenBuf[:]...)
	}
	header = append(header, maskKey...)

	maskedPayload := make([]byte, l)
	for i := range payload {
		maskedPayload[i] = payload[i] ^ maskKey[i%4]
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(maskedPayload); err != nil {
		return err
	}
	return nil
}

// clientReadFrame reads an unmasked server-to-client frame.
func clientReadFrame(r io.Reader) (int, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	opcode := int(header[0] & 0x0F)
	lenField := uint64(header[1] & 0x7F)

	var payloadLen uint64
	switch lenField {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = binary.BigEndian.Uint64(buf[:])
	default:
		payloadLen = lenField
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return opcode, payload, nil
}

func TestWebSocket_HandshakeAndEcho(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := Upgrade(w, r, []string{"binary"})
		if err != nil {
			t.Errorf("server upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		if ws.Subprotocol() != "binary" {
			t.Errorf("expected subprotocol 'binary', got '%s'", ws.Subprotocol())
		}

		for {
			opcode, payload, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if err := ws.WriteMessage(opcode, payload); err != nil {
				return
			}
		}
	}))
	defer ts.Close()

	// Connect TCP client
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect to test server: %v", err)
	}
	defer conn.Close()

	rawKey := make([]byte, 16)
	_, _ = rand.Read(rawKey)
	key := base64.StdEncoding.EncodeToString(rawKey)

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + ts.Listener.Addr().String() + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Protocol: binary\r\n\r\n"

	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("failed to send handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		t.Fatalf("expected 101 Switching Protocols, got %s", statusLine)
	}

	// Consume remaining HTTP headers
	for {
		line, err := br.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}

	// Test 1: Send text frame
	testText := "hello tinyvm websocket"
	if err := clientSendFrame(conn, OpcodeText, []byte(testText)); err != nil {
		t.Fatalf("client failed to send text frame: %v", err)
	}

	op, resp, err := clientReadFrame(br)
	if err != nil {
		t.Fatalf("client failed to read echo text frame: %v", err)
	}
	if op != OpcodeText || string(resp) != testText {
		t.Fatalf("expected echoed text %q, got op=%d payload=%q", testText, op, string(resp))
	}

	// Test 2: Send binary frame
	testBin := []byte{0x00, 0x01, 0x02, 0xFE, 0xFF}
	if err := clientSendFrame(conn, OpcodeBinary, testBin); err != nil {
		t.Fatalf("client failed to send binary frame: %v", err)
	}

	op, resp, err = clientReadFrame(br)
	if err != nil {
		t.Fatalf("client failed to read echo binary frame: %v", err)
	}
	if op != OpcodeBinary || len(resp) != len(testBin) {
		t.Fatalf("binary echo mismatch")
	}

	// Test 3: Send Ping frame
	pingPayload := []byte("ping-data")
	if err := clientSendFrame(conn, OpcodePing, pingPayload); err != nil {
		t.Fatalf("failed to send ping: %v", err)
	}

	op, resp, err = clientReadFrame(br)
	if err != nil {
		t.Fatalf("failed to read pong: %v", err)
	}
	if op != OpcodePong || string(resp) != "ping-data" {
		t.Fatalf("expected pong with payload %q, got op=%d payload=%q", pingPayload, op, resp)
	}
}

func TestWebSocket_BridgeToUnixSocket(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := filepath.Join(tempDir, "mock.sock")

	// Start a mock UNIX domain socket server
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		// Echo back with prefix
		_, _ = conn.Write(append([]byte("PONG:"), buf[:n]...))
	}()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		_ = Bridge(ws, sockPath, BridgeOptions{IsBinary: false})
	}))
	defer ts.Close()

	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("tcp dial failed: %v", err)
	}
	defer conn.Close()

	key := "dGhlIHNhbXBsZSBub25jZQ=="
	req := "GET / HTTP/1.1\r\n" +
		"Host: " + ts.Listener.Addr().String() + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	_, _ = conn.Write([]byte(req))
	br := bufio.NewReader(conn)
	for {
		line, _ := br.ReadString('\n')
		if line == "\r\n" {
			break
		}
	}

	// Send message through WebSocket
	_ = clientSendFrame(conn, OpcodeText, []byte("PING"))

	op, payload, err := clientReadFrame(br)
	if err != nil {
		t.Fatalf("failed reading bridge response: %v", err)
	}
	if op != OpcodeText || string(payload) != "PONG:PING" {
		t.Fatalf("expected PONG:PING from socket bridge, got %q", string(payload))
	}
}

func TestWebSocket_HandleSocketMissing(t *testing.T) {
	tempDir := t.TempDir()
	req := httptest.NewRequest("GET", "/api/v1/vms/test/console", nil)
	w := httptest.NewRecorder()

	err := HandleConsole(w, req, tempDir, nil)
	if err == nil {
		t.Fatalf("expected error when console.sock is missing")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	err = HandleVNC(w2, req, tempDir, nil)
	if err == nil {
		t.Fatalf("expected error when vnc.sock is missing")
	}
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", w2.Code)
	}
}

func TestWebSocket_SecurityLimits(t *testing.T) {
	// Reject non-websocket request
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	_, err := Upgrade(w, req, nil)
	if err == nil || !strings.Contains(err.Error(), "not a valid WebSocket") {
		t.Errorf("expected ErrNotWebSocket, got %v", err)
	}
}
