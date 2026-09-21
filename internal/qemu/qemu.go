package qemu

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"tinyvm/internal/host"
)

// Launcher orchestrates QEMU process execution and configuration.
type Launcher struct {
	binaryPathX86_64 string
	binaryPathI386   string
	useKVM           bool
	kvmStatus        host.KVMStatus
	mu               sync.RWMutex
}

// NewLauncher finds the QEMU binary and checks host KVM support.
func NewLauncher() (*Launcher, error) {
	bin64, err64 := exec.LookPath("qemu-system-x86_64")
	bin32, _ := exec.LookPath("qemu-system-i386")

	if bin64 == "" && bin32 == "" {
		return nil, fmt.Errorf("qemu-system-x86_64 or qemu-system-i386 not found in PATH: %w", err64)
	}

	kvmStatus := host.GetKVMStatus()

	return &Launcher{
		binaryPathX86_64: bin64,
		binaryPathI386:   bin32,
		useKVM:           kvmStatus.Available,
		kvmStatus:        kvmStatus,
	}, nil
}

// KVMStatus returns the host KVM capability status.
func (l *Launcher) KVMStatus() host.KVMStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.kvmStatus
}

// BinaryPath returns the path to the discovered default QEMU executable.
func (l *Launcher) BinaryPath() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.binaryPathX86_64 != "" {
		return l.binaryPathX86_64
	}
	return l.binaryPathI386
}

// BinaryPathForArch returns the appropriate QEMU binary path for the requested architecture.
func (l *Launcher) BinaryPathForArch(arch string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	arch = strings.ToLower(strings.TrimSpace(arch))
	if arch == "x86" || arch == "i386" || arch == "x86_32" {
		if l.binaryPathI386 != "" {
			return l.binaryPathI386
		}
		// Fallback to x86_64 (which executes 32-bit guests natively under KVM)
		return l.binaryPathX86_64
	}
	if l.binaryPathX86_64 != "" {
		return l.binaryPathX86_64
	}
	return l.binaryPathI386
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

	bin := l.BinaryPathForArch(cfg.Arch)
	if bin == "" {
		return nil, fmt.Errorf("no suitable QEMU binary found for architecture '%s'", cfg.Arch)
	}

	return StartProcess(bin, args, paths, cfg.ID)
}
