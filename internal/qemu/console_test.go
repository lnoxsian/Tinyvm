package qemu

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDialConsole(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tinyvm-console-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "console.sock")

	// 1. Dial non-existent socket -> ErrConsoleSocketNotFound
	_, err = DialConsole(sockPath, 50*time.Millisecond)
	if err != ErrConsoleSocketNotFound {
		t.Errorf("expected ErrConsoleSocketNotFound, got %v", err)
	}

	// 2. Start dummy unix listener
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create dummy unix listener: %v", err)
	}
	defer listener.Close()

	// 3. Dial existing socket -> success
	conn, err := DialConsole(sockPath, 1*time.Second)
	if err != nil {
		t.Fatalf("failed to dial existing console socket: %v", err)
	}
	_ = conn.Close()
}
