package qemu

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

var (
	// ErrConsoleSocketNotFound is returned when console.sock does not exist.
	ErrConsoleSocketNotFound = errors.New("serial console socket not found")
)

// DialConsole establishes a connection to the QEMU serial console Unix domain socket.
func DialConsole(sockPath string, timeout time.Duration) (net.Conn, error) {
	if _, err := os.Stat(sockPath); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConsoleSocketNotFound
		}
		return nil, fmt.Errorf("error accessing console socket: %w", err)
	}

	conn, err := net.DialTimeout("unix", sockPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to console socket: %w", err)
	}

	return conn, nil
}
