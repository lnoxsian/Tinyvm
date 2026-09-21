package vm

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tinyvm/internal/qemu"
	"tinyvm/internal/storage"
)

var (
	ErrInvalidVMName  = errors.New("invalid VM name: must not be empty and under 64 characters")
	ErrInvalidCPUs    = errors.New("invalid CPU count: must be between 1 and 256")
	ErrInvalidMemory  = errors.New("invalid memory: must be at least 128 MB and up to 1048576 MB (1 TB)")
	ErrInvalidPort    = errors.New("invalid port number: must be between 1 and 65535")
	ErrDuplicatePort  = errors.New("duplicate host port detected in port forward configuration")
	ErrInvalidDiskName = errors.New("invalid disk name: must be a clean filename without directory separators")
)

var vmNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._ -]{0,63}$`)

// VMState represents the lifecycle state of a VM.
type VMState string

const (
	StateStopped  VMState = "stopped"
	StateStarting VMState = "starting"
	StateRunning  VMState = "running"
	StateStopping VMState = "stopping"
	StatePaused   VMState = "paused"
	StateError    VMState = "error"
)

// SnapshotInfo represents a saved VM snapshot.
type SnapshotInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	VMStateSize int64     `json:"vm_state_size"`
	Date        time.Time `json:"date"`
	Description string    `json:"description,omitempty"`
}

// VMRuntime tracks in-memory process information for a VM.
type VMRuntime struct {
	PID       int       `json:"pid"`
	State     VMState   `json:"state"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

// PortForward configures network port forwarding from host to guest.
type PortForward struct {
	Host     int    `json:"host"`
	Guest    int    `json:"guest"`
	Protocol string `json:"protocol"` // "tcp" or "udp"
}

// NetworkConfig holds network settings for the VM.
type NetworkConfig struct {
	Enabled bool          `json:"enabled"`
	Mode    string        `json:"mode"` // "user"
	SSHPort int           `json:"ssh_port,omitempty"`
	Ports   []PortForward `json:"ports,omitempty"`
}

// VMConfig stores user configuration serialized in config.json.
type VMConfig struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Arch       string        `json:"arch,omitempty"`       // "x86_64" (default) or "x86" / "i386"
	Machine    string        `json:"machine,omitempty"`    // "q35" (default 64-bit) or "pc" (default 32-bit)
	DiskBus    string        `json:"disk_bus,omitempty"`   // "virtio" (default 64-bit), "ide" (default 32-bit), "sata"
	VGAModel   string        `json:"vga_model,omitempty"`  // "virtio" (default 64-bit), "std" (default 32-bit), "qxl", "cirrus"
	NetModel   string        `json:"net_model,omitempty"`  // "virtio-net-pci" (default 64-bit), "e1000" (default 32-bit), "rtl8139"
	CPUs       int           `json:"cpus"`
	MemoryMB   int           `json:"memory_mb"`
	Disk       string        `json:"disk"`
	DiskFormat string        `json:"disk_format"`
	DiskSize   string        `json:"disk_size,omitempty"`
	ISO        string        `json:"iso,omitempty"`
	Firmware   string        `json:"firmware,omitempty"` // "bios" or "uefi"
	OSType     string        `json:"os_type,omitempty"`  // e.g. "Linux", "Debian", "Ubuntu", "Windows"
	BootOrder  string        `json:"boot_order,omitempty"` // e.g. "d" (CD first), "c" (Disk first), "dc"
	Autostart  bool          `json:"autostart,omitempty"`
	Network    NetworkConfig `json:"network"`
}

// VM represents a managed virtual machine instance.
type VM struct {
	Config  VMConfig  `json:"config"`
	Runtime VMRuntime `json:"runtime"`
}

// Validate checks that VM configuration fields comply with security and architecture constraints.
func (c *VMConfig) Validate() error {
	if err := storage.ValidateVMID(c.ID); err != nil {
		return err
	}

	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || !vmNameRegex.MatchString(c.Name) {
		return ErrInvalidVMName
	}

	if c.Arch == "" {
		c.Arch = "x86_64"
	} else {
		c.Arch = strings.ToLower(strings.TrimSpace(c.Arch))
		if c.Arch != "x86_64" && c.Arch != "x86" && c.Arch != "i386" && c.Arch != "x86_32" {
			return errors.New("invalid architecture: must be 'x86_64' or 'x86'")
		}
	}

	if c.CPUs < 1 || c.CPUs > 256 {
		return ErrInvalidCPUs
	}

	if c.MemoryMB < 128 || c.MemoryMB > 1048576 {
		return ErrInvalidMemory
	}

	if c.Disk == "" {
		c.Disk = "disk.qcow2"
	}
	if filepath.Base(c.Disk) != c.Disk || strings.ContainsAny(c.Disk, "/\\") {
		return ErrInvalidDiskName
	}

	if c.DiskFormat == "" {
		c.DiskFormat = "qcow2"
	}
	if err := storage.ValidateDiskFormat(c.DiskFormat); err != nil {
		return err
	}

	if c.DiskBus != "" {
		c.DiskBus = strings.ToLower(strings.TrimSpace(c.DiskBus))
		if c.DiskBus != "virtio" && c.DiskBus != "ide" && c.DiskBus != "sata" {
			return errors.New("invalid disk bus: must be 'virtio', 'ide', or 'sata'")
		}
	}

	if c.VGAModel != "" {
		c.VGAModel = strings.ToLower(strings.TrimSpace(c.VGAModel))
		if c.VGAModel != "virtio" && c.VGAModel != "std" && c.VGAModel != "qxl" && c.VGAModel != "cirrus" {
			return errors.New("invalid vga model: must be 'virtio', 'std', 'qxl', or 'cirrus'")
		}
	}

	if c.NetModel != "" {
		c.NetModel = strings.ToLower(strings.TrimSpace(c.NetModel))
		if c.NetModel != "virtio" && c.NetModel != "virtio-net-pci" && c.NetModel != "e1000" && c.NetModel != "rtl8139" {
			return errors.New("invalid net model: must be 'virtio', 'e1000', or 'rtl8139'")
		}
	}

	if c.Machine != "" {
		c.Machine = strings.ToLower(strings.TrimSpace(c.Machine))
		if c.Machine != "q35" && c.Machine != "pc" && c.Machine != "i440fx" {
			return errors.New("invalid machine: must be 'q35' or 'pc'")
		}
	}

	if c.DiskSize != "" {
		if err := storage.ValidateDiskSize(c.DiskSize); err != nil {
			return err
		}
	}

	if c.ISO != "" {
		if err := storage.ValidateISOName(c.ISO); err != nil {
			return err
		}
	}

	if c.Firmware == "" {
		c.Firmware = "bios"
	} else {
		c.Firmware = strings.ToLower(strings.TrimSpace(c.Firmware))
		if c.Firmware != "bios" && c.Firmware != "uefi" {
			return errors.New("invalid firmware: must be 'bios' or 'uefi'")
		}
	}

	if (c.Arch == "x86" || c.Arch == "i386" || c.Arch == "x86_32") && c.Firmware == "uefi" {
		return errors.New("invalid firmware: 32-bit (x86) VMs must use 'bios' (SeaBIOS) firmware")
	}

	if c.Network.Mode == "" {
		c.Network.Mode = "user"
	}

	// Validate ports
	hostPorts := make(map[int]bool)
	if c.Network.SSHPort > 0 {
		if c.Network.SSHPort > 65535 {
			return ErrInvalidPort
		}
		hostPorts[c.Network.SSHPort] = true
	}

	for _, p := range c.Network.Ports {
		if p.Host < 1 || p.Host > 65535 || p.Guest < 1 || p.Guest > 65535 {
			return ErrInvalidPort
		}
		if hostPorts[p.Host] {
			return fmt.Errorf("%w: %d", ErrDuplicatePort, p.Host)
		}
		hostPorts[p.Host] = true
	}

	return nil
}

// ToQEMUConfig converts VMConfig into arguments configuration for QEMU.
func (c *VMConfig) ToQEMUConfig() *qemu.Config {
	var ports []qemu.PortForward
	for _, p := range c.Network.Ports {
		ports = append(ports, qemu.PortForward{
			Host:     p.Host,
			Guest:    p.Guest,
			Protocol: p.Protocol,
		})
	}

	return &qemu.Config{
		ID:         c.ID,
		Arch:       c.Arch,
		Machine:    c.Machine,
		DiskBus:    c.DiskBus,
		VGAModel:   c.VGAModel,
		NetModel:   c.NetModel,
		CPUs:       c.CPUs,
		MemoryMB:   c.MemoryMB,
		Disk:       c.Disk,
		DiskFormat: c.DiskFormat,
		ISO:        c.ISO,
		Firmware:   c.Firmware,
		BootOrder:  c.BootOrder,
		Network: qemu.NetworkConfig{
			Enabled: c.Network.Enabled,
			Mode:    c.Network.Mode,
			SSHPort: c.Network.SSHPort,
			Ports:   ports,
		},
	}
}
