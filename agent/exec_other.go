//go:build !windows

package main

import "os/exec"

func newPSCmd(args ...string) *exec.Cmd {
	return exec.Command("powershell", args...)
}
