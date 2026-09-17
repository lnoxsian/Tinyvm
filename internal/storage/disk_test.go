package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDiskSize(t *testing.T) {
	validSizes := []string{"100M", "10G", "1T", "500m", "2t"}
	for _, s := range validSizes {
		if err := ValidateDiskSize(s); err != nil {
			t.Errorf("expected '%s' to be valid, got error: %v", s, err)
		}
	}

	invalidSizes := []string{"", "10", "10GB", "0G", "-5G", "abc", "10G;rm", "10.5G"}
	for _, s := range invalidSizes {
		if err := ValidateDiskSize(s); err == nil {
			t.Errorf("expected '%s' to be invalid, got nil", s)
		}
	}
}

func TestValidateDiskFormat(t *testing.T) {
	if err := ValidateDiskFormat("qcow2"); err != nil {
		t.Errorf("expected qcow2 to be valid: %v", err)
	}
	if err := ValidateDiskFormat("RAW"); err != nil {
		t.Errorf("expected RAW to be valid: %v", err)
	}
	if err := ValidateDiskFormat("vmdk"); err != ErrInvalidDiskFormat {
		t.Errorf("expected ErrInvalidDiskFormat for vmdk, got %v", err)
	}
}

func TestCreateAndInspectQCOW2Disk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tinyvm-disk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	diskPath := filepath.Join(tmpDir, "test.qcow2")

	// Create a 50M test QCOW2 disk
	if err := CreateQCOW2Disk(diskPath, "50M"); err != nil {
		t.Fatalf("failed to create QCOW2 disk: %v", err)
	}

	if !DiskExists(diskPath) {
		t.Fatalf("expected disk file to exist at %s", diskPath)
	}

	// Inspect disk using qemu-img info
	info, err := InspectDisk(diskPath)
	if err != nil {
		t.Fatalf("failed to inspect disk: %v", err)
	}

	if info.Format != "qcow2" {
		t.Errorf("expected format 'qcow2', got '%s'", info.Format)
	}

	expectedBytes := int64(50 * 1024 * 1024)
	if info.VirtualSize != expectedBytes {
		t.Errorf("expected virtual size %d, got %d", expectedBytes, info.VirtualSize)
	}

	// Delete disk
	if err := DeleteDisk(diskPath); err != nil {
		t.Fatalf("failed to delete disk: %v", err)
	}
	if DiskExists(diskPath) {
		t.Errorf("expected disk to be deleted")
	}
}
