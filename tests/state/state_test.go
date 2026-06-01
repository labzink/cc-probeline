// Package state_test — RED tests for Phase 6.8.0 persisted session state.
// Contract: plans/phase-6.8/spec-common.md §2.1 (state.Session), §2.2 (Load/Save).
//
// Tests:
//   T-F5: TestState_RoundTrip — Save→Load returns equal Session; missing file → zero non-nil.
package state_test

import (
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/state"
)

// ---------------------------------------------------------------------------
// T-F5: TestState_RoundTrip
// Spec §2.2: state.Save + state.Load round-trip returns equivalent Session.
// Missing file: state.Load returns a non-nil *Session with Initialized=false.
// ---------------------------------------------------------------------------

// TestState_RoundTrip verifies two behaviours:
//  1. Save(sessionID, s) followed by Load(sessionID) returns an equivalent Session.
//  2. Load(sessionID) when no file exists returns a non-nil *Session with Initialized=false.
func TestState_RoundTrip(t *testing.T) {
	// Use a temp dir as the state root so tests are hermetic.
	// state.Load/Save must respect an env var or explicit path override
	// (analogous to mode.Path() using XDG_CONFIG_HOME).
	// Per spec §2.2 contract: Save and Load accept a sessionID string;
	// the implementation resolves the file path internally.
	//
	// To keep the test hermetic without mocking internals, we use
	// t.Setenv to redirect state storage to a temp dir (matching mode.go pattern).
	tmpDir := t.TempDir()
	t.Setenv("CC_PROBELINE_STATE_DIR", tmpDir)

	const sessionID = "test-session-abc-123"

	// --- Sub-test 1: missing file returns zero non-nil Session ---
	t.Run("missing_returns_zero", func(t *testing.T) {
		// Given: no state file exists for the session ID.
		// When: Load is called.
		got := state.Load(sessionID)

		// Then: result must be non-nil and have Initialized=false.
		if got == nil {
			t.Fatal("Load: want non-nil *Session for missing file, got nil")
		}
		if got.Initialized {
			t.Error("Load: Initialized=true, want false (missing file → zero session)")
		}
		if got.BaselineCost != 0 {
			t.Errorf("Load: BaselineCost=%v, want 0", got.BaselineCost)
		}
	})

	// --- Sub-test 2: Save→Load round-trip ---
	t.Run("save_load_round_trip", func(t *testing.T) {
		// Given: a populated Session.
		want := &state.Session{
			Initialized:   true,
			BaselineCost:  1.23,
			BaselineDurMS: 45000,
			LastSeenTotal: 2.46,
			PerTurnCost: map[string]float64{
				"turn-uuid-001": 0.001,
				"turn-uuid-002": 0.002,
			},
			PromptCost: map[int]float64{
				1: 0.10,
				2: 0.20,
			},
			// LastGoodGit is nil (no git status in this test).
			LastGoodGit: nil,
		}

		// When: Save is called.
		if err := state.Save(sessionID, want); err != nil {
			t.Fatalf("Save: unexpected error: %v", err)
		}

		// Verify a state file was created in the temp dir.
		pattern := filepath.Join(tmpDir, "*"+sessionID+"*")
		matches, _ := filepath.Glob(pattern)
		// The file naming convention is implementation-defined; just verify Save didn't panic
		// and Load can retrieve it.
		_ = matches

		// When: Load is called for the same session ID.
		got := state.Load(sessionID)

		// Then: result must be non-nil.
		if got == nil {
			t.Fatal("Load after Save: want non-nil *Session, got nil")
		}

		// Core fields must round-trip exactly.
		if !got.Initialized {
			t.Error("Load: Initialized=false, want true (was saved as true)")
		}
		if got.BaselineCost != want.BaselineCost {
			t.Errorf("Load: BaselineCost=%v, want %v", got.BaselineCost, want.BaselineCost)
		}
		if got.BaselineDurMS != want.BaselineDurMS {
			t.Errorf("Load: BaselineDurMS=%v, want %v", got.BaselineDurMS, want.BaselineDurMS)
		}
		if got.LastSeenTotal != want.LastSeenTotal {
			t.Errorf("Load: LastSeenTotal=%v, want %v", got.LastSeenTotal, want.LastSeenTotal)
		}

		// PerTurnCost map must round-trip.
		if len(got.PerTurnCost) != len(want.PerTurnCost) {
			t.Errorf("Load: len(PerTurnCost)=%d, want %d", len(got.PerTurnCost), len(want.PerTurnCost))
		}
		for k, wv := range want.PerTurnCost {
			gv, ok := got.PerTurnCost[k]
			if !ok {
				t.Errorf("Load: PerTurnCost[%q] missing", k)
				continue
			}
			if gv != wv {
				t.Errorf("Load: PerTurnCost[%q]=%v, want %v", k, gv, wv)
			}
		}

		// PromptCost map must round-trip.
		if len(got.PromptCost) != len(want.PromptCost) {
			t.Errorf("Load: len(PromptCost)=%d, want %d", len(got.PromptCost), len(want.PromptCost))
		}
		for k, wv := range want.PromptCost {
			gv, ok := got.PromptCost[k]
			if !ok {
				t.Errorf("Load: PromptCost[%d] missing", k)
				continue
			}
			if gv != wv {
				t.Errorf("Load: PromptCost[%d]=%v, want %v", k, gv, wv)
			}
		}

		// LastGoodGit: both nil in this test.
		if got.LastGoodGit != nil {
			t.Errorf("Load: LastGoodGit=%v, want nil", got.LastGoodGit)
		}
	})

	// --- Sub-test 3: Save is idempotent (overwrite existing) ---
	t.Run("save_overwrites", func(t *testing.T) {
		// Given: an initial session saved.
		initial := &state.Session{
			Initialized:  true,
			BaselineCost: 0.5,
		}
		if err := state.Save(sessionID+"-ow", initial); err != nil {
			t.Fatalf("first Save: %v", err)
		}

		// When: Save is called again with different data.
		updated := &state.Session{
			Initialized:  true,
			BaselineCost: 9.99,
		}
		if err := state.Save(sessionID+"-ow", updated); err != nil {
			t.Fatalf("second Save: %v", err)
		}

		// Then: Load returns the latest data.
		got := state.Load(sessionID + "-ow")
		if got == nil {
			t.Fatal("Load after overwrite: want non-nil")
		}
		if got.BaselineCost != updated.BaselineCost {
			t.Errorf("Load: BaselineCost=%v, want %v (overwritten value)", got.BaselineCost, updated.BaselineCost)
		}
	})
}
