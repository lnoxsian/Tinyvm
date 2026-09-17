package vm

import (
	"os"
	"testing"
	"time"
)

func TestRestartAndShutdownVM(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping test")
	}

	cfg := VMConfig{
		ID:         "restart-test-vm",
		Name:       "Restart Test VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "20M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// 1. Start VM
	if err := mgr.StartVM("restart-test-vm"); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}

	firstPID := mgr.vms["restart-test-vm"].Runtime.PID
	if firstPID <= 0 {
		t.Fatalf("expected valid PID, got %d", firstPID)
	}

	// 2. Restart VM
	if err := mgr.RestartVM("restart-test-vm"); err != nil {
		t.Fatalf("failed to restart VM: %v", err)
	}

	secondPID := mgr.vms["restart-test-vm"].Runtime.PID
	if secondPID <= 0 {
		t.Fatalf("expected valid second PID, got %d", secondPID)
	}
	if secondPID == firstPID {
		t.Errorf("expected new PID after restart, got same PID %d", secondPID)
	}
	if !mgr.IsVMRunning("restart-test-vm") {
		t.Errorf("expected VM to be running after restart")
	}

	// 3. StatusVM check
	st, err := mgr.StatusVM("restart-test-vm")
	if err != nil {
		t.Fatalf("failed to get detailed status: %v", err)
	}
	if st.Runtime.State != StateRunning {
		t.Errorf("expected running state in status, got %s", st.Runtime.State)
	}
	if st.Runtime.PID != secondPID {
		t.Errorf("expected PID %d, got %d", secondPID, st.Runtime.PID)
	}

	// 4. Stop VM
	if err := mgr.StopVM("restart-test-vm"); err != nil {
		t.Fatalf("failed to stop VM: %v", err)
	}
	if mgr.IsVMRunning("restart-test-vm") {
		t.Errorf("expected VM to be stopped")
	}

	// 5. Delete VM
	if err := mgr.DeleteVM("restart-test-vm"); err != nil {
		t.Fatalf("failed to delete VM: %v", err)
	}
}

func TestLifecycle_ConcurrencySafety(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping test")
	}

	cfg := VMConfig{
		ID:         "concurrency-vm",
		Name:       "Concurrency VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "20M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// Start VM
	if err := mgr.StartVM("concurrency-vm"); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}
	defer mgr.StopVM("concurrency-vm")

	// 1. Test start + start race rejection
	if err := mgr.StartVM("concurrency-vm"); err != ErrVMAlreadyRunning {
		t.Errorf("expected ErrVMAlreadyRunning on double start, got %v", err)
	}

	// 2. Test delete running VM rejection
	if err := mgr.DeleteVM("concurrency-vm"); err != ErrVMAlreadyRunning {
		t.Errorf("expected ErrVMAlreadyRunning when attempting to delete running VM, got %v", err)
	}

	// Stop VM
	if err := mgr.StopVM("concurrency-vm"); err != nil {
		t.Fatalf("failed to stop VM: %v", err)
	}

	// 3. Stop on stopped VM rejection
	if err := mgr.StopVM("concurrency-vm"); err != ErrVMNotRunning {
		t.Errorf("expected ErrVMNotRunning on stopped VM, got %v", err)
	}

	// 4. Clean delete once stopped succeeds
	if err := mgr.DeleteVM("concurrency-vm"); err != nil {
		t.Errorf("expected successful deletion of stopped VM, got %v", err)
	}
}

func TestQMP_StatusAndQuitVM(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping test")
	}

	cfg := VMConfig{
		ID:         "qmp-test-vm",
		Name:       "QMP Test VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// 1. Start VM
	if err := mgr.StartVM("qmp-test-vm"); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}

	// Give QEMU a brief moment to initialize sockets
	time.Sleep(100 * time.Millisecond)

	// 2. Query status via QMP
	st, err := mgr.StatusVM("qmp-test-vm")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if st.QMPStatus != "running" {
		t.Errorf("expected QMP guest status 'running', got '%s'", st.QMPStatus)
	}

	// 3. Clean Quit via QMP
	if err := mgr.QuitVM("qmp-test-vm"); err != nil {
		t.Fatalf("failed to quit VM via QMP: %v", err)
	}

	if mgr.IsVMRunning("qmp-test-vm") {
		t.Errorf("expected VM to be stopped after QMP quit")
	}

	// 4. Clean deletion
	if err := mgr.DeleteVM("qmp-test-vm"); err != nil {
		t.Fatalf("failed to delete VM: %v", err)
	}
}

