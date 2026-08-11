// Package usagerefresh keeps Claude Code's usage cache from going stale.
//
// The model-scoped weekly limit and the official overage figures live only in
// ~/.claude.json under cachedUsageUtilization — the status-line payload carries
// neither. That branch is written by exactly one thing: Claude Code's /usage
// screen. On a machine where nobody opens /usage it is absent entirely, and
// where somebody once did it freezes at that moment (measured: a snapshot three
// hours old while the file itself was rewritten five times for other reasons).
//
// So we run the screen ourselves, headless:
//
//	claude -p "/usage" --no-session-persistence
//
// Measured properties (Claude Code 2.1.227): ~4.5 s wall clock, no model tokens
// spent — the transcript holds no assistant turn, only the local slash command
// and its network call for the limits — and with the flag no session file is
// left behind. Claude Code itself refuses to rewrite the cache more than once
// every five minutes, which is why the TTL here matches that number: asking
// more often burns processes for a file that will not change.
//
// The call is fire-and-forget: rendering a status line has a ~5 ms budget and
// this takes a thousand times that, so the process is detached and never waited
// on. Whatever it writes is picked up by a later render.
package usagerefresh

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	// refreshTTL matches Claude Code's own write gate on the usage cache
	// (300000 ms in the 2.1.227 bundle). Below it a run cannot change anything.
	refreshTTL = 5 * time.Minute

	// gateName is the shared file that throttles refreshes across every Claude
	// Code window on the machine — the status line runs several times per second
	// per window, so the gate has to be shared state, not per-process memory.
	gateName = "usage-refresh.json"

	// NoRefreshEnv is set on the child process. The child is a full Claude Code
	// session and may render this very status line, which would otherwise launch
	// another child, and so on. Also honoured as a user-facing kill switch.
	NoRefreshEnv = "CC_PROBELINE_NO_REFRESH"

	// binEnv overrides the Claude Code binary path (tests, unusual installs).
	binEnv = "CC_PROBELINE_CLAUDE_BIN"
)

// gate is the on-disk throttle: when the last refresh was attempted, success or
// failure alike, so a machine with no `claude` on PATH retries once per TTL
// instead of on every tick. PID is the process we started last, so a refresh
// that hangs (auth stall, wrapper script, dead network) blocks the next one
// instead of accumulating one orphan every five minutes.
type gate struct {
	CheckedAt int64 `json:"checked_at"`
	PID       int   `json:"pid,omitempty"`
}

// launchFn is the process launcher, swapped out in tests so no real Claude Code
// is ever started by the suite. It returns the child's pid.
var launchFn = launch

// Maybe refreshes the usage cache if it is worth refreshing, and returns
// immediately either way.
//
// firstTick is the first render of a session: we always refresh then (subject to
// the shared TTL), because that is how we learn whether this account has a
// model-scoped window or paid overage at all.
//
// needed says the last reading found something worth keeping fresh. When it is
// false and the session is already underway, we stop launching processes
// entirely — an account with neither feature never pays for this.
func Maybe(now time.Time, firstTick, needed bool) {
	if os.Getenv(NoRefreshEnv) != "" {
		slog.Debug("usagerefresh: disabled by env")
		return
	}
	if !firstTick && !needed {
		return
	}

	p := gatePath()
	if p == "" {
		slog.Warn("usagerefresh: data dir unavailable; skipping")
		return
	}

	// claimSlot is the whole throttle: it decides and records under one lock, and
	// says no whenever it cannot record. Launching without a recorded stamp would
	// mean launching again on the very next tick — several times a second.
	if !claimSlot(p, now) {
		return
	}

	bin := claudeBinary()
	if bin == "" {
		// The slot is claimed, so this costs one PATH lookup per TTL rather than
		// one per render on a machine that has no Claude Code CLI.
		slog.Debug("usagerefresh: claude binary not found; skipping")
		return
	}

	pid, err := launchFn(bin)
	if err != nil {
		slog.Warn("usagerefresh: launch failed", "err", err)
		return
	}
	recordPID(p, now, pid)
	slog.Debug("usagerefresh: refresh launched", "pid", pid)
}

// claimSlot reports whether this process may run a refresh now, and reserves the
// slot when it may. Read, decision and write all happen under one lock: reading
// unlocked would let every open Claude Code window see the same stale stamp in
// the same instant and start a process each.
//
// It returns false — refusing the refresh — whenever the stamp cannot be
// written. That is deliberate: an unwritable data directory (root-owned
// ~/.local/share after a sudo install, a read-only container home, a full disk)
// must not degrade into "no throttle at all".
func claimSlot(p string, now time.Time) bool {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("usagerefresh: data dir unusable; not refreshing", "dir", dir, "err", err)
		return false
	}
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		slog.Warn("usagerefresh: flock; not refreshing", "err", err)
		return false
	}
	defer fl.Unlock() //nolint:errcheck

	g, _ := readGate(p) // zero value when absent or corrupt

	age := now.Unix() - g.CheckedAt
	switch {
	case g.CheckedAt == 0:
		// Never refreshed on this machine — go ahead.
	case age < 0:
		// Stamp from the future: the clock jumped back (NTP correction, dual boot,
		// a home directory shared between machines). Treating it as "fresh" would
		// disable refreshing until wall-clock time caught up, so overwrite it.
		slog.Warn("usagerefresh: stamp is in the future; resetting", "skew_s", -age)
	case age < int64(refreshTTL.Seconds()):
		slog.Debug("usagerefresh: within TTL, skipping", "age_s", age)
		return false
	}

	// A refresh we started earlier may still be running: the call takes ~4.5 s,
	// but a stalled one can sit forever. Waiting is better than stacking.
	if g.PID > 0 && processAlive(g.PID) {
		slog.Debug("usagerefresh: previous refresh still running; skipping", "pid", g.PID)
		return false
	}

	if err := writeGate(p, gate{CheckedAt: now.Unix()}); err != nil {
		slog.Warn("usagerefresh: cannot record throttle stamp; not refreshing", "err", err)
		return false
	}
	return true
}

// recordPID stores the child's pid in the already-claimed slot, best effort: if
// it fails the only cost is that a hung child stops blocking the next attempt.
func recordPID(p string, now time.Time, pid int) {
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		return
	}
	defer fl.Unlock() //nolint:errcheck
	if err := writeGate(p, gate{CheckedAt: now.Unix(), PID: pid}); err != nil {
		slog.Debug("usagerefresh: could not record pid", "err", err)
	}
}

// launch starts the headless usage screen, detached, and never waits for it.
// Returns the child's pid so the gate can tell a still-running refresh from a
// finished one.
func launch(bin string) (int, error) {
	cmd := exec.Command(bin, "-p", "/usage", "--no-session-persistence") //nolint:gosec // fixed argv
	cmd.Env = append(os.Environ(), NoRefreshEnv+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	// Run from a neutral directory: the child must not inherit the user's
	// project as its working directory and colour anything it records with it.
	cmd.Dir = os.TempDir()
	detach(cmd)

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// Release, do not Wait: the render must not block on a ~4.5 s call. No zombie
	// results — this process exits within milliseconds and init reaps the child.
	return pid, cmd.Process.Release()
}

// claudeBinary locates the Claude Code CLI: an explicit override first, then
// PATH, then the native installer's default location. Returns "" when not found
// — in which case the feature quietly degrades to reading whatever cache exists.
func claudeBinary() string {
	if p := os.Getenv(binEnv); p != "" {
		return p
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	cand := filepath.Join(home, ".local", "bin", "claude")
	if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
		return cand
	}
	return ""
}

// gateDir resolves the shared data directory, mirroring the price cache so both
// machine-wide files live together.
func gateDir() string {
	if dir := os.Getenv("CC_PROBELINE_USAGE_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "cc-probeline")
}

func gatePath() string {
	dir := gateDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, gateName)
}

func readGate(p string) (gate, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return gate{}, err
	}
	var g gate
	if err := json.Unmarshal(data, &g); err != nil {
		return gate{}, err
	}
	return g, nil
}

// writeGate persists the throttle atomically, the same durability pattern the
// price cache uses. The caller holds the lock and must act on the error: a
// silent failure here would remove the throttle entirely.
func writeGate(p string, g gate) error {
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// SetLauncherForTest swaps the process launcher and returns a restore function.
// Must only be called from tests.
func SetLauncherForTest(f func(bin string) (int, error)) func() {
	prev := launchFn
	launchFn = f
	return func() { launchFn = prev }
}
