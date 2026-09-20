package host

import (
	"bytes"
	"os"
	"strconv"
)

// MemoryInfo captures total, available, and used host RAM in bytes.
type MemoryInfo struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
}

var (
	memTotalPrefix = []byte("MemTotal:")
	memAvailPrefix = []byte("MemAvailable:")
	pageSize       = uint64(os.Getpagesize())
)

// GetMemoryInfo reads host memory metrics from /proc/meminfo.
func GetMemoryInfo() MemoryInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}
	}

	var totalKB, availKB uint64
	var foundTotal, foundAvail bool

	rest := data
	for len(rest) > 0 {
		var line []byte
		idx := bytes.IndexByte(rest, '\n')
		if idx >= 0 {
			line = rest[:idx]
			rest = rest[idx+1:]
		} else {
			line = rest
			rest = nil
		}

		if bytes.HasPrefix(line, memTotalPrefix) {
			fields := bytes.Fields(line)
			if len(fields) >= 2 {
				totalKB, _ = strconv.ParseUint(string(fields[1]), 10, 64)
				foundTotal = true
			}
		} else if bytes.HasPrefix(line, memAvailPrefix) {
			fields := bytes.Fields(line)
			if len(fields) >= 2 {
				availKB, _ = strconv.ParseUint(string(fields[1]), 10, 64)
				foundAvail = true
			}
		}

		if foundTotal && foundAvail {
			break
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

// GetProcessRSSBytes returns the resident set size of a process in bytes from /proc/[pid]/statm.
func GetProcessRSSBytes(pid int) uint64 {
	if pid <= 0 {
		return 0
	}
	path := "/proc/" + strconv.Itoa(pid) + "/statm"
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	fields := bytes.Fields(data)
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		return 0
	}
	return pages * pageSize
}
