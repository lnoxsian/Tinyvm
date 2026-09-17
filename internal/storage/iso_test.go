package storage

import (
	"os"
	"strings"
	"testing"
)

func TestValidateISOName(t *testing.T) {
	validISOs := []string{"ubuntu-24.04.iso", "alpine-virt.ISO", "debian_12.3.iso"}
	for _, name := range validISOs {
		if err := ValidateISOName(name); err != nil {
			t.Errorf("expected '%s' to be valid, got error: %v", name, err)
		}
	}

	invalidISOs := []string{"", "ubuntu.img", "../evil.iso", "/abs/path.iso", "has space.iso", "evil;rm.iso"}
	for _, name := range invalidISOs {
		if err := ValidateISOName(name); err == nil {
			t.Errorf("expected '%s' to be invalid, got nil", name)
		}
	}
}

func TestISOStorage(t *testing.T) {
	s, tmpDir := newTestStorage(t)
	defer os.RemoveAll(tmpDir)

	content := "dummy iso test bytes content for streaming upload"
	r := strings.NewReader(content)

	info, err := s.SaveISO("alpine-3.20.iso", r)
	if err != nil {
		t.Fatalf("failed to save ISO: %v", err)
	}

	if info.Name != "alpine-3.20.iso" {
		t.Errorf("expected name 'alpine-3.20.iso', got '%s'", info.Name)
	}
	if info.SizeBytes != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), info.SizeBytes)
	}

	// Duplicate save should fail
	_, err = s.SaveISO("alpine-3.20.iso", strings.NewReader(content))
	if err != ErrISOExists {
		t.Errorf("expected ErrISOExists on duplicate, got %v", err)
	}

	// List ISOs
	isos, err := s.ListISOs()
	if err != nil {
		t.Fatalf("failed to list ISOs: %v", err)
	}
	if len(isos) != 1 || isos[0].Name != "alpine-3.20.iso" {
		t.Errorf("expected 1 ISO ('alpine-3.20.iso'), got %v", isos)
	}

	// Get ISO
	single, err := s.GetISO("alpine-3.20.iso")
	if err != nil {
		t.Fatalf("failed to get ISO: %v", err)
	}
	if single.SizeBytes != int64(len(content)) {
		t.Errorf("expected matching size %d, got %d", len(content), single.SizeBytes)
	}

	// Delete ISO
	if err := s.DeleteISO("alpine-3.20.iso"); err != nil {
		t.Fatalf("failed to delete ISO: %v", err)
	}

	if _, err := s.GetISO("alpine-3.20.iso"); err != ErrISONotFound {
		t.Errorf("expected ErrISONotFound after delete, got %v", err)
	}
}

func TestCopyISO(t *testing.T) {
	s, tmpDir := newTestStorage(t)
	defer os.RemoveAll(tmpDir)

	// Create a dummy local ISO file
	srcPath := tmpDir + "/source-test.iso"
	if err := os.WriteFile(srcPath, []byte("iso content to copy"), 0644); err != nil {
		t.Fatalf("failed to write source iso: %v", err)
	}

	info, err := s.CopyISO(srcPath)
	if err != nil {
		t.Fatalf("CopyISO failed: %v", err)
	}

	if info.Name != "source-test.iso" {
		t.Errorf("expected 'source-test.iso', got '%s'", info.Name)
	}

	if _, err := s.GetISO("source-test.iso"); err != nil {
		t.Errorf("expected copied ISO to be in storage: %v", err)
	}
}
