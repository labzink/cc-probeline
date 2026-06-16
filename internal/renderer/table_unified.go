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
// Layout rules (N-notch redesign, 2026-06-03):
//   - Rows are pre-sorted newest-first by the caller.
//   - No standalone full-line inter-group separator is emitted between data rows.
//     Instead, the anchor row of each orchestrator group (the chronologically
//     earliest turn = bottom row of the group's block in newest-first order)
//     carries notch dividers: leading │→├, inner │→┼, trailing │→┤ (all dim).
//     Every group, including the freshest, gets exactly one anchor notch row.
//   - Dim rows are wrapped in {{dim}}…{{reset}} (whole line, borders included).
//     Inner {{reset}} tokens are re-dimmed ({{reset}}{{dim}}) so dim stays
//     active across the full visible line — F14 fix.
//   - The legend row ("# role model cache r/w out cost tool") is preceded by a
//     full-line ├─┼─┤ separator and sits immediately before the bottom border.
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
	// Legend separator (T-5): full-line ├─┼─┤ above the legend row.
	// F2 fix: right corner is ┤ (continuous border), not ┐.
	legendSep := hlineSlice(colWidths, '├', '┼', '┤', '─', nil)

	// Pre-compute which rows are anchor rows (notch boundary).
	// An orch row at index i is an anchor iff the next orch row (j > i, not
	// SkipSeparator) belongs to a different GroupID, or no next orch row exists.
	// This identifies the chronologically earliest turn per group in the
	// newest-first slice (bottom row of each group's block).
	isAnchor := computeAnchorRows(rows)

	var sb strings.Builder
	sb.WriteString(topBorder)
	sb.WriteByte('\n')

	for i, r := range rows {
		line := b.renderUnifiedDataRow(r, colWidths, isAnchor[i])
		sb.WriteString(line)
		if r.TTLSuffix != "" {
			sb.WriteByte(' ')
			// Dim rows fade their TTL too: the suffix is appended outside the row's
			// dim wrap, so a faded (older-group) row would otherwise keep a bright
			// TTL. Layer {{dim}} over its colour so it fades to dim-coloured.
			if r.Dim {
				sb.WriteString(dimTTLSuffix(r.TTLSuffix))
			} else {
				sb.WriteString(r.TTLSuffix)
			}
		}
		sb.WriteByte('\n')
	}

	// Legend: full-line ├─┼─┤ separator immediately before the legend row (T-5).
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

// computeAnchorRows returns a boolean slice (same length as rows) indicating
// whether each row is an anchor row for the notch boundary redesign.
//
// An orch row (not SkipSeparator) is an anchor iff there is no subsequent orch
// row with the same GroupID — i.e. it is the last (chronologically earliest)
// turn of its group in the newest-first slice.
func computeAnchorRows(rows []UnifiedRow) []bool {
	anchors := make([]bool, len(rows))

	// For each orch row, check whether any later orch row shares its GroupID.
	// If not → it is the anchor (lowest timestamp, bottom of group's block).
	for i, r := range rows {
		if r.SkipSeparator {
			// Subagent rows never carry notch dividers.
			continue
		}
		// Scan forward for another orch row with the same GroupID.
		sameGroupAfter := false
		for j := i + 1; j < len(rows); j++ {
			if rows[j].SkipSeparator {
				continue
			}
			if rows[j].GroupID == r.GroupID {
				sameGroupAfter = true
				break
			}
		}
		if !sameGroupAfter {
			anchors[i] = true
		}
	}
	return anchors
}

// dimTTLSuffix layers {{dim}} over a colour-marked TTL suffix so a faded
// (older-group) row's TTL fades WITH its colour preserved — e.g. an expired red
// "0m" reads as dim-red, not dim-grey. {{dim}} + {{color:red}} both apply
// (\x1b[2m\x1b[31m); the trailing {{reset}} clears both. When colour is off the
// input has no markers and the result is plain text in {{dim}}…{{reset}}, which
// Apply strips back to plain.
func dimTTLSuffix(suffix string) string {
	if suffix == "" {
		return ""
	}
	return "{{dim}}" + suffix + "{{reset}}"
}

// dimNotchLeading is the notch leading border (├) wrapped in dim markers.
const dimNotchLeading = "{{dim}}├{{reset}}"

// dimNotchInner is the notch inner junction (┼) wrapped in dim markers.
const dimNotchInner = "{{dim}}┼{{reset}}"

// dimNotchTrailing is the notch trailing border (┤) wrapped in dim markers.
const dimNotchTrailing = "{{dim}}┤{{reset}}"

// renderUnifiedDataRow renders one UnifiedRow as a bordered content line.
//
// isAnchor marks the row as a notch-boundary anchor: all vertical dividers
// use notch glyphs (├/┼/┤) instead of │.
//
// Dim rows (r.Dim=true) wrap the whole line in {{dim}}…{{reset}}. To prevent
// inner cell {{reset}} tokens from killing the outer dim (F14), every inner
// {{reset}} is replaced with {{reset}}{{dim}} before the outer wrap is applied.
//
// Fresh (non-dim) rows use dimBar ({{dim}}│{{reset}}) for ALL dividers including
// the leading border (F1 fix). Notch rows likewise use dimBar-style notch glyphs.
func (b *Builder) renderUnifiedDataRow(r UnifiedRow, colWidths []int, isAnchor bool) string {
	cells := b.unifiedCells(r, colWidths)
	n := len(cells)

	if r.Dim {
		// History rows: whole line wrapped in {{dim}}…{{reset}}.
		// F14: replace inner {{reset}} with {{reset}}{{dim}} so dim survives
		// across coloured cells (e.g. {{color:cyan}}role{{reset}} → dim killed).
		// Inside the dim wrapper, use plain box-drawing runes (the outer dim
		// already applies). Notch anchor rows use ├/┼/┤ instead of │.
		var sb strings.Builder
		if isAnchor {
			sb.WriteRune('├')
		} else {
			sb.WriteRune('│')
		}
		for i := range cells {
			sb.WriteString(padCell(cells[i].Content, colWidths[i], cells[i].Align))
			if isAnchor {
				if i < n-1 {
					sb.WriteRune('┼')
				} else {
					sb.WriteRune('┤')
				}
			} else {
				sb.WriteRune('│')
			}
		}
		// F14: re-dim after every inner {{reset}} so dim survives coloured cells.
		content := strings.ReplaceAll(sb.String(), "{{reset}}", "{{reset}}{{dim}}")
		return "{{dim}}" + content + "{{reset}}"
	}

	// Fresh (non-dim) rows: F1 fix — ALL dividers use dimBar pattern, including
	// the leading border. Notch anchor rows use notch glyphs instead of │.
	leading := dimBar
	inner := dimBar
	trailing := dimBar
	if isAnchor {
		leading = dimNotchLeading
		inner = dimNotchInner
		trailing = dimNotchTrailing
	}

	var sb strings.Builder
	sb.WriteString(leading)
	for i := range cells {
		sb.WriteString(padCell(cells[i].Content, colWidths[i], cells[i].Align))
		if i < n-1 {
			sb.WriteString(inner)
		} else {
			sb.WriteString(trailing)
		}
	}
	return sb.String()
}

// unifiedCells builds the seven cells of a row from its UnifiedRow fields.
// colWidths is passed so the cache cell can right-pin the write sub-token (F9).
func (b *Builder) unifiedCells(r UnifiedRow, colWidths []int) Row {
	// Cache column is index 3; inner width = colWidths[3] - 1.
	cacheColWidth := 13 // default matches base column layout
	if len(colWidths) > 3 {
		cacheColWidth = colWidths[3]
	}
	cacheCell := b.cacheRWCell(r.CacheRead, r.CacheCreate, r.RedCacheWrite, cacheColWidth)
	// Orchestrator "#" cells are turn numbers (right-aligned). Subagent cells are
	// "↳N": flush-left so the "↳" arrow always sits in the first column position
	// regardless of single- vs multi-digit N (consistent across all agent rows).
	hashAlign := AlignRight
	if r.IsSidechain {
		hashAlign = AlignLeftFlush
	}
	return Row{
		{Content: r.HashCell, Align: hashAlign},
		{Content: unifiedRoleColour(r.Role, r.IsSidechain), Align: AlignLeft},
		{Content: r.Model, Align: AlignLeft},
		{Content: cacheCell, Align: AlignLeft},
		{Content: format.FormatK(r.Out), Align: AlignRight},
		{Content: r.CostCell, Align: AlignRight},
		{Content: unifiedToolCell(r.Tool), Align: AlignLeft},
	}
}

// cacheRWCell formats the "cache r/w" cell with read pinned left and write
// pinned right within the column's inner width (F9 fix).
//
// The write sub-token is right-justified: visible padding fills the space
// between read and write so they sit at opposing edges of the column.
// When redWrite is set the write token is wrapped in {{color:red}} (T-34..T-36);
// visible width is measured via format.VisualLen so the marker is zero-width.
//
// The returned content is passed to padCell(AlignLeft, colWidth) which prepends
// one leading space and does not add trailing padding when content fills inner.
func (b *Builder) cacheRWCell(read, write int, redWrite bool, colWidth int) string {
	readStr := format.FormatK(read)
	writeStr := format.FormatK(write)
	if redWrite {
		writeStr = "{{color:red}}" + writeStr + "{{reset}}"
	}
	inner := colWidth - 1 // padCell uses inner = colWidth-1 as usable width
	if inner < 0 {
		inner = 0
	}
	readVis := format.VisualLen(readStr)   // visible width of read token
	writeVis := format.VisualLen(writeStr) // visible width of write token (markers zero-width)
	// Reserve one trailing column so the write token gets one space of padding
	// before the right border instead of sitting flush against it. padCell
	// (AlignLeft) fills that reserved column with a trailing space.
	gap := inner - readVis - writeVis - 1
	if gap < 0 {
		gap = 0
	}
	return readStr + strings.Repeat(" ", gap) + writeStr
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
	labels := [7]string{"#", "role", "model", "cache r/w", "out", "~cost", "tool"}
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
