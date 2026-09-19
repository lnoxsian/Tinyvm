package host

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
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

type procSample struct {
	ticks     uint64
	timestamp time.Time
}

var (
	procCPUMu      sync.Mutex
	procCPUSamples = make(map[int]procSample)
)

// GetProcessCPUPercent returns the estimated CPU usage percentage of a process (0.0 to 100.0 * N cores).
// If consecutive calls are made, the delta between calls is used.
// If it is the first call, it estimates against process uptime or returns 0.0.
func GetProcessCPUPercent(pid int) float64 {
	if pid <= 0 {
		return 0.0
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		procCPUMu.Lock()
		delete(procCPUSamples, pid)
		procCPUMu.Unlock()
		return 0.0
	}

	content := string(data)
	lastParen := strings.LastIndex(content, ")")
	if lastParen == -1 || lastParen+2 >= len(content) {
		return 0.0
	}

	rest := strings.TrimSpace(content[lastParen+2:])
	fields := strings.Fields(rest)
	if len(fields) <= 12 {
		return 0.0
	}

	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0.0
	}

	totalTicks := utime + stime
	now := time.Now()

	procCPUMu.Lock()
	defer procCPUMu.Unlock()

	// Periodic cleanup of stale entries if map gets large
	if len(procCPUSamples) > 100 {
		for p, sample := range procCPUSamples {
			if now.Sub(sample.timestamp) > 5*time.Minute {
				delete(procCPUSamples, p)
			}
		}
	}

	prev, exists := procCPUSamples[pid]
	procCPUSamples[pid] = procSample{
		ticks:     totalTicks,
		timestamp: now,
	}

	if exists && totalTicks >= prev.ticks {
		elapsedSecs := now.Sub(prev.timestamp).Seconds()
		if elapsedSecs >= 0.05 {
			deltaTicks := totalTicks - prev.ticks
			pct := (float64(deltaTicks) / elapsedSecs)
			if pct < 0 {
				pct = 0
			}
			return math.Round(pct*10) / 10
		}
	}

	// First sample: try to estimate using starttime and /proc/uptime
	if len(fields) > 19 {
		if starttimeTicks, err := strconv.ParseUint(fields[19], 10, 64); err == nil {
			if uptimeData, err := os.ReadFile("/proc/uptime"); err == nil {
				uptimeFields := strings.Fields(string(uptimeData))
				if len(uptimeFields) > 0 {
					if hostUptimeSecs, err := strconv.ParseFloat(uptimeFields[0], 64); err == nil {
						hostUptimeTicks := hostUptimeSecs * 100.0
						if hostUptimeTicks > float64(starttimeTicks) {
							procElapsedTicks := hostUptimeTicks - float64(starttimeTicks)
							if procElapsedTicks > 0 {
								pct := (float64(totalTicks) / procElapsedTicks) * 100.0
								if pct < 0 {
									pct = 0
								}
								return math.Round(pct*10) / 10
							}
						}
					}
				}
			}
		}
	}

	return 0.0
}

