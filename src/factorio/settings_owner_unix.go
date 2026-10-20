//go:build linux || darwin || freebsd

package factorio

import (
	"fmt"
	"os"
	"syscall"
)

func settingsOwner(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("owner_uid=%d owner_gid=%d", stat.Uid, stat.Gid)
	}
	return "owner=unknown"
}
