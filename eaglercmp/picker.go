package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var stdin = bufio.NewReader(os.Stdin)

// interactive reports whether a person can answer prompts.
func interactive() bool {
	if gui {
		return true
	}
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

const fileDialogScript = `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = 'EaglerCMP - choose your Eaglercraft client'
$d.Filter = 'Eaglercraft client (*.html;*.htm;*.zip)|*.html;*.htm;*.zip|All files (*.*)|*.*'
$owner = New-Object System.Windows.Forms.Form -Property @{TopMost = $true}
if ($d.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileName }`

// pickClient asks the user for a client source: a file dialog on Windows,
// otherwise (or if the dialog is cancelled) a console prompt that accepts a
// dragged-in file, a path or a URL.
func pickClient(ctx context.Context) (string, error) {
	fmt.Println("\nNo Eaglercraft client is installed yet.")
	if runtime.GOOS == "windows" {
		fmt.Println("Choose your Eaglercraft client (.html or .zip) in the file dialog...")
		cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", fileDialogScript)
		hideWindow(cmd)
		out, err := cmd.Output()
		if p := strings.TrimSpace(string(out)); err == nil && p != "" {
			return p, nil
		}
		if gui {
			return "", errors.New("no Eaglercraft client chosen")
		}
	}
	fmt.Print("Drag the Eaglercraft .html/.zip file (or folder) into this window, or paste a path or URL, then press Enter:\n> ")
	line, err := stdin.ReadString('\n')
	line = strings.Trim(strings.TrimSpace(line), `"'`)
	if line == "" {
		if err != nil {
			return "", err
		}
		return "", errors.New("no client chosen")
	}
	return line, nil
}
