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

func TestGetProcessCPUPercent(t *testing.T) {
	pid := os.Getpid()
	pct := GetProcessCPUPercent(pid)
	if pct < 0 {
		t.Errorf("expected CPU percent >= 0, got %f", pct)
	}

	// Do some busy work to consume cycles
	for i := 0; i < 10000000; i++ {
		_ = i * i
	}

	pct2 := GetProcessCPUPercent(pid)
	if pct2 < 0 {
		t.Errorf("expected second CPU percent >= 0, got %f", pct2)
	}

	// Invalid PID should safely return 0
	if GetProcessCPUPercent(-1) != 0.0 {
		t.Errorf("expected 0 CPU percent for PID -1")
	}
	if GetProcessCPUPercent(999999999) != 0.0 {
		t.Errorf("expected 0 CPU percent for non-existent PID")
	}
}

func TestGetVMProcessCPUPercent(t *testing.T) {
	pid := os.Getpid()
	pct := GetVMProcessCPUPercent(pid, 1)
	if pct < 0.0 || pct > 100.0 {
		t.Errorf("expected VM CPU percent between 0 and 100, got %f", pct)
	}

	pct2Cores := GetVMProcessCPUPercent(pid, 2)
	if pct2Cores < 0.0 || pct2Cores > 100.0 {
		t.Errorf("expected VM CPU percent between 0 and 100, got %f", pct2Cores)
	}

	if GetVMProcessCPUPercent(-1, 2) != 0.0 {
		t.Errorf("expected 0 VM CPU percent for PID -1")
	}
	if GetVMProcessCPUPercent(999999999, 2) != 0.0 {
		t.Errorf("expected 0 VM CPU percent for non-existent PID")
	}
}


