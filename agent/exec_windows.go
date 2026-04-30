//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// newPSCmd はウィンドウを表示せずに PowerShell コマンドを実行するコマンドを返す
func newPSCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("powershell", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
