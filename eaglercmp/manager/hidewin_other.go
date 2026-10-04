//go:build !windows

package manager

import "os/exec"

func hideWindow(*exec.Cmd) {}
