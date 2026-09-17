package host

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrKVMDeviceNotFound = errors.New("/dev/kvm device does not exist")
	ErrKVMPermission     = errors.New("insufficient permissions to access /dev/kvm (ensure user is in 'kvm' group)")
)

// KVMStatus holds detailed diagnosis about KVM capability on the host.
type KVMStatus struct {
	Available      bool   `json:"available"`
	DeviceExists   bool   `json:"device_exists"`
	HasPermission  bool   `json:"has_permission"`
	CPUSupport     bool   `json:"cpu_support"`
	Error          string `json:"error,omitempty"`
}

// CheckCPUSupport checks whether /proc/cpuinfo contains virtualization flags (vmx or svm).
func CheckCPUSupport() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "vmx") || strings.Contains(content, "svm")
}

// CheckKVM verifies that /dev/kvm exists and is readable and writable.
func CheckKVM() (bool, error) {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		if os.IsNotExist(err) {
			return false, ErrKVMDeviceNotFound
		}
		return false, fmt.Errorf("error accessing /dev/kvm: %w", err)
	}

	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		if os.IsPermission(err) {
			return false, ErrKVMPermission
		}
		return false, fmt.Errorf("cannot open /dev/kvm: %w", err)
	}
	_ = f.Close()

	return true, nil
}

// GetKVMStatus gathers full diagnostics about KVM availability on the host.
func GetKVMStatus() KVMStatus {
	cpuSupported := CheckCPUSupport()
	_, devErr := os.Stat("/dev/kvm")
	devExists := devErr == nil

	ok, err := CheckKVM()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	return KVMStatus{
		Available:     ok,
		DeviceExists:  devExists,
		HasPermission: ok,
		CPUSupport:    cpuSupported,
		Error:         errMsg,
	}
}
