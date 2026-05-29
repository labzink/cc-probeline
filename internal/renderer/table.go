package renderer

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/parser"
)

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

// widths for each number of visible columns:
//
//	7-col (full):  content=68 borders=8 → total 76
//	6-col (#drop): content=64 borders=7 → total 71
//	5-col (+cost): content=55 borders=6 → total 61
const (
	fullTableWidth  = 76
	sixColWidth     = 71
	fiveColWidth    = 61
	fiveColMinTotal = 46 // cols below this → overflow accepted (tool min=1)
)

// NewBuilder returns a Builder with default layout using the given terminal
// width (cols). If cols <= 0, defaults to 80.
// Column widths (Phase 6.6.c): [#=4, role=7, model=12, cache=13, out=7, cost=9, tool/arg=16].
// Full table width: 4+7+12+13+7+9+16=68 content + 8 borders = 76.
func NewBuilder(cols int) *Builder {
	if cols <= 0 {
		cols = 80
	}
	return &Builder{
		cols:         [7]int{4, 7, 12, 13, 7, 9, 16},
		terminalCols: cols,
	}
}

// effectiveCols returns the column widths as-is.
func (b *Builder) effectiveCols() [7]int {
	return b.cols
}

// padCell pads s to exactly w runes with a 1-space margin:
//   - AlignLeft:  " " + content + trailing_spaces
//   - AlignRight: leading_spaces + content + " "
//
// Content > (w-1) runes is middle-truncated then hard-cut if needed.
func padCell(s string, w int, a Align) string {
	inner := w - 1
	if inner < 0 {
		inner = 0
	}
	runes := []rune(s)
	if len(runes) > inner {
		s = format.MiddleTruncate(s, inner)
		runes = []rune(s)
	}
	if len(runes) > inner {
		runes = runes[:inner]
	}
	pad := inner - len(runes)
	if a == AlignRight {
		return strings.Repeat(" ", pad) + string(runes) + " "
	}
	return " " + string(runes) + strings.Repeat(" ", pad)
}

// costFor returns the formatted cost string for a single turn.
func (b *Builder) costFor(t parser.Turn) string { return cost.Format(cost.Compute(t)) }

// costForAgg returns the formatted total cost string for all aggregated turns.
func (b *Builder) costForAgg() string { return cost.Format(cost.ComputeAggregate(b.turns)) }

// durationAggregate returns the sum of all turn Durations.
func (b *Builder) durationAggregate() time.Duration { return b.aggDur }

// Add appends one per-turn row built from a parser.Turn.
// Orchestrator turns only — aggCache/aggOut/aggDur track only these rows.
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
		{Content: t.Role, Align: AlignLeft},
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
// Column mapping: #=↳, role=AgentType, model=Model, cache=CacheRead/CacheCreate,
// out=Output, cost=— (no source, BL-7), tool/arg=LastTool (empty → —).
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
		row := Row{
			{Content: "↳", Align: AlignLeft},
			{Content: s.AgentType, Align: AlignLeft},
			{Content: model, Align: AlignLeft},
			{Content: cache, Align: AlignLeft},
			{Content: format.FormatK(s.Tokens.Output), Align: AlignRight},
			{Content: "—", Align: AlignRight}, // cost: no source (BL-7)
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
// three-step column-drop strategy (§6.6.c §2.3):
//
//  1. cols >= 76 → full 7-col table.
//  2. cols < 76, >= 71 → drop col "#" (index 0) → 6-col table.
//  3. cols < 71, >= 61 → drop "#" + "cost" (index 5) → 5-col table.
//  4. cols < 61, >= 46 → 5-col + middle-truncate tool/arg.
//  5. cols < 46 → accept overflow (return Render()).
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

	// Step 2: drop "#" col (index 0) → 6-col, width 71.
	c6 := b.colsAfterDrop([]int{colHash})
	if cols >= sixColWidth {
		return b.renderNCols(c6, nil)
	}

	// Step 3: drop "#" + "cost" → 5-col, width 61.
	c5 := b.colsAfterDrop([]int{colHash, colCost})
	if cols >= fiveColWidth {
		return b.renderNCols(c5, nil)
	}

	// Step 4: 5-col + middle-truncate tool/arg.
	// tool/arg is the last column in c5; flex = cols - borders(6) - fixed.
	// Fixed widths in 5-col: role(7)+model(12)+cache(13)+out(7) = 39.
	const borders5 = 6
	const fixed5 = 39
	flex := cols - borders5 - fixed5
	if flex < 1 {
		// cols too narrow even for 5-col with tool min=1 → overflow.
		return full
	}
	// Rebuild c5 with tool/arg column set to flex width.
	c5flex := make([]int, len(c5))
	copy(c5flex, c5)
	c5flex[len(c5flex)-1] = flex
	toolInner := flex - 1
	if toolInner < 1 {
		return full
	}
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

// hlineSlice builds a horizontal border line for an arbitrary number of columns.
func hlineSlice(cols []int, left, join, right, fill rune, mergeAt map[int]rune) string {
	var sb strings.Builder
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
	return sb.String()
}

// renderRowN renders one content row selecting only the original columns in origIndices.
// toolInner, when non-nil and > 0, limits the last column content via MiddleTruncate.
func renderRowN(row Row, colWidths []int, origIndices []int, toolInner *int) string {
	var sb strings.Builder
	sb.WriteRune('│')
	for i, origIdx := range origIndices {
		cell := row[origIdx]
		content := cell.Content
		// Apply tool truncation to the last column (tool/arg).
		if toolInner != nil && i == len(origIndices)-1 && *toolInner > 0 {
			content = format.MiddleTruncate(content, *toolInner)
		}
		sb.WriteString(padCell(content, colWidths[i], cell.Align))
		sb.WriteRune('│')
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
	sb.WriteString("│")
	sb.WriteString(padCell("Total for request", mergedWidth, AlignLeft))
	sb.WriteString("│")

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
		sb.WriteString("│")
	}
	return sb.String()
}
