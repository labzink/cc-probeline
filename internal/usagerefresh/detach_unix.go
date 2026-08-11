//go:build !windows

package usagerefresh

import (
	"os"
	"os/exec"
	"syscall"
)

// detach puts the child in its own session so it survives this process exiting
// — the status line lives for milliseconds, the refresh takes seconds — and so
// it can never receive signals aimed at the terminal's foreground group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// processAlive reports whether a pid we recorded is still running. Signal 0
// performs the permission and existence checks without delivering anything —
// the standard liveness probe on unix.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid) // never fails on unix
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
