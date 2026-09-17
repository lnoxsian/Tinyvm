package vm

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"tinyvm/internal/host"
	"tinyvm/internal/qemu"
	"tinyvm/internal/storage"
)

var (
	ErrVMAlreadyRunning  = errors.New("cannot perform operation: VM is already running")
	ErrVMNotRunning      = errors.New("cannot perform operation: VM is not running")
	ErrVMNotFoundInMgr   = errors.New("VM not found")
	ErrVMAlreadyExists   = errors.New("VM with this ID already exists")
)

// Manager coordinates in-memory VM states and lifecycle operations.
type Manager struct {
	mu        sync.RWMutex
	storage   *storage.Storage
	launcher  *qemu.Launcher
	vms       map[string]*VM
	processes map[string]*qemu.Process
	opLocks   map[string]*sync.Mutex
}

// NewManager creates and initializes a new VM Manager.
func NewManager(s *storage.Storage, launcher *qemu.Launcher) *Manager {
	if launcher == nil {
		launcher, _ = qemu.NewLauncher()
	}

	m := &Manager{
		storage:   s,
		launcher:  launcher,
		vms:       make(map[string]*VM),
		processes: make(map[string]*qemu.Process),
		opLocks:   make(map[string]*sync.Mutex),
	}
	_ = m.RecoverAll()
	return m
}

// GetVMOpLock retrieves or initializes the synchronization lock for a given VM ID.
func (m *Manager) GetVMOpLock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock, exists := m.opLocks[id]
	if !exists {
		lock = &sync.Mutex{}
		m.opLocks[id] = lock
	}
	return lock
}

// Launcher returns the underlying QEMU launcher.
func (m *Manager) Launcher() *qemu.Launcher {
	return m.launcher
}

// SetLauncher sets the QEMU launcher (useful for tests or custom configurations).
func (m *Manager) SetLauncher(l *qemu.Launcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.launcher = l
}

// Storage returns the underlying storage manager.
func (m *Manager) Storage() *storage.Storage {
	return m.storage
}

// DiscoverVMs scans the storage directory and registers existing VMs into memory.
func (m *Manager) DiscoverVMs() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids, err := m.storage.ListVMIDs()
	if err != nil {
		return fmt.Errorf("failed to list VMs from storage: %w", err)
	}

	for _, id := range ids {
		var cfg VMConfig
		if err := m.storage.ReadVMConfig(id, &cfg); err != nil {
			continue
		}

		m.vms[id] = &VM{
			Config: cfg,
			Runtime: VMRuntime{
				PID:   0,
				State: StateStopped,
			},
		}
	}

	return nil
}

// CreateVM validates, provisions storage, optionally creates the QCOW2 disk, and saves configuration.
func (m *Manager) CreateVM(cfg VMConfig) (*VM, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	opLock := m.GetVMOpLock(cfg.ID)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	if _, exists := m.vms[cfg.ID]; exists || m.storage.VMExists(cfg.ID) {
		m.mu.Unlock()
		return nil, ErrVMAlreadyExists
	}
	m.mu.Unlock()

	// 1. Create VM directory and subfolders
	vmDir, err := m.storage.CreateVMStorage(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create VM storage: %w", err)
	}

	// 2. Provision UEFI NVRAM vars if firmware is uefi
	if cfg.Firmware == "uefi" {
		fw := host.DetectUEFIFirmware()
		if !fw.Available {
			_ = m.storage.DeleteVMStorage(cfg.ID)
			return nil, host.ErrUEFINotSupported
		}
		if fw.IsSplit {
			if err := host.InitEFIVars(vmDir, fw); err != nil {
				_ = m.storage.DeleteVMStorage(cfg.ID)
				return nil, fmt.Errorf("failed to initialize EFI vars: %w", err)
			}
		}
	}

	// 3. Create disk if DiskSize is specified
	if cfg.DiskSize != "" {
		diskPath := filepath.Join(vmDir, cfg.Disk)
		if err := storage.CreateDisk(diskPath, cfg.DiskFormat, cfg.DiskSize); err != nil {
			// Rollback directory on disk failure
			_ = m.storage.DeleteVMStorage(cfg.ID)
			return nil, fmt.Errorf("failed to create virtual disk: %w", err)
		}
	}

	// 4. Write config.json
	if err := m.storage.WriteVMConfig(cfg.ID, cfg); err != nil {
		_ = m.storage.DeleteVMStorage(cfg.ID)
		return nil, fmt.Errorf("failed to save VM config: %w", err)
	}

	vm := &VM{
		Config: cfg,
		Runtime: VMRuntime{
			PID:   0,
			State: StateStopped,
		},
	}

	m.vms[cfg.ID] = vm
	return vm, nil
}

// GetVM retrieves a VM by its ID.
func (m *Manager) GetVM(id string) (*VM, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[id]
	if !exists {
		return nil, ErrVMNotFoundInMgr
	}
	return vm, nil
}

// ListVMs returns a list of all registered VMs.
func (m *Manager) ListVMs() []*VM {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		list = append(list, vm)
	}
	return list
}

// DeleteVM removes a VM from storage and memory if it is not currently running.
func (m *Manager) DeleteVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if vm.Runtime.State == StateRunning || vm.Runtime.State == StateStarting || vm.Runtime.State == StateStopping {
		m.mu.Unlock()
		return ErrVMAlreadyRunning
	}
	m.mu.Unlock()

	if err := m.storage.DeleteVMStorage(id); err != nil {
		return fmt.Errorf("failed to delete VM storage: %w", err)
	}

	m.mu.Lock()
	delete(m.vms, id)
	m.mu.Unlock()
	return nil
}
