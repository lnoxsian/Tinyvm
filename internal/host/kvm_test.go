package host

import "testing"

func TestGetKVMStatus(t *testing.T) {
	status := GetKVMStatus()
	// Should not panic, and fields should be populated
	t.Logf("KVM Status: Available=%v, DeviceExists=%v, HasPermission=%v, CPUSupport=%v, Err=%s",
		status.Available, status.DeviceExists, status.HasPermission, status.CPUSupport, status.Error)

	// Since we are running on a Linux system where /dev/kvm is accessible
	if status.DeviceExists && !status.HasPermission {
		t.Errorf("expected permission to access /dev/kvm")
	}
}

func TestCheckCPUSupport(t *testing.T) {
	supported := CheckCPUSupport()
	t.Logf("CPU virtualization support: %v", supported)
}
