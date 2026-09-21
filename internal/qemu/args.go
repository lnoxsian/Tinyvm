package qemu

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"tinyvm/internal/host"
)

// PortForward configures network port forwarding.
type PortForward struct {
	Host     int    `json:"host"`
	Guest    int    `json:"guest"`
	Protocol string `json:"protocol"`
}

// NetworkConfig configures user-mode networking.
type NetworkConfig struct {
	Enabled bool          `json:"enabled"`
	Mode    string        `json:"mode"`
	SSHPort int           `json:"ssh_port,omitempty"`
	Ports   []PortForward `json:"ports,omitempty"`
}

// Config specifies the runtime arguments required for a QEMU instance.
type Config struct {
	ID         string        `json:"id"`
	Arch       string        `json:"arch,omitempty"`      // "x86_64" (default) or "x86" / "i386"
	Machine    string        `json:"machine,omitempty"`   // "q35" (default 64-bit) or "pc" (default 32-bit / legacy)
	DiskBus    string        `json:"disk_bus,omitempty"`  // "virtio" (default 64-bit), "ide" (default 32-bit), "sata"
	VGAModel   string        `json:"vga_model,omitempty"` // "virtio" (default 64-bit), "std" (default 32-bit), "qxl", "cirrus"
	NetModel   string        `json:"net_model,omitempty"` // "virtio-net-pci" (default 64-bit), "e1000" (default 32-bit), "rtl8139"
	CPUs       int           `json:"cpus"`
	MemoryMB   int           `json:"memory_mb"`
	Disk       string        `json:"disk"`
	DiskFormat string        `json:"disk_format"`
	ISO        string        `json:"iso,omitempty"`
	Firmware   string        `json:"firmware,omitempty"` // "bios" or "uefi"
	BootOrder  string        `json:"boot_order,omitempty"`
	Network    NetworkConfig `json:"network"`
}

// QEMUPaths holds required absolute paths for QEMU arguments.
type QEMUPaths struct {
	VMDir       string
	ISODir      string
	DiskPath    string
	ISOPath     string
	EFIVars     string
	QMPSock     string
	ConsoleSock string
	VNCSock     string
	PIDFile     string
}

// BuildPaths derives standard socket and file paths for a given VM.
func BuildPaths(vmDir string, isoDir string, cfg *Config) QEMUPaths {
	diskPath := ""
	if cfg.Disk != "" {
		diskPath = filepath.Join(vmDir, cfg.Disk)
	}

	isoPath := ""
	if cfg.ISO != "" {
		isoPath = filepath.Join(isoDir, cfg.ISO)
	}

	return QEMUPaths{
		VMDir:       vmDir,
		ISODir:      isoDir,
		DiskPath:    diskPath,
		ISOPath:     isoPath,
		EFIVars:     filepath.Join(vmDir, "efivars.fd"),
		QMPSock:     filepath.Join(vmDir, "qmp.sock"),
		ConsoleSock: filepath.Join(vmDir, "console.sock"),
		VNCSock:     filepath.Join(vmDir, "vnc.sock"),
		PIDFile:     filepath.Join(vmDir, "qemu.pid"),
	}
}

// BuildArgs constructs safe, programmatically formatted QEMU arguments.
func BuildArgs(cfg *Config, paths QEMUPaths, useKVM bool) []string {
	var args []string

	arch := strings.ToLower(strings.TrimSpace(cfg.Arch))
	is32Bit := (arch == "x86" || arch == "i386" || arch == "x86_32")

	// 1. Machine & Acceleration
	machineType := strings.ToLower(strings.TrimSpace(cfg.Machine))
	if machineType == "" {
		if is32Bit {
			machineType = "pc" // i440fx: standard PC chipset for legacy & 32-bit OS compatibility
		} else {
			machineType = "q35" // PCIe ICH9 chipset for modern 64-bit OS
		}
	} else if machineType == "i440fx" {
		machineType = "pc"
	}

	if useKVM {
		args = append(args,
			"-enable-kvm",
			"-machine", fmt.Sprintf("%s,accel=kvm", machineType),
			"-cpu", "host",
		)
	} else {
		cpuModel := "qemu64"
		if is32Bit {
			cpuModel = "qemu32"
		}
		args = append(args,
			"-machine", machineType,
			"-cpu", cpuModel,
		)
	}

	// 2. Firmware (BIOS or UEFI)
	if strings.ToLower(cfg.Firmware) == "uefi" {
		fw := host.DetectUEFIFirmware()
		if fw.Available {
			if fw.IsSplit {
				varsPath := paths.EFIVars
				if varsPath == "" {
					varsPath = filepath.Join(paths.VMDir, "efivars.fd")
				}
				_ = host.InitEFIVars(paths.VMDir, fw)
				args = append(args,
					"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", fw.CodePath),
					"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", varsPath),
				)
			} else {
				args = append(args, "-bios", fw.CodePath)
			}
		}
	}

	// 3. SMP & Memory
	args = append(args,
		"-smp", strconv.Itoa(cfg.CPUs),
		"-m", strconv.Itoa(cfg.MemoryMB),
	)

	// 4. Virtual Disk
	if paths.DiskPath != "" {
		format := cfg.DiskFormat
		if format == "" {
			format = "qcow2"
		}
		bus := strings.ToLower(strings.TrimSpace(cfg.DiskBus))
		if bus == "" {
			if is32Bit {
				bus = "ide" // Native IDE controller allows 32-bit installers to detect drive without extra drivers
			} else {
				bus = "virtio"
			}
		}
		driveOpt := fmt.Sprintf("file=%s,format=%s,if=%s", paths.DiskPath, strings.ToLower(format), bus)
		args = append(args, "-drive", driveOpt)
	}

	// 5. ISO / CD-ROM & Boot Order
	if paths.ISOPath != "" {
		isoDrive := fmt.Sprintf("file=%s,media=cdrom,id=cdrom0", paths.ISOPath)
		args = append(args, "-drive", isoDrive)
	}

	bootOrder := strings.TrimSpace(cfg.BootOrder)
	if bootOrder != "" {
		if !strings.Contains(bootOrder, "menu=") {
			args = append(args, "-boot", fmt.Sprintf("order=%s,menu=on", bootOrder))
		} else {
			args = append(args, "-boot", bootOrder)
		}
	} else if paths.ISOPath != "" {
		args = append(args, "-boot", "order=d,menu=on")
	} else {
		args = append(args, "-boot", "order=c")
	}

	// 6. User-mode Networking & Port Forwarding
	if cfg.Network.Enabled {
		netModel := strings.ToLower(strings.TrimSpace(cfg.NetModel))
		if netModel == "" {
			if is32Bit {
				netModel = "e1000" // Intel e1000 has universal in-tree drivers across 32-bit OSes
			} else {
				netModel = "virtio-net-pci"
			}
		} else if netModel == "virtio" {
			netModel = "virtio-net-pci"
		}
		args = append(args, "-device", fmt.Sprintf("%s,netdev=net0", netModel))

		netdevParts := []string{"user", "id=net0"}
		if cfg.Network.SSHPort > 0 {
			netdevParts = append(netdevParts, fmt.Sprintf("hostfwd=tcp::%d-:22", cfg.Network.SSHPort))
		}
		for _, p := range cfg.Network.Ports {
			proto := strings.ToLower(p.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			netdevParts = append(netdevParts, fmt.Sprintf("hostfwd=%s::%d-:%d", proto, p.Host, p.Guest))
		}
		args = append(args, "-netdev", strings.Join(netdevParts, ","))
	}

	// 7. QMP Management Socket
	args = append(args, "-qmp", fmt.Sprintf("unix:%s,server=on,wait=off", paths.QMPSock))

	// 8. Serial Console Socket
	args = append(args, "-serial", fmt.Sprintf("unix:%s,server=on,wait=off", paths.ConsoleSock))

	// 9. Graphical Display / VNC Socket & Tablet
	if paths.VNCSock != "" {
		vga := strings.ToLower(strings.TrimSpace(cfg.VGAModel))
		if vga == "" {
			if is32Bit {
				vga = "std" // Standard VGA / Bochs VBE works across all 32-bit graphical OS installers
			} else {
				vga = "virtio"
			}
		}
		args = append(args,
			"-vga", vga,
			"-vnc", fmt.Sprintf("unix:%s", paths.VNCSock),
		)
		// Enable USB bus and tablet for seamless, drift-free mouse tracking in web console
		args = append(args, "-usb", "-device", "usb-tablet")
	} else {
		args = append(args, "-display", "none")
	}

	// 10. PID File
	args = append(args, "-pidfile", paths.PIDFile)

	return args
}
