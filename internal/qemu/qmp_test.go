package qemu

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// mockQMPServer simulates a QEMU QMP server over a UNIX domain socket.
func mockQMPServer(t *testing.T, sockPath string) (net.Listener, chan struct{}) {
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create mock QMP socket: %v", err)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go handleMockQMPConn(l, conn)
		}
	}()

	return l, done
}

func handleMockQMPConn(l net.Listener, conn net.Conn) {
	defer conn.Close()

	// 1. Send Greeting
	greeting := `{"QMP": {"version": {"qemu": {"major": 8, "minor": 2, "micro": 0}, "package": ""}, "capabilities": []}}` + "\n"
	_, _ = conn.Write([]byte(greeting))

	dec := json.NewDecoder(conn)
	for {
		var req qmpRequest
		if err := dec.Decode(&req); err != nil {
			return
		}

		switch req.Execute {
		case "qmp_capabilities":
			_, _ = conn.Write([]byte(`{"return": {}}` + "\n"))
		case "query-status":
			_, _ = conn.Write([]byte(`{"return": {"running": true, "singlestep": false, "status": "running"}}` + "\n"))
		case "system_powerdown":
			// Emit an async event first, then the return value
			event := `{"event": "POWERDOWN", "timestamp": {"seconds": 1000, "microseconds": 0}}` + "\n"
			_, _ = conn.Write([]byte(event))
			time.Sleep(10 * time.Millisecond)
			_, _ = conn.Write([]byte(`{"return": {}}` + "\n"))
		case "quit":
			_, _ = conn.Write([]byte(`{"return": {}}` + "\n"))
			_ = l.Close()
			return
		case "unknown_cmd":
			_, _ = conn.Write([]byte(`{"error": {"class": "CommandNotFound", "desc": "Command not found"}}` + "\n"))
		default:
			_, _ = conn.Write([]byte(`{"return": {}}` + "\n"))
		}
	}
}

func TestQMPClient_MockServer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tinyvm-qmp-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "test-qmp.sock")
	listener, done := mockQMPServer(t, sockPath)
	defer listener.Close()

	client := NewQMPClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. Test Connect and Greeting
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	defer client.Close()

	greeting := client.Greeting()
	if greeting == nil || greeting.QMP.Version.QEMU.Major != 8 {
		t.Errorf("unexpected greeting: %+v", greeting)
	}

	// 2. Test QueryStatus
	status, err := client.QueryStatus()
	if err != nil {
		t.Fatalf("client.QueryStatus failed: %v", err)
	}
	if !status.Running || status.Status != "running" {
		t.Errorf("unexpected query-status result: %+v", status)
	}

	// 3. Test SystemPowerdown with Async Event Handling
	if err := client.SystemPowerdown(); err != nil {
		t.Fatalf("client.SystemPowerdown failed: %v", err)
	}

	// Verify asynchronous POWERDOWN event was captured
	select {
	case ev := <-client.Events():
		if ev.Event != "POWERDOWN" {
			t.Errorf("expected POWERDOWN event, got %s", ev.Event)
		}
	case <-time.After(500 * time.Millisecond):
		t.Errorf("timed out waiting for POWERDOWN event")
	}

	// 4. Test Error Response
	_, err = client.Execute("unknown_cmd", nil)
	if err == nil {
		t.Errorf("expected error for unknown_cmd, got nil")
	}
	var qmpErr *QMPError
	if !errors.As(err, &qmpErr) || qmpErr.Class != "CommandNotFound" {
		t.Errorf("expected QMPError with class CommandNotFound, got: %v", err)
	}

	// 5. Test Quit
	if err := client.Quit(); err != nil {
		t.Fatalf("client.Quit failed: %v", err)
	}

	select {
	case <-done:
		// server exited cleanly
	case <-time.After(1 * time.Second):
		t.Errorf("timed out waiting for server termination")
	}
}

func TestQMP_LiveQEMU(t *testing.T) {
	binary, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		t.Skip("qemu-system-x86_64 binary not available, skipping live test")
	}

	tmpDir, err := os.MkdirTemp("", "tinyvm-qmp-live-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "qmp.sock")
	pidFile := filepath.Join(tmpDir, "qemu.pid")

	paths := QEMUPaths{
		VMDir:       tmpDir,
		QMPSock:     sockPath,
		ConsoleSock: filepath.Join(tmpDir, "console.sock"),
		PIDFile:     pidFile,
	}

	cfg := &Config{
		ID:       "qmp-live-test",
		CPUs:     1,
		MemoryMB: 256,
	}

	args := BuildArgs(cfg, paths, false) // TCG fallback safe in test containers

	proc, err := StartProcess(binary, args, paths, "qmp-live-test")
	if err != nil {
		t.Fatalf("failed to start QEMU: %v", err)
	}
	defer proc.Kill()

	// Connect to live QEMU via QMP
	client := NewQMPClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect to live QEMU QMP: %v", err)
	}
	defer client.Close()

	// Query status
	status, err := client.QueryStatus()
	if err != nil {
		t.Fatalf("live QueryStatus failed: %v", err)
	}
	if !status.Running {
		t.Errorf("expected live QEMU to be running, got status: %s", status.Status)
	}

	// Send Quit via QMP
	if err := client.Quit(); err != nil {
		t.Fatalf("live QMP Quit failed: %v", err)
	}

	// Wait for process to exit
	select {
	case res := <-proc.ExitChan():
		if res.ExitCode != 0 {
			t.Logf("QEMU exit code: %d", res.ExitCode)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("timed out waiting for QEMU to exit after QMP quit")
	}
}

func TestQMP_OneShotHelpers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tinyvm-qmp-helpers-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sockPath := filepath.Join(tmpDir, "qmp.sock")

	// 1. Non-existent socket error test
	if err := QMPSystemPowerdown(sockPath, 1*time.Second); !errors.Is(err, ErrQMPSocketNotFound) {
		t.Errorf("expected ErrQMPSocketNotFound, got %v", err)
	}

	// 2. Start mock server and test helpers
	listener, done := mockQMPServer(t, sockPath)
	defer listener.Close()

	status, err := QMPQueryStatus(sockPath, 2*time.Second)
	if err != nil {
		t.Fatalf("QMPQueryStatus helper failed: %v", err)
	}
	if !status.Running {
		t.Errorf("expected status.Running = true")
	}

	if err := QMPSystemPowerdown(sockPath, 2*time.Second); err != nil {
		t.Fatalf("QMPSystemPowerdown helper failed: %v", err)
	}

	if err := QMPQuit(sockPath, 2*time.Second); err != nil {
		t.Fatalf("QMPQuit helper failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Errorf("server did not exit")
	}
}
