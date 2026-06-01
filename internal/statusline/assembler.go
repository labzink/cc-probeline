// Package statusline composes the multi-line status output from the Phase 4.1
// probes Registry, the Phase 4.2 box-drawing table, and the Phase 4.4 hint
// widget. It lives above probes and renderer to avoid an import cycle:
// probes depends on renderer (Theme), and the assembler depends on both.
package statusline

import (
	"sort"
	"strings"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
)

// Assembler joins probes from the Registry into the final multi-line status
// string. Cols is the detected terminal width; Mode gates whether the per-turn
// table and footer are emitted.
type Assembler struct {
	Mode   mode.Mode
	Theme  renderer.Theme
	Cols   int
	Config probes.Config
}

// Render produces the full status string with marker tokens (no ANSI escapes).
// Caller is responsible for piping the result through renderer.Apply.
//
// Output structure:
//
//	line0  — probes from Line0Registry, separated by "{{dim}} • {{reset}}"
//	line1  — probes from Line1Registry, separated by "{{dim}} • {{reset}}"
//	line2  — probes from Line2Registry, separated by " | "
//	[table] — Standard mode only: last 20 per-turn rows + footer
//	[hint]  — appended when non-empty (C-12)
func (a *Assembler) Render(d probes.Data) string {
	// Resolve effective terminal width (§4.3 T-10).
	cols := a.Cols
	if cols == 0 {
		cols = renderer.DetectCols()
	}

	// Build the three header lines via FitLine (§4.3 T-11).
	const bulletSep = "{{dim}} • {{reset}}"
	const pipeSep = " | "

	// T-21: line0 uses registry order (no Priority sort) so email/project/quota
	// appear in registration order.
	// I2: line1 restores Priority-based sort so git (P=2) appears to the right of
	// ctx/cost/time (P=1). sortByPriority=false was an over-reach of the T-21 fix.
	// line2 retains ascending-Priority sort (lower number = higher importance = leftmost).
	l0 := renderer.FitLine(a.buildProbeEntries(probes.Line0Registry, d, false), cols, bulletSep)
	l1 := renderer.FitLine(a.buildProbeEntries(probes.Line1Registry, d, true), cols, bulletSep)
	l2 := renderer.FitLine(a.buildProbeEntries(probes.Line2Registry, d, true), cols, pipeSep)

	lines := []string{l0, l1, l2}

	// Standard mode: append per-turn table when there are rows (C-6).
	if a.Mode == mode.Standard {
		tableLines := a.perTurnTable(d, cols)
		lines = append(lines, tableLines...)
	}

	// Phase 4.4 hint widget (C-12): only append when non-empty.
	if h := a.hint(d); h != "" {
		lines = append(lines, h)
	}

	return strings.Join(lines, "\n")
}

// buildProbeEntries converts a probe registry slice into []renderer.ProbeEntry
// for consumption by FitLine. Invisible probes are excluded (§4.3 T-11).
//
// When sortByPriority is false (line0, line1), registry order is preserved:
// probes appear left-to-right in registration order; Priority is used only by
// FitLine/levelForPass for collapse ordering (T-21).
//
// When sortByPriority is true (line2), entries are sorted ascending by Priority
// so lower-number (higher-importance) probes appear leftmost (Phase 4.2 §C-9).
func (a *Assembler) buildProbeEntries(ps []probes.Probe, d probes.Data, sortByPriority bool) []renderer.ProbeEntry {
	entries := make([]renderer.ProbeEntry, 0, len(ps))
	for _, p := range ps {
		if !p.Visible(d, a.Config) {
			continue
		}
		pp := p // capture loop variable
		entries = append(entries, renderer.ProbeEntry{
			Priority: pp.Priority(),
			Render: func(level int) string {
				return pp.Render(d, a.Config, a.Theme, probes.Level(level))
			},
		})
	}
	if sortByPriority {
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Priority < entries[j].Priority
		})
	}
	return entries
}

// perTurnTable builds the redesigned per-turn table for Standard mode (C1 / T-15..T-20).
//
// It merges orchestrator turns (d.Session.Turns) and sidechain turns from
// d.Session.Turns (IsSidechain=true) into a single stream sorted by Timestamp DESC
// (newest-first), then calls renderer.RenderUnified. Cap: last 20 turns (C-6).
//
// Returns nil when there are no turns.
func (a *Assembler) perTurnTable(d probes.Data, cols int) []string {
	if d.Session == nil || len(d.Session.Turns) == 0 {
		return nil
	}

	// Collect all turns: orchestrator + any sidechain turns already embedded in
	// d.Session.Turns (IsSidechain=true). Also fold in SubagentStats.Turns when
	// SubagentStats carries per-turn data (T-15 interleave).
	all := make([]parser.Turn, 0, len(d.Session.Turns))
	all = append(all, d.Session.Turns...)

	// Append per-turn entries from SubagentStats (IsSidechain=true).
	for _, sa := range d.Subagents {
		for _, t := range sa.Turns {
			t.IsSidechain = true
			all = append(all, t)
		}
	}

	// Sort by Timestamp DESC (newest first) for RenderUnified.
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})

	// C-6: cap to last 20 turns (newest-first: keep the first 20 after sort).
	if len(all) > 20 {
		all = all[:20]
	}

	b := renderer.NewBuilder(cols)
	// C1: use RenderUnified with the reconciled state for per-turn cost column.
	// d.State is nil when state not yet loaded; cost.PerTurn handles nil gracefully.
	var st *state.Session
	if d.State != nil {
		st = d.State
	}
	raw := b.RenderUnified(all, st)
	if raw == "" {
		return nil
	}

	// RenderUnified appends a trailing '\n'; split and drop the final empty element.
	parts := strings.Split(raw, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// hint returns the hint widget text for this session's rotation state.
// Returns "" when all hints have been shown and no critical alert is active
// (caller skips adding the hint row in that case, per C-12).
func (a *Assembler) hint(d probes.Data) string {
	state := hint.Load(d.SessionID)

	// D1 guard: skip session-derived alerts when no turns exist (§11).
	// A newly opened session has no turns; surfacing cache alerts at turn-zero
	// is noise because the cache has not been used yet.
	var cacheEvents []parser.CacheEvent
	if d.Session != nil && len(d.Session.Turns) > 0 {
		cacheEvents = d.Session.CacheEvents
	}
	// Merge subagent-scope events (T-9 SendMessageGap, T-10 SlowInternal).
	// SubagentStats live outside SessionStats so Aggregate cannot include them.
	if len(d.Subagents) > 0 {
		cacheEvents = append(cacheEvents,
			parser.DetectSubagentCacheEvents(d.Subagents, d.Now)...)
	}
	// Phase 6: config-error alerts ALWAYS surface, even on an empty session.
	// ExtraCacheEvents are not session-derived and are not gated by D1.
	if len(d.ExtraCacheEvents) > 0 {
		cacheEvents = append(cacheEvents, d.ExtraCacheEvents...)
	}
	w := hint.Widget{
		State:  state,
		Events: cacheEvents,
	}
	// Use d.Now for deterministic clock (hypothesis insurance #3).
	// Fall back to time.Now() so the CLI entrypoint (Phase 5) is safe if
	// it forgets to populate d.Now.
	now := d.Now
	if now.IsZero() {
		now = time.Now()
	}
	text := w.Pick(now)
	// Persist updated state (rotation advanced). Ignore error: on
	// permission-denied or disk-full the widget degrades to memory-only mode.
	if d.SessionID != "" {
		_ = hint.Save(d.SessionID, w.State)
	}
	return text
}

// Compile-time check: Assembler.Render must accept probes.Data.
// (Ensures refactors don't accidentally change the signature.)
var _ func(probes.Data) string = (&Assembler{}).Render
