package renderer

import "github.com/labzink/cc-probeline/internal/parser"

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

// Row holds the seven fixed columns of the per-turn table:
// #, role, model, cache, out, cost, tool/arg.
type Row [7]Cell

// Builder accumulates per-turn rows and renders the full R1-bordered table
// with the merged "Total for request" footer. Real implementation lands in
// Phase 4.2.b; this is a foundation stub.
type Builder struct {
	cols [7]int
	rows []Row
}

// NewBuilder returns a Builder pre-configured with the default 80-column
// layout widths (# 3, role 6, model 10, cache 13, out 6, cost 6, tool/arg 16+).
func NewBuilder() *Builder {
	return &Builder{cols: [7]int{3, 6, 10, 13, 6, 6, 16}}
}

// Add appends one per-turn row built from a parser.Turn. Stub is a no-op.
func (b *Builder) Add(_ parser.Turn) {}

// Render returns the full table string (top border, rows, footer-merge
// separator, footer row, bottom border). Stub returns "".
func (b *Builder) Render() string { return "" }
