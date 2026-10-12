//go:build !windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

func shellCommand(command string) *exec.Cmd {
	c := exec.Command("sh", "-c", command)
	// Separate process group so children are terminated too (go run → binary).
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

func killTree(c *exec.Cmd) {
	pgid := -c.Process.Pid
	syscall.Kill(pgid, syscall.SIGTERM)
	time.AfterFunc(2*time.Second, func() { syscall.Kill(pgid, syscall.SIGKILL) })
}
