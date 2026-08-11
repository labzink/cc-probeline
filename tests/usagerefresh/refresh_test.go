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
	restore := usagerefresh.SetLauncherForTest(func(string) error {
		calls++
		return nil
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
	restore := usagerefresh.SetLauncherForTest(func(string) error {
		calls++
		return errors.New("boom")
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
	restore := usagerefresh.SetLauncherForTest(func(string) error {
		launches++
		return nil
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
