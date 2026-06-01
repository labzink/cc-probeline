// Package probes_test — black-box tests for GitProbe (Phase 4.1.c RED).
//
// Sources:
//   - plans/concepts/phase-4-architecture.md §4.1.c (lines 518-540)
//   - plans/tasks/phase-4-step1-plan.md §Subtask 4.1.c (lines 436-441, 456-462)
//
// GitProbe contract:
//
//	Visible(d, cfg) == false  when d.Git == nil
//	Full/Compact:  "⎇ <branch>" + " ⚠N" if N>0
//	               if Worktree != "" → prefix branch with "-:"
//	Minimal:       middle-truncate branch to min 8 visible chars (…)
//
// All five tests compile-fail in RED because parser.GitStatus and
// probes.GitProbe do not exist yet — that is the intended RED state.
package probes_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// TestGit_Full verifies that GitProbe renders the branch name and modified-file
// count at LevelFull for a typical feature branch.
//
// PLAN line 437: Git={Branch:"agent/plan/phase-1-prep", ModifiedCount:1} →
// expected "⎇ agent/plan/phase-1-prep ⚠1".
func TestGit_Full(t *testing.T) {
	p := &probes.GitProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        "agent/plan/phase-1-prep",
			ModifiedCount: 1,
		},
	}

	got := p.Render(d, cfg, th, probes.LevelFull)
	want := "⎇ agent/plan/phase-1-prep ⚠1"
	if got != want {
		t.Errorf("Render(Full): want %q, got %q", want, got)
	}
}

// TestGit_FullWorktree verifies that a non-empty Worktree field prepends "-:"
// before the branch name in the Full output.
//
// PLAN lines 460-461: if Worktree != "" → prefix "-:" before branch.
// When ModifiedCount==0 the ⚠N suffix must not appear.
func TestGit_FullWorktree(t *testing.T) {
	p := &probes.GitProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        "main",
			ModifiedCount: 0,
			Worktree:      "phase-1",
		},
	}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// Must carry the "-:" worktree prefix before the branch name.
	if !strings.Contains(got, "⎇ -:") {
		t.Errorf("Render(Full, worktree): want prefix %q in output, got %q", "⎇ -:", got)
	}
	if !strings.Contains(got, "main") {
		t.Errorf("Render(Full, worktree): want branch %q in output, got %q", "main", got)
	}
	// No modified count → no warning suffix.
	if strings.Contains(got, "⚠") {
		t.Errorf("Render(Full, worktree, ModifiedCount=0): got unexpected ⚠ in %q", got)
	}
}

// TestGit_Minimal_LongBranch verifies that a branch name that exceeds the
// minimal display width is middle-truncated with "…" and the visible result
// is at least 8 runes long.
//
// PLAN line 439: Minimal → middle-truncate branch "…", min 8 chars.
func TestGit_Minimal_LongBranch(t *testing.T) {
	p := &probes.GitProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        "agent/feature-dev/very-long-branch-name-that-overflows",
			ModifiedCount: 0,
		},
	}

	got := p.Render(d, cfg, th, probes.LevelMinimal)

	// Must contain the truncation marker.
	if !strings.Contains(got, "…") {
		t.Errorf("Render(Minimal, long branch): want truncation marker … in output, got %q", got)
	}
	// Visible length (rune count of the whole output) must be at least 8.
	runeLen := utf8.RuneCountInString(got)
	if runeLen < 8 {
		t.Errorf("Render(Minimal, long branch): want visible length >= 8 runes, got %d in %q",
			runeLen, got)
	}
}

// TestGit_NotInRepo verifies that GitProbe.Visible returns false when d.Git
// is nil — i.e., the parser determined we are not inside a git repository.
//
// PLAN line 440: d.Git=nil → Visible()=false. Render must not be called.
func TestGit_NotInRepo(t *testing.T) {
	p := &probes.GitProbe{}
	cfg := probes.Config{}
	d := probes.Data{Git: nil}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(Git=nil): want false, got true")
	}
}

// TestGit_NoModified verifies that when ModifiedCount is 0 the ⚠N suffix is
// completely absent from the Full render output.
//
// PLAN line 441: ModifiedCount=0 → no "⚠N" suffix in output.
func TestGit_NoModified(t *testing.T) {
	p := &probes.GitProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        "main",
			ModifiedCount: 0,
		},
	}

	got := p.Render(d, cfg, th, probes.LevelFull)
	if strings.Contains(got, "⚠") {
		t.Errorf("Render(Full, ModifiedCount=0): want no ⚠ suffix, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// T-27: git colours unchanged — regression anchor (Phase 6.9.c)
//
// Phase 6.9.c is a no-op for git.go (19=no-op). This test anchors that the
// colour semantics introduced in Phase 6.7 are preserved:
//   - branch segment wraps in {{color:cyan}}…{{reset}} when AnsiEnabled=true
//   - ⚠N warning wraps in {{color:yellow}}…{{reset}} when ModifiedCount > 0
// ---------------------------------------------------------------------------

// TestGit_ColoursUnchanged (T-27) is a regression anchor verifying that
// GitProbe.Render produces cyan-wrapped branch and yellow-wrapped ⚠N when
// AnsiEnabled=true. Phase 6.9.c must not alter git colour behaviour.
func TestGit_ColoursUnchanged(t *testing.T) {
	p := &probes.GitProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	tests := []struct {
		name          string
		branch        string
		modifiedCount int
		wantContains  []string
		wantAbsent    []string
	}{
		{
			// Branch with modifications: cyan branch + yellow warning.
			name:          "branch_with_modifications",
			branch:        "main",
			modifiedCount: 3,
			wantContains: []string{
				"{{color:cyan}}",   // branch must be cyan
				"{{color:yellow}}", // warning must be yellow
				"⚠3",               // warning glyph + count
				"main",             // branch name
			},
			wantAbsent: []string{
				"{{bold}}", // no bold on branch (not part of git colour contract)
			},
		},
		{
			// Branch without modifications: cyan branch only, no yellow warning.
			name:          "branch_no_modifications",
			branch:        "agent/test/phase-6-9",
			modifiedCount: 0,
			wantContains: []string{
				"{{color:cyan}}", // branch must be cyan
				"agent/test/phase-6-9",
			},
			wantAbsent: []string{
				"{{color:yellow}}", // no warning → no yellow marker
				"⚠",                // no warning glyph
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{
				Git: &parser.GitStatus{
					Branch:        tc.branch,
					ModifiedCount: tc.modifiedCount,
				},
			}
			cfg := probes.Config{GitEnabled: true}
			got := p.Render(d, cfg, th, probes.LevelFull)

			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("T-27: %s: want %q in output, got %q", tc.name, want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("T-27: %s: must NOT contain %q, got %q", tc.name, absent, got)
				}
			}
		})
	}
}
