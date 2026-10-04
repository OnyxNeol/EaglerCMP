//go:build !windows

package main

import "os/exec"

func setupConsole() (gui bool)              { return false }
func showDialog(title, text string, _ bool) {}
func hideWindow(*exec.Cmd)                  {}
