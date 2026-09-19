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
	CPUs       int           `json:"cpus"`
	MemoryMB   int           `json:"memory_mb"`
	Disk       string        `json:"disk"`
	DiskFormat string        `json:"disk_format"`
	ISO        string        `json:"iso,omitempty"`
	Firmware   string        `json:"firmware,omitempty"` // "bios" or "uefi"
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

	// 1. Machine & Acceleration
	if useKVM {
		args = append(args,
			"-enable-kvm",
			"-machine", "q35,accel=kvm",
			"-cpu", "host",
		)
	} else {
		args = append(args,
			"-machine", "q35",
			"-cpu", "qemu64",
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

	// 3. Virtual Disk
	if paths.DiskPath != "" {
		format := cfg.DiskFormat
		if format == "" {
			format = "qcow2"
		}
		driveOpt := fmt.Sprintf("file=%s,format=%s,if=virtio", paths.DiskPath, strings.ToLower(format))
		args = append(args, "-drive", driveOpt)
	}

	// 4. ISO / CD-ROM & Boot Order
	if paths.ISOPath != "" {
		isoDrive := fmt.Sprintf("file=%s,media=cdrom", paths.ISOPath)
		args = append(args, "-drive", isoDrive)
		args = append(args, "-boot", "order=d,menu=on")
	} else {
		args = append(args, "-boot", "order=c")
	}

	// 5. User-mode Networking & Port Forwarding
	if cfg.Network.Enabled {
		args = append(args, "-device", "virtio-net-pci,netdev=net0")

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

	// 6. QMP Management Socket
	args = append(args, "-qmp", fmt.Sprintf("unix:%s,server=on,wait=off", paths.QMPSock))

	// 7. Serial Console Socket
	args = append(args, "-serial", fmt.Sprintf("unix:%s,server=on,wait=off", paths.ConsoleSock))

	// 8. Graphical Display / VNC Socket
	if paths.VNCSock != "" {
		args = append(args,
			"-vga", "virtio",
			"-vnc", fmt.Sprintf("unix:%s", paths.VNCSock),
		)
	} else {
		args = append(args, "-display", "none")
	}

	// 9. PID File
	args = append(args, "-pidfile", paths.PIDFile)

	return args
}
