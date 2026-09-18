package host

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// CPUInfo represents basic telemetry and specifications of the host CPU.
type CPUInfo struct {
	Count        int    `json:"count"`
	Model        string `json:"model"`
	UsagePercent int    `json:"usage_percent"`
}

var (
	cpuMu        sync.Mutex
	lastCPUTotal uint64
	lastCPUBusy  uint64
)

// GetCPUUsagePercent reads /proc/stat to calculate CPU usage percentage,
// falling back to 1-minute load average if delta is unavailable.
func GetCPUUsagePercent() int {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return getFallbackCPUUsage()
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "cpu" {
			var total, busy uint64
			for i := 1; i < len(fields); i++ {
				val, err := strconv.ParseUint(fields[i], 10, 64)
				if err != nil {
					continue
				}
				total += val
				// user(1), nice(2), system(3), irq(6), softirq(7), steal(8)
				if i == 1 || i == 2 || i == 3 || i == 6 || i == 7 || i == 8 {
					busy += val
				}
			}

			cpuMu.Lock()
			defer cpuMu.Unlock()

			if lastCPUTotal == 0 || total <= lastCPUTotal {
				lastCPUTotal = total
				lastCPUBusy = busy
				return getFallbackCPUUsage()
			}

			deltaTotal := total - lastCPUTotal
			deltaBusy := busy - lastCPUBusy
			lastCPUTotal = total
			lastCPUBusy = busy

			if deltaTotal == 0 {
				return 0
			}

			percent := int((deltaBusy * 100) / deltaTotal)
			if percent < 0 {
				percent = 0
			}
			if percent > 100 {
				percent = 100
			}
			return percent
		}
	}

	return getFallbackCPUUsage()
}

func getFallbackCPUUsage() int {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) > 0 {
		if load, err := strconv.ParseFloat(fields[0], 64); err == nil {
			cores := float64(runtime.NumCPU())
			if cores <= 0 {
				cores = 1
			}
			pct := int((load / cores) * 100)
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			return pct
		}
	}
	return 0
}

// GetCPUInfo returns the logical CPU core count, processor model name, and usage percentage.
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
		Count:        runtime.NumCPU(),
		Model:        model,
		UsagePercent: GetCPUUsagePercent(),
	}
}
