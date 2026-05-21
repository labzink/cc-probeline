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
)

// Assembler joins probes from the Registry into the final multi-line status
// string. Cols is the detected terminal width; Mode gates whether the per-turn
// table and footer are emitted. Phase 4.2.d fills in Render; this is a
// foundation stub.
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
//	[hint]  — Phase 4.4 stub; empty for now
func (a *Assembler) Render(d probes.Data) string {
	// Resolve effective terminal width (§4.3 T-10).
	cols := a.Cols
	if cols == 0 {
		cols = renderer.DetectCols()
	}

	// Build the three header lines via FitLine (§4.3 T-11).
	const bulletSep = "{{dim}} • {{reset}}"
	const pipeSep = " | "

	l0 := renderer.FitLine(a.buildProbeEntries(probes.Line0Registry, d), cols, bulletSep)
	l1 := renderer.FitLine(a.buildProbeEntries(probes.Line1Registry, d), cols, bulletSep)
	l2 := renderer.FitLine(a.buildProbeEntries(probes.Line2Registry, d), cols, pipeSep)

	lines := []string{l0, l1, l2}

	// Standard mode: append per-turn table when there are rows (C-6).
	if a.Mode == mode.Standard {
		tableLines := a.perTurnTable(d, cols)
		lines = append(lines, tableLines...)
	}

	// Phase 4.4 hint widget stub (C-12): only append when non-empty.
	if h := a.hint(d); h != "" {
		lines = append(lines, h)
	}

	return strings.Join(lines, "\n")
}

// buildProbeEntries converts a probe registry slice into []renderer.ProbeEntry
// for consumption by FitLine. Invisible probes are excluded (§4.3 T-11).
func (a *Assembler) buildProbeEntries(ps []probes.Probe, d probes.Data) []renderer.ProbeEntry {
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
	// Sort by priority ascending so FitLine sees probes in display order.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Priority < entries[j].Priority
	})
	return entries
}

// perTurnTable builds the box-drawing table for Standard mode.
// It caps at the last 20 turns (C-6), applies column-drop truncation via
// Builder.RenderForCols(cols), and returns a slice of non-empty lines.
// Returns nil when there are no turns (table block is skipped).
func (a *Assembler) perTurnTable(d probes.Data, cols int) []string {
	if d.Session == nil || len(d.Session.Turns) == 0 {
		return nil
	}

	turns := d.Session.Turns
	// C-6: cap to last 20 turns.
	if len(turns) > 20 {
		turns = turns[len(turns)-20:]
	}

	b := renderer.NewBuilder(cols)
	for _, t := range turns {
		b.Add(t)
	}

	// Use RenderForCols for column-drop truncation (§4.3 T-9).
	raw := b.RenderForCols(cols)
	if raw == "" {
		return nil
	}

	// RenderForCols appends a trailing '\n'; split and drop the final empty element.
	all := strings.Split(raw, "\n")
	if len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	return all
}

// hint returns the hint widget text for this session's rotation state.
// Returns "" when all hints have been shown and no critical alert is active
// (caller skips adding the hint row in that case, per C-12).
func (a *Assembler) hint(d probes.Data) string {
	state := hint.Load(d.SessionID)

	// Build a Widget using the real CacheEvents from the session.
	var cacheEvents []parser.CacheEvent
	if d.Session != nil {
		cacheEvents = d.Session.CacheEvents
	}
	// Merge subagent-scope events (T-9 SendMessageGap, T-10 SlowInternal).
	// SubagentStats live outside SessionStats so Aggregate cannot include them.
	if len(d.Subagents) > 0 {
		cacheEvents = append(cacheEvents,
			parser.DetectSubagentCacheEvents(d.Subagents, d.Now)...)
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
