package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStorage(t *testing.T) (*Storage, string) {
	tmpDir, err := os.MkdirTemp("", "tinyvm-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	s, err := New(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to initialize storage: %v", err)
	}

	return s, tmpDir
}

func TestValidateVMID(t *testing.T) {
	validIDs := []string{
		"ubuntu-24",
		"debian_12",
		"alpine.vm",
		"test123",
		"web-server-01",
	}

	for _, id := range validIDs {
		if err := ValidateVMID(id); err != nil {
			t.Errorf("expected valid ID '%s', got error: %v", id, err)
		}
	}

	invalidIDs := []string{
		"../escaped",
		"/absolute/path",
		"-leading-dash",
		".leading-dot",
		"has space",
		"evil;cmd",
		"",
		"aVeryLongStringThatExceedsTheSixtyFourCharacterLimitAllowedForVirtualMachineIdentifiersInTinyVM123456",
	}

	for _, id := range invalidIDs {
		if err := ValidateVMID(id); err == nil {
			t.Errorf("expected error for invalid ID '%s', got nil", id)
		}
	}
}

func TestVMDir_PathTraversal(t *testing.T) {
	s, tmpDir := newTestStorage(t)
	defer os.RemoveAll(tmpDir)

	// Valid ID
	path, err := s.VMDir("my-vm")
	if err != nil {
		t.Fatalf("unexpected error for my-vm: %v", err)
	}
	expected := filepath.Join(s.VMsDir(), "my-vm")
	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}

	// Traversal attempt
	_, err = s.VMDir("../etc")
	if err == nil {
		t.Errorf("expected path traversal error, got nil")
	}
}

func TestCreateAndDeleteVMStorage(t *testing.T) {
	s, tmpDir := newTestStorage(t)
	defer os.RemoveAll(tmpDir)

	vmDir, err := s.CreateVMStorage("vm-test-1")
	if err != nil {
		t.Fatalf("failed to create VM storage: %v", err)
	}

	if !s.VMExists("vm-test-1") {
		t.Errorf("expected VM to exist in storage")
	}

	// Verify logs subdirectory
	logsDir := filepath.Join(vmDir, "logs")
	if info, err := os.Stat(logsDir); err != nil || !info.IsDir() {
		t.Errorf("expected logs directory to exist: %v", err)
	}

	// Duplicate creation should fail
	_, err = s.CreateVMStorage("vm-test-1")
	if err != ErrVMExists {
		t.Errorf("expected ErrVMExists, got %v", err)
	}

	// Delete
	if err := s.DeleteVMStorage("vm-test-1"); err != nil {
		t.Fatalf("failed to delete VM storage: %v", err)
	}

	if s.VMExists("vm-test-1") {
		t.Errorf("expected VM storage to be deleted")
	}
}

func TestWriteAndReadVMConfig(t *testing.T) {
	s, tmpDir := newTestStorage(t)
	defer os.RemoveAll(tmpDir)

	_, err := s.CreateVMStorage("cfg-test")
	if err != nil {
		t.Fatalf("failed to create VM storage: %v", err)
	}

	type DummyConfig struct {
		Name string `json:"name"`
		CPUs int    `json:"cpus"`
	}

	original := DummyConfig{Name: "Alpine", CPUs: 2}
	if err := s.WriteVMConfig("cfg-test", original); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var loaded DummyConfig
	if err := s.ReadVMConfig("cfg-test", &loaded); err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if loaded.Name != original.Name || loaded.CPUs != original.CPUs {
		t.Errorf("loaded config %+v does not match original %+v", loaded, original)
	}

	ids, err := s.ListVMIDs()
	if err != nil {
		t.Fatalf("failed to list VM IDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "cfg-test" {
		t.Errorf("expected ['cfg-test'], got %v", ids)
	}
}
