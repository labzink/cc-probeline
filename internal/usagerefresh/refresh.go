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
// instead of on every tick.
type gate struct {
	CheckedAt int64 `json:"checked_at"`
}

// launchFn is the process launcher, swapped out in tests so no real Claude Code
// is ever started by the suite.
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
	g, _ := readGate(p) // zero value when absent or corrupt
	if g.CheckedAt > 0 && now.Unix()-g.CheckedAt < int64(refreshTTL.Seconds()) {
		slog.Debug("usagerefresh: within TTL, skipping", "age_s", now.Unix()-g.CheckedAt)
		return
	}

	bin := claudeBinary()
	if bin == "" {
		slog.Debug("usagerefresh: claude binary not found; skipping")
		// Stamp anyway: without this a machine without the binary would stat the
		// PATH on every single tick.
		writeGate(p, gate{CheckedAt: now.Unix()})
		return
	}

	// Stamp BEFORE launching. Ten Claude Code windows tick at the same moment;
	// stamping afterwards would let all ten start a process each.
	writeGate(p, gate{CheckedAt: now.Unix()})

	if err := launchFn(bin); err != nil {
		slog.Warn("usagerefresh: launch failed", "err", err)
		return
	}
	slog.Debug("usagerefresh: refresh launched")
}

// launch starts the headless usage screen, detached, and never waits for it.
func launch(bin string) error {
	cmd := exec.Command(bin, "-p", "/usage", "--no-session-persistence") //nolint:gosec // fixed argv
	cmd.Env = append(os.Environ(), NoRefreshEnv+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	// Run from a neutral directory: the child must not inherit the user's
	// project as its working directory and colour anything it records with it.
	cmd.Dir = os.TempDir()
	detach(cmd)

	if err := cmd.Start(); err != nil {
		return err
	}
	// Release, do not Wait: the render must not block on a ~4.5 s call.
	return cmd.Process.Release()
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

// writeGate persists the throttle atomically under a lock, the same durability
// pattern the price cache uses. Fail-soft: the gate is disposable, and losing it
// costs at most one extra refresh.
func writeGate(p string, g gate) {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("usagerefresh: mkdir", "dir", dir, "err", err)
		return
	}
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		slog.Warn("usagerefresh: flock", "err", err)
		return
	}
	defer fl.Unlock() //nolint:errcheck

	data, err := json.Marshal(g)
	if err != nil {
		slog.Warn("usagerefresh: encode", "err", err)
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Warn("usagerefresh: write tmp", "err", err)
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("usagerefresh: rename", "err", err)
	}
}

// SetLauncherForTest swaps the process launcher and returns a restore function.
// Must only be called from tests.
func SetLauncherForTest(f func(bin string) error) func() {
	prev := launchFn
	launchFn = f
	return func() { launchFn = prev }
}
