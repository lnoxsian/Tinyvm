package qemu

import (
	"strings"
	"testing"
)

func findArg(args []string, flag string) (string, bool) {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func TestBuildArgs_WithKVM(t *testing.T) {
	cfg := &Config{
		ID:         "ubuntu-server",
		CPUs:       4,
		MemoryMB:   4096,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		Network: NetworkConfig{
			Enabled: true,
			SSHPort: 2222,
			Ports: []PortForward{
				{Host: 8080, Guest: 80, Protocol: "tcp"},
			},
		},
	}

	paths := BuildPaths("/var/lib/tinyvm/vms/ubuntu-server", "/var/lib/tinyvm/iso", cfg)
	args := BuildArgs(cfg, paths, true)

	// Verify KVM acceleration flags
	if !containsFlag(args, "-enable-kvm") {
		t.Errorf("expected -enable-kvm in args")
	}

	mach, ok := findArg(args, "-machine")
	if !ok || mach != "q35,accel=kvm" {
		t.Errorf("expected -machine q35,accel=kvm, got %s", mach)
	}

	cpu, ok := findArg(args, "-cpu")
	if !ok || cpu != "host" {
		t.Errorf("expected -cpu host, got %s", cpu)
	}

	smp, ok := findArg(args, "-smp")
	if !ok || smp != "4" {
		t.Errorf("expected -smp 4, got %s", smp)
	}

	mem, ok := findArg(args, "-m")
	if !ok || mem != "4096" {
		t.Errorf("expected -m 4096, got %s", mem)
	}

	// Verify drive
	drive, ok := findArg(args, "-drive")
	if !ok || !strings.Contains(drive, "disk.qcow2") || !strings.Contains(drive, "format=qcow2") {
		t.Errorf("expected valid disk drive argument, got %s", drive)
	}

	// Verify netdev hostfwd
	netdev, ok := findArg(args, "-netdev")
	if !ok || !strings.Contains(netdev, "hostfwd=tcp::2222-:22") || !strings.Contains(netdev, "hostfwd=tcp::8080-:80") {
		t.Errorf("expected port forwarding in netdev, got %s", netdev)
	}

	// Verify sockets and display
	qmp, ok := findArg(args, "-qmp")
	if !ok || !strings.Contains(qmp, "qmp.sock") {
		t.Errorf("expected qmp.sock unix socket, got %s", qmp)
	}

	serial, ok := findArg(args, "-serial")
	if !ok || !strings.Contains(serial, "console.sock") {
		t.Errorf("expected console.sock serial socket, got %s", serial)
	}

	disp, ok := findArg(args, "-display")
	if !ok || disp != "none" {
		t.Errorf("expected -display none, got %s", disp)
	}
}

func TestBuildArgs_WithoutKVM(t *testing.T) {
	cfg := &Config{
		ID:       "fallback-vm",
		CPUs:     1,
		MemoryMB: 1024,
	}

	paths := BuildPaths("/tmp/vm", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, false)

	if containsFlag(args, "-enable-kvm") {
		t.Errorf("did not expect -enable-kvm when KVM is disabled")
	}

	mach, _ := findArg(args, "-machine")
	if mach != "q35" {
		t.Errorf("expected -machine q35, got %s", mach)
	}

	cpu, _ := findArg(args, "-cpu")
	if cpu != "qemu64" {
		t.Errorf("expected -cpu qemu64, got %s", cpu)
	}
}

func TestBuildArgs_WithISO(t *testing.T) {
	cfg := &Config{
		ID:         "installer-vm",
		CPUs:       2,
		MemoryMB:   2048,
		ISO:        "debian-12.iso",
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
	}

	paths := BuildPaths("/tmp/vms/installer-vm", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, true)

	boot, ok := findArg(args, "-boot")
	if !ok || boot != "order=d,menu=on" {
		t.Errorf("expected -boot order=d,menu=on for ISO installation, got %s", boot)
	}
}

func TestBuildArgs_WithUEFI(t *testing.T) {
	cfg := &Config{
		ID:       "uefi-vm",
		CPUs:     2,
		MemoryMB: 2048,
		Firmware: "uefi",
	}

	paths := BuildPaths("/tmp/vms/uefi-vm", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, true)

	// Verify either pflash or -bios is present in args
	hasUEFI := false
	for i, arg := range args {
		if arg == "-bios" && i+1 < len(args) && strings.Contains(args[i+1], "OVMF") {
			hasUEFI = true
			break
		}
		if arg == "-drive" && i+1 < len(args) && strings.Contains(args[i+1], "pflash") {
			hasUEFI = true
			break
		}
	}

	if !hasUEFI {
		t.Errorf("expected UEFI firmware (-drive if=pflash or -bios) in args, got: %v", args)
	}
}

func TestBuildArgs_WithBIOS(t *testing.T) {
	cfg := &Config{
		ID:       "bios-vm",
		CPUs:     1,
		MemoryMB: 1024,
		Firmware: "bios",
	}

	paths := BuildPaths("/tmp/vms/bios-vm", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, true)

	// BIOS should NOT include pflash or -bios flags
	for i, arg := range args {
		if arg == "-bios" {
			t.Errorf("unexpected -bios flag for BIOS VM")
		}
		if arg == "-drive" && i+1 < len(args) && strings.Contains(args[i+1], "pflash") {
			t.Errorf("unexpected pflash drive for BIOS VM")
		}
	}
}
