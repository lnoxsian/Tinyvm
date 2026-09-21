package vm

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPauseAndResumeVM(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping test")
	}

	cfg := VMConfig{
		ID:         "pause-resume-vm",
		Name:       "Pause Resume VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	if err := mgr.StartVM(cfg.ID); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Test Pause
	if err := mgr.PauseVM(cfg.ID); err != nil {
		t.Fatalf("failed to pause VM: %v", err)
	}
	v, err := mgr.GetVM(cfg.ID)
	if err != nil || v.Runtime.State != StatePaused {
		t.Fatalf("expected state %s, got %s", StatePaused, v.Runtime.State)
	}

	// Test Resume
	if err := mgr.ResumeVM(cfg.ID); err != nil {
		t.Fatalf("failed to resume VM: %v", err)
	}
	v, err = mgr.GetVM(cfg.ID)
	if err != nil || v.Runtime.State != StateRunning {
		t.Fatalf("expected state %s, got %s", StateRunning, v.Runtime.State)
	}

	// Pause again, then test StopVM directly from paused state
	if err := mgr.PauseVM(cfg.ID); err != nil {
		t.Fatalf("failed to pause VM: %v", err)
	}
	if err := mgr.StopVM(cfg.ID); err != nil {
		t.Fatalf("failed to stop VM from paused state: %v", err)
	}
	if mgr.IsVMRunning(cfg.ID) {
		t.Errorf("expected VM to be stopped")
	}

	_ = mgr.DeleteVM(cfg.ID)
}

func TestResetVM(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping test")
	}

	cfg := VMConfig{
		ID:         "reset-test-vm",
		Name:       "Reset Test VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	if err := mgr.StartVM(cfg.ID); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Test ResetVM
	if err := mgr.ResetVM(cfg.ID); err != nil {
		t.Fatalf("failed to reset VM: %v", err)
	}

	v, err := mgr.GetVM(cfg.ID)
	if err != nil || v.Runtime.State != StateRunning {
		t.Fatalf("expected state %s after reset, got %s", StateRunning, v.Runtime.State)
	}

	_ = mgr.StopVM(cfg.ID)
	_ = mgr.DeleteVM(cfg.ID)
}

func TestStopDuringStoppingState(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping test")
	}

	cfg := VMConfig{
		ID:         "stop-stopping-vm",
		Name:       "Stop Stopping VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	if err := mgr.StartVM(cfg.ID); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Simulate VM in StateStopping (e.g. user initiated graceful shutdown that guest ignored)
	mgr.mu.Lock()
	mgr.vms[cfg.ID].Runtime.State = StateStopping
	mgr.mu.Unlock()

	// StopVM should succeed when state is StateStopping, terminating process cleanly
	if err := mgr.StopVM(cfg.ID); err != nil {
		t.Fatalf("expected StopVM to succeed on StateStopping, got: %v", err)
	}

	if mgr.IsVMRunning(cfg.ID) {
		t.Errorf("expected VM to be stopped after StopVM")
	}

	_ = mgr.DeleteVM(cfg.ID)
}

func TestDiskResizeAndSnapshots(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "disk-snap-vm",
		Name:       "Disk Snap VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// 1. Test ResizeVMDisk
	if err := mgr.ResizeVMDisk(cfg.ID, "30M"); err != nil {
		t.Fatalf("failed to resize disk: %v", err)
	}
	v, _ := mgr.GetVM(cfg.ID)
	if v.Config.DiskSize != "30M" {
		t.Errorf("expected disk size 30M, got %s", v.Config.DiskSize)
	}

	// 2. Test Snapshots on stopped VM
	if err := mgr.CreateVMSnapshot(cfg.ID, "snapshot-1", "test description"); err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	snaps, err := mgr.ListVMSnapshots(cfg.ID)
	if err != nil {
		t.Fatalf("failed to list snapshots: %v", err)
	}
	if len(snaps) != 1 || snaps[0].Name != "snapshot-1" {
		t.Fatalf("expected 1 snapshot named 'snapshot-1', got %+v", snaps)
	}

	// 2b. Test snapshot operations blocked when VM is running
	mgr.mu.Lock()
	mgr.vms[cfg.ID].Runtime.State = StateRunning
	mgr.mu.Unlock()

	if err := mgr.CreateVMSnapshot(cfg.ID, "snap-running", ""); err == nil {
		t.Errorf("expected error creating snapshot on running VM, got nil")
	}
	if err := mgr.RollbackVMSnapshot(cfg.ID, "snapshot-1"); err == nil {
		t.Errorf("expected error rolling back snapshot on running VM, got nil")
	}
	if err := mgr.DeleteVMSnapshot(cfg.ID, "snapshot-1"); err == nil {
		t.Errorf("expected error deleting snapshot on running VM, got nil")
	}

	mgr.mu.Lock()
	mgr.vms[cfg.ID].Runtime.State = StateStopped
	mgr.mu.Unlock()

	// 3. Rollback snapshot
	if err := mgr.RollbackVMSnapshot(cfg.ID, "snapshot-1"); err != nil {
		t.Fatalf("failed to rollback snapshot: %v", err)
	}

	// 4. Delete snapshot
	if err := mgr.DeleteVMSnapshot(cfg.ID, "snapshot-1"); err != nil {
		t.Fatalf("failed to delete snapshot: %v", err)
	}

	snapsAfter, _ := mgr.ListVMSnapshots(cfg.ID)
	if len(snapsAfter) != 0 {
		t.Errorf("expected 0 snapshots after delete, got %d", len(snapsAfter))
	}

	_ = mgr.DeleteVM(cfg.ID)
}

func TestUpdateVMConfig(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "config-update-vm",
		Name:       "Config Update VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
		BootOrder:  "d",
		Autostart:  false,
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	cfg.CPUs = 2
	cfg.MemoryMB = 512
	cfg.BootOrder = "c"
	cfg.Autostart = true
	cfg.OSType = "Debian"

	if err := mgr.UpdateVMConfig(cfg.ID, cfg); err != nil {
		t.Fatalf("UpdateVMConfig failed: %v", err)
	}

	v, err := mgr.GetVM(cfg.ID)
	if err != nil {
		t.Fatalf("failed to get VM: %v", err)
	}
	if v.Config.CPUs != 2 || v.Config.MemoryMB != 512 || v.Config.BootOrder != "c" || !v.Config.Autostart || v.Config.OSType != "Debian" {
		t.Errorf("unexpected updated config: %+v", v.Config)
	}

	// Verify disk persistence
	var diskCfg VMConfig
	if err := mgr.Storage().ReadVMConfig(cfg.ID, &diskCfg); err != nil {
		t.Fatalf("failed to read persisted config: %v", err)
	}
	if diskCfg.CPUs != 2 || diskCfg.MemoryMB != 512 || diskCfg.BootOrder != "c" || !diskCfg.Autostart {
		t.Errorf("persisted config mismatch: %+v", diskCfg)
	}

	_ = mgr.DeleteVM(cfg.ID)
}

func TestCloneVM(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "source-vm",
		Name:       "Source Template VM",
		CPUs:       2,
		MemoryMB:   512,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
		BootOrder:  "c",
	}

	if _, err := mgr.CreateVM(cfg); err != nil {
		t.Fatalf("failed to create source VM: %v", err)
	}

	cloned, err := mgr.CloneVM("source-vm", "cloned-vm", "Cloned Instance")
	if err != nil {
		t.Fatalf("CloneVM failed: %v", err)
	}

	if cloned.Config.ID != "cloned-vm" || cloned.Config.Name != "Cloned Instance" {
		t.Errorf("cloned VM ID/Name mismatch: %+v", cloned.Config)
	}
	if cloned.Config.CPUs != 2 || cloned.Config.MemoryMB != 512 {
		t.Errorf("cloned VM hardware spec mismatch: %+v", cloned.Config)
	}

	// Verify cloned disk file exists
	clonedVMDir, err := mgr.Storage().VMDir("cloned-vm")
	if err != nil {
		t.Fatalf("failed to get cloned vm dir: %v", err)
	}
	clonedDisk := filepath.Join(clonedVMDir, cloned.Config.Disk)
	if _, err := os.Stat(clonedDisk); os.IsNotExist(err) {
		t.Fatalf("expected cloned disk to exist at %s", clonedDisk)
	}

	_ = mgr.DeleteVM("source-vm")
	_ = mgr.DeleteVM("cloned-vm")
}
