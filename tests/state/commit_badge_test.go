// Package state_test — Phase 6.95.a commit-badge transition tests.
// CommitBadgeTick is a pure state transition: it arms a "✓ N committed" badge
// when the working tree's modified count drops from N>0 to 0, shows it for
// exactly one refresh, then clears it.
package state_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/state"
)

// TestCommitBadge_TransitionShowsOnceThenClears is the core contract: a 3→0
// transition shows "✓ 3" on the triggering refresh and nothing on the next.
func TestCommitBadge_TransitionShowsOnceThenClears(t *testing.T) {
	s := &state.Session{}

	// Refresh 1: 3 modified files → 0 (a commit happened). Badge shows 3.
	if got := s.CommitBadgeTick(3, 0, true); got != 3 {
		t.Fatalf("transition 3→0: badge = %d; want 3", got)
	}
	if !s.CommitBadge.Shown {
		t.Errorf("after showing: CommitBadge.Shown = false; want true")
	}

	// Refresh 2: clean tree stays clean (0→0). Badge must be gone now.
	if got := s.CommitBadgeTick(0, 0, true); got != 0 {
		t.Errorf("refresh after show: badge = %d; want 0 (cleared)", got)
	}
	if s.CommitBadge.Count != 0 {
		t.Errorf("after clear: CommitBadge.Count = %d; want 0", s.CommitBadge.Count)
	}
}

// TestCommitBadge_NoTransitionNoBadge verifies that a modified count that drops
// but does not reach 0 (3→2) does not arm the badge.
func TestCommitBadge_NoTransitionNoBadge(t *testing.T) {
	s := &state.Session{}
	if got := s.CommitBadgeTick(3, 2, true); got != 0 {
		t.Errorf("3→2 (still dirty): badge = %d; want 0", got)
	}
	if s.CommitBadge.Count != 0 {
		t.Errorf("3→2: CommitBadge armed unexpectedly: %+v", s.CommitBadge)
	}
}

// TestCommitBadge_StaysClearWhenAlreadyClean verifies that 0→0 with no prior
// badge never produces output.
func TestCommitBadge_StaysClearWhenAlreadyClean(t *testing.T) {
	s := &state.Session{}
	if got := s.CommitBadgeTick(0, 0, true); got != 0 {
		t.Errorf("0→0: badge = %d; want 0", got)
	}
}

// TestCommitBadge_GitFailSuppressesTrigger verifies that a prev>0, curr==0 shape
// does NOT arm the badge when the current git status was not detected (gitOK=false):
// curr==0 is then meaningless (it is the zero fallback, not a real clean tree).
func TestCommitBadge_GitFailSuppressesTrigger(t *testing.T) {
	s := &state.Session{}
	if got := s.CommitBadgeTick(3, 0, false); got != 0 {
		t.Errorf("3→0 with gitOK=false: badge = %d; want 0 (no trigger)", got)
	}
	if s.CommitBadge.Count != 0 {
		t.Errorf("gitOK=false: badge armed unexpectedly: %+v", s.CommitBadge)
	}
}

// TestCommitBadge_SurvivesIntermediateRefresh verifies that once armed, the badge
// shows on the very next tick even if that tick itself sees no new transition
// (e.g. a refresh where git detection failed). It then clears on the following tick.
func TestCommitBadge_ArmedThenShownThenCleared(t *testing.T) {
	s := &state.Session{}

	// Arm via a real transition.
	if got := s.CommitBadgeTick(2, 0, true); got != 2 {
		t.Fatalf("arm: badge = %d; want 2", got)
	}
	// Following tick clears it.
	if got := s.CommitBadgeTick(0, 0, true); got != 0 {
		t.Fatalf("clear tick: badge = %d; want 0", got)
	}
	// And it stays cleared.
	if got := s.CommitBadgeTick(0, 0, true); got != 0 {
		t.Errorf("stays cleared: badge = %d; want 0", got)
	}
}
