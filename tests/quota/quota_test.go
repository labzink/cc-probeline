// Package quota_test — black-box tests for internal/quota.
//
// Tests T-Q1 and T-Q2 verify the global freshness-snapshot store:
//   - T-Q1 (TestQuota_UpdateNewerOnly): Update writes only when TS is fresher.
//   - T-Q2 (TestQuota_Freshest): Freshest returns the last snapshot; empty dir → (zero,false).
//
// Isolation: CC_PROBELINE_QUOTA_DIR is set to t.TempDir() so each test case
// operates on its own directory and cannot interfere with the real user state.
package quota_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/quota"
)

// TestQuota_UpdateNewerOnly (T-Q1 / T-10) verifies that quota.Update writes
// the snapshot only when the incoming TS is strictly newer than the stored one.
//
// Steps:
//  1. Write a "fresh" snapshot with TS=now.
//  2. Verify Freshest returns it (FiveHourPct=67).
//  3. Call Update again with an older TS (now-5min) and FiveHourPct=64.
//  4. Verify Freshest still returns 67 (the older snapshot was rejected).
func TestQuota_UpdateNewerOnly(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now().UnixMilli()
	stale := now - 5*60*1000 // 5 minutes earlier

	fresh := quota.Snapshot{
		TS:          now,
		FiveHourPct: 67.0,
		SevenDayPct: 42.0,
	}
	if err := quota.Update(fresh); err != nil {
		t.Fatalf("Update(fresh): unexpected error: %v", err)
	}

	// Verify the fresh snapshot is stored.
	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("Freshest after Update(fresh): want (snapshot,true), got (_, false)")
	}
	if got.FiveHourPct != 67.0 {
		t.Errorf("Freshest.FiveHourPct after fresh Update: want 67.0, got %v", got.FiveHourPct)
	}

	// Now try to update with a stale snapshot (lower TS, different FiveHourPct).
	staleSnapshot := quota.Snapshot{
		TS:          stale,
		FiveHourPct: 64.0, // would overwrite if Update accepted stale TS
		SevenDayPct: 42.0,
	}
	if err := quota.Update(staleSnapshot); err != nil {
		t.Fatalf("Update(stale): unexpected error: %v", err)
	}

	// Freshest must still return the original 67% snapshot.
	got2, ok2 := quota.Freshest()
	if !ok2 {
		t.Fatal("Freshest after stale Update: want (snapshot,true), got (_, false)")
	}
	if got2.FiveHourPct != 67.0 {
		t.Errorf("Freshest.FiveHourPct after stale Update: want 67.0 (unchanged), got %v — stale TS must not overwrite fresh data", got2.FiveHourPct)
	}
	if got2.TS != now {
		t.Errorf("Freshest.TS after stale Update: want %d (original), got %d", now, got2.TS)
	}
}

// TestQuota_Freshest (T-Q2 / T-11) verifies:
//   - When no file exists (fresh TempDir), Freshest returns (zero, false).
//   - After a successful Update, Freshest returns (snapshot, true) with matching fields.
func TestQuota_Freshest(t *testing.T) {
	// Sub-case A: empty directory → (zero, false).
	t.Run("empty_dir_returns_false", func(t *testing.T) {
		t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

		got, ok := quota.Freshest()
		if ok {
			t.Errorf("Freshest on empty dir: want (zero,false), got (%+v, true)", got)
		}
		if got != (quota.Snapshot{}) {
			t.Errorf("Freshest on empty dir: want zero Snapshot, got %+v", got)
		}
	})

	// Sub-case B: after Update, Freshest returns matching snapshot.
	t.Run("after_update_returns_snapshot", func(t *testing.T) {
		t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

		ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
		want := quota.Snapshot{
			TS:            ts,
			FiveHourPct:   55.5,
			SevenDayPct:   83.3,
			FiveHourReset: 1748786400, // arbitrary non-zero unix timestamp
			SevenDayReset: 1748872800,
		}

		if err := quota.Update(want); err != nil {
			t.Fatalf("Update: unexpected error: %v", err)
		}

		got, ok := quota.Freshest()
		if !ok {
			t.Fatal("Freshest after Update: want (snapshot, true), got (_, false)")
		}

		if got.TS != want.TS {
			t.Errorf("Freshest.TS: want %d, got %d", want.TS, got.TS)
		}
		if got.FiveHourPct != want.FiveHourPct {
			t.Errorf("Freshest.FiveHourPct: want %v, got %v", want.FiveHourPct, got.FiveHourPct)
		}
		if got.SevenDayPct != want.SevenDayPct {
			t.Errorf("Freshest.SevenDayPct: want %v, got %v", want.SevenDayPct, got.SevenDayPct)
		}
		if got.FiveHourReset != want.FiveHourReset {
			t.Errorf("Freshest.FiveHourReset: want %d, got %d", want.FiveHourReset, got.FiveHourReset)
		}
		if got.SevenDayReset != want.SevenDayReset {
			t.Errorf("Freshest.SevenDayReset: want %d, got %d", want.SevenDayReset, got.SevenDayReset)
		}
	})
}
