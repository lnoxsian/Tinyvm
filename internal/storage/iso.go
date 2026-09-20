package storage

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// ISOExists checks whether the specified ISO file exists in the ISO storage pool.
func (s *Storage) ISOExists(name string) bool {
	isoPath, err := s.ISOPath(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(isoPath)
	return err == nil && !info.IsDir()
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

	isos := make([]ISOInfo, 0, len(entries))
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

// CopyISO imports a local ISO file into the ISO storage pool by streaming its contents.
func (s *Storage) CopyISO(srcPath string) (*ISOInfo, error) {
	cleanSrc := filepath.Clean(srcPath)
	f, err := os.Open(cleanSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to open source ISO file: %w", err)
	}
	defer f.Close()

	name := filepath.Base(cleanSrc)
	return s.SaveISO(name, f)
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

// DownloadISO downloads an ISO file directly from a remote HTTP/HTTPS URL into storage.
func (s *Storage) DownloadISO(urlStr string, customName string) (*ISOInfo, error) {
	trimmedURL := strings.TrimSpace(urlStr)
	parsed, err := url.Parse(trimmedURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid download URL: must start with http:// or https://")
	}

	targetName := strings.TrimSpace(customName)
	if targetName == "" {
		targetName = filepath.Base(parsed.Path)
	}
	if !strings.HasSuffix(strings.ToLower(targetName), ".iso") {
		targetName += ".iso"
	}

	if err := ValidateISOName(targetName); err != nil {
		return nil, fmt.Errorf("invalid ISO filename '%s': %w", targetName, err)
	}

	if s.ISOExists(targetName) {
		return nil, ErrISOExists
	}

	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	req, err := http.NewRequest("GET", trimmedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("User-Agent", "TinyVM-Downloader/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to download URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download server returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return s.SaveISO(targetName, resp.Body)
}
