package vm

import (
	"fmt"
	"time"

	"tinyvm/internal/qemu"
)

// StartVM launches the QEMU process for the specified VM.
func (m *Manager) StartVM(id string) error {
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

// StopVM forces termination of the running QEMU process via SIGTERM then SIGKILL.
func (m *Manager) StopVM(id string) error {
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
	if !hasProc || proc == nil {
		vm.Runtime.State = StateStopped
		vm.Runtime.PID = 0
		m.mu.Unlock()
		return nil
	}

	vm.Runtime.State = StateStopping
	m.mu.Unlock()

	// Send SIGTERM, fallback to SIGKILL after 5 seconds
	if err := proc.Terminate(5 * time.Second); err != nil {
		m.setVMState(id, StateError, 0)
		return fmt.Errorf("failed to terminate VM process: %w", err)
	}

	m.mu.Lock()
	vm.Runtime.State = StateStopped
	vm.Runtime.PID = 0
	delete(m.processes, id)
	m.mu.Unlock()

	return nil
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
	if !exists || proc == nil {
		return false
	}
	return proc.IsRunning()
}

func (m *Manager) setVMState(id string, state VMState, pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if vm, exists := m.vms[id]; exists {
		vm.Runtime.State = state
		vm.Runtime.PID = pid
	}
}
