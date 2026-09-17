package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrInvalidISOName is returned when the ISO filename is invalid or contains path traversal.
	ErrInvalidISOName = errors.New("invalid ISO filename: must be a clean filename ending with .iso without directory components")
	// ErrISONotFound is returned when requested ISO file does not exist.
	ErrISONotFound = errors.New("ISO image not found")
	// ErrISOExists is returned when trying to create/upload an ISO that already exists.
	ErrISOExists = errors.New("ISO image already exists")
)

var isoNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*\.[iI][sS][oO]$`)

// ISOInfo represents metadata for an ISO image file.
type ISOInfo struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	ModTime   time.Time `json:"mod_time"`
	Path      string    `json:"path"`
}

// ValidateISOName checks if the filename is a valid, safe ISO filename.
func ValidateISOName(name string) error {
	trimmed := strings.TrimSpace(name)
	if filepath.Base(trimmed) != trimmed || strings.ContainsAny(trimmed, "/\\") || strings.Contains(trimmed, "..") {
		return ErrInvalidISOName
	}
	if !isoNameRegex.MatchString(trimmed) {
		return ErrInvalidISOName
	}
	return nil
}

// ISOPath returns the absolute path for an ISO image, validating against path traversal.
func (s *Storage) ISOPath(name string) (string, error) {
	if err := ValidateISOName(name); err != nil {
		return "", err
	}

	targetPath := filepath.Clean(filepath.Join(s.isoDir, name))
	rel, err := filepath.Rel(s.isoDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", ErrPathEscapesRoot
	}

	return targetPath, nil
}

// ListISOs lists all ISO files stored in the ISO directory.
func (s *Storage) ListISOs() ([]ISOInfo, error) {
	entries, err := os.ReadDir(s.isoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read ISO directory: %w", err)
	}

	var isos []ISOInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := ValidateISOName(name); err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		isos = append(isos, ISOInfo{
			Name:      name,
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
			Path:      filepath.Join(s.isoDir, name),
		})
	}

	return isos, nil
}

// GetISO retrieves details for a single ISO by filename.
func (s *Storage) GetISO(name string) (*ISOInfo, error) {
	isoPath, err := s.ISOPath(name)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(isoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrISONotFound
		}
		return nil, fmt.Errorf("failed to stat ISO: %w", err)
	}

	if info.IsDir() {
		return nil, ErrISONotFound
	}

	return &ISOInfo{
		Name:      name,
		SizeBytes: info.Size(),
		ModTime:   info.ModTime(),
		Path:      isoPath,
	}, nil
}

// SaveISO streams an ISO from an io.Reader directly to disk without loading it entirely into RAM.
func (s *Storage) SaveISO(name string, r io.Reader) (*ISOInfo, error) {
	isoPath, err := s.ISOPath(name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(isoPath); err == nil {
		return nil, ErrISOExists
	}

	tmpPath := isoPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary ISO file: %w", err)
	}

	// Stream write using a 1MB buffer for high throughput
	buf := make([]byte, 1024*1024)
	written, err := io.CopyBuffer(tmpFile, r, buf)
	_ = tmpFile.Close()

	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to stream ISO data: %w", err)
	}

	if err := os.Rename(tmpPath, isoPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to commit ISO file: %w", err)
	}

	info, _ := os.Stat(isoPath)
	return &ISOInfo{
		Name:      name,
		SizeBytes: written,
		ModTime:   info.ModTime(),
		Path:      isoPath,
	}, nil
}

// DeleteISO removes an ISO from storage.
func (s *Storage) DeleteISO(name string) error {
	isoPath, err := s.ISOPath(name)
	if err != nil {
		return err
	}

	if _, err := os.Stat(isoPath); err != nil {
		if os.IsNotExist(err) {
			return ErrISONotFound
		}
		return err
	}

	return os.Remove(isoPath)
}
