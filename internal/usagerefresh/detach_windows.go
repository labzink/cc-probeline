//go:build windows

package usagerefresh

import (
	"os"
	"os/exec"
	"syscall"
)

// detachedProcess is CREATE_NO_WINDOW | DETACHED_PROCESS: the child gets no
// console of its own and does not attach to ours, so nothing flashes on screen
// every five minutes.
const detachedProcess = 0x08000000 | 0x00000008

// detach makes the child independent of this short-lived process, as it is on
// unix — see the sibling file for why. Note the Windows install channel remains
// best-effort: it has had no hands-on verification.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess}
}

// processAlive reports whether a pid we recorded is still running. Windows has
// no signal-0 probe; OpenProcess (what FindProcess does here) failing is the
// available approximation. Worst case we misread a finished refresh as live and
// wait one more TTL, which is the harmless direction.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
