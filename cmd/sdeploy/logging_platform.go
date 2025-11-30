package main

import (
	"fmt"
	"os"
	"os/user"
	"syscall"
)

// getFileOwnerInfo returns owner information for a file (Unix implementation)
func getFileOwnerInfo(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	uid := stat.Uid
	gid := stat.Gid

	// Try to resolve UID to username
	userName := fmt.Sprintf("%d", uid)
	if u, err := user.LookupId(fmt.Sprintf("%d", uid)); err == nil {
		userName = u.Username
	}

	// Try to resolve GID to group name
	groupName := fmt.Sprintf("%d", gid)
	if g, err := user.LookupGroupId(fmt.Sprintf("%d", gid)); err == nil {
		groupName = g.Name
	}

	return fmt.Sprintf("%s:%s (uid=%d, gid=%d)", userName, groupName, uid, gid)
}
