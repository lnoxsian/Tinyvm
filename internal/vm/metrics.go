package vm

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tinyvm/internal/host"
)

// VMMetrics captures point-in-time runtime metrics for an individual virtual machine.
type VMMetrics struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Status           VMState   `json:"status"`
	PID              int       `json:"pid"`
	Uptime           string    `json:"uptime,omitempty"`
	UptimeSeconds    int64     `json:"uptime_seconds"`
	CPUPercent       float64   `json:"cpu_percent"`
	MemoryConfigMB   int       `json:"memory_config_mb"`
	MemoryRSSBytes   uint64    `json:"memory_rss_bytes"`
	MemoryRSSMB      float64   `json:"memory_rss_mb"`
	DiskPath         string    `json:"disk_path,omitempty"`
	DiskFormat       string    `json:"disk_format,omitempty"`
	DiskVirtualBytes int64     `json:"disk_virtual_bytes"`
	DiskActualBytes  int64     `json:"disk_actual_bytes"`
	CollectedAt      time.Time `json:"collected_at"`
}

// HostMetrics represents system-level telemetry and aggregate VM statistics.
type HostMetrics struct {
	CPUPercent     int         `json:"cpu_percent"`
	CPUCores       int         `json:"cpu_cores"`
	CPUModel       string      `json:"cpu_model"`
	MemTotalBytes  uint64      `json:"mem_total_bytes"`
	MemUsedBytes   uint64      `json:"mem_used_bytes"`
	MemFreeBytes   uint64      `json:"mem_free_bytes"`
	MemPercent     int         `json:"mem_percent"`
	DiskTotalBytes uint64      `json:"disk_total_bytes"`
	DiskUsedBytes  uint64      `json:"disk_used_bytes"`
	DiskFreeBytes  uint64      `json:"disk_free_bytes"`
	DiskPercent    int         `json:"disk_percent"`
	KVMEnabled     bool        `json:"kvm_enabled"`
	TotalVMs       int         `json:"total_vms"`
	RunningVMs     int         `json:"running_vms"`
	StoppedVMs     int         `json:"stopped_vms"`
	CollectedAt    time.Time   `json:"collected_at"`
	VMs            []VMMetrics `json:"vms,omitempty"`
}

// FormatDuration formats a time.Duration into a concise human-readable string.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// ParseDiskSizeBytes parses human-readable disk sizes like "20G", "500M" into bytes.
func ParseDiskSizeBytes(size string) int64 {
	size = strings.TrimSpace(size)
	if len(size) < 2 {
		return 0
	}
	unit := size[len(size)-1]
	valStr := size[:len(size)-1]
	val, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil || val <= 0 {
		return 0
	}
	switch unit {
	case 'K', 'k':
		return val * 1024
	case 'M', 'm':
		return val * 1024 * 1024
	case 'G', 'g':
		return val * 1024 * 1024 * 1024
	case 'T', 't':
		return val * 1024 * 1024 * 1024 * 1024
	default:
		return val
	}
}

// GetVMMetrics collects runtime and resource metrics for a specific VM.
func (m *Manager) GetVMMetrics(id string) (*VMMetrics, error) {
	m.mu.RLock()
	v, exists := m.vms[id]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrVMNotFoundInMgr
	}
	cfg := v.Config
	runtime := v.Runtime
	m.mu.RUnlock()

	return m.collectVMMetrics(cfg, runtime), nil
}

func (m *Manager) collectVMMetrics(cfg VMConfig, runtime VMRuntime) *VMMetrics {
	metrics := &VMMetrics{
		ID:             cfg.ID,
		Name:           cfg.Name,
		Status:         runtime.State,
		PID:            runtime.PID,
		MemoryConfigMB: cfg.MemoryMB,
		DiskFormat:     cfg.DiskFormat,
		CollectedAt:    time.Now(),
	}

	if runtime.State == StateRunning && !runtime.StartedAt.IsZero() {
		dur := time.Since(runtime.StartedAt)
		metrics.Uptime = FormatDuration(dur)
		metrics.UptimeSeconds = int64(dur.Seconds())
	}

	if runtime.State == StateRunning && runtime.PID > 0 {
		metrics.CPUPercent = host.GetProcessCPUPercent(runtime.PID)
		metrics.MemoryRSSBytes = host.GetProcessRSSBytes(runtime.PID)
		metrics.MemoryRSSMB = math.Round((float64(metrics.MemoryRSSBytes)/(1024*1024))*10) / 10
	}

	if m.storage != nil {
		vmDir, err := m.storage.VMDir(cfg.ID)
		if err == nil && cfg.Disk != "" {
			diskPath := filepath.Join(vmDir, cfg.Disk)
			metrics.DiskPath = diskPath
			if fi, err := os.Stat(diskPath); err == nil {
				var actualBytes int64 = fi.Size()
				if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
					actualBytes = stat.Blocks * 512
				}
				metrics.DiskActualBytes = actualBytes

				if cfg.DiskSize != "" {
					metrics.DiskVirtualBytes = ParseDiskSizeBytes(cfg.DiskSize)
				}
				if metrics.DiskVirtualBytes <= 0 {
					metrics.DiskVirtualBytes = fi.Size()
				}
			}
		}
	}

	return metrics
}

// GetAllVMMetrics returns telemetry metrics for all registered VMs.
func (m *Manager) GetAllVMMetrics() []VMMetrics {
	m.mu.RLock()
	type vmSnapshot struct {
		cfg     VMConfig
		runtime VMRuntime
	}
	snaps := make([]vmSnapshot, 0, len(m.vms))
	for _, v := range m.vms {
		snaps = append(snaps, vmSnapshot{
			cfg:     v.Config,
			runtime: v.Runtime,
		})
	}
	m.mu.RUnlock()

	result := make([]VMMetrics, 0, len(snaps))
	for _, snap := range snaps {
		if met := m.collectVMMetrics(snap.cfg, snap.runtime); met != nil {
			result = append(result, *met)
		}
	}
	return result
}

// GetHostMetrics retrieves complete host and aggregate virtual machine telemetry.
func (m *Manager) GetHostMetrics() *HostMetrics {
	cpuInfo := host.GetCPUInfo()
	memInfo := host.GetMemoryInfo()

	var diskTotal, diskUsed, diskFree uint64
	var diskPercent int
	if m.storage != nil {
		if diskInfo, err := host.GetDiskInfo(m.storage.DataDir()); err == nil && diskInfo.TotalBytes > 0 {
			diskTotal = diskInfo.TotalBytes
			diskUsed = diskInfo.UsedBytes
			diskFree = diskInfo.FreeBytes
			diskPercent = int((float64(diskUsed) / float64(diskTotal)) * 100)
		}
	}

	var memPercent int
	if memInfo.TotalBytes > 0 {
		memPercent = int((float64(memInfo.UsedBytes) / float64(memInfo.TotalBytes)) * 100)
	}

	vmMetrics := m.GetAllVMMetrics()
	runningCount := 0
	stoppedCount := 0
	for _, vm := range vmMetrics {
		if vm.Status == StateRunning {
			runningCount++
		} else {
			stoppedCount++
		}
	}

	kvmStatus := host.GetKVMStatus()

	return &HostMetrics{
		CPUPercent:     cpuInfo.UsagePercent,
		CPUCores:       cpuInfo.Count,
		CPUModel:       cpuInfo.Model,
		MemTotalBytes:  memInfo.TotalBytes,
		MemUsedBytes:   memInfo.UsedBytes,
		MemFreeBytes:   memInfo.AvailableBytes,
		MemPercent:     memPercent,
		DiskTotalBytes: diskTotal,
		DiskUsedBytes:  diskUsed,
		DiskFreeBytes:  diskFree,
		DiskPercent:    diskPercent,
		KVMEnabled:     kvmStatus.Available,
		TotalVMs:       len(vmMetrics),
		RunningVMs:     runningCount,
		StoppedVMs:     stoppedCount,
		CollectedAt:    time.Now(),
		VMs:            vmMetrics,
	}
}
