//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
)

// setupConsole wires up standard I/O for the GUI-subsystem exe. Redirected
// handles (pipes, CI) are kept; when started from a terminal it attaches to
// that console; when double-clicked there is no console and it reports GUI
// mode, in which output only goes to the log files and errors to a dialog.
func setupConsole() (gui bool) {
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil && h != 0 && h != syscall.InvalidHandle {
		return false
	}
	const attachParentProcess = ^uintptr(0)
	if r, _, _ := kernel32.NewProc("AttachConsole").Call(attachParentProcess); r == 0 {
		return true
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = f
	}
	return false
}

// showDialog shows a Windows message box.
func showDialog(title, text string, isError bool) {
	const mbIconError, mbIconInfo, mbTopmost = 0x10, 0x40, 0x40000
	flags := uintptr(mbIconInfo | mbTopmost)
	if isError {
		flags = mbIconError | mbTopmost
	}
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
}

// hideWindow stops helper processes (PowerShell) from flashing a console.
func hideWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
