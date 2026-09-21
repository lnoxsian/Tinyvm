package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrInvalidVMID is returned when a VM identifier contains illegal characters.
	ErrInvalidVMID = errors.New("invalid VM ID: must start with alphanumeric and contain only alphanumeric, '.', '_', or '-' (max 64 chars)")
	// ErrPathEscapesRoot is returned when a resolved path escapes the intended root directory.
	ErrPathEscapesRoot = errors.New("path traversal detected: path escapes storage root")
	// ErrVMExists is returned when trying to create a VM that already exists.
	ErrVMExists = errors.New("VM already exists in storage")
	// ErrVMNotFound is returned when a VM directory does not exist.
	ErrVMNotFound = errors.New("VM not found in storage")
)

var vmidRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Storage manages on-disk storage directories, disk images, and ISO files.
type Storage struct {
	dataDir   string
	vmsDir    string
	isoDir    string
	configDir string
}

// New creates and initializes a Storage instance at the specified data directory.
func New(dataDir string) (*Storage, error) {
	cleanPath, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("invalid storage data directory: %w", err)
	}

	s := &Storage{
		dataDir:   cleanPath,
		vmsDir:    filepath.Join(cleanPath, "vms"),
		isoDir:    filepath.Join(cleanPath, "iso"),
		configDir: filepath.Join(cleanPath, "config"),
	}

	if err := s.EnsureDirs(); err != nil {
		return nil, err
	}

	return s, nil
}

// DataDir returns the root data directory.
func (s *Storage) DataDir() string { return s.dataDir }

// VMsDir returns the virtual machines directory.
func (s *Storage) VMsDir() string { return s.vmsDir }

// ISODir returns the ISO storage directory.
func (s *Storage) ISODir() string { return s.isoDir }

// ConfigDir returns the configuration directory.
func (s *Storage) ConfigDir() string { return s.configDir }

// EnsureDirs creates required base storage directories.
func (s *Storage) EnsureDirs() error {
	dirs := []string{s.dataDir, s.vmsDir, s.isoDir, s.configDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// ValidateVMID validates that the VM ID contains only safe characters.
func ValidateVMID(id string) error {
	if !vmidRegex.MatchString(id) {
		return ErrInvalidVMID
	}
	return nil
}

// VMDir returns the absolute directory path for a given VM ID, ensuring no path traversal.
func (s *Storage) VMDir(vmID string) (string, error) {
	if err := ValidateVMID(vmID); err != nil {
		return "", err
	}

	targetPath := filepath.Clean(filepath.Join(s.vmsDir, vmID))
	// Ensure targetPath is strictly inside vmsDir
	rel, err := filepath.Rel(s.vmsDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", ErrPathEscapesRoot
	}

	return targetPath, nil
}

// CreateVMStorage creates the VM directory and its subdirectories (e.g. logs/).
func (s *Storage) CreateVMStorage(vmID string) (string, error) {
	vmDir, err := s.VMDir(vmID)
	if err != nil {
		return "", err
	}

	if s.VMExists(vmID) {
		return "", ErrVMExists
	}

	if err := os.MkdirAll(vmDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create VM directory: %w", err)
	}

	logsDir := filepath.Join(vmDir, "logs")
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create logs directory: %w", err)
	}

	return vmDir, nil
}

// DeleteVMStorage removes the entire VM directory and its contents.
func (s *Storage) DeleteVMStorage(vmID string) error {
	vmDir, err := s.VMDir(vmID)
	if err != nil {
		return err
	}

	if !s.VMExists(vmID) {
		return ErrVMNotFound
	}

	return os.RemoveAll(vmDir)
}

// VMExists checks if a VM directory exists and is a directory.
func (s *Storage) VMExists(vmID string) bool {
	vmDir, err := s.VMDir(vmID)
	if err != nil {
		return false
	}
	info, err := os.Stat(vmDir)
	return err == nil && info.IsDir()
}

// WriteVMConfig writes VM configuration JSON atomically into <vmDir>/config.json.
func (s *Storage) WriteVMConfig(vmID string, cfgData any) error {
	vmDir, err := s.VMDir(vmID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfgData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize VM configuration: %w", err)
	}

	configPath := filepath.Join(vmDir, "config.json")
	tmpPath := configPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0640); err != nil {
		return fmt.Errorf("failed to write temporary config file: %w", err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to commit config file: %w", err)
	}

	return nil
}

// ReadVMConfig reads and deserializes the VM's config.json.
func (s *Storage) ReadVMConfig(vmID string, target any) error {
	vmDir, err := s.VMDir(vmID)
	if err != nil {
		return err
	}

	configPath := filepath.Join(vmDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrVMNotFound
		}
		return fmt.Errorf("failed to read VM config: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse VM config: %w", err)
	}

	return nil
}

// ListVMIDs scans the vms directory and returns a list of VM IDs that contain a config.json.
func (s *Storage) ListVMIDs() ([]string, error) {
	entries, err := os.ReadDir(s.vmsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read VMs directory: %w", err)
	}

	vmIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		vmID := entry.Name()
		if err := ValidateVMID(vmID); err != nil {
			continue
		}

		cfgPath := filepath.Join(s.vmsDir, vmID, "config.json")
		if _, err := os.Stat(cfgPath); err == nil {
			vmIDs = append(vmIDs, vmID)
		}
	}

	return vmIDs, nil
}

// FormatBytes formats a byte size into human-readable representation.
func FormatBytes(bytes int64) string {
	if bytes < 0 {
		return "0 B"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		f := float64(bytes) / float64(gb)
		if f == float64(int64(f)) {
			return fmt.Sprintf("%.0f GB", f)
		}
		return fmt.Sprintf("%.2f GB", f)
	case bytes >= mb:
		f := float64(bytes) / float64(mb)
		if f == float64(int64(f)) {
			return fmt.Sprintf("%.0f MB", f)
		}
		return fmt.Sprintf("%.1f MB", f)
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

