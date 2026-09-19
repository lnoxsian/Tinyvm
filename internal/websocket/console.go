package websocket

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// HandleConsole upgrades an HTTP request and bridges it to the VM's serial console.sock.
func HandleConsole(w http.ResponseWriter, r *http.Request, vmDir string, logger *slog.Logger) error {
	sockPath := filepath.Join(vmDir, "console.sock")
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		http.Error(w, "VM serial console socket unavailable (VM may be stopped)", http.StatusServiceUnavailable)
		return fmt.Errorf("console socket not found at %s", sockPath)
	}

	ws, err := Upgrade(w, r, nil)
	if err != nil {
		if logger != nil {
			logger.Warn("Failed to upgrade serial console WebSocket", "err", err)
		}
		return err
	}

	if logger != nil {
		logger.Info("Serial console WebSocket connected", "remote_addr", ws.RemoteAddr().String(), "socket", sockPath)
	}

	err = Bridge(ws, sockPath, BridgeOptions{
		IsBinary: false,
		Logger:   logger,
	})

	if logger != nil {
		logger.Info("Serial console WebSocket disconnected", "remote_addr", ws.RemoteAddr().String())
	}
	return err
}

// HandleVNC upgrades an HTTP request and bridges it to the VM's graphical vnc.sock (RFB protocol).
func HandleVNC(w http.ResponseWriter, r *http.Request, vmDir string, logger *slog.Logger) error {
	sockPath := filepath.Join(vmDir, "vnc.sock")
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		http.Error(w, "VM VNC socket unavailable (VM may be stopped or graphical display disabled)", http.StatusServiceUnavailable)
		return fmt.Errorf("vnc socket not found at %s", sockPath)
	}

	// noVNC requires "binary" subprotocol negotiation
	ws, err := Upgrade(w, r, []string{"binary"})
	if err != nil {
		if logger != nil {
			logger.Warn("Failed to upgrade VNC WebSocket", "err", err)
		}
		return err
	}

	if logger != nil {
		logger.Info("VNC graphical WebSocket connected", "remote_addr", ws.RemoteAddr().String(), "socket", sockPath, "subprotocol", ws.Subprotocol())
	}

	err = Bridge(ws, sockPath, BridgeOptions{
		IsBinary: true,
		Logger:   logger,
	})

	if logger != nil {
		logger.Info("VNC graphical WebSocket disconnected", "remote_addr", ws.RemoteAddr().String())
	}
	return err
}
