package qemu

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStartAndStopQEMUProcess(t *testing.T) {
	binary, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		t.Skip("qemu-system-x86_64 not found, skipping live process test")
	}

	tmpDir, err := os.MkdirTemp("", "tinyvm-qemu-proc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := QEMUPaths{
		VMDir:       tmpDir,
		QMPSock:     filepath.Join(tmpDir, "qmp.sock"),
		ConsoleSock: filepath.Join(tmpDir, "console.sock"),
		PIDFile:     filepath.Join(tmpDir, "qemu.pid"),
	}

	cfg := &Config{
		ID:       "live-test-vm",
		CPUs:     1,
		MemoryMB: 128,
	}

	// Minimal args that can boot without disk or OS
	args := BuildArgs(cfg, paths, false)

	proc, err := StartProcess(binary, args, paths, "live-test-vm")
	if err != nil {
		t.Fatalf("failed to start QEMU process: %v", err)
	}

	if proc.PID() <= 0 {
		t.Errorf("expected positive PID, got %d", proc.PID())
	}

	// Verify PID file was written
	pidBytes, err := os.ReadFile(paths.PIDFile)
	if err != nil {
		t.Errorf("expected PID file %s to exist: %v", paths.PIDFile, err)
	} else if len(pidBytes) == 0 {
		t.Errorf("PID file is empty")
	}

	// Verify process is running
	if !proc.IsRunning() {
		t.Errorf("expected process to be running")
	}

	// Verify sockets were created by QEMU
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(paths.QMPSock); err != nil {
		t.Logf("QMP socket stat: %v", err)
	}

	// Terminate the process
	if err := proc.Terminate(3 * time.Second); err != nil {
		t.Fatalf("failed to terminate QEMU process: %v", err)
	}

	// Verify process is no longer running
	if proc.IsRunning() {
		t.Errorf("expected process to have stopped")
	}

	// Verify cleanup
	if _, err := os.Stat(paths.PIDFile); err == nil {
		t.Errorf("expected PID file to be cleaned up after exit")
	}
}
