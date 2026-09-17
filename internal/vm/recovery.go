package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"tinyvm/internal/qemu"
	"tinyvm/internal/storage"
)

// ReconcileVM inspects a VM's on-disk artifacts and resolves stale PIDs and sockets.
func ReconcileVM(s *storage.Storage, vmID string) (int, VMState, error) {
	vmDir, err := s.VMDir(vmID)
	if err != nil {
		return 0, StateError, err
	}

	paths := qemu.QEMUPaths{
		VMDir:       vmDir,
		QMPSock:     filepath.Join(vmDir, "qmp.sock"),
		ConsoleSock: filepath.Join(vmDir, "console.sock"),
		PIDFile:     filepath.Join(vmDir, "qemu.pid"),
	}

	pidBytes, err := os.ReadFile(paths.PIDFile)
	if err != nil {
		// No PID file: clean any orphaned sockets and mark stopped
		qemu.CleanStaleArtifacts(paths)
		return 0, StateStopped, nil
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		// Corrupted PID file: purge and mark stopped
		qemu.CleanStaleArtifacts(paths)
		return 0, StateStopped, nil
	}

	// Verify if process is actually running
	if isQEMUProcess(pid) {
		return pid, StateRunning, nil
	}

	// Stale process detected (host reboot or crash)
	qemu.CleanStaleArtifacts(paths)
	return 0, StateStopped, nil
}

// isQEMUProcess verifies that the PID exists and its command line belongs to qemu.
func isQEMUProcess(pid int) bool {
	// Signal 0 checks process existence without sending a signal
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}

	// Check /proc/<pid>/cmdline to confirm it's actually QEMU (prevents PID reuse false-positives)
	cmdlineBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}

	cmdline := string(cmdlineBytes)
	return strings.Contains(cmdline, "qemu-system")
}

// RecoverAll reconciles all registered VMs on storage startup.
func (m *Manager) RecoverAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids, err := m.storage.ListVMIDs()
	if err != nil {
		return fmt.Errorf("failed to list VMs during recovery: %w", err)
	}

	for _, id := range ids {
		var cfg VMConfig
		if err := m.storage.ReadVMConfig(id, &cfg); err != nil {
			continue
		}

		pid, state, _ := ReconcileVM(m.storage, id)

		m.vms[id] = &VM{
			Config: cfg,
			Runtime: VMRuntime{
				PID:   pid,
				State: state,
			},
		}
	}

	return nil
}
