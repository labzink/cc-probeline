package renderer

import (
	"regexp"
	"strings"

	"github.com/labzink/cc-probeline/internal/format"
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
	// AlignLeftFlush left-aligns content with NO leading space, flush against the
	// column's left border, filling the remaining width with trailing spaces.
	// Used for subagent "↳N" cells so the arrow always sits in the first position
	// regardless of single- vs multi-digit N (unlike AlignLeft, which indents 1).
	AlignLeftFlush
)

// Cell is one slot in the R1 box-drawing table.
type Cell struct {
	Content string
	Align   Align
}

// Row holds the seven fixed columns: #, role, model, cache, out, cost, tool/arg.
type Row [7]Cell

// Builder holds the terminal width used when constructing the per-turn table.
// The legacy Add/Render/RenderForCols API was removed in Phase 7 (BL-28);
// use NewBuilder + RenderUnifiedRows / RenderUnified instead.
type Builder struct {
	cols         [7]int
	terminalCols int // target terminal width (default 80)
}

// NewBuilder returns a Builder with default layout using the given terminal
// width (cols). If cols <= 0, defaults to 80.
// Column widths (Phase 6.9.e): [#=4, role=14, model=12, cache=13, out=7, cost=9, tool=13].
// Full table width: 4+14+12+13+7+9+13=72 content + 8 borders = 80.
func NewBuilder(cols int) *Builder {
	if cols <= 0 {
		cols = 80
	}
	return &Builder{
		cols:         [7]int{4, 14, 12, 13, 7, 9, 13},
		terminalCols: cols,
	}
}

// effectiveCols returns the column widths, widening the tool column from 13 to
// 33 (+20) when the terminal is wider than 100 columns. The extra room lets the
// subagent instance-name prefix ("<name≤16>: <tool>") fit (spec §2.3, T-30).
func (b *Builder) effectiveCols() [7]int {
	cols := b.cols
	if b.terminalCols > 100 {
		cols[6] = 33
	}
	return cols
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
	if a == AlignLeftFlush {
		// Flush left: no leading space; content fills the full width w with
		// trailing spaces. Returns exactly w visual chars so borders stay aligned.
		if w < 0 {
			w = 0
		}
		vlen := format.VisualLen(s)
		if vlen > w {
			prefix, core, suffix := splitWrapMarkers(s)
			core = format.MiddleTruncate(format.StripMarkers(core), w)
			if format.VisualLen(core) > w {
				runes := []rune(core)
				core = string(runes[:w])
			}
			s = prefix + core + suffix
			vlen = format.VisualLen(s)
		}
		return s + strings.Repeat(" ", w-vlen)
	}
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
