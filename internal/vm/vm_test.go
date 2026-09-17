package vm

import (
	"os"
	"path/filepath"
	"testing"

	"tinyvm/internal/storage"
)

func newTestManager(t *testing.T) (*Manager, string) {
	tmpDir, err := os.MkdirTemp("", "tinyvm-vm-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	s, err := storage.New(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to initialize storage: %v", err)
	}

	mgr := NewManager(s, nil)
	return mgr, tmpDir
}

// TestCreateVM_WithoutStartingQEMU satisfies the Phase 2 milestone:
// Test creating a VM without starting QEMU.
func TestCreateVM_WithoutStartingQEMU(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	cfg := VMConfig{
		ID:         "ubuntu-server",
		Name:       "Ubuntu Server Test",
		CPUs:       2,
		MemoryMB:   2048,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "100M",
		ISO:        "ubuntu-24.04.iso",
		Network: NetworkConfig{
			Enabled: true,
			Mode:    "user",
			SSHPort: 2222,
			Ports: []PortForward{
				{Host: 8080, Guest: 80, Protocol: "tcp"},
			},
		},
	}

	// Create VM
	vm, err := mgr.CreateVM(cfg)
	if err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	if vm.Config.ID != "ubuntu-server" {
		t.Errorf("expected VM ID 'ubuntu-server', got '%s'", vm.Config.ID)
	}
	if vm.Runtime.State != StateStopped {
		t.Errorf("expected initial state to be 'stopped', got '%s'", vm.Runtime.State)
	}
	if vm.Runtime.PID != 0 {
		t.Errorf("expected initial PID to be 0, got %d", vm.Runtime.PID)
	}

	// Verify on-disk storage layout
	vmDir := filepath.Join(tmpDir, "vms", "ubuntu-server")
	if info, err := os.Stat(vmDir); err != nil || !info.IsDir() {
		t.Fatalf("expected VM directory %s to exist", vmDir)
	}

	// Verify config.json
	var loadedCfg VMConfig
	if err := mgr.Storage().ReadVMConfig("ubuntu-server", &loadedCfg); err != nil {
		t.Fatalf("failed to read on-disk config.json: %v", err)
	}
	if loadedCfg.CPUs != 2 || loadedCfg.MemoryMB != 2048 || loadedCfg.Network.SSHPort != 2222 {
		t.Errorf("loaded config does not match: %+v", loadedCfg)
	}

	// Verify disk.qcow2 created by qemu-img
	diskPath := filepath.Join(vmDir, "disk.qcow2")
	diskInfo, err := storage.InspectDisk(diskPath)
	if err != nil {
		t.Fatalf("failed to inspect created QCOW2 disk: %v", err)
	}
	if diskInfo.Format != "qcow2" {
		t.Errorf("expected disk format 'qcow2', got '%s'", diskInfo.Format)
	}
	expectedVirtualSize := int64(100 * 1024 * 1024)
	if diskInfo.VirtualSize != expectedVirtualSize {
		t.Errorf("expected virtual size %d, got %d", expectedVirtualSize, diskInfo.VirtualSize)
	}

	// Test GetVM and ListVMs
	retrieved, err := mgr.GetVM("ubuntu-server")
	if err != nil {
		t.Fatalf("failed to retrieve VM: %v", err)
	}
	if retrieved.Config.Name != "Ubuntu Server Test" {
		t.Errorf("expected name 'Ubuntu Server Test', got '%s'", retrieved.Config.Name)
	}

	list := mgr.ListVMs()
	if len(list) != 1 {
		t.Errorf("expected 1 VM in list, got %d", len(list))
	}

	// Test VM Discovery / Recovery on startup
	mgr2 := NewManager(mgr.Storage(), nil)
	recovered, err := mgr2.GetVM("ubuntu-server")
	if err != nil {
		t.Fatalf("failed to discover VM after manager reload: %v", err)
	}
	if recovered.Runtime.State != StateStopped {
		t.Errorf("expected discovered VM state to be 'stopped', got '%s'", recovered.Runtime.State)
	}

	// Test DeleteVM
	if err := mgr.DeleteVM("ubuntu-server"); err != nil {
		t.Fatalf("failed to delete VM: %v", err)
	}

	if mgr.Storage().VMExists("ubuntu-server") {
		t.Errorf("expected VM storage to be deleted")
	}
	if _, err := mgr.GetVM("ubuntu-server"); err != ErrVMNotFoundInMgr {
		t.Errorf("expected ErrVMNotFoundInMgr, got %v", err)
	}
}

// TestStartAndStopVM_WithQEMU starts a real VM using QEMU/KVM and verifies PID tracking and stopping.
func TestStartAndStopVM_WithQEMU(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping live VM test")
	}

	cfg := VMConfig{
		ID:         "live-qemu-test",
		Name:       "Live QEMU Test VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "20M",
	}

	// 1. Create VM
	vm, err := mgr.CreateVM(cfg)
	if err != nil {
		t.Fatalf("failed to create VM: %v", err)
	}

	// 2. Start VM
	if err := mgr.StartVM("live-qemu-test"); err != nil {
		t.Fatalf("failed to start VM: %v", err)
	}

	// 3. Verify process is tracked and running
	if !mgr.IsVMRunning("live-qemu-test") {
		t.Errorf("expected VM to be running")
	}
	if vm.Runtime.State != StateRunning {
		t.Errorf("expected runtime state 'running', got '%s'", vm.Runtime.State)
	}
	if vm.Runtime.PID <= 0 {
		t.Errorf("expected positive PID, got %d", vm.Runtime.PID)
	}

	// Verify duplicate start is rejected
	if err := mgr.StartVM("live-qemu-test"); err != ErrVMAlreadyRunning {
		t.Errorf("expected ErrVMAlreadyRunning, got %v", err)
	}

	// Verify log file was written
	vmDir := filepath.Join(tmpDir, "vms", "live-qemu-test")
	logPath := filepath.Join(vmDir, "logs", "qemu.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("expected QEMU log file to exist at %s: %v", logPath, err)
	}

	// 4. Stop VM
	if err := mgr.StopVM("live-qemu-test"); err != nil {
		t.Fatalf("failed to stop VM: %v", err)
	}

	if mgr.IsVMRunning("live-qemu-test") {
		t.Errorf("expected VM to be stopped")
	}
	if vm.Runtime.State != StateStopped {
		t.Errorf("expected runtime state 'stopped', got '%s'", vm.Runtime.State)
	}
	if vm.Runtime.PID != 0 {
		t.Errorf("expected PID 0 after stop, got %d", vm.Runtime.PID)
	}

	// 5. Clean deletion
	if err := mgr.DeleteVM("live-qemu-test"); err != nil {
		t.Fatalf("failed to delete stopped VM: %v", err)
	}
}

func TestVMConfig_Validation(t *testing.T) {
	cases := []struct {
		name    string
		modify  func(*VMConfig)
		wantErr bool
	}{
		{
			name:    "valid",
			modify:  func(c *VMConfig) {},
			wantErr: false,
		},
		{
			name: "invalid ID with space",
			modify: func(c *VMConfig) {
				c.ID = "invalid id"
			},
			wantErr: true,
		},
		{
			name: "invalid CPUs (0)",
			modify: func(c *VMConfig) {
				c.CPUs = 0
			},
			wantErr: true,
		},
		{
			name: "invalid Memory (< 128MB)",
			modify: func(c *VMConfig) {
				c.MemoryMB = 64
			},
			wantErr: true,
		},
		{
			name: "invalid Port (> 65535)",
			modify: func(c *VMConfig) {
				c.Network.SSHPort = 70000
			},
			wantErr: true,
		},
		{
			name: "duplicate Host Port",
			modify: func(c *VMConfig) {
				c.Network.SSHPort = 2222
				c.Network.Ports = []PortForward{
					{Host: 2222, Guest: 80, Protocol: "tcp"},
				}
			},
			wantErr: true,
		},
		{
			name: "valid firmware (uefi)",
			modify: func(c *VMConfig) {
				c.Firmware = "uefi"
			},
			wantErr: false,
		},
		{
			name: "valid firmware (bios)",
			modify: func(c *VMConfig) {
				c.Firmware = "bios"
			},
			wantErr: false,
		},
		{
			name: "invalid firmware",
			modify: func(c *VMConfig) {
				c.Firmware = "openfirmware"
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := VMConfig{
				ID:         "test-vm",
				Name:       "Test VM",
				CPUs:       1,
				MemoryMB:   512,
				Disk:       "disk.qcow2",
				DiskFormat: "qcow2",
			}
			tc.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestCreateAndStartVM_UEFI(t *testing.T) {
	mgr, tmpDir := newTestManager(t)
	defer os.RemoveAll(tmpDir)

	if mgr.Launcher() == nil {
		t.Skip("QEMU launcher not available, skipping UEFI live test")
	}

	cfg := VMConfig{
		ID:         "uefi-live-test",
		Name:       "UEFI Live Test VM",
		CPUs:       1,
		MemoryMB:   256,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskSize:   "10M",
		Firmware:   "uefi",
	}

	vm, err := mgr.CreateVM(cfg)
	if err != nil {
		t.Fatalf("failed to create UEFI VM: %v", err)
	}

	if vm.Config.Firmware != "uefi" {
		t.Errorf("expected firmware 'uefi', got '%s'", vm.Config.Firmware)
	}

	// Verify NVRAM vars file created in VM directory
	vmDir := filepath.Join(tmpDir, "vms", "uefi-live-test")
	varsFile := filepath.Join(vmDir, "efivars.fd")
	if _, err := os.Stat(varsFile); err != nil {
		t.Errorf("expected efivars.fd to be created in VM dir: %v", err)
	}

	// Start UEFI VM
	if err := mgr.StartVM("uefi-live-test"); err != nil {
		t.Fatalf("failed to start UEFI VM: %v", err)
	}

	st, err := mgr.StatusVM("uefi-live-test")
	if err != nil {
		t.Fatalf("failed to get status for UEFI VM: %v", err)
	}
	if st.Firmware != "uefi" {
		t.Errorf("expected status firmware 'uefi', got '%s'", st.Firmware)
	}

	// Stop UEFI VM
	if err := mgr.StopVM("uefi-live-test"); err != nil {
		t.Fatalf("failed to stop UEFI VM: %v", err)
	}

	// Delete VM
	if err := mgr.DeleteVM("uefi-live-test"); err != nil {
		t.Fatalf("failed to delete UEFI VM: %v", err)
	}
}

