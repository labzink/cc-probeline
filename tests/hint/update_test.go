// Package hint_test — Phase 7.46 Wave B / BL-36 update-hint (#7c) tests.
package hint_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/hint"
)

// TestUpdateText covers the version-comparison contract: a hint appears ONLY when
// latest is strictly higher and both versions parse as numbers.
func TestUpdateText(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		wantHint        bool
	}{
		{"newer patch", "0.1.0", "0.1.1", true},
		{"newer minor", "0.1.9", "0.2.0", true},
		{"newer major", "0.9.9", "1.0.0", true},
		{"equal", "0.1.1", "0.1.1", false},
		{"older latest", "0.2.0", "0.1.9", false},
		{"v prefixes", "v0.1.0", "v0.1.1", true},
		{"prerelease suffix on current", "0.1.0-rc1", "0.1.0", false}, // 0.1.0 == 0.1.0
		{"prerelease suffix newer", "0.1.0-rc1", "0.1.1", true},
		{"dev current never nags", "dev", "0.1.1", false},
		{"garbage latest ignored", "0.1.0", "not-a-version", false},
		{"empty latest", "0.1.0", "", false},
		{"two-component versions", "0.1", "0.2", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hint.UpdateText(c.current, c.latest)
			if c.wantHint && got == "" {
				t.Errorf("UpdateText(%q,%q) = \"\"; want a hint", c.current, c.latest)
			}
			if !c.wantHint && got != "" {
				t.Errorf("UpdateText(%q,%q) = %q; want \"\"", c.current, c.latest, got)
			}
			if c.wantHint && !strings.Contains(got, "/cc-probeline-update") {
				t.Errorf("UpdateText(%q,%q) = %q; missing the /cc-probeline-update call to action", c.current, c.latest, got)
			}
		})
	}
}

// TestUpdateText_ShowsBothVersions confirms the hint names the running and the
// available version so the user sees the jump.
func TestUpdateText_ShowsBothVersions(t *testing.T) {
	got := hint.UpdateText("v0.1.0", "0.1.1")
	if !strings.Contains(got, "0.1.0") || !strings.Contains(got, "0.1.1") {
		t.Errorf("UpdateText = %q; want both 0.1.0 and 0.1.1", got)
	}
}

// TestWidget_Pick_UpdateSticky verifies that once every tutorial tip plus the
// appended update slot has been shown, the row keeps surfacing the update hint
// instead of hiding (the sticky #7c behaviour).
func TestWidget_Pick_UpdateSticky(t *testing.T) {
	update := hint.UpdateText("0.1.0", "0.1.1")
	if update == "" {
		t.Fatal("precondition: UpdateText returned empty")
	}
	total := len(hint.DefaultHints) + 1 // tips + the appended update slot
	shown := make([]int, total)
	for i := range shown {
		shown[i] = i
	}
	w := hint.Widget{
		State:      hint.State{ShownIndices: shown, CurrentIndex: total - 1},
		UpdateHint: update,
	}
	if got := w.Pick(baseTime); got != update {
		t.Errorf("Pick(allShown, update set) = %q; want sticky update %q", got, update)
	}
}

// TestWidget_Pick_NoUpdateStillHides confirms the sticky path does not change the
// default hide-when-all-shown behaviour when there is no update.
func TestWidget_Pick_NoUpdateStillHides(t *testing.T) {
	total := len(hint.DefaultHints)
	shown := make([]int, total)
	for i := range shown {
		shown[i] = i
	}
	w := hint.Widget{State: hint.State{ShownIndices: shown, CurrentIndex: total - 1}}
	if got := w.Pick(baseTime); got != "" {
		t.Errorf("Pick(allShown, no update) = %q; want \"\"", got)
	}
}
