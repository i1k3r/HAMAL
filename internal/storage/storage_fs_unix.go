//go:build !windows

package storage

import (
	"fmt"
	"syscall"
)

// checkFreeSpaceOS queries the filesystem containing path for free bytes available to unprivileged caller on Unix/Linux.
func checkFreeSpaceOS(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs(%s): %w", path, err)
	}

	// Bavail is the number of free blocks available to unprivileged users.
	// Bsize is the filesystem block size.
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
