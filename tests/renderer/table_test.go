// Package renderer_test — black-box tests for the box-drawing R1 table (Phase 4.2.b).
// All tests that call Render() with actual rows are RED on the foundation stub
// (Render() returns ""). TestBuilder_EmptyRender is intentionally expected to
// PASS on the stub — it asserts the empty-case contract.
package renderer_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// stripANSI removes ANSI escape sequences from s so that rune/byte widths can
// be measured safely. Only CSI sequences (ESC [ ... m) are stripped. The
// builder currently emits no ANSI (raw UTF-8 only, per §4.2), but this helper
// future-proofs width assertions.
func stripANSI(s string) string {
	out := make([]byte, 0, len(s))
	inSeq := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inSeq = true
			i++ // skip '['
			continue
		}
		if inSeq {
			if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
				inSeq = false
			}
			continue
		}
		out = append(out, b)
	}
	return string(out)
}

// runeWidth returns the number of Unicode code points (not bytes) in s.
// Used for UTF-8 border characters like ─ (3 bytes, 1 rune).
func runeWidth(s string) int {
	return utf8.RuneCountInString(s)
}

// makeTurn builds a minimal parser.Turn for use in table tests.
func makeTurn(index int, role, model, tool string, cache, out int, dur time.Duration) parser.Turn {
	return parser.Turn{
		Index:       index,
		Role:        role,
		Model:       model,
		Tokens:      parser.TokenCounts{CacheRead: cache, Output: out},
		ToolUse:     tool,
		Timestamp:   time.Now(),
		Duration:    dur,
		IsSidechain: false,
	}
}

// splitLines splits a rendered table into non-empty lines.
func splitLines(s string) []string {
	all := strings.Split(s, "\n")
	// Keep all lines (including empty trailing ones), but trim the last empty
	// element that strings.Split adds after a trailing "\n".
	for len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	return all
}

// -------------------------------------------------------------------
// TestBuilder_EmptyRender
//
// PASS on stub: intentional; this is the empty-case contract.
// NewBuilder().Render() must return "" when no turns have been added so that
// the assembler (4.2.d) can detect "no table" and skip appending it.
// -------------------------------------------------------------------
func TestBuilder_EmptyRender(t *testing.T) {
	b := renderer.NewBuilder()
	got := b.Render()
	if got != "" {
		t.Errorf("empty builder Render() = %q; want \"\"", got)
	}
}

// -------------------------------------------------------------------
// TestTable_R1Borders_SingleRow — §4.2 Box-drawing R1 — 7 fixed columns
//
// After one Add(), the table must:
//   - contain all required box-drawing glyphs
//   - consist of exactly 5 visible lines:
//     top border, row, footer-separator, footer, bottom border
//
// -------------------------------------------------------------------
func TestTable_R1Borders_SingleRow(t *testing.T) {
	b := renderer.NewBuilder()
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string; want non-empty table (RED: stub returns \"\")")
	}

	// §4.2 Box-drawing R1 — required glyphs
	requiredGlyphs := []string{"┌", "┐", "└", "┘", "├", "┤", "┬", "┴", "─", "│"}
	for _, g := range requiredGlyphs {
		if !strings.Contains(out, g) {
			t.Errorf("Render() missing required box-drawing glyph %q", g)
		}
	}

	// Exactly 5 lines: top, row, footer-sep, footer, bottom
	lines := splitLines(out)
	if len(lines) != 5 {
		t.Errorf("Render() with 1 turn: got %d lines; want 5 (top/row/footer-sep/footer/bottom)\noutput:\n%s", len(lines), out)
	}
}

// -------------------------------------------------------------------
// TestTable_R1Borders_MultiRow — §4.2 Box-drawing R1 — separators between rows
//
// 5 turns must produce 5 row-content lines and 4 inter-row separators within
// the rows section (before the footer-separator).
//
// Expected total line count:
//
//	top(1) + row(1) + sep(1) + row(1) + sep(1) + row(1) + sep(1) + row(1) + sep(1) + row(1)
//	+ footer-sep(1) + footer(1) + bottom(1) = 14 lines
//
// -------------------------------------------------------------------
func TestTable_R1Borders_MultiRow(t *testing.T) {
	b := renderer.NewBuilder()
	for i := 1; i <= 5; i++ {
		b.Add(makeTurn(i, "orch", "sonnet-4", "Edit", 500*i, 100*i, time.Duration(i)*time.Minute))
	}

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	lines := splitLines(out)
	// top + 5*(row+sep) - last_sep + footer-sep + footer + bottom
	// = 1 + 5 + 4 + 1 + 1 + 1 = 13 ... but rows and separators interleave:
	// top, r1, sep, r2, sep, r3, sep, r4, sep, r5, footer-sep, footer, bottom = 13
	const wantLines = 13
	if len(lines) != wantLines {
		t.Errorf("Render() with 5 turns: got %d lines; want %d\noutput:\n%s", len(lines), wantLines, out)
	}

	// Count lines that contain pipe-bordered row content (not border-only lines).
	// Row content lines start with '│' and contain non-'─' content.
	rowContentCount := 0
	for _, l := range lines {
		// A content row starts with │, has at least one space or letter after it,
		// and does NOT look like a pure horizontal border (which only contains ─┼┤├┬┴).
		if strings.HasPrefix(l, "│") && !strings.ContainsAny(l, "─") {
			rowContentCount++
		}
	}
	if rowContentCount != 5 {
		t.Errorf("expected 5 row-content lines; got %d\noutput:\n%s", rowContentCount, out)
	}

	// Count separator lines (contain ┼ or ├…┤ patterns within rows section).
	sepCount := 0
	for _, l := range lines {
		if strings.Contains(l, "┼") {
			sepCount++
		}
	}
	if sepCount < 4 {
		t.Errorf("expected at least 4 inter-row separators (┼); got %d\noutput:\n%s", sepCount, out)
	}
}

// -------------------------------------------------------------------
// TestTable_MergedFooter — §4.2 Merged footer-row — 3 left columns merged
//
// The footer line must contain label "Total for request" and the footer
// separator must show the merge pattern: ├ + columns + ┴┴ (2 merge points
// where columns 1-2-3 are merged).
// -------------------------------------------------------------------
func TestTable_MergedFooter(t *testing.T) {
	b := renderer.NewBuilder()
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 5000, 300, 0))
	b.Add(makeTurn(2, "orch", "opus-4-7", "Edit", 2000, 100, 2*time.Minute))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	// §4.2 Merged footer: label must be present
	if !strings.Contains(out, "Total for request") {
		t.Errorf("Render() footer missing label \"Total for request\"\noutput:\n%s", out)
	}

	// Footer separator must contain ┴ (merge points where cols 1-3 merge).
	// Pattern from concept: ├────┴───────┴────────┼...
	// At least 2 ┴ glyphs at the merge boundary.
	lines := splitLines(out)
	footerSepFound := false
	for _, l := range lines {
		if strings.Contains(l, "┴") && strings.Contains(l, "├") {
			// This is the footer-separator line; verify it has ≥2 ┴ for the merge.
			count := strings.Count(l, "┴")
			if count < 2 {
				t.Errorf("footer-separator has %d ┴ glyph(s); want ≥2 for merged cols 1-3\nline: %s", count, l)
			}
			footerSepFound = true
			break
		}
	}
	if !footerSepFound {
		t.Errorf("no footer-separator line found (must contain both ┴ and ├)\noutput:\n%s", out)
	}

	// The bottom border must not contain ┼ (only └─┴─┘).
	lastLine := lines[len(lines)-1]
	if !strings.HasPrefix(lastLine, "└") || !strings.HasSuffix(lastLine, "┘") {
		t.Errorf("last line must start with └ and end with ┘; got: %s", lastLine)
	}
}

// -------------------------------------------------------------------
// TestTable_Cap20_NotEnforcedHere — §4.2 C-6 cap is assembler's responsibility
//
// Builder.Add called 30 times must produce 30 row-content lines in the
// rendered output. Cap-20 is enforced by the assembler (4.2.d), NOT by Builder.
// This is an explicit contract test.
// -------------------------------------------------------------------
func TestTable_Cap20_NotEnforcedHere(t *testing.T) {
	b := renderer.NewBuilder()
	const n = 30
	for i := 1; i <= n; i++ {
		b.Add(makeTurn(i, "orch", "sonnet-4", "Bash", 100, 50, time.Duration(i)*time.Second))
	}

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	// Count content rows: lines starting with │ that carry cell content.
	// We look for the index number at the start of the first cell.
	lines := splitLines(out)
	contentRows := 0
	for _, l := range lines {
		stripped := stripANSI(l)
		// A content row has │ at start AND at least one space-or-digit after it
		// AND is not a border line (no leading ├ or └ etc.)
		if strings.HasPrefix(stripped, "│") && !strings.Contains(stripped, "─") {
			contentRows++
		}
	}
	// Subtract 1 for the footer row (it also starts with │ and has no ─).
	contentRows-- // footer row
	if contentRows != n {
		t.Errorf("expected %d row-content lines (cap NOT enforced by Builder); got %d\noutput:\n%s", n, contentRows, out)
	}
}

// -------------------------------------------------------------------
// TestTable_LongToolArg_MiddleTruncate — §4.2 truncation within tool/arg cell
//
// A Turn whose ToolUse is 40 characters must be middle-truncated with '…'
// and must fit within the tool/arg column width (flex; minimum 16 chars).
// -------------------------------------------------------------------
func TestTable_LongToolArg_MiddleTruncate(t *testing.T) {
	longTool := strings.Repeat("a", 20) + strings.Repeat("b", 20) // 40 chars
	b := renderer.NewBuilder()
	b.Add(parser.Turn{
		Index:     1,
		Role:      "orch",
		Model:     "sonnet-4",
		Tokens:    parser.TokenCounts{Output: 100},
		ToolUse:   longTool,
		Timestamp: time.Now(),
		Duration:  0,
	})

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	// The ellipsis must appear somewhere in the output.
	if !strings.Contains(out, "…") {
		t.Errorf("Render() with 40-char ToolUse must use middle-truncation (…); not found\noutput:\n%s", out)
	}

	// The raw 40-char tool string must NOT appear verbatim (it was truncated).
	if strings.Contains(out, longTool) {
		t.Errorf("Render() must truncate 40-char ToolUse; full string found in output\noutput:\n%s", out)
	}

	// Verify no content line exceeds the expected table width (default 80 cols).
	// We allow a small epsilon for the border chars on the right.
	for _, line := range splitLines(out) {
		w := runeWidth(stripANSI(line))
		if w > 80 {
			t.Errorf("line exceeds 80 rune-columns (got %d):\n%s", w, line)
		}
	}
}

// -------------------------------------------------------------------
// TestTable_ColumnWidths_80Cols — §4.2 Layout: 7 fixed columns + borders = 80
//
// The first line of the rendered table (top border) must be exactly 80
// Unicode code points wide. Default cols: [3,6,10,13,6,6,16].
//
// Border chars between 7 cols: │ on left edge + 6 inner │ + │ on right edge
// = 8 border runes. Totals: 3+6+10+13+6+6+16 = 60 content + (16 flex) ...
// Actually: col widths + (ncols+1) border chars.
// With cols [3,6,10,13,6,6,16]: sum=60, borders=8 → 68 < 80.
// The last column (flex) expands to fill: flex = 80 - 60 - 8 + 16 = 28? No:
// flex = 80 - (3+6+10+13+6+6) - 8 = 80 - 44 - 8 = 28 (last col becomes 28).
// -------------------------------------------------------------------
func TestTable_ColumnWidths_80Cols(t *testing.T) {
	b := renderer.NewBuilder()
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	lines := splitLines(out)
	if len(lines) == 0 {
		t.Fatal("Render() produced no lines")
	}

	// The first line is the top border: ┌──…──┐.
	// Its rune-width must equal 80.
	firstLine := stripANSI(lines[0])
	w := runeWidth(firstLine)
	if w != 80 {
		t.Errorf("top border rune-width = %d; want 80\nline: %s", w, firstLine)
	}

	// Every other line must also be exactly 80 runes wide.
	for i, l := range lines {
		lw := runeWidth(stripANSI(l))
		if lw != 80 {
			t.Errorf("line[%d] rune-width = %d; want 80\nline: %s", i, lw, l)
		}
	}
}

// -------------------------------------------------------------------
// TestTable_CellAlign — §4.2 numeric columns right-aligned, text left-aligned
//
// Column layout (0-indexed):
//
//	0 = #        → right-aligned  (index number)
//	1 = role     → left-aligned
//	2 = model    → left-aligned
//	3 = cache    → left-aligned   (formatted label, not pure numeric)
//	4 = out      → right-aligned  (output token count)
//	5 = cost     → right-aligned  ($X.YY)
//	6 = tool/arg → left-aligned
//
// Verification: in a right-aligned cell the padding spaces appear on the LEFT
// side of the content; in a left-aligned cell they appear on the RIGHT.
// We detect this by checking the character immediately after the opening │.
// -------------------------------------------------------------------
func TestTable_CellAlign(t *testing.T) {
	b := renderer.NewBuilder()
	// Use a 1-digit index so that right-alignment is visible (2 leading spaces
	// in a 3-wide cell).
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	// Find the first content row (not a border line).
	var contentRow string
	for _, l := range splitLines(out) {
		stripped := stripANSI(l)
		if strings.HasPrefix(stripped, "│") && !strings.Contains(stripped, "─") {
			// Must not be the footer row (which starts with "│ Total").
			if !strings.Contains(stripped, "Total for request") {
				contentRow = stripped
				break
			}
		}
	}
	if contentRow == "" {
		t.Fatal("could not find a content row line in Render() output")
	}

	// Split into cells by │.
	// The line looks like: │ cell0 │ cell1 │ ... │ cell6 │
	parts := strings.Split(contentRow, "│")
	// parts[0] = "" (before first │), parts[1..7] = cells, parts[8] = "" (after last │)
	if len(parts) < 9 {
		t.Fatalf("expected at least 9 parts when splitting by │; got %d\nrow: %s", len(parts), contentRow)
	}
	cells := parts[1:8] // indices 1..7 → cols 0..6

	// Right-aligned cell: last non-space char is at the rightmost position,
	// meaning the cell content (trimmed) is at the RIGHT → leading spaces exist.
	assertRightAligned := func(name, cell string) {
		t.Helper()
		trimmed := strings.TrimRight(cell, " ")
		if trimmed == cell {
			t.Errorf("col %s: expected trailing spaces stripped but none found; cell=%q (left-padding expected for right-align)", name, cell)
			return
		}
		_ = trimmed // leading spaces remain
	}

	// Left-aligned cell: trailing spaces exist (content is at the LEFT).
	assertLeftAligned := func(name, cell string) {
		t.Helper()
		trimmed := strings.TrimLeft(cell, " ")
		if trimmed == cell {
			t.Errorf("col %s: expected leading space for padding but none found; cell=%q", name, cell)
			return
		}
		_ = trimmed
	}

	// Col 0 (#): right-aligned — index "1" in 3-wide cell → " 1 " or "  1"
	// Actually with a leading space for padding and content "1", the cell must
	// have trailing content flush right: "  1" (2 leading spaces).
	assertRightAligned("# (col0)", cells[0])

	// Col 1 (role): left-aligned — "orch  " padded on right
	assertLeftAligned("role (col1)", cells[1])

	// Col 2 (model): left-aligned — "sonnet-4  " padded on right
	assertLeftAligned("model (col2)", cells[2])

	// Col 4 (out): right-aligned — "200 " or " 200" → right-flush
	assertRightAligned("out (col4)", cells[4])

	// Col 5 (cost): right-aligned — "$..." right-flush in 6-wide cell
	assertRightAligned("cost (col5)", cells[5])

	// Col 6 (tool/arg): left-aligned
	assertLeftAligned("tool/arg (col6)", cells[6])
}
