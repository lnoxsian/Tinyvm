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
	StateError    VMState = "error"
)

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
	CPUs       int           `json:"cpus"`
	MemoryMB   int           `json:"memory_mb"`
	Disk       string        `json:"disk"`
	DiskFormat string        `json:"disk_format"`
	DiskSize   string        `json:"disk_size,omitempty"`
	ISO        string        `json:"iso,omitempty"`
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
		CPUs:       c.CPUs,
		MemoryMB:   c.MemoryMB,
		Disk:       c.Disk,
		DiskFormat: c.DiskFormat,
		ISO:        c.ISO,
		Network: qemu.NetworkConfig{
			Enabled: c.Network.Enabled,
			Mode:    c.Network.Mode,
			SSHPort: c.Network.SSHPort,
			Ports:   ports,
		},
	}
}
