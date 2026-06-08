// Package hint provides the rotating hint widget appended at the bottom of
// the status output. Each session shows the default hints (Phase 6.95.c) on
// 60-second rotation; critical cache invalidation events override the
// rotation. Rotation state is persisted per-session in state.Session.HintRotation;
// the starting hint advances account-wide each session via quota.HintStart.
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

// hintMarker is prepended to every default hint so the row reads
// unambiguously as a tutorial tip (a plain ℹ, no ANSI markers, so it renders
// identically with colour on or off). Custom hints and cache-event alerts do
// not get the marker.
const hintMarker = "ℹ "

// dh builds a default hint, prepending hintMarker to its text.
func dh(index int, text string) Hint {
	return Hint{Index: index, Text: hintMarker + text}
}

// DefaultHints holds the rotating hint strings (Phase 6.95.c, draft §2A).
// Each is prefixed with hintMarker (ℹ) to mark the row as a tip.
// Colour = feature colour (visual link): #0 reasoning magenta, #1 split
// cyan+yellow mirroring the git segment, #3 cache green, #5 settings dim;
// #2/#4 stay plain. Markers are {{color:X}}/{{dim}} text tokens converted by
// renderer.Apply (stripped when colour is off), never raw ANSI.
var DefaultHints = []Hint{
	dh(0, "{{color:magenta}}Reasoning: ○ low · ◔ medium · ◑ high · ◕ xhigh · ● max{{reset}}"),
	dh(1, "{{color:cyan}}⎇ git branch (worktree){{reset}} · {{color:yellow}}⚠ N modified files{{reset}}"),
	dh(2, "ctx N/M — context load (used / max window)"),
	dh(3, "{{color:green}}⏱ N — cache lives N after last request (orch ~60m · agent ~5m){{reset}}"),
	dh(4, "↻ N — time until rate-limit reset (5h · 7d windows)"),
	dh(5, "{{dim}}⚙ /cc-probeline-config — customise your line: probes · table size · colours & more{{reset}}"),
}

// rotateInterval is the minimum time between hint rotations.
const rotateInterval = 60 * time.Second

// Widget composes a per-session rotating hint with cache-event alert override.
// Hints is optional; when nil, DefaultHints is used.
type Widget struct {
	Hints []Hint
	State State
	// StartIndex is the hint index this session opens on (account-wide rotating
	// offset, see quota.HintStart). Used only on the first call (zero LastSwitch);
	// thereafter the per-session State drives the rotation. Out-of-range values
	// are wrapped modulo the hint count.
	StartIndex int
	Events     []parser.CacheEvent
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
//  3. If first call (LastSwitch zero): stamp StartIndex as shown, record LastSwitch.
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

	// First call: initialize without rotating — show the rotating start index.
	if w.State.LastSwitch.IsZero() {
		start := ((w.StartIndex % len(hs)) + len(hs)) % len(hs)
		w.State.ShownIndices = append(w.State.ShownIndices, start)
		w.State.CurrentIndex = start
		w.State.LastSwitch = now
		return hs[start].Text
	}

	// After rotate interval: advance to the next unseen hint.
	if now.Sub(w.State.LastSwitch) >= rotateInterval {
		w.State.Advance(len(hs), now)
	}

	return hs[w.State.CurrentIndex].Text
}
