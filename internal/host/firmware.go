package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrUEFINotSupported = errors.New("UEFI firmware (OVMF) is not installed on the host system")
)

// UEFIFirmware holds paths to OVMF code and vars files.
type UEFIFirmware struct {
	Available bool   `json:"available"`
	CodePath  string `json:"code_path"`
	VarsPath  string `json:"vars_path"`
	IsSplit   bool   `json:"is_split"` // True if separate code and vars templates exist
}

// Well-known paths for OVMF split code and vars templates across Linux distros.
var ovmfSplitPairs = []struct {
	code string
	vars string
}{
	// Debian / Ubuntu / Mint (4M standard)
	{code: "/usr/share/OVMF/OVMF_CODE_4M.fd", vars: "/usr/share/OVMF/OVMF_VARS_4M.fd"},
	// Debian / Ubuntu / Mint (standard 2M)
	{code: "/usr/share/OVMF/OVMF_CODE.fd", vars: "/usr/share/OVMF/OVMF_VARS.fd"},
	// Fedora / RHEL / CentOS
	{code: "/usr/share/edk2/ovmf/OVMF_CODE.fd", vars: "/usr/share/edk2/ovmf/OVMF_VARS.fd"},
	// Arch Linux
	{code: "/usr/share/edk2-ovmf/x64/OVMF_CODE.fd", vars: "/usr/share/edk2-ovmf/x64/OVMF_VARS.fd"},
	// openSUSE
	{code: "/usr/share/qemu/ovmf-x86_64-code.bin", vars: "/usr/share/qemu/ovmf-x86_64-vars.bin"},
}

// Well-known paths for single/combined OVMF binary.
var ovmfSingleFiles = []string{
	"/usr/share/ovmf/OVMF.fd",
	"/usr/share/qemu/OVMF.fd",
	"/usr/share/edk2/ovmf/OVMF.fd",
}

var (
	uefiOnce   sync.Once
	cachedUEFI UEFIFirmware
)

// DetectUEFIFirmware checks the host for installed OVMF UEFI firmware packages (cached after initial discovery).
func DetectUEFIFirmware() UEFIFirmware {
	uefiOnce.Do(func() {
		cachedUEFI = detectUEFIFirmwareUncached()
	})
	return cachedUEFI
}

func detectUEFIFirmwareUncached() UEFIFirmware {
	for _, pair := range ovmfSplitPairs {
		if fileExists(pair.code) && fileExists(pair.vars) {
			return UEFIFirmware{
				Available: true,
				CodePath:  pair.code,
				VarsPath:  pair.vars,
				IsSplit:   true,
			}
		}
	}

	for _, file := range ovmfSingleFiles {
		if fileExists(file) {
			return UEFIFirmware{
				Available: true,
				CodePath:  file,
				IsSplit:   false,
			}
		}
	}

	return UEFIFirmware{Available: false}
}

// InitEFIVars provisions a persistent per-VM NVRAM vars file from the host template.
func InitEFIVars(vmDir string, fw UEFIFirmware) error {
	if !fw.Available {
		return ErrUEFINotSupported
	}
	if !fw.IsSplit || fw.VarsPath == "" {
		return nil
	}

	destPath := filepath.Join(vmDir, "efivars.fd")
	if fileExists(destPath) {
		return nil // already initialized
	}

	srcFile, err := os.Open(fw.VarsPath)
	if err != nil {
		return fmt.Errorf("failed to open EFI vars template: %w", err)
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create per-VM EFI vars file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy EFI vars template: %w", err)
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
