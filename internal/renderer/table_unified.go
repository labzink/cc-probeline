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

// ThinkingGlyph is the text shown in the tool column when a turn is thinking
// (Turn.Thinking) or has produced no tool_use yet (empty ToolUse). Phase 6.9.e
// replaced the old emoji glyph with the literal word (T-7 / T-8).
const ThinkingGlyph = "thinking..."

// UnifiedRow is a fully-prepared per-turn table row carrying every piece of
// presentation state the renderer needs. The assembler builds these (collapsing
// each subagent into a single row, joining instance names, computing cumulative
// cost) so RenderUnifiedRows stays a pure layout pass.
type UnifiedRow struct {
	// HashCell is the "#" column content: a turn number for orchestrator rows
	// ("1", "2", …) or "↳N" for subagent rows.
	HashCell string
	// Role is the un-coloured role label (orchestrator role or AgentType).
	Role string
	// Model is the canonical model name (the "claude-" prefix is trimmed here).
	Model string
	// CacheRead / CacheCreate are token counts for the "cache r/w" column.
	CacheRead, CacheCreate int
	// Out is the output-token count.
	Out int
	// CostCell is the pre-formatted cost cell ("$X.XX" for orch, "Σ $X.XX" for
	// subagent).
	CostCell string
	// Tool is the tool-column content (may already include an instance-name
	// prefix "<name>: <tool>" for subagent rows at wide terminals).
	Tool string

	// IsSidechain marks subagent rows (yellow role).
	IsSidechain bool
	// Dim wraps the whole row (and its borders) in {{dim}} (older orch groups
	// and subagents anchored outside the freshest group).
	Dim bool
	// GroupID is the orchestrator group used to detect separators. Subagent rows
	// carry 0 and never trigger a separator (handled by SkipSeparator).
	GroupID int
	// SkipSeparator suppresses group-boundary detection for this row (subagents).
	SkipSeparator bool
	// RedCacheWrite paints the cache_create (write) sub-token in {{color:red}}
	// (cache collapse / model switch — T-34/T-35/T-36). read is untouched.
	RedCacheWrite bool
	// TTLSuffix, when non-empty, is appended after the right border of this row
	// (live TTL on the fresh orch row, per-agent TTL on subagent rows, frozen
	// "⏱ 0m" on expired older orch rows). Already colour-marked.
	TTLSuffix string
}

// RenderUnifiedRows renders the redesigned per-turn table from pre-built rows.
//
// Layout rules:
//   - Rows are pre-sorted newest-first by the caller.
//   - A group separator (├─┼─┤) is inserted before the first row of each new
//     orchestrator GroupID scanning top-to-bottom. Rows with SkipSeparator
//     (subagents) do not participate in boundary detection.
//   - Dim rows are wrapped in {{dim}}…{{reset}} (whole line, borders included).
//   - The legend row ("# role model cache r/w out cost tool") is preceded by a
//     ├─┼─┤ separator and sits immediately before the bottom border.
//   - TTLSuffix is appended after the right border of each row that carries one.
func (b *Builder) RenderUnifiedRows(rows []UnifiedRow) string {
	slog.Debug("renderer.RenderUnifiedRows start", "rows", len(rows))
	if len(rows) == 0 {
		return ""
	}

	cols := b.effectiveCols()
	colWidths := cols[:]

	topBorder := hlineSlice(colWidths, '┌', '┬', '┐', '─', nil)
	bottomBorder := hlineSlice(colWidths, '└', '┴', '┘', '─', nil)
	groupSep := hlineSlice(colWidths, '├', '┼', '┤', '─', nil)
	// The legend separator (T-5) starts ├ and uses ┼ junctions like a group
	// separator, but its right corner is ┐ so it is distinguishable from a
	// group-boundary separator (which ends ┤). This lets callers count data
	// group boundaries without the legend separator inflating the total (T-3).
	legendSep := hlineSlice(colWidths, '├', '┼', '┐', '─', nil)

	var sb strings.Builder
	sb.WriteString(topBorder)
	sb.WriteByte('\n')

	// Track the previous orchestrator GroupID to detect boundaries. Initialise
	// to the first non-skipped row's group so the very first data row never
	// emits a preceding separator.
	prevGroupID := 0
	havePrev := false
	for _, r := range rows {
		if !r.SkipSeparator {
			if havePrev && r.GroupID != prevGroupID {
				sb.WriteString(groupSep)
				sb.WriteByte('\n')
			}
			prevGroupID = r.GroupID
			havePrev = true
		}

		line := b.renderUnifiedDataRow(r, colWidths)
		sb.WriteString(line)
		if r.TTLSuffix != "" {
			sb.WriteByte(' ')
			sb.WriteString(r.TTLSuffix)
		}
		sb.WriteByte('\n')
	}

	// Legend: ├─┼─┐ separator immediately before the legend row (T-5).
	sb.WriteString(legendSep)
	sb.WriteByte('\n')
	sb.WriteString(renderUnifiedLegend(colWidths))
	sb.WriteByte('\n')
	sb.WriteString(bottomBorder)
	sb.WriteByte('\n')

	result := sb.String()
	slog.Debug("renderer.RenderUnifiedRows complete", "lines", strings.Count(result, "\n"))
	return result
}

// renderUnifiedDataRow renders one UnifiedRow as a │-bordered content line.
// Current-group rows use {{dim}}│{{reset}} dividers (T-6); dim rows are wrapped
// whole in {{dim}}…{{reset}}.
func (b *Builder) renderUnifiedDataRow(r UnifiedRow, colWidths []int) string {
	cells := b.unifiedCells(r)
	var sb strings.Builder
	if r.Dim {
		// History/anchored rows: plain │ dividers, whole line wrapped in dim.
		sb.WriteRune('│')
		for i := range cells {
			sb.WriteString(padCell(cells[i].Content, colWidths[i], cells[i].Align))
			sb.WriteRune('│')
		}
		return "{{dim}}" + sb.String() + "{{reset}}"
	}
	// Current-group rows: the internal/trailing dividers are {{dim}}│{{reset}} so
	// the bars render dim while cell content keeps its own colour (T-6). The
	// leading border is a plain │ so the line does NOT start with a {{dim}}
	// wrapper — that prefix is reserved for whole-row dim (history) rows (T-4).
	sb.WriteRune('│')
	for i := range cells {
		sb.WriteString(padCell(cells[i].Content, colWidths[i], cells[i].Align))
		sb.WriteString(dimBar)
	}
	return sb.String()
}

// unifiedCells builds the seven cells of a row from its UnifiedRow fields.
func (b *Builder) unifiedCells(r UnifiedRow) Row {
	cacheCell := b.cacheRWCell(r.CacheRead, r.CacheCreate, r.RedCacheWrite)
	return Row{
		{Content: r.HashCell, Align: AlignRight},
		{Content: unifiedRoleColour(r.Role, r.IsSidechain), Align: AlignLeft},
		{Content: r.Model, Align: AlignLeft},
		{Content: cacheCell, Align: AlignLeft},
		{Content: format.FormatK(r.Out), Align: AlignRight},
		{Content: r.CostCell, Align: AlignRight},
		{Content: unifiedToolCell(r.Tool), Align: AlignLeft},
	}
}

// cacheRWCell formats the "cache r/w" cell as "<read>/<write>". When redWrite is
// set the write sub-token (cache_create) is wrapped in {{color:red}} so a cache
// collapse / cold cache is visible without touching the read part (T-34..T-36).
func (b *Builder) cacheRWCell(read, write int, redWrite bool) string {
	readStr := format.FormatK(read)
	writeStr := format.FormatK(write)
	if redWrite {
		writeStr = "{{color:red}}" + writeStr + "{{reset}}"
	}
	return readStr + "/" + writeStr
}

// unifiedToolCell renders the tool-column content. Empty tool → "thinking..."
// (the no-activity / thinking state, T-8).
func unifiedToolCell(tool string) string {
	if tool == "" {
		return ThinkingGlyph
	}
	return tool
}

// unifiedRoleColour wraps a role label with colour markers based on IsSidechain:
//   - orchestrator (IsSidechain=false) → cyan
//   - sidechain agent (IsSidechain=true) → yellow
func unifiedRoleColour(role string, isSidechain bool) string {
	if !isSidechain {
		return "{{color:cyan}}" + role + "{{reset}}"
	}
	return "{{color:yellow}}" + role + "{{reset}}"
}

// renderUnifiedLegend renders the legend content row with column header labels.
// The cache column label is "cache r/w" (T-31). Uses {{dim}}│{{reset}} dividers.
func renderUnifiedLegend(colWidths []int) string {
	labels := [7]string{"#", "role", "model", "cache r/w", "out", "cost", "tool"}
	aligns := [7]Align{AlignRight, AlignLeft, AlignLeft, AlignLeft, AlignRight, AlignRight, AlignLeft}

	var sb strings.Builder
	sb.WriteString(dimBar)
	for i, lbl := range labels {
		sb.WriteString(padCell(lbl, colWidths[i], aligns[i]))
		sb.WriteString(dimBar)
	}
	return sb.String()
}

// RenderUnified renders the redesigned per-turn table directly from turns.
//
// It is the backward-compatible entry-point used by structural tests and any
// caller that does not need the rich assembler-level state (collapsed subagent
// rows, TTL suffixes, instance names). Each turn becomes one row; subagent turns
// get the "↳" prefix and a "Σ $" cost cell, orchestrator turns get a per-turn
// cost cell. Dim is applied to turns from older orchestrator groups.
func (b *Builder) RenderUnified(turns []parser.Turn, st *state.Session) string {
	if len(turns) == 0 {
		return ""
	}

	maxOrchGroup := 0
	for _, t := range turns {
		if !t.IsSidechain && t.GroupID > maxOrchGroup {
			maxOrchGroup = t.GroupID
		}
	}

	rows := make([]UnifiedRow, 0, len(turns))
	for _, t := range turns {
		model := strings.TrimPrefix(t.Model, "claude-")

		costCell := "—"
		hash := ""
		if t.IsSidechain {
			hash = "↳"
		} else if t.Index > 0 {
			hash = fmt.Sprintf("%d", t.Index)
		}
		if !t.IsSidechain {
			if v, ok := cost.PerTurn(st, t.UUID); ok {
				costCell = cost.Format(v)
			}
		}

		tool := t.ToolUse
		if t.Thinking {
			tool = ThinkingGlyph
		}

		rows = append(rows, UnifiedRow{
			HashCell:      hash,
			Role:          t.Role,
			Model:         model,
			CacheRead:     t.Tokens.CacheRead,
			CacheCreate:   t.Tokens.CacheCreate,
			Out:           t.Tokens.Output,
			CostCell:      costCell,
			Tool:          tool,
			IsSidechain:   t.IsSidechain,
			Dim:           !t.IsSidechain && t.GroupID < maxOrchGroup,
			GroupID:       t.GroupID,
			SkipSeparator: t.IsSidechain,
		})
	}
	return b.RenderUnifiedRows(rows)
}
