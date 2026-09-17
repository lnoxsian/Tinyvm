package host

import (
	"syscall"
)

// DiskInfo captures total, free, and used disk capacity in bytes for a given path.
type DiskInfo struct {
	Path       string `json:"path"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

// GetDiskInfo queries filesystem capacity metrics for the specified path using Statfs.
func GetDiskInfo(path string) (DiskInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskInfo{}, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := uint64(0)
	if total >= free {
		used = total - free
	}

	return DiskInfo{
		Path:       path,
		TotalBytes: total,
		FreeBytes:  free,
		UsedBytes:  used,
	}, nil
}
