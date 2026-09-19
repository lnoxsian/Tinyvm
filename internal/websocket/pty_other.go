//go:build !linux

package websocket

import (
	"errors"
	"os"
	"os/exec"
)

// Winsize represents terminal window dimensions.
type Winsize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// PTYSession stub for non-Linux platforms.
type PTYSession struct{}

func (s *PTYSession) File() *os.File { return nil }
func (s *PTYSession) Resize(rows, cols uint16) error { return nil }
func (s *PTYSession) Close() error { return nil }
func (s *PTYSession) Wait() error { return nil }

// StartPTY returns an error on non-Linux platforms.
func StartPTY(cmd *exec.Cmd, initialRows, initialCols uint16) (*PTYSession, error) {
	return nil, errors.New("interactive PTY shell is only supported on Linux")
}
