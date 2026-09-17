package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileVM_CleanStop(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "clean-stop-vm",
		Name:       "Clean Stop VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// Create orphan sockets without PID file to test cleanup
	vmDir, err := mgr.Storage().VMDir("clean-stop-vm")
	if err != nil {
		t.Fatalf("failed to get VM dir: %v", err)
	}
	orphanSock := filepath.Join(vmDir, "qmp.sock")
	if err := os.WriteFile(orphanSock, []byte("fake-sock"), 0600); err != nil {
		t.Fatalf("failed to write fake sock: %v", err)
	}

	pid, state, err := ReconcileVM(mgr.Storage(), "clean-stop-vm")
	if err != nil {
		t.Fatalf("ReconcileVM failed: %v", err)
	}

	if pid != 0 {
		t.Errorf("expected PID 0, got %d", pid)
	}
	if state != StateStopped {
		t.Errorf("expected state StateStopped, got %s", state)
	}

	// Stale socket should have been removed
	if _, err := os.Stat(orphanSock); !os.IsNotExist(err) {
		t.Errorf("expected orphan socket to be removed")
	}
}

func TestReconcileVM_StalePID(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "stale-pid-vm",
		Name:       "Stale PID VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	vmDir, err := mgr.Storage().VMDir("stale-pid-vm")
	if err != nil {
		t.Fatalf("failed to get VM dir: %v", err)
	}

	pidFile := filepath.Join(vmDir, "qemu.pid")
	qmpSock := filepath.Join(vmDir, "qmp.sock")
	consoleSock := filepath.Join(vmDir, "console.sock")

	// Write an impossible/dead PID
	deadPID := 9999999
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", deadPID)), 0644); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}
	if err := os.WriteFile(qmpSock, []byte("fake"), 0600); err != nil {
		t.Fatalf("failed to write qmp sock: %v", err)
	}
	if err := os.WriteFile(consoleSock, []byte("fake"), 0600); err != nil {
		t.Fatalf("failed to write console sock: %v", err)
	}

	pid, state, err := ReconcileVM(mgr.Storage(), "stale-pid-vm")
	if err != nil {
		t.Fatalf("ReconcileVM failed: %v", err)
	}

	if pid != 0 {
		t.Errorf("expected PID 0 for stale process, got %d", pid)
	}
	if state != StateStopped {
		t.Errorf("expected state StateStopped, got %s", state)
	}

	// Verify all stale artifacts were cleaned
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("expected stale pid file to be deleted")
	}
	if _, err := os.Stat(qmpSock); !os.IsNotExist(err) {
		t.Errorf("expected stale qmp socket to be deleted")
	}
	if _, err := os.Stat(consoleSock); !os.IsNotExist(err) {
		t.Errorf("expected stale console socket to be deleted")
	}
}

func TestReconcileVM_CorruptPID(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "corrupt-pid-vm",
		Name:       "Corrupt PID VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	vmDir, err := mgr.Storage().VMDir("corrupt-pid-vm")
	if err != nil {
		t.Fatalf("failed to get VM dir: %v", err)
	}

	pidFile := filepath.Join(vmDir, "qemu.pid")
	if err := os.WriteFile(pidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	pid, state, err := ReconcileVM(mgr.Storage(), "corrupt-pid-vm")
	if err != nil {
		t.Fatalf("ReconcileVM failed: %v", err)
	}

	if pid != 0 || state != StateStopped {
		t.Errorf("expected pid=0, state=stopped; got pid=%d, state=%s", pid, state)
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("expected corrupt pid file to be deleted")
	}
}

func TestRecoverAll(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	// Create 2 VMs
	vm1 := VMConfig{
		ID:         "vm-alpha",
		Name:       "VM Alpha",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}
	vm2 := VMConfig{
		ID:         "vm-beta",
		Name:       "VM Beta",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(vm1); err != nil {
		t.Fatalf("failed to create vm1: %v", err)
	}
	if _, err := mgr.CreateVM(vm2); err != nil {
		t.Fatalf("failed to create vm2: %v", err)
	}

	// Create a stale PID file in vm2
	vm2Dir, _ := mgr.Storage().VMDir("vm-beta")
	_ = os.WriteFile(filepath.Join(vm2Dir, "qemu.pid"), []byte("9999999\n"), 0644)

	// Create a fresh manager instance pointing to the same storage to test startup recovery
	newMgr := NewManager(mgr.Storage(), nil)
	if err := newMgr.RecoverAll(); err != nil {
		t.Fatalf("RecoverAll failed: %v", err)
	}

	vms := newMgr.ListVMs()
	if len(vms) != 2 {
		t.Fatalf("expected 2 VMs recovered, got %d", len(vms))
	}

	for _, v := range vms {
		if v.Runtime.State != StateStopped {
			t.Errorf("expected VM %s to be stopped, got %s", v.Config.ID, v.Runtime.State)
		}
		if v.Runtime.PID != 0 {
			t.Errorf("expected VM %s to have PID 0, got %d", v.Config.ID, v.Runtime.PID)
		}
	}
}
