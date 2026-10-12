//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func shellCommand(command string) *exec.Cmd {
	c := exec.Command("cmd")
	// Set CmdLine directly so cmd /C receives the command without extra quote escaping.
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd /S /C \"" + command + "\""}
	return c
}

func killTree(c *exec.Cmd) {
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid)).Run()
}
