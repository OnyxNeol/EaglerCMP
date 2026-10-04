//go:build !windows

package manager

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// profileInUse reports whether a browser still holds the profile: Chromium's
// <profile>/SingletonLock symlink points at "<host>-<pid>" while it runs.
func profileInUse(profile string) bool {
	target, err := os.Readlink(filepath.Join(profile, "SingletonLock"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(target[strings.LastIndexByte(target, '-')+1:])
	if err != nil || pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
