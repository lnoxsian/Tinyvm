package host

import (
	"os"
	"strings"
	"testing"
)

func TestGetCPUInfo(t *testing.T) {
	info := GetCPUInfo()
	if info.Count <= 0 {
		t.Errorf("expected positive CPU count, got %d", info.Count)
	}
	if info.Model == "" {
		t.Errorf("expected non-empty CPU model")
	}
}

func TestGetMemoryInfo(t *testing.T) {
	info := GetMemoryInfo()
	// On Linux /proc/meminfo should yield positive bytes
	if info.TotalBytes == 0 {
		t.Logf("TotalBytes is 0 (may occur in non-Linux or restricted environment)")
	} else {
		if info.AvailableBytes > info.TotalBytes {
			t.Errorf("available bytes %d cannot exceed total bytes %d", info.AvailableBytes, info.TotalBytes)
		}
	}
}

func TestGetDiskInfo(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}

	info, err := GetDiskInfo(wd)
	if err != nil {
		t.Fatalf("GetDiskInfo failed: %v", err)
	}

	if info.TotalBytes == 0 {
		t.Errorf("expected non-zero total disk bytes")
	}
	if info.Path != wd {
		t.Errorf("expected path %s, got %s", wd, info.Path)
	}
}

func TestKVMUnavailableNotice(t *testing.T) {
	notice := KVMUnavailableNotice()
	if !strings.Contains(notice, "/dev/kvm") {
		t.Errorf("expected notice to mention /dev/kvm")
	}
}
