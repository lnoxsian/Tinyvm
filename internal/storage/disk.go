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

// QEMUSnapshotRaw represents the snapshot entry from qemu-img info json.
type QEMUSnapshotRaw struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	VMStateSize int64  `json:"vm-state-size"`
	DateSec     int64  `json:"date-sec"`
	DateNsec    int64  `json:"date-nsec"`
}

// DiskSnapshot represents parsed snapshot metadata.
type DiskSnapshot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	VMStateSize int64     `json:"vm_state_size"`
	Date        time.Time `json:"date"`
}

// DiskInfo represents metadata extracted from `qemu-img info`.
type DiskInfo struct {
	VirtualSize int64             `json:"virtual-size"`
	Filename    string            `json:"filename"`
	Format      string            `json:"format"`
	ActualSize  int64             `json:"actual-size"`
	DirtyFlag   bool              `json:"dirty-flag,omitempty"`
	Snapshots   []QEMUSnapshotRaw `json:"snapshots,omitempty"`
}

// ValidateDiskSize checks if the disk size string matches valid QEMU unit formats (e.g. 10G, 500M).
func ValidateDiskSize(size string) error {
	trimmed := strings.TrimSpace(size)
	if len(trimmed) < 2 {
		return ErrInvalidDiskSize
	}
	if trimmed[0] < '1' || trimmed[0] > '9' {
		return ErrInvalidDiskSize
	}
	for i := 1; i < len(trimmed)-1; i++ {
		if trimmed[i] < '0' || trimmed[i] > '9' {
			return ErrInvalidDiskSize
		}
	}
	switch trimmed[len(trimmed)-1] {
	case 'M', 'G', 'T', 'm', 'g', 't':
		return nil
	default:
		return ErrInvalidDiskSize
	}
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

// ResizeDisk expands or modifies the virtual disk image size using qemu-img resize.
func ResizeDisk(path string, size string) error {
	cleanPath := filepath.Clean(path)
	if !DiskExists(cleanPath) {
		return ErrDiskNotFound
	}

	trimmed := strings.TrimSpace(size)
	valSize := trimmed
	if strings.HasPrefix(valSize, "+") {
		valSize = valSize[1:]
	}
	if err := ValidateDiskSize(valSize); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "resize", cleanPath, trimmed)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img resize failed: %s (%w)", strings.TrimSpace(string(output)), err)
	}
	return nil
}

var snapshotNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// ValidateSnapshotName checks that the snapshot name is alphanumeric and safe.
func ValidateSnapshotName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || !snapshotNameRegex.MatchString(trimmed) {
		return errors.New("invalid snapshot name: must be alphanumeric (a-z, 0-9, ., _, -) and up to 64 chars")
	}
	return nil
}

// CreateDiskSnapshot creates an internal snapshot in the qcow2 disk.
func CreateDiskSnapshot(diskPath string, snapName string) error {
	if err := ValidateSnapshotName(snapName); err != nil {
		return err
	}
	cleanPath := filepath.Clean(diskPath)
	if !DiskExists(cleanPath) {
		return ErrDiskNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "snapshot", "-c", snapName, cleanPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img snapshot create failed: %s (%w)", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// ListDiskSnapshots retrieves all internal snapshots stored in the qcow2 disk.
func ListDiskSnapshots(diskPath string) ([]DiskSnapshot, error) {
	info, err := InspectDisk(diskPath)
	if err != nil {
		return nil, err
	}

	snapshots := make([]DiskSnapshot, 0, len(info.Snapshots))
	for _, s := range info.Snapshots {
		var snapTime time.Time
		if s.DateSec > 0 {
			snapTime = time.Unix(s.DateSec, s.DateNsec)
		}
		snapshots = append(snapshots, DiskSnapshot{
			ID:          s.ID,
			Name:        s.Name,
			VMStateSize: s.VMStateSize,
			Date:        snapTime,
		})
	}
	return snapshots, nil
}

// ApplyDiskSnapshot rolls back the disk to a named snapshot.
func ApplyDiskSnapshot(diskPath string, snapName string) error {
	if err := ValidateSnapshotName(snapName); err != nil {
		return err
	}
	cleanPath := filepath.Clean(diskPath)
	if !DiskExists(cleanPath) {
		return ErrDiskNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "snapshot", "-a", snapName, cleanPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img snapshot rollback failed: %s (%w)", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// DeleteDiskSnapshot removes an internal snapshot from the qcow2 disk.
func DeleteDiskSnapshot(diskPath string, snapName string) error {
	if err := ValidateSnapshotName(snapName); err != nil {
		return err
	}
	cleanPath := filepath.Clean(diskPath)
	if !DiskExists(cleanPath) {
		return ErrDiskNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "snapshot", "-d", snapName, cleanPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img snapshot delete failed: %s (%w)", strings.TrimSpace(string(output)), err)
	}
	return nil
}
