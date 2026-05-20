// Package statusline composes the multi-line status output from the Phase 4.1
// probes Registry, the Phase 4.2 box-drawing table, and the Phase 4.4 hint
// widget. It lives above probes and renderer to avoid an import cycle:
// probes depends on renderer (Theme), and the assembler depends on both.
package statusline

import (
	"sort"
	"strings"

	"github.com/labzink/cc-probeline/internal/mode"
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

// renderLine iterates a probe registry, filters visible probes, sorts by
// Priority ascending, renders each, and joins with sep. Level is passed
// through to each probe's Render call (C-11).
func (a *Assembler) renderLine(ps []probes.Probe, sep string, level probes.Level, d probes.Data) string {
	// Collect visible probes preserving original slice indices for stable sort.
	type entry struct {
		priority int
		out      string
	}

	var entries []entry
	for _, p := range ps {
		if !p.Visible(d, a.Config) {
			continue
		}
		entries = append(entries, entry{
			priority: p.Priority(),
			out:      p.Render(d, a.Config, a.Theme, level),
		})
	}

	// Sort by priority ascending (lower number = higher importance = leftmost).
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})

	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, e.out)
	}
	return strings.Join(parts, sep)
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

	b := renderer.NewBuilder(a.Cols)
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

// hint returns an optional hint string appended after the table.
// TODO: Phase 4.4 hint widget — currently always returns "".
func (a *Assembler) hint(_ probes.Data) string { return "" }

// Compile-time check: Assembler.Render must accept probes.Data.
// (Ensures refactors don't accidentally change the signature.)
var _ func(probes.Data) string = (&Assembler{}).Render
