//go:build linux

package websocket

import (
	"bufio"
	"net"
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

func TestPTY_BridgeResizeDropped(t *testing.T) {
	cmd := exec.Command("cat")
	session, err := StartPTY(cmd, 24, 80)
	if err != nil {
		t.Fatalf("StartPTY failed: %v", err)
	}
	defer session.Close()

	c1, c2 := net.Pipe()
	ws := &Conn{
		rwc: c1,
		br:  bufio.NewReader(c1),
		bw:  bufio.NewWriter(c1),
	}

	go func() {
		_ = BridgePTY(ws, session, nil)
	}()

	// Send concatenated resize commands from the client end c2
	resizePayload := []byte(`{"type":"resize","cols":226,"rows":46}{"type":"resize","cols":226,"rows":46}`)
	if err := clientSendFrame(c2, OpcodeText, resizePayload); err != nil {
		t.Fatalf("send resize frame failed: %v", err)
	}

	// Send an actual input line
	if err := clientSendFrame(c2, OpcodeText, []byte("echo-me\n")); err != nil {
		t.Fatalf("send input frame failed: %v", err)
	}

	// Read output from c2
	op, payload, err := clientReadFrame(c2)
	if err != nil {
		t.Fatalf("clientReadFrame failed: %v", err)
	}
	if op != OpcodeText {
		t.Errorf("expected OpcodeText, got %d", op)
	}
	received := string(payload)
	if strings.Contains(received, "resize") {
		t.Errorf("resize JSON command leaked to PTY output: %q", received)
	}
	if !strings.Contains(received, "echo-me") {
		t.Errorf("expected 'echo-me' in output, got %q", received)
	}
	_ = c2.Close()
}
