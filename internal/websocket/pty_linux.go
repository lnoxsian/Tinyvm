//go:build linux

package websocket

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// Winsize represents terminal window dimensions for TIOCSWINSZ.
type Winsize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// PTYSession encapsulates an active pseudo-terminal attached to an operating system command.
type PTYSession struct {
	pty *os.File
	cmd *exec.Cmd
}

// OpenPTY opens a new master/slave PTY pair using Linux /dev/ptmx.
func OpenPTY() (pty, tty *os.File, err error) {
	pty, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open /dev/ptmx: %w", err)
	}

	// TIOCGPTN: retrieve slave PTY index
	var ptn uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptn))); errno != 0 {
		_ = pty.Close()
		return nil, nil, fmt.Errorf("ioctl TIOCGPTN failed: %w", errno)
	}

	// TIOCSPTLCK: unlock slave PTY
	var unlock int32 = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, pty.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		_ = pty.Close()
		return nil, nil, fmt.Errorf("ioctl TIOCSPTLCK failed: %w", errno)
	}

	ptsName := fmt.Sprintf("/dev/pts/%d", ptn)
	tty, err = os.OpenFile(ptsName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = pty.Close()
		return nil, nil, fmt.Errorf("failed to open %s: %w", ptsName, err)
	}

	return pty, tty, nil
}

// SetWinsize updates the PTY dimensions.
func SetWinsize(fd uintptr, rows, cols uint16) error {
	type winsize struct {
		ws_row    uint16
		ws_col    uint16
		ws_xpixel uint16
		ws_ypixel uint16
	}
	ws := winsize{ws_row: rows, ws_col: cols}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws))); errno != 0 {
		return errno
	}
	return nil
}

// StartPTY launches an exec.Cmd connected to a newly allocated Linux PTY.
func StartPTY(cmd *exec.Cmd, initialRows, initialCols uint16) (*PTYSession, error) {
	pty, tty, err := OpenPTY()
	if err != nil {
		return nil, err
	}
	defer tty.Close()

	if initialRows > 0 && initialCols > 0 {
		_ = SetWinsize(pty.Fd(), initialRows, initialCols)
	}

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		_ = pty.Close()
		return nil, fmt.Errorf("failed to start command in PTY: %w", err)
	}

	return &PTYSession{
		pty: pty,
		cmd: cmd,
	}, nil
}

// File returns the master PTY descriptor for reading and writing.
func (s *PTYSession) File() *os.File {
	return s.pty
}

// Resize changes the terminal window dimensions.
func (s *PTYSession) Resize(rows, cols uint16) error {
	if s.pty == nil {
		return nil
	}
	return SetWinsize(s.pty.Fd(), rows, cols)
}

// Close terminates the child process and closes the PTY master descriptor.
func (s *PTYSession) Close() error {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.pty != nil {
		_ = s.pty.Close()
	}
	return nil
}

// Wait waits for the child process to exit.
func (s *PTYSession) Wait() error {
	if s.cmd != nil {
		return s.cmd.Wait()
	}
	return nil
}
