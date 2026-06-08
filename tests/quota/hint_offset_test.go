// Package quota_test — tests for the rotating-hint offset that rides inside the
// existing global quota.json (Phase 6.95): HintStart reads it, BumpHintStart
// advances it (mod total) preserving quota fields, and quota.Update never resets
// it.
package quota_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/quota"
)

// TestHintStart_AbsentFile returns 0 and BumpHintStart does not create the file.
func TestHintStart_AbsentFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", dir)

	if got := quota.HintStart(); got != 0 {
		t.Errorf("HintStart() on absent file = %d; want 0", got)
	}
	quota.BumpHintStart(6) // must be a no-op: nothing to ride on yet
	if _, err := os.Stat(filepath.Join(dir, "quota.json")); !os.IsNotExist(err) {
		t.Errorf("BumpHintStart created quota.json with no quota data; want no file (err=%v)", err)
	}
	if got := quota.HintStart(); got != 0 {
		t.Errorf("HintStart() after no-op Bump = %d; want 0", got)
	}
}

// TestBumpHintStart_AdvancesAndWraps verifies the offset cycles 0→1→…→0 once the
// file exists, while preserving the quota payload across bumps.
func TestBumpHintStart_AdvancesAndWraps(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	// Seed real quota data so the file exists and we can check preservation.
	seed := quota.Snapshot{
		TS: 1234, FiveHourPct: 42, SevenDayPct: 17,
		FiveHourReset: 999, SevenDayReset: 888,
	}
	if err := quota.Update(seed); err != nil {
		t.Fatalf("Update seed: %v", err)
	}

	total := 6
	// Expected sequence of stored offsets after each bump, starting from 0.
	for step := 1; step <= total+1; step++ {
		quota.BumpHintStart(total)
		want := step % total
		if got := quota.HintStart(); got != want {
			t.Errorf("after %d bumps: HintStart()=%d; want %d", step, got, want)
		}
		// Quota payload must survive every bump.
		snap, ok := quota.Freshest()
		if !ok {
			t.Fatalf("after %d bumps: Freshest() reports no snapshot", step)
		}
		if snap.FiveHourPct != 42 || snap.SevenDayPct != 17 ||
			snap.FiveHourReset != 999 || snap.SevenDayReset != 888 || snap.TS != 1234 {
			t.Errorf("after %d bumps: quota fields mutated: %+v", step, snap)
		}
	}
}

// TestUpdate_PreservesHintStart verifies a quota refresh does not clobber the
// hint offset (it is owned by Bump/HintStart, not the quota payload).
func TestUpdate_PreservesHintStart(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	// Establish a non-zero offset on an existing file.
	if err := quota.Update(quota.Snapshot{TS: 1, FiveHourReset: 100}); err != nil {
		t.Fatalf("Update initial: %v", err)
	}
	quota.BumpHintStart(6) // offset → 1
	quota.BumpHintStart(6) // offset → 2
	if got := quota.HintStart(); got != 2 {
		t.Fatalf("setup: HintStart()=%d; want 2", got)
	}

	// A fresher quota snapshot arrives (later reset window → accepted by fresher()).
	if err := quota.Update(quota.Snapshot{TS: 2, FiveHourPct: 80, FiveHourReset: 200}); err != nil {
		t.Fatalf("Update fresher: %v", err)
	}

	if got := quota.HintStart(); got != 2 {
		t.Errorf("HintStart() after quota refresh = %d; want 2 (preserved)", got)
	}
	snap, _ := quota.Freshest()
	if snap.FiveHourPct != 80 || snap.FiveHourReset != 200 {
		t.Errorf("quota refresh did not apply: %+v", snap)
	}
}
