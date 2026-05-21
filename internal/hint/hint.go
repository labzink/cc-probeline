// Package hint provides the rotating hint widget appended at the bottom of
// the status output. Each session shows up to 8 default hints (brainstorm
// Batch 3) on 120-second rotation; critical cache invalidation events
// override the rotation. State is persisted per-session under XDG_CACHE_HOME.
package hint

import (
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// Hint is one rotation slot. Index is used as the persistent key in State.
type Hint struct {
	Index int
	Text  string
}

// DefaultHints holds the 8 fixed brainstorm Batch 3 hint strings.
var DefaultHints = []Hint{
	{0, "⚠ cache miss/alert · [high] reasoning effort"},
	{1, "⎇ git branch (worktree) · ⚠N modified files in working tree"},
	{2, "ctx N/M — context window load (cached / max context)"},
	{3, "⏱ N — cache lives N after last request (orch ~60m, agent ~5m)"},
	{4, "↻ N — time until rate limits reset (5h window / 7d window)"},
	{5, "≡ cache tokens (read/create) · ↗ out (output tokens)"},
	{6, "/cp-mode toggles view: super-compact ↔ standard (cap 20 turns)"},
	{7, "cc-probeline diagnostics — raw stdin dump for debugging/bug reports"},
}

// rotateInterval is the minimum time between hint rotations.
const rotateInterval = 120 * time.Second

// Widget composes a per-session rotating hint with cache-event alert override.
// Hints is optional; when nil, DefaultHints is used.
type Widget struct {
	Hints  []Hint
	State  State
	Events []parser.CacheEvent
}

// hints returns the active hint slice, falling back to DefaultHints.
func (w *Widget) hints() []Hint {
	if w.Hints == nil {
		return DefaultHints
	}
	return w.Hints
}

// Pick returns the text to render now: critical alert > rotation > "" (hide).
//
// Algorithm:
//  1. If any critical alert event: return alert text (even if all hints shown).
//  2. If all hints shown: return "" (hide row).
//  3. If first call (LastSwitch zero): stamp index 0 as shown, record LastSwitch.
//  4. If rotate interval elapsed: call Advance to move to next unseen index.
//  5. Return hs[CurrentIndex].Text.
func (w *Widget) Pick(now time.Time) string {
	// Critical alert overrides rotation and hide.
	if alert := BuildAlert(w.Events); alert != "" {
		return alert
	}

	hs := w.hints()

	// Hide row once all hints have been shown.
	if w.State.AllShown(len(hs)) {
		return ""
	}

	// First call: initialize without rotating — show index 0.
	if w.State.LastSwitch.IsZero() {
		w.State.ShownIndices = append(w.State.ShownIndices, 0)
		w.State.CurrentIndex = 0
		w.State.LastSwitch = now
		return hs[0].Text
	}

	// After rotate interval: advance to the next unseen hint.
	if now.Sub(w.State.LastSwitch) >= rotateInterval {
		w.State.Advance(len(hs), now)
	}

	return hs[w.State.CurrentIndex].Text
}
