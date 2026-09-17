package qemu

import (
	"fmt"
	"os/exec"
	"sync"

	"tinyvm/internal/host"
)

// Launcher orchestrates QEMU process execution and configuration.
type Launcher struct {
	binaryPath string
	useKVM     bool
	kvmStatus  host.KVMStatus
	mu         sync.RWMutex
}

// NewLauncher finds the QEMU binary and checks host KVM support.
func NewLauncher() (*Launcher, error) {
	bin, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		return nil, fmt.Errorf("qemu-system-x86_64 not found in PATH: %w", err)
	}

	kvmStatus := host.GetKVMStatus()

	return &Launcher{
		binaryPath: bin,
		useKVM:     kvmStatus.Available,
		kvmStatus:  kvmStatus,
	}, nil
}

// KVMStatus returns the host KVM capability status.
func (l *Launcher) KVMStatus() host.KVMStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.kvmStatus
}

// BinaryPath returns the path to the discovered qemu-system-x86_64 executable.
func (l *Launcher) BinaryPath() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.binaryPath
}

// UseKVM returns true if KVM hardware acceleration is available and enabled.
func (l *Launcher) UseKVM() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.useKVM
}

// SetUseKVM allows overriding KVM usage (e.g. for testing without acceleration).
func (l *Launcher) SetUseKVM(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.useKVM = enabled
}

// Launch launches a QEMU process for the specified VM configuration.
func (l *Launcher) Launch(cfg *Config, vmDir string, isoDir string) (*Process, error) {
	paths := BuildPaths(vmDir, isoDir, cfg)
	args := BuildArgs(cfg, paths, l.UseKVM())

	return StartProcess(l.BinaryPath(), args, paths, cfg.ID)
}
