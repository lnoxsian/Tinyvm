package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectUEFIFirmware(t *testing.T) {
	fw := DetectUEFIFirmware()
	t.Logf("Detected UEFI firmware: Available=%v, IsSplit=%v, CodePath=%s, VarsPath=%s",
		fw.Available, fw.IsSplit, fw.CodePath, fw.VarsPath)

	if !fw.Available {
		t.Skip("UEFI firmware not installed on host, skipping NVRAM init test")
	}

	tmpDir, err := os.MkdirTemp("", "tinyvm-uefi-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := InitEFIVars(tmpDir, fw); err != nil {
		t.Fatalf("InitEFIVars failed: %v", err)
	}

	if fw.IsSplit {
		destPath := filepath.Join(tmpDir, "efivars.fd")
		if !fileExists(destPath) {
			t.Errorf("expected efivars.fd to exist at %s", destPath)
		}
	}
}
