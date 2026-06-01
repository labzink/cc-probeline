// Package parser_test — Phase 6.8.c T-G2: anti-flicker git probe behaviour.
// Contract: spec-common.md §2.3 — anti-flicker lives in probe/main layer.
//           T-14: on DetectGit error, probe shows LastGoodGit (never empty).
//
// The anti-flicker contract: if DetectGit returns an error, the probe/main
// layer must return state.Session.LastGoodGit (the previous successful result)
// rather than showing an empty or error state. On success, LastGoodGit is
// updated and persisted via state.Save.
//
// The logic is extracted into parser.ResolveGitStatus so it can be unit-tested
// independently of cmd/cc-probeline/main.go.
// Signature (to be implemented by dev):
//
//	func ResolveGitStatus(fresh *GitStatus, err error, last *GitStatus) *GitStatus
//
// Returns:
//   - fresh (non-nil) when err == nil: the new successful result.
//   - last (possibly nil) when err != nil: fall back to previous good state.
package parser_test

import (
	"errors"
	"testing"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// T-G2: TestGit_AntiFlicker — Phase 6.8.c
// Contract: spec-common.md §2.3, T-14.
// ---------------------------------------------------------------------------

func TestGit_AntiFlicker(t *testing.T) {
	knownGood := &parser.GitStatus{
		Branch:        "main",
		ModifiedCount: 2,
	}

	tests := []struct {
		name     string
		fresh    *parser.GitStatus // result from DetectGit
		err      error             // error from DetectGit
		last     *parser.GitStatus // state.Session.LastGoodGit
		wantNil  bool              // expect nil return
		wantGit  *parser.GitStatus // exact expected pointer (or content)
	}{
		{
			// When DetectGit succeeds, the fresh result is returned.
			// LastGoodGit should be updated to fresh (done by caller, not tested here).
			name:    "success: returns fresh result",
			fresh:   &parser.GitStatus{Branch: "feature/new", ModifiedCount: 1},
			err:     nil,
			last:    knownGood,
			wantGit: &parser.GitStatus{Branch: "feature/new", ModifiedCount: 1},
		},
		{
			// When DetectGit fails and LastGoodGit is set, return LastGoodGit.
			// This is the core anti-flicker behaviour (T-14).
			name:    "error with LastGoodGit: returns last good state",
			fresh:   nil,
			err:     errors.New("git subprocess failed"),
			last:    knownGood,
			wantGit: knownGood,
		},
		{
			// When DetectGit fails and no LastGoodGit is available (first run),
			// return nil (caller must handle gracefully, e.g. hide probe segment).
			name:    "error without LastGoodGit: returns nil",
			fresh:   nil,
			err:     errors.New("not a git repo"),
			last:    nil,
			wantNil: true,
		},
		{
			// When DetectGit succeeds and no LastGoodGit was previously set,
			// fresh result is returned normally.
			name:    "success without prior LastGoodGit: returns fresh",
			fresh:   &parser.GitStatus{Branch: "main", ModifiedCount: 0},
			err:     nil,
			last:    nil,
			wantGit: &parser.GitStatus{Branch: "main", ModifiedCount: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: the result of a DetectGit call (fresh, err) and the
			//        previously persisted good state (last = Session.LastGoodGit).
			// When: ResolveGitStatus applies the anti-flicker logic.
			result := parser.ResolveGitStatus(tc.fresh, tc.err, tc.last)

			// Then:
			if tc.wantNil {
				if result != nil {
					t.Errorf("expected nil result when error and no LastGoodGit, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected non-nil *GitStatus, got nil")
			}
			if result.Branch != tc.wantGit.Branch {
				t.Errorf("Branch: got %q, want %q", result.Branch, tc.wantGit.Branch)
			}
			if result.ModifiedCount != tc.wantGit.ModifiedCount {
				t.Errorf("ModifiedCount: got %d, want %d", result.ModifiedCount, tc.wantGit.ModifiedCount)
			}
		})
	}
}

// TestGit_AntiFlicker_LastGoodNotMutated verifies that ResolveGitStatus does
// not mutate the LastGoodGit pointer on error — the caller owns update/persist.
func TestGit_AntiFlicker_LastGoodNotMutated(t *testing.T) {
	original := &parser.GitStatus{Branch: "stable", ModifiedCount: 0}
	// Take a copy to detect mutation.
	copyBranch := original.Branch
	copyCount := original.ModifiedCount

	result := parser.ResolveGitStatus(nil, errors.New("timeout"), original)

	// result must equal original (same pointer or equivalent value)
	if result == nil {
		t.Fatal("expected LastGoodGit to be returned, got nil")
	}
	// original must not have been mutated
	if original.Branch != copyBranch {
		t.Errorf("LastGoodGit.Branch mutated: was %q, now %q", copyBranch, original.Branch)
	}
	if original.ModifiedCount != copyCount {
		t.Errorf("LastGoodGit.ModifiedCount mutated: was %d, now %d", copyCount, original.ModifiedCount)
	}
}
