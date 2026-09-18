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

func TestGetProcessRSSBytes(t *testing.T) {
	// Current test runner process should have non-zero RSS on Linux
	rss := GetProcessRSSBytes(os.Getpid())
	if rss == 0 {
		t.Logf("GetProcessRSSBytes returned 0 (non-Linux or permission restricted)")
	} else {
		t.Logf("Process %d RSS: %d bytes (%.2f MB)", os.Getpid(), rss, float64(rss)/(1024*1024))
	}
	// Invalid PID should safely return 0
	if GetProcessRSSBytes(-1) != 0 {
		t.Errorf("expected 0 RSS for PID -1")
	}
}
