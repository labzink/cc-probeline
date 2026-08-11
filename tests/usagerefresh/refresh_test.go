// Package usagerefresh_test — Phase 7.48: the gate that decides whether to run
// Claude Code's usage screen headlessly.
//
// No test here starts a real process: the launcher is swapped for a counter, so
// what is under test is purely the decision — which is the part that can go
// wrong expensively (a status line ticks several times per second, in every open
// window). All tests are hermetic: the shared gate file lives in a t.TempDir.
package usagerefresh_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/usagerefresh"
)

var refreshNow = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

// harness isolates the gate directory, points the package at a fake binary and
// counts launches instead of performing them.
func harness(t *testing.T) *int {
	t.Helper()
	t.Setenv("CC_PROBELINE_USAGE_DIR", t.TempDir())
	t.Setenv("CC_PROBELINE_CLAUDE_BIN", "/nonexistent/claude")
	t.Setenv(usagerefresh.NoRefreshEnv, "")

	calls := 0
	restore := usagerefresh.SetLauncherForTest(func(string) (int, error) {
		calls++
		return 0, nil // pid 0: no process to track, so liveness never blocks
	})
	t.Cleanup(restore)
	return &calls
}

// ---------------------------------------------------------------------------
// Property: the first tick of a session always refreshes — that is how we learn
// whether this account has a model-scoped window at all — but a second tick
// inside the TTL does not, no matter how many renders happen in between.
// ---------------------------------------------------------------------------

func TestMaybe_FirstTickThenThrottled(t *testing.T) {
	calls := harness(t)

	usagerefresh.Maybe(refreshNow, true, false)
	if *calls != 1 {
		t.Fatalf("first tick: launches = %d, want 1", *calls)
	}

	// A busy minute of rendering: every tick inside the TTL must be silent.
	for i := 1; i <= 20; i++ {
		usagerefresh.Maybe(refreshNow.Add(time.Duration(i)*time.Second), false, true)
	}
	if *calls != 1 {
		t.Errorf("inside TTL: launches = %d, want 1", *calls)
	}

	// Past the TTL one more refresh is allowed.
	usagerefresh.Maybe(refreshNow.Add(5*time.Minute+time.Second), false, true)
	if *calls != 2 {
		t.Errorf("after TTL: launches = %d, want 2", *calls)
	}
}

// ---------------------------------------------------------------------------
// Property: an account with neither a model-scoped window nor paid overage stops
// costing anything after the opening tick. This is the majority case, so it is
// the one that must not spawn processes forever.
// ---------------------------------------------------------------------------

func TestMaybe_NothingToWatch_StopsAfterFirstTick(t *testing.T) {
	calls := harness(t)

	usagerefresh.Maybe(refreshNow, true, false)
	usagerefresh.Maybe(refreshNow.Add(time.Hour), false, false)
	usagerefresh.Maybe(refreshNow.Add(24*time.Hour), false, false)

	if *calls != 1 {
		t.Errorf("launches = %d, want 1 (only the opening tick)", *calls)
	}
}

// ---------------------------------------------------------------------------
// Property: the recursion guard. The child we launch is a full Claude Code
// session that may render this very status line; without this, one refresh
// would breed another without end.
// ---------------------------------------------------------------------------

func TestMaybe_NeverRunsInsideItsOwnChild(t *testing.T) {
	calls := harness(t)
	t.Setenv(usagerefresh.NoRefreshEnv, "1")

	usagerefresh.Maybe(refreshNow, true, true)
	usagerefresh.Maybe(refreshNow.Add(time.Hour), true, true)

	if *calls != 0 {
		t.Errorf("launches = %d, want 0 while the guard is set", *calls)
	}
}

// ---------------------------------------------------------------------------
// Property: a failing launch still stamps the gate. Otherwise a machine where
// the launch always fails would retry on every tick — several times a second.
// ---------------------------------------------------------------------------

func TestMaybe_FailedLaunchStillThrottles(t *testing.T) {
	t.Setenv("CC_PROBELINE_USAGE_DIR", t.TempDir())
	t.Setenv("CC_PROBELINE_CLAUDE_BIN", "/nonexistent/claude")
	t.Setenv(usagerefresh.NoRefreshEnv, "")

	calls := 0
	restore := usagerefresh.SetLauncherForTest(func(string) (int, error) {
		calls++
		return 0, errors.New("boom")
	})
	t.Cleanup(restore)

	usagerefresh.Maybe(refreshNow, true, true)
	usagerefresh.Maybe(refreshNow.Add(time.Second), true, true)

	if calls != 1 {
		t.Errorf("launch attempts = %d, want 1 (failure must still throttle)", calls)
	}
}

// ---------------------------------------------------------------------------
// Property: the throttle is shared across processes, not held in memory. Two
// Claude Code windows ticking at the same moment must produce one refresh, and
// the only thing they share is that file.
// ---------------------------------------------------------------------------

func TestMaybe_GateIsSharedOnDisk(t *testing.T) {
	dir := t.TempDir()
	launches := 0
	restore := usagerefresh.SetLauncherForTest(func(string) (int, error) {
		launches++
		return 0, nil
	})
	t.Cleanup(restore)
	t.Setenv("CC_PROBELINE_CLAUDE_BIN", "/nonexistent/claude")
	t.Setenv(usagerefresh.NoRefreshEnv, "")

	// Same directory, two independent "windows" opening at once.
	for i := 0; i < 2; i++ {
		t.Setenv("CC_PROBELINE_USAGE_DIR", dir)
		usagerefresh.Maybe(refreshNow, true, true)
	}

	if launches != 1 {
		t.Errorf("launches = %d, want 1 across windows sharing the gate file", launches)
	}
}

// ---------------------------------------------------------------------------
// Property: when the throttle cannot be recorded, no refresh happens. A gate we
// cannot write is a gate that does not exist, and a status line ticks several
// times per second — "launch anyway" would mean a Claude Code process per
// render. Reachable in the wild: a root-owned ~/.local/share after a sudo
// install, a read-only home in a container, a full disk.
// ---------------------------------------------------------------------------

func TestMaybe_UnwritableGateRefusesToLaunch(t *testing.T) {
	parent := t.TempDir()
	readonly := filepath.Join(parent, "ro")
	if err := os.Mkdir(readonly, 0o500); err != nil { // r-x: cannot create inside
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("CC_PROBELINE_USAGE_DIR", filepath.Join(readonly, "cc-probeline"))
	t.Setenv("CC_PROBELINE_CLAUDE_BIN", "/nonexistent/claude")
	t.Setenv(usagerefresh.NoRefreshEnv, "")

	launches := 0
	restore := usagerefresh.SetLauncherForTest(func(string) (int, error) {
		launches++
		return 0, nil
	})
	t.Cleanup(restore)

	for i := 0; i < 25; i++ { // a few seconds of rendering
		usagerefresh.Maybe(refreshNow.Add(time.Duration(i)*time.Second), true, true)
	}

	if launches != 0 {
		t.Errorf("launches = %d, want 0 — an unrecordable throttle must fail closed", launches)
	}
}

// ---------------------------------------------------------------------------
// Property: a stamp from the future does not freeze refreshing forever. Clocks
// do jump backwards (NTP correction, dual boot, a home directory shared between
// machines); treating a future stamp as "fresh" would hide the model window and
// the badge until wall-clock time caught up, with nothing to diagnose.
// ---------------------------------------------------------------------------

func TestMaybe_FutureStampDoesNotFreezeRefresh(t *testing.T) {
	calls := harness(t)

	// Stamp "now", then ask again as if the clock had jumped an hour backwards.
	usagerefresh.Maybe(refreshNow, true, true)
	usagerefresh.Maybe(refreshNow.Add(-time.Hour), false, true)

	if *calls != 2 {
		t.Errorf("launches = %d, want 2 (the future stamp must be overwritten, not obeyed)", *calls)
	}
}

// ---------------------------------------------------------------------------
// Property: a refresh that is still running blocks the next one. The call takes
// ~4.5 s normally, but a stalled one (auth prompt, dead network, wrapper script)
// can sit forever — without this check every TTL would add another orphan.
// ---------------------------------------------------------------------------

func TestMaybe_LiveChildBlocksTheNextRefresh(t *testing.T) {
	t.Setenv("CC_PROBELINE_USAGE_DIR", t.TempDir())
	t.Setenv("CC_PROBELINE_CLAUDE_BIN", "/nonexistent/claude")
	t.Setenv(usagerefresh.NoRefreshEnv, "")

	launches := 0
	restore := usagerefresh.SetLauncherForTest(func(string) (int, error) {
		launches++
		return os.Getpid(), nil // our own pid: guaranteed alive for this test
	})
	t.Cleanup(restore)

	usagerefresh.Maybe(refreshNow, true, true)
	usagerefresh.Maybe(refreshNow.Add(2*time.Hour), false, true) // well past the TTL

	if launches != 1 {
		t.Errorf("launches = %d, want 1 while the previous refresh is still alive", launches)
	}
}
