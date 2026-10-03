//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow evita che ffmpeg apra una finestra di console.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
