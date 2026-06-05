// Package hint_test verifies the per-session hint State rotation logic
// (AllShown / Advance). Phase 6.95.b retired the separate hint-<sid>.json
// persistence layer; rotation now lives in state.Session.HintRotation, so the
// disk I/O tests (StatePath/Save/Load) were removed with the I/O itself.
package hint_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
)

// ---------------------------------------------------------------------------
// AllShown
// ---------------------------------------------------------------------------

// TestState_AllShown_ZeroOfTotal verifies that a zero State (no shown indices)
// returns false for AllShown(8).
func TestState_AllShown_ZeroOfTotal(t *testing.T) {
	s := hint.State{}
	if s.AllShown(8) {
		t.Error("AllShown(8) on empty state = true; want false")
	}
}

// TestState_AllShown_AllOfTotal verifies that when all 8 indices are shown,
// AllShown(8) returns true.
func TestState_AllShown_AllOfTotal(t *testing.T) {
	s := hint.State{ShownIndices: []int{0, 1, 2, 3, 4, 5, 6, 7}}
	if !s.AllShown(8) {
		t.Error("AllShown(8) on full shown = false; want true")
	}
}

// TestState_AllShown_PartialOfTotal verifies that when only 2 of 8 are shown,
// AllShown(8) returns false.
func TestState_AllShown_PartialOfTotal(t *testing.T) {
	s := hint.State{ShownIndices: []int{0, 1}}
	if s.AllShown(8) {
		t.Error("AllShown(8) on partial shown = true; want false")
	}
}

// ---------------------------------------------------------------------------
// Advance
// ---------------------------------------------------------------------------

// TestState_Advance_FromZero verifies that Advance on a zero State (CurrentIndex=0,
// no shown indices) picks index 1 (next from default 0) and stamps LastSwitch.
func TestState_Advance_FromZero(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := hint.State{}
	s.Advance(8, now)
	if s.CurrentIndex != 1 {
		t.Errorf("Advance(8) from zero: CurrentIndex = %d; want 1", s.CurrentIndex)
	}
	if len(s.ShownIndices) != 1 || s.ShownIndices[0] != 1 {
		t.Errorf("Advance(8) from zero: ShownIndices = %v; want [1]", s.ShownIndices)
	}
	if !s.LastSwitch.Equal(now) {
		t.Errorf("Advance(8): LastSwitch = %v; want %v", s.LastSwitch, now)
	}
}

// TestState_Advance_SkipsAlreadyShown verifies that Advance skips indices already
// in ShownIndices and moves to the next unseen one.
func TestState_Advance_SkipsAlreadyShown(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// Shown: 0 and 2; current is 2 — next unseen should be 3.
	s := hint.State{
		ShownIndices: []int{0, 2},
		CurrentIndex: 2,
	}
	s.Advance(8, now)
	if s.CurrentIndex != 3 {
		t.Errorf("Advance: CurrentIndex = %d; want 3", s.CurrentIndex)
	}
}

// TestState_Advance_WrapsAround verifies that when the shown set is {3,4,5,6,7}
// and CurrentIndex=7, Advance wraps around and picks index 0.
func TestState_Advance_WrapsAround(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := hint.State{
		ShownIndices: []int{3, 4, 5, 6, 7},
		CurrentIndex: 7,
	}
	s.Advance(8, now)
	if s.CurrentIndex != 0 {
		t.Errorf("Advance(wrap): CurrentIndex = %d; want 0", s.CurrentIndex)
	}
}
