package websocket

import (
	"bytes"
	"log/slog"
	"net"
	"sync"
)

// BridgeOptions configures the bidirectional streaming bridge between a WebSocket and a UNIX socket.
type BridgeOptions struct {
	IsBinary bool // true for VNC (RFB), false for serial text console
	Logger   *slog.Logger
}

// Bridge streams data bidirectionally between a WebSocket connection and a UNIX domain socket.
func Bridge(ws *Conn, unixSockPath string, opts BridgeOptions) error {
	sock, err := net.Dial("unix", unixSockPath)
	if err != nil {
		return err
	}
	defer sock.Close()
	defer ws.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: WebSocket -> Unix Domain Socket
	go func() {
		defer wg.Done()
		defer sock.Close()
		defer ws.Close()

		for {
			opcode, payload, err := ws.ReadMessage()
			if err != nil {
				break
			}
			if (opcode == OpcodeText || opcode == OpcodeBinary) && len(payload) > 0 {
				trimmed := bytes.TrimSpace(payload)
				if bytes.HasPrefix(trimmed, []byte(`{`)) && bytes.Contains(trimmed, []byte(`"resize"`)) {
					continue
				}
				if _, writeErr := sock.Write(payload); writeErr != nil {
					break
				}
			}
		}
	}()

	// Goroutine 2: Unix Domain Socket -> WebSocket
	go func() {
		defer wg.Done()
		defer sock.Close()
		defer ws.Close()

		buf := make([]byte, 16384) // 16 KB buffer for high-throughput RFB frames
		outOpcode := OpcodeText
		if opts.IsBinary {
			outOpcode = OpcodeBinary
		}

		for {
			n, readErr := sock.Read(buf)
			if n > 0 {
				if writeErr := ws.WriteMessage(outOpcode, buf[:n]); writeErr != nil {
					break
				}
			}
			if readErr != nil {
				break
			}
		}
	}()

	wg.Wait()
	return nil
}
