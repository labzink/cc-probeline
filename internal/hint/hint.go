// Package hint provides the rotating hint widget appended at the bottom of
// the status output. Each session shows the default hints (Phase 6.95.c) on
// 120-second rotation; critical cache invalidation events override the
// rotation. Rotation state is persisted per-session in state.Session.HintRotation.
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

// DefaultHints holds the rotating hint strings (Phase 6.95.c, draft §2A).
// Colour = feature colour (visual link): #0 reasoning magenta, #1 split
// cyan+yellow mirroring the git segment, #3 cache green, #5 settings dim;
// #2/#4 stay plain. Markers are {{color:X}}/{{dim}} text tokens converted by
// renderer.Apply (stripped when colour is off), never raw ANSI.
var DefaultHints = []Hint{
	{0, "{{color:magenta}}Reasoning: ○ low · ◔ medium · ◑ high · ◕ xhigh · ● max{{reset}}"},
	{1, "{{color:cyan}}⎇ git branch (worktree){{reset}} · {{color:yellow}}⚠ N modified files{{reset}}"},
	{2, "ctx N/M — context load (used / max window)"},
	{3, "{{color:green}}⏱ N — cache lives N after last request (orch ~60m · agent ~5m){{reset}}"},
	{4, "↻ N — time until rate-limit reset (5h · 7d windows)"},
	{5, "{{dim}}⚙ /cp-config — customise your line: probes · table size · colours & more{{reset}}"},
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
