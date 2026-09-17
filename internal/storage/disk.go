package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrInvalidDiskSize is returned when the disk size string format is invalid.
	ErrInvalidDiskSize = errors.New("invalid disk size: must be a positive integer followed by M, G, or T (e.g., '20G', '500M')")
	// ErrInvalidDiskFormat is returned when an unsupported disk format is requested.
	ErrInvalidDiskFormat = errors.New("invalid disk format: only 'qcow2' and 'raw' are supported")
	// ErrDiskExists is returned when trying to create a disk image that already exists.
	ErrDiskExists = errors.New("disk image already exists")
	// ErrDiskNotFound is returned when operating on a disk that does not exist.
	ErrDiskNotFound = errors.New("disk image not found")
)

var diskSizeRegex = regexp.MustCompile(`^[1-9][0-9]*[MGTmgt]$`)

// DiskInfo represents metadata extracted from `qemu-img info`.
type DiskInfo struct {
	VirtualSize int64  `json:"virtual-size"`
	Filename    string `json:"filename"`
	Format      string `json:"format"`
	ActualSize  int64  `json:"actual-size"`
	DirtyFlag   bool   `json:"dirty-flag,omitempty"`
}

// ValidateDiskSize checks if the disk size string matches valid QEMU unit formats (e.g. 10G, 500M).
func ValidateDiskSize(size string) error {
	trimmed := strings.TrimSpace(size)
	if !diskSizeRegex.MatchString(trimmed) {
		return ErrInvalidDiskSize
	}
	return nil
}

// ValidateDiskFormat ensures the format is either qcow2 or raw.
func ValidateDiskFormat(format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "qcow2", "raw":
		return nil
	default:
		return ErrInvalidDiskFormat
	}
}

// CreateQCOW2Disk creates a QCOW2 disk image using qemu-img.
func CreateQCOW2Disk(path string, size string) error {
	return CreateDisk(path, "qcow2", size)
}

// CreateDisk creates a virtual disk image using qemu-img at the specified destination.
func CreateDisk(path string, format string, size string) error {
	if err := ValidateDiskFormat(format); err != nil {
		return err
	}
	if err := ValidateDiskSize(size); err != nil {
		return err
	}

	cleanPath := filepath.Clean(path)
	if DiskExists(cleanPath) {
		return ErrDiskExists
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
		return fmt.Errorf("failed to create parent directory for disk: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "create", "-f", strings.ToLower(format), cleanPath, strings.ToUpper(size))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img create failed: %s (%w)", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// InspectDisk runs `qemu-img info` and decodes the disk's technical information.
func InspectDisk(path string) (*DiskInfo, error) {
	cleanPath := filepath.Clean(path)
	if !DiskExists(cleanPath) {
		return nil, ErrDiskNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", cleanPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("qemu-img info failed: %w", err)
	}

	var info DiskInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to decode qemu-img info output: %w", err)
	}

	return &info, nil
}

// DiskExists checks whether the disk file exists.
func DiskExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DeleteDisk removes a disk file if it exists.
func DeleteDisk(path string) error {
	cleanPath := filepath.Clean(path)
	if !DiskExists(cleanPath) {
		return ErrDiskNotFound
	}
	return os.Remove(cleanPath)
}
