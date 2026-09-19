package websocket

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
)

// BridgePTY streams data bidirectionally between a WebSocket connection and a PTYSession,
// while handling terminal window resize control frames from xterm.js.
func BridgePTY(ws *Conn, session *PTYSession, logger *slog.Logger) error {
	defer session.Close()
	defer ws.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: WebSocket -> PTY (keystrokes and resize commands)
	go func() {
		defer wg.Done()
		defer session.Close()
		defer ws.Close()

		for {
			opcode, payload, err := ws.ReadMessage()
			if err != nil {
				break
			}
			if opcode == OpcodeText || opcode == OpcodeBinary {
				if len(payload) == 0 {
					continue
				}

				// Intercept window resize commands formatted as JSON
				if bytes.HasPrefix(payload, []byte(`{"type":"resize"`)) {
					var r struct {
						Cols uint16 `json:"cols"`
						Rows uint16 `json:"rows"`
					}
					if err := json.Unmarshal(payload, &r); err == nil && r.Cols > 0 && r.Rows > 0 {
						_ = session.Resize(r.Rows, r.Cols)
						continue
					}
				}

				// Forward user input bytes to PTY
				if _, writeErr := session.File().Write(payload); writeErr != nil {
					break
				}
			}
		}
	}()

	// Goroutine 2: PTY -> WebSocket (output bytes to xterm.js)
	go func() {
		defer wg.Done()
		defer session.Close()
		defer ws.Close()

		buf := make([]byte, 4096)
		for {
			n, readErr := session.File().Read(buf)
			if n > 0 {
				if writeErr := ws.WriteMessage(OpcodeText, buf[:n]); writeErr != nil {
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
