package host

import (
	"os"
	"runtime"
	"strings"
)

// CPUInfo represents basic telemetry and specifications of the host CPU.
type CPUInfo struct {
	Count int    `json:"count"`
	Model string `json:"model"`
}

// GetCPUInfo returns the logical CPU core count and processor model name.
func GetCPUInfo() CPUInfo {
	model := "Generic x86_64 Processor"
	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					model = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	return CPUInfo{
		Count: runtime.NumCPU(),
		Model: model,
	}
}
