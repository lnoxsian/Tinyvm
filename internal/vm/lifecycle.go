package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tinyvm/internal/host"
	"tinyvm/internal/qemu"
	"tinyvm/internal/storage"
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
	CPUPercent      float64       `json:"cpu_percent,omitempty"`
	MemoryRSSBytes  uint64        `json:"memory_rss_bytes,omitempty"`
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

	if vm.Runtime.State != StateRunning && vm.Runtime.State != StateStarting && vm.Runtime.State != StateStopping && vm.Runtime.State != StatePaused {
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

	vmDir, _ := m.storage.VMDir(id)
	qmpSock := filepath.Join(vmDir, "qmp.sock")

	// 1. Try clean QMP quit first (flushes all disk buffers and exits in <100ms)
	_ = qemu.QMPQuit(qmpSock, 1500*time.Millisecond)

	// 2. Escalate via OS signals if still active
	if hasProc && proc != nil {
		if proc.IsRunning() {
			if err := proc.Terminate(3 * time.Second); err != nil {
				m.setVMState(id, StateError, 0)
				return fmt.Errorf("failed to terminate VM process: %w", err)
			}
		}
	} else if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		deadline := time.Now().Add(3 * time.Second)
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

	return m.stopVMLocked(id)
}

// ShutdownVM requests a graceful ACPI shutdown.
func (m *Manager) ShutdownVM(id string) error {
	return m.ShutdownVMWithFallback(id, 15*time.Second, false)
}

// ShutdownVMWithFallback requests an ACPI shutdown. It waits up to 2 seconds for immediate shutdown.
// If the guest has not yet halted, it leaves the state as StateStopping and continues monitoring in background.
// If forceFallback is true, it forces a clean stop if ACPI does not power down before timeout.
func (m *Manager) ShutdownVMWithFallback(id string, timeout time.Duration, forceFallback bool) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if vm.Runtime.State != StateRunning && vm.Runtime.State != StateStopping && vm.Runtime.State != StatePaused {
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
		if hasProc && proc != nil {
			_ = proc.Signal(os.Interrupt)
		} else if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGINT)
		}
	}

	// 2. Wait up to 3.5 seconds synchronously so normal shutdowns update instantly
	immediateWait := 3500 * time.Millisecond
	if timeout < immediateWait {
		immediateWait = timeout
	}

	if hasProc && proc != nil {
		select {
		case <-proc.ExitChan():
			m.mu.Lock()
			vm.Runtime.State = StateStopped
			vm.Runtime.PID = 0
			delete(m.processes, id)
			m.mu.Unlock()
			return nil
		case <-time.After(immediateWait):
			remaining := timeout - immediateWait
			if remaining > 0 {
				go m.watchShutdown(id, proc, pid, remaining, forceFallback)
			}
			return nil
		}
	} else if pid > 0 {
		deadline := time.Now().Add(immediateWait)
		stopped := false
		for time.Now().Before(deadline) {
			if syscall.Kill(pid, 0) != nil {
				stopped = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if stopped {
			m.mu.Lock()
			vm.Runtime.State = StateStopped
			vm.Runtime.PID = 0
			m.mu.Unlock()
			return nil
		}
		remaining := timeout - immediateWait
		if remaining > 0 {
			go m.watchShutdown(id, nil, pid, remaining, forceFallback)
		}
		return nil
	}

	return nil
}

func (m *Manager) watchShutdown(id string, proc *qemu.Process, pid int, timeout time.Duration, forceFallback bool) {
	if proc != nil {
		select {
		case <-proc.ExitChan():
			m.mu.Lock()
			if vm, exists := m.vms[id]; exists {
				vm.Runtime.State = StateStopped
				vm.Runtime.PID = 0
			}
			delete(m.processes, id)
			m.mu.Unlock()
		case <-time.After(timeout):
			if forceFallback {
				_ = m.StopVM(id)
			} else {
				m.mu.Lock()
				if vm, exists := m.vms[id]; exists && vm.Runtime.State == StateStopping {
					vm.Runtime.State = StateRunning
				}
				m.mu.Unlock()
			}
		}
	} else if pid > 0 {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if syscall.Kill(pid, 0) != nil {
				m.mu.Lock()
				if vm, exists := m.vms[id]; exists {
					vm.Runtime.State = StateStopped
					vm.Runtime.PID = 0
				}
				m.mu.Unlock()
				return
			}
			time.Sleep(150 * time.Millisecond)
		}
		if forceFallback {
			_ = m.StopVM(id)
		} else {
			m.mu.Lock()
			if vm, exists := m.vms[id]; exists && vm.Runtime.State == StateStopping {
				vm.Runtime.State = StateRunning
			}
			m.mu.Unlock()
		}
	}
}

// ResetVM hard-resets the virtual machine via QMP system_reset.
func (m *Manager) ResetVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.RLock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return ErrVMNotFoundInMgr
	}
	if vm.Runtime.State != StateRunning && vm.Runtime.State != StatePaused {
		m.mu.RUnlock()
		return ErrVMNotRunning
	}
	m.mu.RUnlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	qmpSock := filepath.Join(vmDir, "qmp.sock")
	return qemu.QMPSystemReset(qmpSock, 2*time.Second)
}

// PauseVM pauses CPU execution of a running VM via QMP.
func (m *Manager) PauseVM(id string) error {
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
	m.mu.Unlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	qmpSock := filepath.Join(vmDir, "qmp.sock")
	if err := qemu.QMPPause(qmpSock, 2*time.Second); err != nil {
		return err
	}

	m.mu.Lock()
	vm.Runtime.State = StatePaused
	m.mu.Unlock()
	return nil
}

// ResumeVM resumes CPU execution of a paused VM via QMP.
func (m *Manager) ResumeVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	vm, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}
	if vm.Runtime.State != StatePaused {
		m.mu.Unlock()
		return fmt.Errorf("VM is not paused (current state: %s)", vm.Runtime.State)
	}
	m.mu.Unlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	qmpSock := filepath.Join(vmDir, "qmp.sock")
	if err := qemu.QMPResume(qmpSock, 2*time.Second); err != nil {
		return err
	}

	m.mu.Lock()
	vm.Runtime.State = StateRunning
	m.mu.Unlock()
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
	var cpuPercent float64
	var memRSS uint64
	if runtime.State == StateRunning && !runtime.StartedAt.IsZero() {
		uptime = time.Since(runtime.StartedAt).Round(time.Second)
	}
	if runtime.State == StateRunning && runtime.PID > 0 {
		cpuPercent = host.GetProcessCPUPercent(runtime.PID)
		memRSS = host.GetProcessRSSBytes(runtime.PID)
	}

	var diskActualBytes int64
	if cfg.Disk != "" {
		diskPath := filepath.Join(vmDir, cfg.Disk)
		if fi, err := os.Stat(diskPath); err == nil {
			diskActualBytes = fi.Size()
			if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
				diskActualBytes = stat.Blocks * 512
			}
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
		CPUPercent:      cpuPercent,
		MemoryRSSBytes:  memRSS,
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

// EjectISO detaches the ISO image from the VM configuration and ejects it via QMP if running.
func (m *Manager) EjectISO(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if v.Config.ISO == "" {
		m.mu.Unlock()
		return nil
	}

	isRunning := v.Runtime.State == StateRunning
	v.Config.ISO = ""
	cfgCopy := v.Config
	m.mu.Unlock()

	if isRunning {
		vmDir, err := m.storage.VMDir(id)
		if err == nil {
			qmpSock := filepath.Join(vmDir, "qmp.sock")
			_ = qemu.QMPEjectCDROM(qmpSock, 2*time.Second)
		}
	}

	if err := m.storage.WriteVMConfig(id, cfgCopy); err != nil {
		return fmt.Errorf("failed to save VM config after eject: %w", err)
	}

	return nil
}

// BootVM boots a VM directly from its virtual disk. If an ISO is currently attached,
// it is detached first. If the VM is running, it restarts cleanly into the installed OS.
func (m *Manager) BootVM(id string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.RLock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return ErrVMNotFoundInMgr
	}
	wasRunning := v.Runtime.State == StateRunning || v.Runtime.State == StateStarting
	hasISO := v.Config.ISO != ""
	m.mu.RUnlock()

	if hasISO {
		m.mu.Lock()
		v.Config.ISO = ""
		cfgCopy := v.Config
		m.mu.Unlock()

		if wasRunning {
			vmDir, err := m.storage.VMDir(id)
			if err == nil {
				qmpSock := filepath.Join(vmDir, "qmp.sock")
				_ = qemu.QMPEjectCDROM(qmpSock, 2*time.Second)
			}
		}

		if err := m.storage.WriteVMConfig(id, cfgCopy); err != nil {
			return fmt.Errorf("failed to save VM config during boot: %w", err)
		}
	}

	if wasRunning {
		if err := m.stopVMLocked(id); err != nil {
			return fmt.Errorf("failed to stop VM during boot restart: %w", err)
		}
	}

	return m.startVMLocked(id)
}

// AttachISO attaches an ISO image to the VM configuration, updating QMP medium if VM is running.
func (m *Manager) AttachISO(id string, isoName string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	isoName = strings.TrimSpace(isoName)
	var fullISOPath string
	if isoName != "" {
		if err := storage.ValidateISOName(isoName); err != nil {
			return err
		}
		if !m.storage.ISOExists(isoName) {
			return storage.ErrISONotFound
		}
		var err error
		fullISOPath, err = m.storage.ISOPath(isoName)
		if err != nil {
			return err
		}
	}

	m.mu.Lock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	isRunning := v.Runtime.State == StateRunning
	v.Config.ISO = isoName
	cfgCopy := v.Config
	m.mu.Unlock()

	if isRunning && fullISOPath != "" {
		vmDir, err := m.storage.VMDir(id)
		if err == nil {
			qmpSock := filepath.Join(vmDir, "qmp.sock")
			_ = qemu.QMPChangeCDROM(qmpSock, fullISOPath, 2*time.Second)
		}
	}

	if err := m.storage.WriteVMConfig(id, cfgCopy); err != nil {
		return fmt.Errorf("failed to save VM config: %w", err)
	}

	return nil
}

// UpdateVMConfig updates editable VM configuration settings (CPUs, RAM, boot order, network, etc.).
func (m *Manager) UpdateVMConfig(id string, updated VMConfig) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if updated.Name != "" {
		v.Config.Name = updated.Name
	}
	if updated.CPUs > 0 {
		v.Config.CPUs = updated.CPUs
	}
	if updated.MemoryMB >= 128 {
		v.Config.MemoryMB = updated.MemoryMB
	}
	if updated.Firmware != "" {
		v.Config.Firmware = updated.Firmware
	}
	if updated.OSType != "" {
		v.Config.OSType = updated.OSType
	}
	if updated.BootOrder != "" {
		v.Config.BootOrder = updated.BootOrder
	}
	v.Config.Autostart = updated.Autostart
	v.Config.Network.Enabled = updated.Network.Enabled
	if updated.Network.SSHPort > 0 {
		v.Config.Network.SSHPort = updated.Network.SSHPort
	}
	if updated.Network.Ports != nil {
		v.Config.Network.Ports = updated.Network.Ports
	}

	if err := v.Config.Validate(); err != nil {
		m.mu.Unlock()
		return err
	}

	cfgCopy := v.Config
	m.mu.Unlock()

	return m.storage.WriteVMConfig(id, cfgCopy)
}

// ResizeVMDisk expands the virtual disk image for a VM.
func (m *Manager) ResizeVMDisk(id string, sizeDelta string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.Lock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.Unlock()
		return ErrVMNotFoundInMgr
	}

	if v.Runtime.State == StateRunning || v.Runtime.State == StateStarting {
		m.mu.Unlock()
		return fmt.Errorf("cannot resize disk while VM is running (please shut down or stop the VM first)")
	}

	diskName := v.Config.Disk
	if diskName == "" {
		diskName = "disk.qcow2"
	}
	m.mu.Unlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	diskPath := filepath.Join(vmDir, diskName)

	if err := storage.ResizeDisk(diskPath, sizeDelta); err != nil {
		return err
	}

	// Update DiskSize in config
	info, err := storage.InspectDisk(diskPath)
	if err == nil && info != nil {
		m.mu.Lock()
		const gib = 1024 * 1024 * 1024
		const mib = 1024 * 1024
		if info.VirtualSize >= gib && info.VirtualSize%gib == 0 {
			v.Config.DiskSize = fmt.Sprintf("%dG", info.VirtualSize/gib)
		} else if info.VirtualSize >= gib {
			v.Config.DiskSize = fmt.Sprintf("%.1fG", float64(info.VirtualSize)/float64(gib))
		} else {
			v.Config.DiskSize = fmt.Sprintf("%dM", info.VirtualSize/mib)
		}
		cfgCopy := v.Config
		m.mu.Unlock()

		_ = m.storage.WriteVMConfig(id, cfgCopy)
	}

	return nil
}

// CreateVMSnapshot creates an internal qcow2 snapshot for a VM.
func (m *Manager) CreateVMSnapshot(id string, name string, desc string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.RLock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return ErrVMNotFoundInMgr
	}
	if v.Runtime.State == StateRunning || v.Runtime.State == StateStarting {
		m.mu.RUnlock()
		return fmt.Errorf("cannot create snapshot while VM is running (stop VM first)")
	}
	diskName := v.Config.Disk
	if diskName == "" {
		diskName = "disk.qcow2"
	}
	m.mu.RUnlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	diskPath := filepath.Join(vmDir, diskName)

	return storage.CreateDiskSnapshot(diskPath, name)
}

// ListVMSnapshots retrieves snapshots for a VM.
func (m *Manager) ListVMSnapshots(id string) ([]SnapshotInfo, error) {
	m.mu.RLock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrVMNotFoundInMgr
	}
	diskName := v.Config.Disk
	if diskName == "" {
		diskName = "disk.qcow2"
	}
	m.mu.RUnlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return nil, err
	}
	diskPath := filepath.Join(vmDir, diskName)

	diskSnaps, err := storage.ListDiskSnapshots(diskPath)
	if err != nil {
		return nil, err
	}

	snaps := make([]SnapshotInfo, 0, len(diskSnaps))
	for _, ds := range diskSnaps {
		snaps = append(snaps, SnapshotInfo{
			ID:          ds.ID,
			Name:        ds.Name,
			VMStateSize: ds.VMStateSize,
			Date:        ds.Date,
		})
	}
	return snaps, nil
}

// RollbackVMSnapshot rolls back a VM's disk to a snapshot.
func (m *Manager) RollbackVMSnapshot(id string, name string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.RLock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return ErrVMNotFoundInMgr
	}
	if v.Runtime.State == StateRunning || v.Runtime.State == StateStarting {
		m.mu.RUnlock()
		return fmt.Errorf("cannot rollback snapshot while VM is running (stop VM first)")
	}
	diskName := v.Config.Disk
	if diskName == "" {
		diskName = "disk.qcow2"
	}
	m.mu.RUnlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	diskPath := filepath.Join(vmDir, diskName)

	return storage.ApplyDiskSnapshot(diskPath, name)
}

// DeleteVMSnapshot deletes a snapshot from a VM's disk.
func (m *Manager) DeleteVMSnapshot(id string, name string) error {
	opLock := m.GetVMOpLock(id)
	opLock.Lock()
	defer opLock.Unlock()

	m.mu.RLock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return ErrVMNotFoundInMgr
	}
	if v.Runtime.State == StateRunning || v.Runtime.State == StateStarting {
		m.mu.RUnlock()
		return fmt.Errorf("cannot delete snapshot while VM is running (stop VM first)")
	}
	diskName := v.Config.Disk
	if diskName == "" {
		diskName = "disk.qcow2"
	}
	m.mu.RUnlock()

	vmDir, err := m.storage.VMDir(id)
	if err != nil {
		return err
	}
	diskPath := filepath.Join(vmDir, diskName)

	return storage.DeleteDiskSnapshot(diskPath, name)
}

// CloneVM creates a new VM as a full clone of an existing VM.
func (m *Manager) CloneVM(sourceID string, newID string, newName string) (*VM, error) {
	newID = strings.TrimSpace(newID)
	newName = strings.TrimSpace(newName)

	if err := storage.ValidateVMID(newID); err != nil {
		return nil, err
	}

	m.mu.RLock()
	srcVM, exists := m.vms[sourceID]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrVMNotFoundInMgr
	}
	if srcVM.Runtime.State == StateRunning || srcVM.Runtime.State == StateStarting {
		m.mu.RUnlock()
		return nil, fmt.Errorf("source VM '%s' must be stopped before cloning", sourceID)
	}
	if _, existsNew := m.vms[newID]; existsNew {
		m.mu.RUnlock()
		return nil, ErrVMAlreadyExists
	}
	srcCfg := srcVM.Config
	m.mu.RUnlock()

	// Clone config
	newCfg := srcCfg
	newCfg.ID = newID
	if newName != "" {
		newCfg.Name = newName
	} else {
		newCfg.Name = fmt.Sprintf("%s-clone", srcCfg.Name)
	}
	// Avoid port collisions on clone
	if newCfg.Network.SSHPort > 0 {
		newCfg.Network.SSHPort = 0
	}
	newCfg.Network.Ports = nil

	if err := newCfg.Validate(); err != nil {
		return nil, err
	}

	// Create VM directory
	if _, err := m.storage.CreateVMStorage(newID); err != nil {
		return nil, err
	}

	if err := m.storage.WriteVMConfig(newID, newCfg); err != nil {
		_ = m.storage.DeleteVMStorage(newID)
		return nil, err
	}

	// Copy virtual disk
	srcDir, _ := m.storage.VMDir(sourceID)
	newDir, _ := m.storage.VMDir(newID)
	srcDisk := filepath.Join(srcDir, srcCfg.Disk)
	newDisk := filepath.Join(newDir, newCfg.Disk)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Convert/copy with qemu-img for clean new image
	cmd := exec.CommandContext(ctx, "qemu-img", "convert", "-O", newCfg.DiskFormat, srcDisk, newDisk)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = m.storage.DeleteVMStorage(newID)
		return nil, fmt.Errorf("disk cloning failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	vmInstance := &VM{
		Config: newCfg,
		Runtime: VMRuntime{
			State: StateStopped,
		},
	}

	m.mu.Lock()
	m.vms[newID] = vmInstance
	m.mu.Unlock()

	return vmInstance, nil
}
