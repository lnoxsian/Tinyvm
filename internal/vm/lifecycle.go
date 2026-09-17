package vm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"tinyvm/internal/qemu"
)

var (
	ErrShutdownTimeout = errors.New("VM did not shut down within timeout (use stop to force terminate)")
)

// VMStatusInfo holds detailed operational and resource information about a VM.
type VMStatusInfo struct {
	ID              string        `json:"id"`
	Status          string        `json:"status"`
	CPUs            int           `json:"cpus"`
	MemoryMB        int           `json:"memory_mb"`
	Config          VMConfig      `json:"config"`
	Runtime         VMRuntime     `json:"runtime"`
	Uptime          time.Duration `json:"uptime,omitempty"`
	UptimeSeconds   int64         `json:"uptime_seconds,omitempty"`
	DiskActualBytes int64         `json:"disk_actual_bytes,omitempty"`
	Firmware        string        `json:"firmware"`
	EFIVarsPath     string        `json:"efi_vars_path,omitempty"`
	QMPStatus       string        `json:"qmp_status,omitempty"`
	LogPath         string        `json:"log_path"`
	QMPSockPath     string        `json:"qmp_sock_path"`
	ConsoleSockPath string        `json:"console_sock_path"`
}

// StartVM launches the QEMU process for the specified VM.
func (m *Manager) StartVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	return m.startVMLocked(id)
}

func (m *Manager) startVMLocked(id string) error {
	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if vm.Runtime.State == StateRunning || vm.Runtime.State == StateStarting {
		m.mu.Unlock()
		return ErrVMAlreadyRunning
	}

	if m.launcher == nil {
		m.mu.Unlock()
		return fmt.Errorf("QEMU launcher is not configured")
	}

	vm.Runtime.State = StateStarting
	m.mu.Unlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		m.setVMState(id, StateError, 0)
		return err
	}

	qemuCfg := vm.Config.ToQEMUConfig()
	proc, err := m.launcher.Launch(qemuCfg, vmDir, m.storage.ISODir())
	if err != nil {
		m.setVMState(id, StateError, 0)
		return fmt.Errorf("failed to launch VM %s: %w", id, err)
	}

	m.mu.Lock()
	vm.Runtime.State = StateRunning
	vm.Runtime.PID = proc.PID()
	vm.Runtime.StartedAt = time.Now()
	m.processes[id] = proc
	m.mu.Unlock()

	// Watch process termination in background
	go m.watchProcess(id, proc)

	return nil
}

// StopVM forces termination of the running QEMU process via SIGTERM then SIGKILL.
func (m *Manager) StopVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	return m.stopVMLocked(id)
}

func (m *Manager) stopVMLocked(id string) error {
	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if vm.Runtime.State != StateRunning && vm.Runtime.State != StateStarting {
		m.mu.Unlock()
		return ErrVMNotRunning
	}

	proc, hasProc := m.processes[id]
	pid := vm.Runtime.PID
	if (!hasProc || proc == nil) && pid <= 0 {
		vm.Runtime.State = StateStopped
		vm.Runtime.PID = 0
		m.mu.Unlock()
		return nil
	}

	vm.Runtime.State = StateStopping
	m.mu.Unlock()

	if hasProc && proc != nil {
		// Send SIGTERM, fallback to SIGKILL after 5 seconds
		if err := proc.Terminate(5 * time.Second); err != nil {
			m.setVMState(id, StateError, 0)
			return fmt.Errorf("failed to terminate VM process: %w", err)
		}
	} else if pid > 0 {
		// Terminate via OS signals for standalone CLI invocation
		_ = syscall.Kill(pid, syscall.SIGTERM)
		deadline := time.Now().Add(5 * time.Second)
		stopped := false
		for time.Now().Before(deadline) {
			if syscall.Kill(pid, 0) != nil {
				stopped = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !stopped {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			time.Sleep(100 * time.Millisecond)
		}
		vmDir, _ := m.storage.VMDir(id)
		qemu.CleanStaleArtifacts(qemu.QEMUPaths{
			QMPSock:     filepath.Join(vmDir, "qmp.sock"),
			ConsoleSock: filepath.Join(vmDir, "console.sock"),
			PIDFile:     filepath.Join(vmDir, "qemu.pid"),
		})
	}

	m.mu.Lock()
	vm.Runtime.State = StateStopped
	vm.Runtime.PID = 0
	delete(m.processes, id)
	m.mu.Unlock()

	return nil
}

// QuitVM instructs the QEMU process to quit immediately and cleanly via QMP.
func (m *Manager) QuitVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if vm.Runtime.State != StateRunning && vm.Runtime.State != StateStarting {
		m.mu.Unlock()
		return ErrVMNotRunning
	}

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	qmpSock := filepath.Join(vmDir, "qmp.sock")

	vm.Runtime.State = StateStopping
	m.mu.Unlock()

	// Send QMP quit
	if err := qemu.QMPQuit(qmpSock, 2*time.Second); err != nil {
		// Fallback to force stop if QMP quit fails
		return m.stopVMLocked(id)
	}

	m.mu.Lock()
	vm.Runtime.State = StateStopped
	vm.Runtime.PID = 0
	delete(m.processes, id)
	m.mu.Unlock()

	return nil
}

// ShutdownVM requests a graceful ACPI shutdown.
func (m *Manager) ShutdownVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if vm.Runtime.State != StateRunning {
		m.mu.Unlock()
		return ErrVMNotRunning
	}

	proc, hasProc := m.processes[id]
	pid := vm.Runtime.PID
	if (!hasProc || proc == nil) && pid <= 0 {
		vm.Runtime.State = StateStopped
		vm.Runtime.PID = 0
		m.mu.Unlock()
		return nil
	}

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	qmpSock := filepath.Join(vmDir, "qmp.sock")

	vm.Runtime.State = StateStopping
	m.mu.Unlock()

	// 1. Send ACPI system_powerdown via QMP
	qmpErr := qemu.QMPSystemPowerdown(qmpSock, 2*time.Second)
	if qmpErr != nil {
		// Fallback to process signal if QMP socket was unreachable
		if hasProc && proc != nil {
			_ = proc.Signal(os.Interrupt)
		} else if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGINT)
		}
	}

	// 2. Wait up to 15s for guest to cleanly power down and process to exit
	if hasProc && proc != nil {
		select {
		case <-proc.ExitChan():
			m.mu.Lock()
			vm.Runtime.State = StateStopped
			vm.Runtime.PID = 0
			delete(m.processes, id)
			m.mu.Unlock()
			return nil
		case <-time.After(15 * time.Second):
			m.mu.Lock()
			vm.Runtime.State = StateRunning
			m.mu.Unlock()
			return ErrShutdownTimeout
		}
	} else if pid > 0 {
		deadline := time.Now().Add(15 * time.Second)
		stopped := false
		for time.Now().Before(deadline) {
			if syscall.Kill(pid, 0) != nil {
				stopped = true
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		if !stopped {
			m.mu.Lock()
			vm.Runtime.State = StateRunning
			m.mu.Unlock()
			return ErrShutdownTimeout
		}
		qemu.CleanStaleArtifacts(qemu.QEMUPaths{
			QMPSock:     filepath.Join(vmDir, "qmp.sock"),
			ConsoleSock: filepath.Join(vmDir, "console.sock"),
			PIDFile:     filepath.Join(vmDir, "qemu.pid"),
		})
		m.mu.Lock()
		vm.Runtime.State = StateStopped
		vm.Runtime.PID = 0
		m.mu.Unlock()
		return nil
	}

	return nil
}

// RestartVM stops the VM and starts it back up cleanly.
func (m *Manager) RestartVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.RLock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return ErrVMNotFoundInMgr
	}
	wasRunning := vm.Runtime.State == StateRunning || vm.Runtime.State == StateStarting
	m.mu.RUnlock()

	if wasRunning {
		if err := m.stopVMLocked(id); err != nil {
			return fmt.Errorf("failed to stop VM during restart: %w", err)
		}
	}

	return m.startVMLocked(id)
}

// StatusVM gathers full runtime, storage, and telemetry details for a VM.
func (m *Manager) StatusVM(id string) (*VMStatusInfo, error) {
	m.mu.RLock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrVMNotFoundInMgr
	}
	cfg := vm.Config
	runtime := vm.Runtime
	m.mu.RUnlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return nil, err
	}

	var uptime time.Duration
	if runtime.State == StateRunning && !runtime.StartedAt.IsZero() {
		uptime = time.Since(runtime.StartedAt).Round(time.Second)
	}

	var diskActualBytes int64
	if cfg.Disk != "" {
		diskPath := filepath.Join(vmDir, cfg.Disk)
		if fi, err := os.Stat(diskPath); err == nil {
			diskActualBytes = fi.Size()
		}
	}

	var efiVarsPath string
	if cfg.Firmware == "uefi" {
		efiVarsPath = filepath.Join(vmDir, "efivars.fd")
	}

	var qmpStatus string
	if runtime.State == StateRunning {
		qmpSock := filepath.Join(vmDir, "qmp.sock")
		if res, err := qemu.QMPQueryStatus(qmpSock, 500*time.Millisecond); err == nil && res != nil {
			qmpStatus = res.Status
		}
	}

	var uptimeSecs int64
	if uptime > 0 {
		uptimeSecs = int64(uptime.Seconds())
	}

	return &VMStatusInfo{
		ID:              cfg.ID,
		Status:          string(runtime.State),
		CPUs:            cfg.CPUs,
		MemoryMB:        cfg.MemoryMB,
		Config:          cfg,
		Runtime:         runtime,
		Uptime:          uptime,
		UptimeSeconds:   uptimeSecs,
		DiskActualBytes: diskActualBytes,
		Firmware:        cfg.Firmware,
		EFIVarsPath:     efiVarsPath,
		QMPStatus:       qmpStatus,
		LogPath:         filepath.Join(vmDir, "logs", "qemu.log"),
		QMPSockPath:     filepath.Join(vmDir, "qmp.sock"),
		ConsoleSockPath: filepath.Join(vmDir, "console.sock"),
	}, nil
}

// watchProcess monitors the QEMU process termination and transitions VM state to stopped.
func (m *Manager) watchProcess(id string, proc *qemu.Process) {
	<-proc.ExitChan()

	m.mu.Lock()
	defer m.mu.Unlock()

	if vm, exists := m.vms[id]; exists {
		vm.Runtime.State = StateStopped
		vm.Runtime.PID = 0
	}
	delete(m.processes, id)
}

// GetProcess returns the active QEMU process for a running VM.
func (m *Manager) GetProcess(id string) (*qemu.Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	proc, exists := m.processes[id]
	return proc, exists
}

// IsVMRunning checks whether the VM is actively executing.
func (m *Manager) IsVMRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	proc, exists := m.processes[id]
	if exists && proc != nil {
		return proc.IsRunning()
	}
	if vm, exists := m.vms[id]; exists && vm.Runtime.PID > 0 {
		return isQEMUProcess(vm.Runtime.PID)
	}
	return false
}

func (m *Manager) setVMState(id string, state VMState, pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if vm, exists := m.vms[id]; exists {
		vm.Runtime.State = state
		vm.Runtime.PID = pid
	}
}
