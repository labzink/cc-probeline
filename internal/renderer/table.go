package renderer

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/parser"
)

// leadMarkerRE / trailMarkerRE match a run of consecutive {{marker}} tokens at
// the start / end of a string. Used by padCell to peel a cell's colour wrapper
// off before truncating the visible core, then re-attach it — so colour
// survives truncation instead of being stripped (cells are wrapped as
// {{color:X}}…{{reset}}).
var (
	leadMarkerRE  = regexp.MustCompile(`^(?:\{\{[a-z][a-z0-9:_-]*\}\})+`)
	trailMarkerRE = regexp.MustCompile(`(?:\{\{[a-z][a-z0-9:_-]*\}\})+$`)
)

// splitWrapMarkers separates s into a leading marker run, the middle core, and
// a trailing marker run. For an unwrapped string prefix and suffix are "".
func splitWrapMarkers(s string) (prefix, core, suffix string) {
	prefix = leadMarkerRE.FindString(s)
	rest := s[len(prefix):]
	suffix = trailMarkerRE.FindString(rest)
	core = rest[:len(rest)-len(suffix)]
	return prefix, core, suffix
}

// Align controls horizontal alignment inside a Cell.
type Align int

const (
	AlignLeft Align = iota
	AlignRight
	AlignCenter
)

// Cell is one slot in the R1 box-drawing table.
type Cell struct {
	Content string
	Align   Align
}

// Row holds the seven fixed columns: #, role, model, cache, out, cost, tool/arg.
type Row [7]Cell

// Builder accumulates per-turn rows and renders the full R1-bordered table
// with a merged "Total for request" footer (§4.2 C-4 / C-5).
type Builder struct {
	cols         [7]int
	terminalCols int // target terminal width (default 80)
	rows         []Row
	agentRows    []Row         // subagent rows — appended after orchestrator rows
	turns        []parser.Turn // raw turns for cost aggregation
	aggCache     [2]int        // [0]=total CacheRead, [1]=total CacheCreate
	aggOut       int           // total output tokens
	aggDur       time.Duration // sum of all turn Durations
}

// Column index constants for drop-order logic.
const (
	colHash = 0 // "#" column — first to drop
	colCost = 5 // "cost" column — second to drop
)

// widths for each number of visible columns (Phase 6.6.d):
//
//	7-col (full):  content=72 borders=8 → total 80  (4+13+12+13+7+8+15)
//	6-col (#drop): content=68 borders=7 → total 75  (drop #=4)
//	5-col (+cost): content=60 borders=6 → total 66  (drop #+cost=8)
const (
	fullTableWidth  = 80
	sixColWidth     = 75
	fiveColWidth    = 66
	fiveColMinTotal = 53 // min width for 5-col with a 1-rune tool cell: fixed5=45 + borders5=6 + tool col 2; below this → overflow
)

// NewBuilder returns a Builder with default layout using the given terminal
// width (cols). If cols <= 0, defaults to 80.
// Column widths (Phase 6.6.d): [#=4, role=13, model=12, cache=13, out=7, cost=8, tool/arg=15].
// Full table width: 4+13+12+13+7+8+15=72 content + 8 borders = 80.
func NewBuilder(cols int) *Builder {
	if cols <= 0 {
		cols = 80
	}
	return &Builder{
		cols:         [7]int{4, 13, 12, 13, 7, 8, 15},
		terminalCols: cols,
	}
}

// effectiveCols returns the column widths as-is.
func (b *Builder) effectiveCols() [7]int {
	return b.cols
}

// padCell pads s to exactly w visual columns with a 1-space margin:
//   - AlignLeft:  " " + content + trailing_spaces
//   - AlignRight: leading_spaces + content + " "
//
// Content wider than (w-1) visual columns is middle-truncated. The cell's
// leading/trailing colour markers are preserved across truncation (only the
// visible core is shortened) so a long coloured cell keeps its colour instead
// of degrading to plain text. Visual width is computed via format.VisualLen so
// that {{marker}} tokens are treated as zero-width.
func padCell(s string, w int, a Align) string {
	inner := w - 1
	if inner < 0 {
		inner = 0
	}
	vlen := format.VisualLen(s)
	if vlen > inner {
		// Peel the colour wrapper, truncate the visible core, re-attach the wrapper.
		prefix, core, suffix := splitWrapMarkers(s)
		core = format.MiddleTruncate(format.StripMarkers(core), inner)
		if format.VisualLen(core) > inner {
			// Hard-cut as a last resort.
			runes := []rune(core)
			core = string(runes[:inner])
		}
		s = prefix + core + suffix
		vlen = format.VisualLen(s)
	}
	pad := inner - vlen
	if a == AlignRight {
		return strings.Repeat(" ", pad) + s + " "
	}
	return " " + s + strings.Repeat(" ", pad)
}

// costFor returns the formatted cost string for a single turn.
func (b *Builder) costFor(t parser.Turn) string { return cost.Format(cost.Compute(t)) }

// costForAgg returns the formatted total cost string for all aggregated turns.
func (b *Builder) costForAgg() string { return cost.Format(cost.ComputeAggregate(b.turns)) }

// durationAggregate returns the sum of all turn Durations.
func (b *Builder) durationAggregate() time.Duration { return b.aggDur }

// subagentElapsedCell returns the elapsed-time cell for a subagent row,
// colour-wrapped per spec §2.3:
//
//	span > 300s → {{color:red}}⏱MM:SS{{reset}}   (long-running agent)
//	span ≤ 300s → {{color:yellow}}⏱MM:SS{{reset}} (active agent)
//
// Span is the agent's active window LastTimestamp − FirstTimestamp (no wall
// clock needed). When either timestamp is missing or the span is non-positive
// the cell falls back to "—" (no usable duration).
func subagentElapsedCell(first, last time.Time) string {
	if first.IsZero() || last.IsZero() || !last.After(first) {
		return "—"
	}
	span := last.Sub(first)
	text := "⏱" + format.FormatMMSS(span.Milliseconds())
	if span > 300*time.Second {
		return "{{color:red}}" + text + "{{reset}}"
	}
	return "{{color:yellow}}" + text + "{{reset}}"
}

// roleColour wraps a role string with the appropriate colour marker:
//   - "orch" (orchestrator) → cyan
//   - all other values (agent names / types) → yellow
func roleColour(role string) string {
	if role == "orch" {
		return "{{color:cyan}}" + role + "{{reset}}"
	}
	return "{{color:yellow}}" + role + "{{reset}}"
}

// Add appends one per-turn row built from a parser.Turn.
// Orchestrator turns only — aggCache/aggOut/aggDur track only these rows.
// The role cell is wrapped with colour markers (spec §2.3).
func (b *Builder) Add(t parser.Turn) {
	model := t.Model
	if strings.HasPrefix(model, "claude-") {
		model = model[len("claude-"):]
	}
	cache := fmt.Sprintf("%s/%s",
		format.FormatK(t.Tokens.CacheRead),
		format.FormatK(t.Tokens.CacheCreate),
	)

	row := Row{
		{Content: fmt.Sprintf("%d", t.Index), Align: AlignRight},
		{Content: roleColour(t.Role), Align: AlignLeft},
		{Content: model, Align: AlignLeft},
		{Content: cache, Align: AlignLeft},
		{Content: format.FormatK(t.Tokens.Output), Align: AlignRight},
		{Content: b.costFor(t), Align: AlignRight},
		{Content: t.ToolUse, Align: AlignLeft},
	}
	b.rows = append(b.rows, row)
	b.turns = append(b.turns, t)
	b.aggCache[0] += t.Tokens.CacheRead
	b.aggCache[1] += t.Tokens.CacheCreate
	b.aggOut += t.Tokens.Output
	b.aggDur += t.Duration
}

// AddSubagents appends subagent rows (layout A) after orchestrator rows.
// Subagent tokens are NOT included in aggCache/aggOut (footer Total is orchestrator-only).
// Column mapping: #=↳, role=AgentType (yellow), model=Model, cache=CacheRead/CacheCreate,
// out=Output, cost-slot=⏱elapsed (red>300s / yellow, span Last−First; "—" when unknown),
// tool/arg=LastTool (empty → —).
// AgentType is wrapped with {{color:yellow}}...{{reset}} per spec §2.3; the
// elapsed cell carries its own colour (see subagentElapsedCell). The cost
// column has no subagent source (BL-7), so the slot surfaces elapsed instead.
func (b *Builder) AddSubagents(subs []parser.SubagentStats) {
	slog.Debug("renderer.table: AddSubagents", "count", len(subs))
	for _, s := range subs {
		model := s.Model
		if strings.HasPrefix(model, "claude-") {
			model = model[len("claude-"):]
		}
		cache := fmt.Sprintf("%s/%s",
			format.FormatK(s.Tokens.CacheRead),
			format.FormatK(s.Tokens.CacheCreate),
		)
		tool := s.LastTool
		if tool == "" {
			tool = "—"
		}
		// Subagent AgentType is displayed as the "role" cell → yellow (spec §2.3).
		agentType := "{{color:yellow}}" + s.AgentType + "{{reset}}"
		row := Row{
			{Content: "↳", Align: AlignLeft},
			{Content: agentType, Align: AlignLeft},
			{Content: model, Align: AlignLeft},
			{Content: cache, Align: AlignLeft},
			{Content: format.FormatK(s.Tokens.Output), Align: AlignRight},
			{Content: subagentElapsedCell(s.FirstTimestamp, s.LastTimestamp), Align: AlignRight}, // cost-slot → ⏱elapsed (BL-7: no cost source)
			{Content: tool, Align: AlignLeft},
		}
		b.agentRows = append(b.agentRows, row)
	}
}

// allRows returns orchestrator rows followed by agent rows (for rendering).
// Orchestrator rows are newest-first; agent rows appear after them in their
// natural (insertion) order.
func (b *Builder) allRows() []Row {
	total := make([]Row, 0, len(b.rows)+len(b.agentRows))
	// Orchestrator rows: newest-first.
	for i := len(b.rows) - 1; i >= 0; i-- {
		total = append(total, b.rows[i])
	}
	// Agent rows: appended after (in insertion order, newest subagent first from CollectSubagents).
	total = append(total, b.agentRows...)
	return total
}

// Render returns the full table string in the §6.5 B6 order:
// topBorder-merge → footerRow → separator-split → data rows → bottomBorder.
// Returns "" when no rows have been added.
func (b *Builder) Render() string {
	if len(b.rows) == 0 {
		return ""
	}
	cols := b.effectiveCols()
	return b.renderNCols(cols[:], nil)
}

// RenderForCols renders the table targeting cols terminal width, applying a
// three-step column-drop strategy (§6.6.d §2.4):
//
//  1. cols >= 80 → full 7-col table.
//  2. cols < 80, >= 75 → drop col "#" (index 0) → 6-col table.
//  3. cols < 75, >= 66 → drop "#" + "cost" (index 5) → 5-col table.
//  4. cols < 66, >= 53 → 5-col + middle-truncate tool/arg.
//  5. cols < 53 → accept overflow (return Render()).
//
// When cols == 0 the result is identical to Render() (no truncation).
func (b *Builder) RenderForCols(cols int) string {
	if cols == 0 {
		return b.Render()
	}
	full := b.Render()
	if full == "" {
		return full
	}
	if cols >= fullTableWidth {
		return full
	}

	// Step 2: drop "#" col (index 0) → 6-col, width 75.
	c6 := b.colsAfterDrop([]int{colHash})
	if cols >= sixColWidth {
		return b.renderNCols(c6, nil)
	}

	// Step 3: drop "#" + "cost" → 5-col, width 66.
	c5 := b.colsAfterDrop([]int{colHash, colCost})
	if cols >= fiveColWidth {
		return b.renderNCols(c5, nil)
	}

	// Step 4: 5-col + middle-truncate tool/arg.
	// Below fiveColMinTotal there is not enough width for a 5-col layout with a
	// ≥1-rune tool/arg cell, so overflow is accepted (see const block).
	if cols < fiveColMinTotal {
		return full
	}
	// tool/arg is the last column in c5; flex = cols - borders(6) - fixed.
	// Fixed widths in 5-col: role(13)+model(12)+cache(13)+out(7) = 45.
	const borders5 = 6
	const fixed5 = 45
	flex := cols - borders5 - fixed5
	// Rebuild c5 with tool/arg column set to flex width.
	// cols >= fiveColMinTotal guarantees flex >= 2, so toolInner >= 1.
	c5flex := make([]int, len(c5))
	copy(c5flex, c5)
	c5flex[len(c5flex)-1] = flex
	toolInner := flex - 1
	return b.renderNColsTrunc(c5flex, toolInner)
}

// colsAfterDrop returns a slice of column widths with the given indices removed.
// dropIdx must be a sorted list of col indices to drop.
func (b *Builder) colsAfterDrop(dropIdx []int) []int {
	dropped := make(map[int]bool, len(dropIdx))
	for _, i := range dropIdx {
		dropped[i] = true
	}
	result := make([]int, 0, 7-len(dropIdx))
	for i, w := range b.cols {
		if !dropped[i] {
			result = append(result, w)
		}
	}
	return result
}

// renderNCols renders the table with the given column widths (arbitrary count).
// dropIdx is used to map 7-col rows to the remaining columns for row rendering.
// toolMaxInner, when > 0, limits the last column (tool/arg) via MiddleTruncate.
//
// Order: topBorder-merge → footerRow → separator-split → data rows → bottomBorder.
func (b *Builder) renderNCols(colWidths []int, toolInner *int) string {
	if len(b.rows) == 0 {
		return ""
	}

	// Determine which original column indices are present.
	nCols := len(colWidths)
	// The dropped set: we reconstruct from colWidths vs b.cols.
	// Since colsAfterDrop preserves order, we can infer the mapping.
	origIndices := b.resolveOrigIndices(nCols)

	topBorder := hlineSlice(colWidths, '┌', '┬', '┐', '─', b.mergeAtMap(nCols, 0))
	separator := hlineSlice(colWidths, '├', '┼', '┤', '─', b.mergeAtMap(nCols, 1))
	bottomBorder := hlineSlice(colWidths, '└', '┴', '┘', '─', nil)

	var sb strings.Builder
	sb.WriteString(topBorder)
	sb.WriteByte('\n')
	sb.WriteString(b.buildFooterRowN(colWidths, origIndices))
	sb.WriteByte('\n')
	sb.WriteString(separator)
	sb.WriteByte('\n')
	for _, row := range b.allRows() {
		sb.WriteString(renderRowN(row, colWidths, origIndices, toolInner))
		sb.WriteByte('\n')
	}
	sb.WriteString(bottomBorder)
	sb.WriteByte('\n')
	return sb.String()
}

// renderNColsTrunc renders with the given column widths and tool/arg truncation.
func (b *Builder) renderNColsTrunc(colWidths []int, toolInner int) string {
	return b.renderNCols(colWidths, &toolInner)
}

// resolveOrigIndices returns the original column indices (0-6) present when
// using nCols columns. For 7-col all present; for 6-col "#" dropped; for 5-col
// "#" and "cost" dropped.
func (b *Builder) resolveOrigIndices(nCols int) []int {
	switch nCols {
	case 7:
		return []int{0, 1, 2, 3, 4, 5, 6}
	case 6:
		// drop col 0 (#)
		return []int{1, 2, 3, 4, 5, 6}
	case 5:
		// drop col 0 (#) and col 5 (cost)
		return []int{1, 2, 3, 4, 6}
	default:
		// Fallback: include all up to nCols (shouldn't happen in practice).
		idx := make([]int, nCols)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
}

// mergeAtMap returns the mergeAt rune map for the top border (mode=0) or
// separator (mode=1). In the top border, col boundaries 0/1 use '─' (merged
// span). In the separator, col boundaries 0/1 use '┬' (sprout downward).
// This applies when the "#" column (col 0) is present; otherwise no merge.
func (b *Builder) mergeAtMap(nCols, mode int) map[int]rune {
	// Only apply merge when "#" col is present (7-col layout).
	if nCols < 7 {
		// No merge override — separator uses ┬ at position 0 to mark the split
		// between the merged footer label and data columns.
		if mode == 1 && nCols >= 5 {
			// For 6-col and 5-col: the footer label merges col 0+1+2 → boundaries 0/1 use ┬.
			return map[int]rune{0: '┬', 1: '┬'}
		}
		return nil
	}
	if mode == 0 {
		return map[int]rune{0: '─', 1: '─'}
	}
	return map[int]rune{0: '┬', 1: '┬'}
}

// dimBar is the vertical cell separator wrapped in {{dim}} so that, like the
// horizontal borders, it renders in the terminal's dim colour (spec §2.3 —
// borders → dim). Keeping all border runes dim avoids the mixed light/dark
// look that occurs when only some borders carry the dim attribute.
const dimBar = "{{dim}}│{{reset}}"

// hlineSlice builds a horizontal border line for an arbitrary number of columns.
//
// The entire line — corner runes, fill runs and join characters — is wrapped in
// {{dim}}...{{reset}} so that Apply renders the whole border in the terminal's
// dim colour (spec §2.3 — borders → dim), uniformly with the vertical bars.
func hlineSlice(cols []int, left, join, right, fill rune, mergeAt map[int]rune) string {
	var sb strings.Builder
	sb.WriteString("{{dim}}")
	sb.WriteRune(left)
	for i, w := range cols {
		sb.WriteString(strings.Repeat(string(fill), w))
		if i < len(cols)-1 {
			if r, ok := mergeAt[i]; ok {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(join)
			}
		}
	}
	sb.WriteRune(right)
	sb.WriteString("{{reset}}")
	return sb.String()
}

// renderRowN renders one content row selecting only the original columns in origIndices.
// toolInner, when non-nil and > 0, limits the last column content via MiddleTruncate.
func renderRowN(row Row, colWidths []int, origIndices []int, toolInner *int) string {
	var sb strings.Builder
	sb.WriteString(dimBar)
	for i, origIdx := range origIndices {
		cell := row[origIdx]
		content := cell.Content
		// Apply tool truncation to the last column (tool/arg).
		if toolInner != nil && i == len(origIndices)-1 && *toolInner > 0 {
			content = format.MiddleTruncate(content, *toolInner)
		}
		sb.WriteString(padCell(content, colWidths[i], cell.Align))
		sb.WriteString(dimBar)
	}
	return sb.String()
}

// buildFooterRowN returns the footer line for an N-column layout.
// The footer label "Total for request" spans the first three columns (merged).
// Aggregates are orchestrator-only (subagent tokens excluded per §6.6.c).
func (b *Builder) buildFooterRowN(colWidths []int, origIndices []int) string {
	// Merged label spans first 3 column widths + 2 inner borders between them.
	var mergedWidth int
	if len(colWidths) >= 3 {
		mergedWidth = colWidths[0] + 1 + colWidths[1] + 1 + colWidths[2]
	} else {
		mergedWidth = 0
		for i, w := range colWidths {
			if i > 0 {
				mergedWidth++
			}
			mergedWidth += w
		}
	}

	aggCacheStr := fmt.Sprintf("≡ %s/%s",
		format.FormatK(b.aggCache[0]),
		format.FormatK(b.aggCache[1]),
	)
	outStr := fmt.Sprintf("↗ %s", format.FormatK(b.aggOut))
	durMS := b.durationAggregate().Milliseconds()
	durStr := fmt.Sprintf("⏱ %s", format.FormatMMSS(durMS))

	// Columns after the merged label (starting at index 3 in origIndices).
	// We map origIndices[3:] to the remaining colWidths[3:].
	// origIndices: determines which "slot" each post-label cell corresponds to.
	// Slots: 3=cache, 4=out, 5=cost, 6=tool/arg.
	var sb strings.Builder
	sb.WriteString(dimBar)
	sb.WriteString(padCell("Total for request", mergedWidth, AlignLeft))
	sb.WriteString(dimBar)

	// Build remaining cells based on which original columns are present.
	remaining := origIndices[3:]
	for j, origIdx := range remaining {
		w := colWidths[3+j]
		switch origIdx {
		case 3: // cache
			sb.WriteString(padCell(aggCacheStr, w, AlignLeft))
		case 4: // out
			sb.WriteString(padCell(outStr, w, AlignRight))
		case 5: // cost
			sb.WriteString(padCell(b.costForAgg(), w, AlignRight))
		case 6: // tool/arg (duration in footer)
			sb.WriteString(padCell(durStr, w, AlignLeft))
		}
		sb.WriteString(dimBar)
	}
	return sb.String()
}
