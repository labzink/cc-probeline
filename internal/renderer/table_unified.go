package renderer

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// thinkingGlyph is the symbol shown in the tool column when Turn.Thinking=true.
// The exact glyph is not fixed by spec; tests verify non-empty and not a tool name.
const thinkingGlyph = "💭"

// RenderUnified renders the redesigned per-turn table (Phase 6.8.d).
//
// Layout rules:
//   - Turns are expected pre-sorted by Timestamp DESC (newest first).
//   - A group-separator line (├─┼─┤) is inserted before the first row of each
//     new GroupID encountered scanning top-to-bottom. No separator before the
//     very first data row (it would produce an unwanted top-of-data separator).
//   - Rows belonging to the current group (max GroupID) are plain.
//     Rows from older groups are wrapped in {{dim}}…{{reset}}.
//   - Footer: legend row ("# role model cache out cost tool") as a content line
//     (│-bordered) immediately before the bottom border. No totals row.
//   - tool column: thinkingGlyph when Turn.Thinking; ToolUse name otherwise.
//   - cost column: "—" for sidechain turns; cost.PerTurn value for orchestrator.
func (b *Builder) RenderUnified(turns []parser.Turn, st *state.Session) string {
	slog.Debug("renderer.RenderUnified start", "turns", len(turns))
	if len(turns) == 0 {
		return ""
	}

	// Determine current group = max GroupID across all turns.
	maxGroupID := 0
	for _, t := range turns {
		if t.GroupID > maxGroupID {
			maxGroupID = t.GroupID
		}
	}

	cols := b.effectiveCols()
	colWidths := cols[:]

	// Build border / separator lines.
	topBorder := hlineSlice(colWidths, '┌', '┬', '┐', '─', nil)
	bottomBorder := hlineSlice(colWidths, '└', '┴', '┘', '─', nil)
	// Group separator: ├─┼─┤ (same fill as top/bottom but with join chars ├/┼/┤).
	groupSep := hlineSlice(colWidths, '├', '┼', '┤', '─', nil)

	var sb strings.Builder
	sb.WriteString(topBorder)
	sb.WriteByte('\n')

	// Scan turns top-to-bottom (already newest-first). Track previous GroupID to
	// detect group boundaries. prevGroupID is initialised to the first turn's
	// GroupID so the very first data row does NOT emit a preceding separator.
	prevGroupID := turns[0].GroupID
	for _, t := range turns {
		// Insert group-separator line before the first row of a new group.
		if t.GroupID != prevGroupID {
			sb.WriteString(groupSep)
			sb.WriteByte('\n')
			prevGroupID = t.GroupID
		}

		row := b.buildUnifiedRow(t, st)
		isHistory := t.GroupID < maxGroupID
		var line string
		if isHistory {
			// History rows use plain │ dividers; the whole line is then wrapped in
			// {{dim}}…{{reset}} so the entire row (borders included) renders dim.
			line = "{{dim}}" + renderRowNPlainBar(row, colWidths) + "{{reset}}"
		} else {
			// Current-group rows: plain │ dividers, no dim wrapper. The test
			// asserts the current row must NOT contain {{dim}}.
			line = renderRowNPlainBar(row, colWidths)
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	// Legend row: column header labels placed before the bottom border.
	sb.WriteString(renderUnifiedLegend(colWidths))
	sb.WriteByte('\n')
	sb.WriteString(bottomBorder)
	sb.WriteByte('\n')

	result := sb.String()
	slog.Debug("renderer.RenderUnified complete", "lines", strings.Count(result, "\n"))
	return result
}

// renderRowNPlainBar renders one content row using plain │ as cell dividers
// (no dim markers). Used for rows in RenderUnified where the caller controls
// the dim wrapper on the whole line.
func renderRowNPlainBar(row Row, colWidths []int) string {
	origIndices := []int{0, 1, 2, 3, 4, 5, 6}
	var sb strings.Builder
	sb.WriteRune('│')
	for i, origIdx := range origIndices {
		cell := row[origIdx]
		sb.WriteString(padCell(cell.Content, colWidths[i], cell.Align))
		sb.WriteRune('│')
	}
	return sb.String()
}

// unifiedRoleColour wraps a role label with colour markers based on IsSidechain:
//   - IsSidechain=false (orchestrator) → cyan
//   - IsSidechain=true  (sidechain agent) → yellow
//
// This supersedes the legacy roleColour("orch") string-match approach, which
// failed because Turn.Role is "orchestrator" (not "orch") in production data.
func unifiedRoleColour(role string, isSidechain bool) string {
	if !isSidechain {
		return "{{color:cyan}}" + role + "{{reset}}"
	}
	return "{{color:yellow}}" + role + "{{reset}}"
}

// buildUnifiedRow constructs a Row for a single Turn using the redesigned layout.
//
// Column mapping: # / role / model / cache / out / cost / tool
//   - cost: "—" for sidechain turns; cost.PerTurn formatted (or "—" if !ok) otherwise
//   - tool: thinkingGlyph when Turn.Thinking; ToolUse name otherwise
func (b *Builder) buildUnifiedRow(t parser.Turn, st *state.Session) Row {
	model := t.Model
	if strings.HasPrefix(model, "claude-") {
		model = model[len("claude-"):]
	}
	cache := fmt.Sprintf("%s/%s",
		format.FormatK(t.Tokens.CacheRead),
		format.FormatK(t.Tokens.CacheCreate),
	)

	// Cost cell: "—" for sidechain; cost.PerTurn for orchestrator.
	costCell := "—"
	if !t.IsSidechain {
		if v, ok := cost.PerTurn(st, t.UUID); ok {
			costCell = cost.Format(v)
		}
	}

	// Tool cell: thinking glyph takes priority over ToolUse name.
	toolCell := t.ToolUse
	if t.Thinking {
		toolCell = thinkingGlyph
	}

	idxStr := ""
	if t.Index > 0 {
		idxStr = fmt.Sprintf("%d", t.Index)
	}

	return Row{
		{Content: idxStr, Align: AlignRight},
		{Content: unifiedRoleColour(t.Role, t.IsSidechain), Align: AlignLeft},
		{Content: model, Align: AlignLeft},
		{Content: cache, Align: AlignLeft},
		{Content: format.FormatK(t.Tokens.Output), Align: AlignRight},
		{Content: costCell, Align: AlignRight},
		{Content: toolCell, Align: AlignLeft},
	}
}

// renderUnifiedLegend renders the legend content row with column header labels.
// Uses the standard dimBar ({{dim}}│{{reset}}) separators, identical to data rows.
func renderUnifiedLegend(colWidths []int) string {
	labels := [7]string{"#", "role", "model", "cache", "out", "cost", "tool"}
	aligns := [7]Align{AlignRight, AlignLeft, AlignLeft, AlignLeft, AlignRight, AlignRight, AlignLeft}

	var sb strings.Builder
	sb.WriteString(dimBar)
	for i, lbl := range labels {
		sb.WriteString(padCell(lbl, colWidths[i], aligns[i]))
		sb.WriteString(dimBar)
	}
	return sb.String()
}
