//go:build linux

package websocket

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPTY_StartAndRead(t *testing.T) {
	cmd := exec.Command("echo", "hello from pty")
	session, err := StartPTY(cmd, 24, 80)
	if err != nil {
		t.Fatalf("StartPTY failed: %v", err)
	}
	defer session.Close()

	buf := make([]byte, 128)
	n, err := session.File().Read(buf)
	if err != nil {
		t.Fatalf("session.File().Read failed: %v", err)
	}

	out := string(buf[:n])
	if !strings.Contains(out, "hello from pty") {
		t.Errorf("expected 'hello from pty' in output, got %q", out)
	}

	if err := session.Resize(40, 100); err != nil {
		t.Errorf("session.Resize failed: %v", err)
	}
}

func TestPTY_InteractiveEcho(t *testing.T) {
	cmd := exec.Command("cat")
	session, err := StartPTY(cmd, 24, 80)
	if err != nil {
		t.Fatalf("StartPTY failed: %v", err)
	}
	defer session.Close()

	input := "interactive test\n"
	if _, err := session.File().Write([]byte(input)); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	buf := make([]byte, 128)
	n, err := session.File().Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if !strings.Contains(string(buf[:n]), "interactive test") {
		t.Errorf("expected echo 'interactive test', got %q", string(buf[:n]))
	}
}
