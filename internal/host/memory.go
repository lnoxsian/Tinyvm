package host

import (
	"os"
	"strconv"
	"strings"
)

// MemoryInfo captures total, available, and used host RAM in bytes.
type MemoryInfo struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
}

// GetMemoryInfo reads host memory metrics from /proc/meminfo.
func GetMemoryInfo() MemoryInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}
	}

	var totalKB, availKB uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemAvailable:":
			availKB, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}

	total := totalKB * 1024
	avail := availKB * 1024
	used := uint64(0)
	if total >= avail {
		used = total - avail
	}

	return MemoryInfo{
		TotalBytes:     total,
		AvailableBytes: avail,
		UsedBytes:      used,
	}
}
