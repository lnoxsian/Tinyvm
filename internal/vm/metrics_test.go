package vm

import (
	"os"
	"testing"
	"time"
)

func TestParseDiskSizeBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"100M", 100 * 1024 * 1024},
		{"20G", 20 * 1024 * 1024 * 1024},
		{"1T", 1 * 1024 * 1024 * 1024 * 1024},
		{"512K", 512 * 1024},
		{"", 0},
		{"invalid", 0},
		{"-5G", 0},
	}

	for _, tt := range tests {
		got := ParseDiskSizeBytes(tt.input)
		if got != tt.expected {
			t.Errorf("ParseDiskSizeBytes(%q) = %d, expected %d", tt.input, got, tt.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{25 * time.Second, "25s"},
		{2*time.Minute + 15*time.Second, "2m 15s"},
		{3*time.Hour + 4*time.Minute + 10*time.Second, "3h 4m"},
		{-10 * time.Second, "0s"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.expected {
			t.Errorf("FormatDuration(%v) = %q, expected %q", tt.d, got, tt.expected)
		}
	}
}

func TestGetVMMetrics(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "metrics-test-vm",
		Name:       "Metrics Test VM",
		CPUs:       2,
		MemoryMB:   1024,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10G",
	}

	createdVM, err := mgr.CreateVM(cfg)
	if err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// 1. Metrics for stopped VM
	mStopped, err := mgr.GetVMMetrics(createdVM.Config.ID)
	if err != nil {
		t.Fatalf("GetVMMetrics returned error: %v", err)
	}
	if mStopped.Status != StateStopped {
		t.Errorf("expected state stopped, got %s", mStopped.Status)
	}
	if mStopped.DiskVirtualBytes != 10*1024*1024*1024 {
		t.Errorf("expected virtual disk 10GB, got %d", mStopped.DiskVirtualBytes)
	}

	// 2. Simulate running state with current test process PID
	mgr.mu.Lock()
	createdVM.Runtime.State = StateRunning
	createdVM.Runtime.PID = os.Getpid()
	createdVM.Runtime.StartedAt = time.Now().Add(-45 * time.Second)
	mgr.mu.Unlock()

	mRunning, err := mgr.GetVMMetrics(createdVM.Config.ID)
	if err != nil {
		t.Fatalf("GetVMMetrics returned error on running VM: %v", err)
	}
	if mRunning.Status != StateRunning {
		t.Errorf("expected state running, got %s", mRunning.Status)
	}
	if mRunning.PID != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), mRunning.PID)
	}
	if mRunning.UptimeSeconds < 40 {
		t.Errorf("expected uptime >= 40s, got %d", mRunning.UptimeSeconds)
	}
	if mRunning.MemoryRSSBytes == 0 {
		t.Logf("Process memory RSS reported 0 (non-Linux or permission restricted)")
	}
	if mRunning.CPUPercent < 0.0 || mRunning.CPUPercent > 100.0 {
		t.Errorf("expected CPUPercent between 0.0 and 100.0, got %f", mRunning.CPUPercent)
	}

	// 3. Non-existent VM
	_, err = mgr.GetVMMetrics("non-existent-vm-xyz")
	if err == nil {
		t.Errorf("expected error for non-existent VM, got nil")
	}
}

func TestGetHostMetrics(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "host-metrics-vm",
		Name:       "Host Metrics VM",
		CPUs:       1,
		MemoryMB:   512,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
	}

	_, err := mgr.CreateVM(cfg)
	if err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	hostMetrics := mgr.GetHostMetrics()
	if hostMetrics == nil {
		t.Fatal("expected non-nil HostMetrics")
	}
	if hostMetrics.CPUCores <= 0 {
		t.Errorf("expected positive CPUCores, got %d", hostMetrics.CPUCores)
	}
	if hostMetrics.TotalVMs != 1 {
		t.Errorf("expected TotalVMs=1, got %d", hostMetrics.TotalVMs)
	}
	if hostMetrics.StoppedVMs != 1 {
		t.Errorf("expected StoppedVMs=1, got %d", hostMetrics.StoppedVMs)
	}
	if len(hostMetrics.VMs) != 1 {
		t.Errorf("expected 1 VM in HostMetrics.VMs list, got %d", len(hostMetrics.VMs))
	}
}
