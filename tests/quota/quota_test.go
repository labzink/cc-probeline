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

// --- Phase 6.9.b: T-22 freshest-by-data (reset-window / used%) ---

// TestQuotaUpdate_StaleDoesNotOverwrite (T-22a) verifies that a snapshot with
// an older reset-window does NOT overwrite even when its TS is strictly newer.
//
// Scenario:
//   stored: FiveHourReset=100, FiveHourPct=60, TS=now
//   incoming: FiveHourReset=90 (older window), FiveHourPct=70, TS=now+1s (newer TS)
//
// New contract (§2.3 freshest-by-data): accept only if reset-window is later
// OR (equal reset-window AND higher used%). A newer TS alone is not sufficient.
// The incoming snapshot here has both older reset-window AND lower used% in that
// window — it must be rejected.
//
// Current behaviour (TS-only): would accept because TS is newer → test is RED.
func TestQuotaUpdate_StaleDoesNotOverwrite(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	baseTS := now.UnixMilli()

	// Write the "good" snapshot with a later reset-window and 60% usage.
	stored := quota.Snapshot{
		TS:            baseTS,
		FiveHourPct:   60.0,
		SevenDayPct:   40.0,
		FiveHourReset: 1000, // later reset-window
		SevenDayReset: 2000,
	}
	if err := quota.Update(stored); err != nil {
		t.Fatalf("Update(stored): %v", err)
	}

	// Incoming: newer TS but older reset-window — must be rejected per new contract.
	stale := quota.Snapshot{
		TS:            baseTS + 1000, // strictly newer TS (1 second later)
		FiveHourPct:   70.0,          // higher usage — but reset-window is older
		SevenDayPct:   50.0,
		FiveHourReset: 900, // older reset-window → must lose
		SevenDayReset: 1900,
	}
	if err := quota.Update(stale); err != nil {
		t.Fatalf("Update(stale): %v", err)
	}

	// Freshest must still reflect the stored snapshot (FiveHourReset=1000).
	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("Freshest after stale overwrite attempt: want (snapshot,true), got (_,false)")
	}
	if got.FiveHourReset != stored.FiveHourReset {
		t.Errorf("T-22a: FiveHourReset: want %d (stored, later window), got %d — stale snapshot with older reset-window must NOT overwrite",
			stored.FiveHourReset, got.FiveHourReset)
	}
	if got.FiveHourPct != stored.FiveHourPct {
		t.Errorf("T-22a: FiveHourPct: want %.1f (stored), got %.1f", stored.FiveHourPct, got.FiveHourPct)
	}
}

// TestQuotaUpdate_FresherWindowWins (T-22b) verifies that a snapshot with a
// later FiveHourReset (newer reset-window) is accepted even when its TS is older.
//
// Scenario:
//   stored: FiveHourReset=100, TS=now+5s
//   incoming: FiveHourReset=200 (later window), TS=now (older TS)
//
// New contract: later reset-window wins regardless of TS.
// Current behaviour (TS-only): would reject because TS is older → test is RED.
func TestQuotaUpdate_FresherWindowWins(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	baseTS := now.UnixMilli()

	// Write initial snapshot with older reset-window but newer TS.
	stored := quota.Snapshot{
		TS:            baseTS + 5000, // newer TS
		FiveHourPct:   30.0,
		SevenDayPct:   20.0,
		FiveHourReset: 100, // older reset-window
		SevenDayReset: 200,
	}
	if err := quota.Update(stored); err != nil {
		t.Fatalf("Update(stored): %v", err)
	}

	// Incoming: older TS but later reset-window — must be accepted.
	fresher := quota.Snapshot{
		TS:            baseTS, // older TS
		FiveHourPct:   55.0,
		SevenDayPct:   45.0,
		FiveHourReset: 200, // later reset-window → must win
		SevenDayReset: 300,
	}
	if err := quota.Update(fresher); err != nil {
		t.Fatalf("Update(fresher): %v", err)
	}

	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("Freshest after fresher-window update: want (snapshot,true), got (_,false)")
	}
	if got.FiveHourReset != fresher.FiveHourReset {
		t.Errorf("T-22b: FiveHourReset: want %d (fresher window), got %d — later reset-window must be accepted even with older TS",
			fresher.FiveHourReset, got.FiveHourReset)
	}
	if got.FiveHourPct != fresher.FiveHourPct {
		t.Errorf("T-22b: FiveHourPct: want %.1f, got %.1f", fresher.FiveHourPct, got.FiveHourPct)
	}
}

// TestQuotaUpdate_SameWindowHigherPctWins (T-22c) verifies that when two
// snapshots have equal FiveHourReset, the one with higher FiveHourPct wins.
//
// Scenario:
//   stored: FiveHourReset=500, FiveHourPct=40, TS=now+2s
//   incoming: FiveHourReset=500 (equal window), FiveHourPct=75, TS=now (older TS)
//
// New contract: equal reset-window + higher used% → accept.
// Current behaviour (TS-only): would reject because TS is older → test is RED.
func TestQuotaUpdate_SameWindowHigherPctWins(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	baseTS := now.UnixMilli()

	// Write initial snapshot: same window, lower usage, newer TS.
	stored := quota.Snapshot{
		TS:            baseTS + 2000, // newer TS
		FiveHourPct:   40.0,
		SevenDayPct:   30.0,
		FiveHourReset: 500,
		SevenDayReset: 600,
	}
	if err := quota.Update(stored); err != nil {
		t.Fatalf("Update(stored): %v", err)
	}

	// Incoming: same reset-window, higher usage, older TS — must be accepted.
	higher := quota.Snapshot{
		TS:            baseTS, // older TS
		FiveHourPct:   75.0,   // higher usage in same window → must win
		SevenDayPct:   60.0,
		FiveHourReset: 500, // same window
		SevenDayReset: 600,
	}
	if err := quota.Update(higher); err != nil {
		t.Fatalf("Update(higher): %v", err)
	}

	got, ok := quota.Freshest()
	if !ok {
		t.Fatal("Freshest after same-window higher-pct update: want (snapshot,true), got (_,false)")
	}
	if got.FiveHourPct != higher.FiveHourPct {
		t.Errorf("T-22c: FiveHourPct: want %.1f (higher pct in same window), got %.1f — higher used%% in equal window must win",
			higher.FiveHourPct, got.FiveHourPct)
	}
	if got.FiveHourReset != stored.FiveHourReset {
		t.Errorf("T-22c: FiveHourReset: want %d (unchanged), got %d", stored.FiveHourReset, got.FiveHourReset)
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
