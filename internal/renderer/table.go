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
	Width   int
	Content string
	Align   Align
}

// Row holds the seven fixed columns: #, role, model, cache, out, cost, tool/arg.
type Row [7]Cell

// Builder accumulates per-turn rows and renders the full R1-bordered table
// with a merged "Total for request" footer (§4.2 C-4 / C-5).
type Builder struct {
	cols     [7]int
	rows     []Row
	aggCache [2]int // [0]=total CacheRead, [1]=total CacheCreate
	aggOut   int    // total output tokens
}

// NewBuilder returns a Builder with default 80-column layout:
// [#=3, role=6, model=10, cache=13, out=6, cost=6, tool/arg=16+flex].
func NewBuilder() *Builder {
	return &Builder{cols: [7]int{3, 6, 10, 13, 6, 6, 16}}
}

// effectiveCols expands the last (flex) column so total rune-width is 80.
// Formula: 80 = sum(cols) + 8 borders  →  flex = 72 - sum(cols[0..5]).
func (b *Builder) effectiveCols() [7]int {
	c := b.cols
	fixed := 0
	for i := 0; i < 6; i++ {
		fixed += c[i]
	}
	flex := 72 - fixed
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

// Add appends one per-turn row built from a parser.Turn.
func (b *Builder) Add(t parser.Turn) {
	cols := b.effectiveCols()

	model := t.Model
	if strings.HasPrefix(model, "claude-") {
		model = model[len("claude-"):]
	}
	cache := fmt.Sprintf("%s/%s",
		format.FormatK(t.Tokens.CacheRead),
		format.FormatK(t.Tokens.CacheCreate),
	)

	row := Row{
		{Width: cols[0], Content: fmt.Sprintf("%d", t.Index), Align: AlignRight},
		{Width: cols[1], Content: t.Role, Align: AlignLeft},
		{Width: cols[2], Content: model, Align: AlignLeft},
		{Width: cols[3], Content: cache, Align: AlignLeft},
		{Width: cols[4], Content: format.FormatK(t.Tokens.Output), Align: AlignRight},
		{Width: cols[5], Content: "$0.00", Align: AlignRight},
		{Width: cols[6], Content: t.ToolUse, Align: AlignLeft},
	}
	b.rows = append(b.rows, row)
	b.aggCache[0] += t.Tokens.CacheRead
	b.aggCache[1] += t.Tokens.CacheCreate
	b.aggOut += t.Tokens.Output
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
		padCell("$0.00", cols[5], AlignRight) + "│" +
		padCell("", cols[6], AlignLeft) + "│"
}
