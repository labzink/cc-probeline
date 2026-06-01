// Package quota_test — additional edge-case tests for internal/quota.
//
// These tests cover error paths not exercised by the primary T-Q1/T-Q2 tests:
//   - Empty HOME (no quota dir resolvable) → Update and Freshest return graceful error/false.
//   - Corrupt file on disk → Freshest returns false (decode error).
//   - Update when existing file is corrupt → overwrite succeeds (lenient-overwrite path).
package quota_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/quota"
)

// TestQuota_UpdateNoDir verifies that Update returns an error when the quota
// directory cannot be determined (HOME not set and no env overrides).
func TestQuota_UpdateNoDir(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	snap := quota.Snapshot{
		TS:          time.Now().UnixMilli(),
		FiveHourPct: 50.0,
		SevenDayPct: 30.0,
	}
	err := quota.Update(snap)
	if err == nil {
		t.Error("Update with no HOME/dir: want error, got nil")
	}
}

// TestQuota_FreshestNoDir verifies that Freshest returns (zero, false) when the
// quota directory cannot be determined (HOME not set and no env overrides).
func TestQuota_FreshestNoDir(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	got, ok := quota.Freshest()
	if ok {
		t.Errorf("Freshest with no HOME/dir: want (zero,false), got (%+v, true)", got)
	}
	if got != (quota.Snapshot{}) {
		t.Errorf("Freshest with no HOME/dir: want zero Snapshot, got %+v", got)
	}
}

// TestQuota_FreshestCorruptFile verifies that Freshest returns (zero, false) when
// the quota.json file exists but contains malformed JSON.
func TestQuota_FreshestCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", dir)

	// Write corrupt JSON directly without using quota.Update.
	p := filepath.Join(dir, "quota.json")
	if err := os.WriteFile(p, []byte("not valid json{{{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok := quota.Freshest()
	if ok {
		t.Errorf("Freshest with corrupt file: want (zero,false), got (%+v, true)", got)
	}
	if got != (quota.Snapshot{}) {
		t.Errorf("Freshest with corrupt file: want zero Snapshot, got %+v", got)
	}
}

// TestQuota_UpdateOverwritesCorruptFile verifies that Update succeeds (overwrites)
// when the existing quota.json is corrupt (lenient-overwrite path).
func TestQuota_UpdateOverwritesCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", dir)

	// Pre-populate with corrupt JSON.
	p := filepath.Join(dir, "quota.json")
	if err := os.WriteFile(p, []byte("not valid json{{{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	snap := quota.Snapshot{
		TS:          time.Now().UnixMilli(),
		FiveHourPct: 70.0,
		SevenDayPct: 55.0,
	}
	if err := quota.Update(snap); err != nil {
		t.Fatalf("Update over corrupt file: unexpected error: %v", err)
	}

	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("Freshest after overwrite of corrupt file: want (snapshot,true), got (_,false)")
	}
	if got.FiveHourPct != snap.FiveHourPct {
		t.Errorf("Freshest.FiveHourPct: want %v, got %v", snap.FiveHourPct, got.FiveHourPct)
	}
}

// TestQuota_FreshestXDGDataHome verifies path resolution via XDG_DATA_HOME
// when CC_PROBELINE_QUOTA_DIR is not set.
func TestQuota_FreshestXDGDataHome(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", "")
	t.Setenv("XDG_DATA_HOME", xdgDir)

	// Empty XDG dir → (zero, false).
	got, ok := quota.Freshest()
	if ok {
		t.Errorf("Freshest via XDG with empty dir: want (zero,false), got (%+v,true)", got)
	}
	if got != (quota.Snapshot{}) {
		t.Errorf("Freshest via XDG with empty dir: want zero, got %+v", got)
	}

	// Write via Update and verify round-trip.
	snap := quota.Snapshot{
		TS:          time.Now().UnixMilli(),
		FiveHourPct: 22.0,
		SevenDayPct: 11.0,
	}
	if err := quota.Update(snap); err != nil {
		t.Fatalf("Update via XDG: %v", err)
	}
	got2, ok2 := quota.Freshest()
	if !ok2 {
		t.Fatal("Freshest via XDG after Update: want (snapshot,true), got (_,false)")
	}
	if got2.FiveHourPct != snap.FiveHourPct {
		t.Errorf("Freshest via XDG: FiveHourPct want %v got %v", snap.FiveHourPct, got2.FiveHourPct)
	}
}
