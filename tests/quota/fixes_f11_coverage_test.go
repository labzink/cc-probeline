// Package quota_test — F11 coverage tests for internal/quota.
//
// These tests raise coverage of fresher() and Update() toward >= 90%
// by exercising branches not yet hit by the existing T-Q1/T-Q2/T-22a-c suite:
//
//	fresher:
//	  F11a: SevenDayReset-wins (s.SevenDayReset > stored.SevenDayReset)
//	  F11b: Same SevenDayReset, higher SevenDayPct wins
//	  F11c: TS tie-break (all reset+pct equal, greater TS wins)
//
//	Update:
//	  F11d: atomicWrite tmp-write error (quota dir exists but file is read-only)
//	  F11e: atomicWrite rename error path (tmp written but destination locked)
//
// Isolation: all tests use t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())
// so no real ~/.local/share/cc-probeline is touched.
package quota_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/quota"
)

// TestF11_Fresher_SevenDayResetWins (F11a) verifies that when s.SevenDayReset
// is strictly greater than stored.SevenDayReset, the snapshot is accepted
// even when FiveHour fields are unchanged or lower.
//
// This exercises the `s.SevenDayReset > stored.SevenDayReset` branch
// in fresher() (currently 0% coverage per F11 analysis).
func TestF11_Fresher_SevenDayResetWins(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	baseTS := now.UnixMilli()

	// Write initial: FiveHourReset=500 (equal to incoming), SevenDayReset=1000.
	stored := quota.Snapshot{
		TS:            baseTS,
		FiveHourPct:   40.0,
		SevenDayPct:   30.0,
		FiveHourReset: 500,
		SevenDayReset: 1000,
	}
	if err := quota.Update(stored); err != nil {
		t.Fatalf("Update(stored): %v", err)
	}

	// Incoming: same FiveHour fields, but later SevenDayReset — must be accepted.
	incoming := quota.Snapshot{
		TS:            baseTS + 1, // slightly newer just to be deterministic
		FiveHourPct:   40.0,       // same
		SevenDayPct:   35.0,       // higher pct within the newer 7d window
		FiveHourReset: 500,        // same as stored
		SevenDayReset: 2000,       // later 7d reset → must win
	}
	if err := quota.Update(incoming); err != nil {
		t.Fatalf("Update(incoming): %v", err)
	}

	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("F11a: Freshest after SevenDayReset-wins update: want (snapshot,true), got (_,false)")
	}
	if got.SevenDayReset != incoming.SevenDayReset {
		t.Errorf("F11a: SevenDayReset: want %d (fresher 7d window), got %d — later SevenDayReset must win",
			incoming.SevenDayReset, got.SevenDayReset)
	}
	if got.SevenDayPct != incoming.SevenDayPct {
		t.Errorf("F11a: SevenDayPct: want %.1f, got %.1f", incoming.SevenDayPct, got.SevenDayPct)
	}
}

// TestF11_Fresher_SameSevenDayResetHigherPctWins (F11b) verifies that when
// SevenDayReset values are equal, higher SevenDayPct causes acceptance.
//
// Exercises the `s.SevenDayReset == stored.SevenDayReset && s.SevenDayPct > stored.SevenDayPct`
// branch in fresher().
func TestF11_Fresher_SameSevenDayResetHigherPctWins(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	baseTS := now.UnixMilli()

	// Write initial: SevenDayReset=3000, SevenDayPct=25.
	stored := quota.Snapshot{
		TS:            baseTS,
		FiveHourPct:   50.0,
		SevenDayPct:   25.0,
		FiveHourReset: 700, // same as incoming
		SevenDayReset: 3000,
	}
	if err := quota.Update(stored); err != nil {
		t.Fatalf("Update(stored): %v", err)
	}

	// Incoming: same SevenDayReset=3000, but higher SevenDayPct=60 — must be accepted.
	// FiveHour fields are equal so only the 7d pct dimension triggers the win.
	incoming := quota.Snapshot{
		TS:            baseTS - 1, // older TS to ensure TS alone does not drive acceptance
		FiveHourPct:   50.0,       // equal
		SevenDayPct:   60.0,       // higher pct in same window → must win
		FiveHourReset: 700,        // equal
		SevenDayReset: 3000,       // equal
	}
	if err := quota.Update(incoming); err != nil {
		t.Fatalf("Update(incoming): %v", err)
	}

	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("F11b: Freshest: want (snapshot,true), got (_,false)")
	}
	if got.SevenDayPct != incoming.SevenDayPct {
		t.Errorf("F11b: SevenDayPct: want %.1f (higher pct in same 7d window), got %.1f",
			incoming.SevenDayPct, got.SevenDayPct)
	}
}

// TestF11_Fresher_TSTieBreak (F11c) verifies the TS tie-break: when all reset
// windows and pct values are equal, the snapshot with a greater TS is accepted.
//
// Exercises lines 89-95 of fresher() — the compound equality + TS comparison.
// This branch is at 0% coverage today.
func TestF11_Fresher_TSTieBreak(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	baseTS := now.UnixMilli()

	// All data fields are identical; only TS differs.
	stored := quota.Snapshot{
		TS:            baseTS,
		FiveHourPct:   33.0,
		SevenDayPct:   44.0,
		FiveHourReset: 111,
		SevenDayReset: 222,
	}
	if err := quota.Update(stored); err != nil {
		t.Fatalf("Update(stored): %v", err)
	}

	// Incoming: all data identical, but TS is 500ms later — must be accepted (tie-break).
	newer := quota.Snapshot{
		TS:            baseTS + 500, // strictly newer TS
		FiveHourPct:   33.0,         // equal
		SevenDayPct:   44.0,         // equal
		FiveHourReset: 111,          // equal
		SevenDayReset: 222,          // equal
	}
	if err := quota.Update(newer); err != nil {
		t.Fatalf("Update(newer): %v", err)
	}

	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("F11c: Freshest: want (snapshot,true), got (_,false)")
	}
	if got.TS != newer.TS {
		t.Errorf("F11c: TS tie-break: want TS=%d (newer observation), got TS=%d — equal data fields must resolve by newer TS",
			newer.TS, got.TS)
	}
}

// TestF11_Update_AtomicWriteError (F11d) verifies that Update returns an error
// when the atomic tmp-write fails (e.g. quota dir exists but is read-only so
// WriteFile(tmp) cannot create a new file).
//
// We make the quota *directory* read-only after MkdirAll so that the next
// Update call (which creates a .tmp file inside the dir) fails at os.WriteFile.
//
// Note: this test is skipped if run as root (root ignores file permissions).
func TestF11_Update_AtomicWriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("F11d: running as root — permission checks are bypassed; skip")
	}

	dir := t.TempDir()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", dir)

	// First, write a valid snapshot so the dir and quota.json exist.
	initial := quota.Snapshot{
		TS:            time.Now().UnixMilli(),
		FiveHourPct:   50.0,
		SevenDayPct:   30.0,
		FiveHourReset: 999,
		SevenDayReset: 888,
	}
	if err := quota.Update(initial); err != nil {
		t.Fatalf("F11d: initial Update: %v", err)
	}

	// Make the quota dir read-only so no new files can be created inside.
	// This will cause the WriteFile(tmp) call to fail.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("F11d: chmod dir read-only: %v", err)
	}
	// Restore permissions so t.TempDir() cleanup can remove the directory.
	t.Cleanup(func() { os.Chmod(dir, 0o755) }) //nolint:errcheck

	// Fresher snapshot — would overwrite if write succeeds.
	fresherSnap := quota.Snapshot{
		TS:            time.Now().UnixMilli() + 1000,
		FiveHourPct:   80.0,
		SevenDayPct:   60.0,
		FiveHourReset: 9999, // later window — would be accepted by fresher()
		SevenDayReset: 8888,
	}
	err := quota.Update(fresherSnap)
	if err == nil {
		t.Error("F11d: Update with read-only quota dir: want error (tmp write fails), got nil")
	}
}

// TestF11_Update_RenameError (F11e) verifies that Update returns an error and
// removes the .tmp file when os.Rename fails (e.g. the destination path is
// a directory, preventing rename from completing).
//
// We simulate a rename failure by pre-creating a *directory* at the exact path
// where quota.json would be written — renaming a file over a directory fails.
//
// Note: on macOS, os.Rename(file→dir) returns an error as expected.
// Note: skipped as root for same reason as F11d.
func TestF11_Update_RenameError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("F11e: running as root — skip permission-based rename test")
	}

	dir := t.TempDir()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", dir)

	// Pre-create quota.json as a *directory* — rename(quota.json.tmp, quota.json)
	// will fail because it cannot replace a directory with a file.
	quotaPath := filepath.Join(dir, "quota.json")
	if err := os.MkdirAll(quotaPath, 0o755); err != nil {
		t.Fatalf("F11e: create quota.json as dir: %v", err)
	}

	snap := quota.Snapshot{
		TS:            time.Now().UnixMilli(),
		FiveHourPct:   70.0,
		SevenDayPct:   55.0,
		FiveHourReset: 777,
		SevenDayReset: 666,
	}
	err := quota.Update(snap)
	if err == nil {
		t.Error("F11e: Update with quota.json blocked by directory: want error (rename fails), got nil")
	}

	// The .tmp file must be cleaned up after the rename error.
	tmpPath := quotaPath + ".tmp"
	if _, statErr := os.Stat(tmpPath); statErr == nil {
		t.Errorf("F11e: quota.json.tmp must be removed after rename error, but it still exists at %s", tmpPath)
	}
}
