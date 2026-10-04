//go:build windows

package manager

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// profileInUse reports whether a browser still holds the profile: Chromium
// keeps <profile>/lockfile open without write sharing while it runs.
func profileInUse(profile string) bool {
	p := filepath.Join(profile, "lockfile")
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err == nil {
		f.Close()
		return false
	}
	return !errors.Is(err, fs.ErrNotExist)
}
