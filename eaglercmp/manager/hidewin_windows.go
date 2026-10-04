//go:build windows

package manager

import (
	"os/exec"
	"syscall"
)

// hideWindow stops java.exe from opening a console next to the game window.
func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
