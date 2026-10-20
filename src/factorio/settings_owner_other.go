//go:build !linux && !darwin && !freebsd

package factorio

import "os"

func settingsOwner(info os.FileInfo) string { return "owner=unavailable" }
