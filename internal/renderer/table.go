package renderer

import (
	"fmt"
	"strings"

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
	aggCache     [2]int // [0]=total CacheRead, [1]=total CacheCreate
	aggOut       int    // total output tokens
}

// NewBuilder returns a Builder with default layout using the given terminal
// width (cols). If cols <= 0, defaults to 80.
// Column defaults: [#=3, role=6, model=10, cache=13, out=6, cost=6, tool/arg=16+flex].
func NewBuilder(cols int) *Builder {
	if cols <= 0 {
		cols = 80
	}
	return &Builder{
		cols:         [7]int{3, 6, 10, 13, 6, 6, 16},
		terminalCols: cols,
	}
}

// effectiveCols expands the last (flex) column so total rune-width equals
// the configured terminal width.
// Formula: terminalCols = sum(cols) + 8 borders  →  flex = (terminalCols-8) - sum(cols[0..5]).
func (b *Builder) effectiveCols() [7]int {
	c := b.cols
	fixed := 0
	for i := 0; i < 6; i++ {
		fixed += c[i]
	}
	// 8 border runes: left edge + 6 inner + right edge.
	flex := (b.terminalCols - 8) - fixed
	if flex < c[6] {
		flex = c[6]
	}
	c[6] = flex
	return c
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

// hline builds a horizontal border line. mergeAt overrides join rune at
// specific column-boundary indices (keyed by col index i, applied after col i).
func hline(cols [7]int, left, join, right, fill rune, mergeAt map[int]rune) string {
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

// renderRow returns one content line: │cell0│cell1│...│cell6│
func renderRow(row Row, cols [7]int) string {
	var sb strings.Builder
	sb.WriteRune('│')
	for i, cell := range row {
		sb.WriteString(padCell(cell.Content, cols[i], cell.Align))
		sb.WriteRune('│')
	}
	return sb.String()
}

// costFor returns the formatted cost string for a single turn.
// TODO(phase-4.4): replace stub with CostCalculator.
func (b *Builder) costFor(_ parser.Turn) string { return "$0.00" }

// costForAgg returns the formatted total cost string for all aggregated turns.
// TODO(phase-4.4): replace stub with CostCalculator.
func (b *Builder) costForAgg() string { return "$0.00" }

// Add appends one per-turn row built from a parser.Turn.
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
	b.aggCache[0] += t.Tokens.CacheRead
	b.aggCache[1] += t.Tokens.CacheCreate
	b.aggOut += t.Tokens.Output
}

// RenderForCols renders the table targeting cols terminal width, applying a
// two-step column-drop strategy (§4.3 T-9):
//
//  1. If the full table already fits within cols, return Render() unchanged.
//  2. Drop the lowest-priority numeric column (cost, col index 5) and render
//     a 6-column table at the requested cols. If that fits, return it.
//  3. If the 6-column table still overflows, middle-truncate the tool/arg
//     cell content so that each line fits within cols.
//  4. If cols is too narrow even for the minimal layout (cols < 30), accept
//     overflow and return Render() unchanged.
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
	if maxLineWidth(full) <= cols {
		return full
	}

	// Step 1: try dropping the cost column (P3, col index 5).
	// A 6-column layout uses borders: left + 5 inner + right = 7 runes.
	const borders6 = 7
	// Fixed cols: #(3) + role(6) + model(10) + cache(13) + out(6) = 38.
	const fixed6 = 38
	flexMin := 1 // tool/arg column minimum visible width
	flex6 := (cols - borders6) - fixed6
	if flex6 < flexMin {
		// cols too narrow for any 6-col layout; accept overflow.
		return full
	}
	narrowed := b.render6Cols(cols, flex6, 0)
	if maxLineWidth(narrowed) <= cols {
		return narrowed
	}

	// Step 2: middle-truncate tool/arg cell content.
	// inner width of tool/arg cell = flex6 - 1 (1-space margin each side minus one).
	toolInner := flex6 - 1
	if toolInner < 1 {
		return full
	}
	return b.render6Cols(cols, flex6, toolInner)
}

// maxLineWidth returns the maximum rune-count among all newline-delimited lines in s.
func maxLineWidth(s string) int {
	max := 0
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			w := 0
			for _, r := range line {
				_ = r
				w++
			}
			if w > max {
				max = w
			}
			start = i + 1
		}
	}
	return max
}

// render6Cols renders the table with 6 columns (cost column dropped).
// termCols is the target terminal width (used to compute the flex column).
// toolMaxInner, when > 0, limits the tool/arg cell content via MiddleTruncate.
// When toolMaxInner == 0, the cell content is not truncated beyond normal padCell rules.
func (b *Builder) render6Cols(termCols, flexWidth, toolMaxInner int) string {
	if len(b.rows) == 0 {
		return ""
	}

	// 6-column widths: #, role, model, cache, out, tool/arg(flex).
	cols := [6]int{3, 6, 10, 13, 6, flexWidth}

	topBorder := hline6(cols, '┌', '┬', '┐', '─', nil)
	rowSep := hline6(cols, '├', '┼', '┤', '─', nil)
	// Footer separator: cols 0-2 merged.
	footerSep := hline6(cols, '├', '┼', '┤', '─', map[int]rune{0: '┴', 1: '┴'})
	// Bottom border: cols 0-2 merged.
	bottomBorder := hline6(cols, '└', '┴', '┘', '─', map[int]rune{0: '─', 1: '─'})

	var sb strings.Builder
	sb.WriteString(topBorder)
	sb.WriteByte('\n')
	for i, row := range b.rows {
		sb.WriteString(renderRow6(row, cols, toolMaxInner))
		sb.WriteByte('\n')
		if i < len(b.rows)-1 {
			sb.WriteString(rowSep)
			sb.WriteByte('\n')
		}
	}
	sb.WriteString(footerSep)
	sb.WriteByte('\n')
	sb.WriteString(b.buildFooterRow6(cols))
	sb.WriteByte('\n')
	sb.WriteString(bottomBorder)
	sb.WriteByte('\n')
	return sb.String()
}

// hline6 builds a horizontal border line for a 6-column table.
func hline6(cols [6]int, left, join, right, fill rune, mergeAt map[int]rune) string {
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

// renderRow6 renders one content row for the 6-column layout (cost dropped).
// toolMaxInner, when > 0, limits tool/arg cell via MiddleTruncate.
func renderRow6(row Row, cols [6]int, toolMaxInner int) string {
	// Map 7-col row to 6-col: skip col 5 (cost).
	// Indices: 0=#, 1=role, 2=model, 3=cache, 4=out, 5=tool/arg (was col 6).
	mapped := [6]Cell{row[0], row[1], row[2], row[3], row[4], row[6]}
	if toolMaxInner > 0 {
		tool := mapped[5].Content
		mapped[5].Content = format.MiddleTruncate(tool, toolMaxInner)
	}
	var sb strings.Builder
	sb.WriteRune('│')
	for i, cell := range mapped {
		sb.WriteString(padCell(cell.Content, cols[i], cell.Align))
		sb.WriteRune('│')
	}
	return sb.String()
}

// buildFooterRow6 returns the footer line for the 6-column layout.
func (b *Builder) buildFooterRow6(cols [6]int) string {
	mergedWidth := cols[0] + 1 + cols[1] + 1 + cols[2]
	aggCacheStr := fmt.Sprintf("%s/%s",
		format.FormatK(b.aggCache[0]),
		format.FormatK(b.aggCache[1]),
	)
	return "│" +
		padCell("Total for request", mergedWidth, AlignLeft) + "│" +
		padCell(aggCacheStr, cols[3], AlignLeft) + "│" +
		padCell(format.FormatK(b.aggOut), cols[4], AlignRight) + "│" +
		padCell("", cols[5], AlignLeft) + "│"
}

// Render returns the full table string (top border, rows, footer-separator,
// footer, bottom border). Returns "" when no rows have been added.
func (b *Builder) Render() string {
	if len(b.rows) == 0 {
		return ""
	}
	cols := b.effectiveCols()

	topBorder := hline(cols, '┌', '┬', '┐', '─', nil)
	rowSep := hline(cols, '├', '┼', '┤', '─', nil)
	// Footer separator: cols 0-2 merged (┴ at boundaries 0 and 1).
	footerSep := hline(cols, '├', '┼', '┤', '─', map[int]rune{0: '┴', 1: '┴'})
	// Bottom border: cols 0-2 merged (─ at boundaries 0 and 1, ┴ elsewhere).
	bottomBorder := hline(cols, '└', '┴', '┘', '─', map[int]rune{0: '─', 1: '─'})

	var sb strings.Builder
	sb.WriteString(topBorder)
	sb.WriteByte('\n')
	for i, row := range b.rows {
		sb.WriteString(renderRow(row, cols))
		sb.WriteByte('\n')
		if i < len(b.rows)-1 {
			sb.WriteString(rowSep)
			sb.WriteByte('\n')
		}
	}
	sb.WriteString(footerSep)
	sb.WriteByte('\n')
	sb.WriteString(b.buildFooterRow(cols))
	sb.WriteByte('\n')
	sb.WriteString(bottomBorder)
	sb.WriteByte('\n')
	return sb.String()
}

// buildFooterRow returns the footer line with "Total for request" label spanning
// the merged cols 0+1+2 region, and aggregated token/cost totals in cols 3-6.
func (b *Builder) buildFooterRow(cols [7]int) string {
	mergedWidth := cols[0] + 1 + cols[1] + 1 + cols[2]
	aggCacheStr := fmt.Sprintf("%s/%s",
		format.FormatK(b.aggCache[0]),
		format.FormatK(b.aggCache[1]),
	)
	return "│" +
		padCell("Total for request", mergedWidth, AlignLeft) + "│" +
		padCell(aggCacheStr, cols[3], AlignLeft) + "│" +
		padCell(format.FormatK(b.aggOut), cols[4], AlignRight) + "│" +
		padCell(b.costForAgg(), cols[5], AlignRight) + "│" +
		padCell("", cols[6], AlignLeft) + "│"
}
