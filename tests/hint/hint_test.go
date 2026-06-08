// Package hint_test verifies the Widget rotation and critical-alert override
// logic for the hint row in the cc-probeline status line.
//
// §4.4.b Hint widget + State — RED phase.
// All tests fail because internal/hint/hint.go is a stub (Pick always returns "").
package hint_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/parser"
)

// baseTime is the fixed clock used across all hint tests for determinism.
var baseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Rotation
// ---------------------------------------------------------------------------

// TestWidget_Pick_FirstCall_RotatesToFirst verifies that a fresh Widget with
// zero State returns DefaultHints[0].Text on the first Pick call, and that
// State.LastSwitch is stamped to now.
func TestWidget_Pick_FirstCall_RotatesToFirst(t *testing.T) {
	w := hint.Widget{}
	got := w.Pick(baseTime)
	want := hint.DefaultHints[0].Text
	if got != want {
		t.Errorf("Pick(firstCall) = %q; want %q", got, want)
	}
	if !w.State.LastSwitch.Equal(baseTime) {
		t.Errorf("State.LastSwitch = %v; want %v", w.State.LastSwitch, baseTime)
	}
}

// TestWidget_Pick_BeforeRotateInterval verifies that a second Pick call within
// the 60-second rotate interval returns the same text as the first.
func TestWidget_Pick_BeforeRotateInterval(t *testing.T) {
	w := hint.Widget{}
	first := w.Pick(baseTime)
	// Advance 30 seconds — still within the 60-second interval.
	second := w.Pick(baseTime.Add(30 * time.Second))
	if first != second {
		t.Errorf("Pick before interval changed: first=%q second=%q", first, second)
	}
}

// TestWidget_Pick_AfterRotateInterval verifies that a Pick call after 2m+1s
// returns the next hint (DefaultHints[1]).
func TestWidget_Pick_AfterRotateInterval(t *testing.T) {
	w := hint.Widget{}
	_ = w.Pick(baseTime)
	// Advance beyond the 60-second rotate interval.
	got := w.Pick(baseTime.Add(60*time.Second + 1*time.Second))
	want := hint.DefaultHints[1].Text
	if got != want {
		t.Errorf("Pick after interval = %q; want %q", got, want)
	}
}

// TestWidget_Pick_AllShown_ReturnsEmpty verifies that once all 8 hints are
// marked shown, Pick returns "" (hide row).
func TestWidget_Pick_AllShown_ReturnsEmpty(t *testing.T) {
	w := hint.Widget{
		State: hint.State{
			ShownIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
			CurrentIndex: 7,
		},
	}
	got := w.Pick(baseTime)
	if got != "" {
		t.Errorf("Pick(allShown) = %q; want empty string", got)
	}
}

// TestWidget_Pick_CriticalOverride verifies that a critical event returns the
// alert text even when all hints are shown (row would otherwise be hidden).
func TestWidget_Pick_CriticalOverride(t *testing.T) {
	w := hint.Widget{
		State: hint.State{
			ShownIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
			CurrentIndex: 7,
		},
		Events: []parser.CacheEvent{
			{Type: parser.OrchTTL},
		},
	}
	got := w.Pick(baseTime)
	want := "{{color:red}}⚠ Cache rebuilt · 60-min idle TTL passed{{reset}}"
	if got != want {
		t.Errorf("Pick(criticalOverride, allShown) = %q; want %q", got, want)
	}
}

// TestWidget_Pick_CriticalBeforeRotation verifies that when Events contains a
// ModelSwitched alert, Pick returns the alert text instead of the next hint
// in the rotation sequence.
func TestWidget_Pick_CriticalBeforeRotation(t *testing.T) {
	w := hint.Widget{
		State: hint.State{
			ShownIndices: []int{0},
			CurrentIndex: 0,
			LastSwitch:   baseTime,
		},
		Events: []parser.CacheEvent{
			{Type: parser.ModelSwitched, Detail: "opus → sonnet"},
		},
	}
	// Advance past rotate interval — without the critical override the widget
	// would return DefaultHints[1].Text.
	got := w.Pick(baseTime.Add(3 * time.Minute))
	want := "{{color:red}}⚠ Cache rebuilt · model switched (opus → sonnet){{reset}}"
	if got != want {
		t.Errorf("Pick(criticalBeforeRotation) = %q; want %q", got, want)
	}
}

// TestWidget_Pick_NoCustomHints_UsesDefaults verifies that Widget{Hints: nil}
// falls back to DefaultHints and returns the first default hint text.
func TestWidget_Pick_NoCustomHints_UsesDefaults(t *testing.T) {
	w := hint.Widget{Hints: nil}
	got := w.Pick(baseTime)
	want := hint.DefaultHints[0].Text
	if got != want {
		t.Errorf("Pick(nil hints) = %q; want %q", got, want)
	}
}

// TestWidget_Pick_CustomHints verifies that when Hints is provided explicitly,
// Pick uses those hints instead of DefaultHints.
func TestWidget_Pick_CustomHints(t *testing.T) {
	custom := []hint.Hint{{Index: 0, Text: "custom hint text"}}
	w := hint.Widget{Hints: custom}
	got := w.Pick(baseTime)
	if got != "custom hint text" {
		t.Errorf("Pick(customHints) = %q; want %q", got, "custom hint text")
	}
}
