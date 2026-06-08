// Package hint_test — tests for the account-wide rotating start offset
// (Phase 6.95): a session opens on Widget.StartIndex instead of a hardcoded 0,
// and the rotation wraps so it ends on the hint just before the start.
package hint_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
)

// collectCycle drives Pick across rotate intervals starting from a fresh widget
// with the given StartIndex, returning the ordered sequence of shown indices
// (decoded back from text) until the row hides ("").
func collectCycle(t *testing.T, start int) []int {
	t.Helper()
	w := hint.Widget{StartIndex: start}
	now := baseTime
	var seq []int
	for i := 0; i < len(hint.DefaultHints)+2; i++ {
		got := w.Pick(now)
		if got == "" {
			break
		}
		idx := indexOfHint(t, got)
		seq = append(seq, idx)
		now = now.Add(61 * time.Second) // beyond the 60s rotate interval
	}
	return seq
}

// indexOfHint maps a rendered hint text back to its DefaultHints index.
func indexOfHint(t *testing.T, text string) int {
	t.Helper()
	for _, h := range hint.DefaultHints {
		if h.Text == text {
			return h.Index
		}
	}
	t.Fatalf("text %q does not match any DefaultHints entry", text)
	return -1
}

// TestWidget_StartOffset_FirstCall verifies the opening hint honours StartIndex.
func TestWidget_StartOffset_FirstCall(t *testing.T) {
	for _, start := range []int{0, 1, 3, 5} {
		w := hint.Widget{StartIndex: start}
		got := w.Pick(baseTime)
		want := hint.DefaultHints[start].Text
		if got != want {
			t.Errorf("StartIndex=%d: first Pick = %q; want %q", start, got, want)
		}
	}
}

// TestWidget_StartOffset_WrapsAndEndsBeforeStart verifies the full cycle:
// starting at index N the rotation visits all hints once, wrapping around, and
// the last visible hint is N-1 (mod total) — then the row hides.
func TestWidget_StartOffset_WrapsAndEndsBeforeStart(t *testing.T) {
	total := len(hint.DefaultHints)
	for _, start := range []int{0, 1, 2, 5} {
		seq := collectCycle(t, start)

		// Every hint shown exactly once, in wrapping order from start.
		if len(seq) != total {
			t.Fatalf("start=%d: cycle length %d; want %d (seq=%v)", start, len(seq), total, seq)
		}
		for i := 0; i < total; i++ {
			want := (start + i) % total
			if seq[i] != want {
				t.Errorf("start=%d: seq[%d]=%d; want %d (full seq=%v)", start, i, seq[i], want, seq)
			}
		}
		// Ends on start-1 (mod total).
		wantLast := ((start-1)%total + total) % total
		if seq[len(seq)-1] != wantLast {
			t.Errorf("start=%d: ends on %d; want %d (start-1)", start, seq[len(seq)-1], wantLast)
		}
	}
}

// TestWidget_StartOffset_OutOfRangeWraps verifies StartIndex beyond the hint
// count is taken modulo (defensive — main passes a moduloed value already).
func TestWidget_StartOffset_OutOfRangeWraps(t *testing.T) {
	total := len(hint.DefaultHints)
	w := hint.Widget{StartIndex: total + 2} // wraps to index 2
	got := w.Pick(baseTime)
	want := hint.DefaultHints[2].Text
	if got != want {
		t.Errorf("StartIndex=%d (total=%d): first Pick = %q; want index-2 %q", total+2, total, got, want)
	}
}
