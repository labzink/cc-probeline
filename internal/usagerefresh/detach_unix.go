//go:build !windows

package usagerefresh

import (
	"os/exec"
	"syscall"
)

// detach puts the child in its own session so it survives this process exiting
// — the status line lives for milliseconds, the refresh takes seconds — and so
// it can never receive signals aimed at the terminal's foreground group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
