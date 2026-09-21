package qemu

import (
	"strings"
	"testing"

	"tinyvm/internal/host"
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

	vga, ok := findArg(args, "-vga")
	if !ok || vga != "virtio" {
		t.Errorf("expected -vga virtio, got %s", vga)
	}

	vnc, ok := findArg(args, "-vnc")
	if !ok || !strings.Contains(vnc, "vnc.sock") {
		t.Errorf("expected -vnc unix:.../vnc.sock, got %s", vnc)
	}

	// Verify headless fallback when VNCSock is disabled
	pathsNoVNC := paths
	pathsNoVNC.VNCSock = ""
	argsNoVNC := BuildArgs(cfg, pathsNoVNC, true)
	disp, ok := findArg(argsNoVNC, "-display")
	if !ok || disp != "none" {
		t.Errorf("expected -display none when VNCSock is empty, got %s", disp)
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
	fw := host.DetectUEFIFirmware()
	if !fw.Available {
		t.Skip("host does not have OVMF UEFI firmware installed, skipping test")
	}

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

func TestBuildArgs_32Bit_WithKVM(t *testing.T) {
	cfg := &Config{
		ID:         "winxp-32bit",
		Arch:       "x86",
		CPUs:       2,
		MemoryMB:   2048,
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		Network: NetworkConfig{
			Enabled: true,
			SSHPort: 2222,
		},
	}

	paths := BuildPaths("/tmp/vms/winxp-32bit", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, true)

	if !containsFlag(args, "-enable-kvm") {
		t.Errorf("expected -enable-kvm for 32-bit VM with KVM")
	}

	mach, ok := findArg(args, "-machine")
	if !ok || mach != "pc,accel=kvm" {
		t.Errorf("expected -machine pc,accel=kvm for 32-bit VM, got %s", mach)
	}

	cpu, ok := findArg(args, "-cpu")
	if !ok || cpu != "host" {
		t.Errorf("expected -cpu host for 32-bit KVM VM, got %s", cpu)
	}

	drive, ok := findArg(args, "-drive")
	if !ok || !strings.Contains(drive, "if=ide") {
		t.Errorf("expected if=ide disk drive for 32-bit OS compatibility, got %s", drive)
	}

	vga, ok := findArg(args, "-vga")
	if !ok || vga != "std" {
		t.Errorf("expected -vga std for 32-bit graphical installer compatibility, got %s", vga)
	}

	if !containsFlag(args, "-usb") {
		t.Errorf("expected -usb flag for tablet support")
	}

	// Verify netdev and e1000 device
	foundE1000 := false
	for i, arg := range args {
		if arg == "-device" && i+1 < len(args) && strings.HasPrefix(args[i+1], "e1000") {
			foundE1000 = true
			break
		}
	}
	if !foundE1000 {
		t.Errorf("expected -device e1000 for 32-bit VM, args: %v", args)
	}
}

func TestBuildArgs_32Bit_WithoutKVM(t *testing.T) {
	cfg := &Config{
		ID:       "x86-emulated",
		Arch:     "i386",
		CPUs:     1,
		MemoryMB: 1024,
	}

	paths := BuildPaths("/tmp/vms/x86-emulated", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, false)

	if containsFlag(args, "-enable-kvm") {
		t.Errorf("did not expect -enable-kvm when KVM is disabled")
	}

	mach, _ := findArg(args, "-machine")
	if mach != "pc" {
		t.Errorf("expected -machine pc, got %s", mach)
	}

	cpu, _ := findArg(args, "-cpu")
	if cpu != "qemu32" {
		t.Errorf("expected -cpu qemu32 for emulated 32-bit VM, got %s", cpu)
	}
}

func TestBuildArgs_32Bit_CustomOverrides(t *testing.T) {
	cfg := &Config{
		ID:         "debian-i386-virtio",
		Arch:       "x86",
		Machine:    "q35",
		Disk:       "disk.qcow2",
		DiskFormat: "qcow2",
		DiskBus:    "virtio",
		VGAModel:   "virtio",
		NetModel:   "virtio-net-pci",
		Network: NetworkConfig{
			Enabled: true,
		},
	}

	paths := BuildPaths("/tmp/vms/debian-i386-virtio", "/tmp/iso", cfg)
	args := BuildArgs(cfg, paths, true)

	mach, _ := findArg(args, "-machine")
	if mach != "q35,accel=kvm" {
		t.Errorf("expected overridden -machine q35,accel=kvm, got %s", mach)
	}

	drive, _ := findArg(args, "-drive")
	if !strings.Contains(drive, "if=virtio") {
		t.Errorf("expected overridden if=virtio, got %s", drive)
	}

	vga, _ := findArg(args, "-vga")
	if vga != "virtio" {
		t.Errorf("expected overridden -vga virtio, got %s", vga)
	}
}
