package main

import (
	"syscall"
)

// getFileOwnership returns the UID and GID of a file (Unix implementation)
func getFileOwnership(stat interface{}) (int, int) {
	if sysStat, ok := stat.(*syscall.Stat_t); ok {
		return int(sysStat.Uid), int(sysStat.Gid)
	}
	return -1, -1
}
