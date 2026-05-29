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
	b := renderer.NewBuilder(80)
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
	b := renderer.NewBuilder(80)
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
// TestTable_R1Borders_MultiRow — §6.5 B6 new order
//
// §6.5 B6 new order — top + footer + separator-split + 5 data rows + bottom = 9 lines.
// No inter-row separators.
//
// -------------------------------------------------------------------
func TestTable_R1Borders_MultiRow(t *testing.T) {
	b := renderer.NewBuilder(80)
	for i := 1; i <= 5; i++ {
		b.Add(makeTurn(i, "orch", "sonnet-4", "Edit", 500*i, 100*i, time.Duration(i)*time.Minute))
	}

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	lines := splitLines(out)
	// §6.5 B6 new order: top + footer + separator-split + 5 data rows + bottom = 9.
	// (No inter-row separators between data rows; rowSep removed.)
	const wantLines = 9
	if len(lines) != wantLines {
		t.Errorf("Render() with 5 turns: got %d lines; want %d\noutput:\n%s", len(lines), wantLines, out)
	}

	// Count lines that contain pipe-bordered row content (not border-only lines).
	// Row content lines start with '│' and contain no '─'.
	// Footer row also matches, so subtract 1.
	rowContentCount := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "│") && !strings.ContainsAny(l, "─") {
			rowContentCount++
		}
	}
	rowContentCount-- // footer row
	if rowContentCount != 5 {
		t.Errorf("expected 5 row-content lines (after footer subtraction); got %d\noutput:\n%s", rowContentCount, out)
	}

	// §6.5 B6: only one separator line (separator-split between footer and data rows).
	// It contains ┼ at col boundaries 2-5. No inter-row separators in data section.
	sepCount := 0
	for _, l := range lines {
		if strings.Contains(l, "┼") {
			sepCount++
		}
	}
	if sepCount < 1 {
		t.Errorf("expected at least 1 separator line (┼) (separator-split); got %d\noutput:\n%s", sepCount, out)
	}
}

// -------------------------------------------------------------------
// TestTable_MergedFooter — §6.5 B6 Merged footer-row — 3 left columns merged
//
// The footer line (line[1]) must contain label "Total for request".
// The separator-split (line[2]) separates footer from data rows:
// pattern ├───┬──────┬──────────┼... — ┬ at col boundaries 0/1 (sprout downward),
// ┼ at boundaries 2-5. At least 2 ┬ glyphs at the merge split points.
// -------------------------------------------------------------------
func TestTable_MergedFooter(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 5000, 300, 0))
	b.Add(makeTurn(2, "orch", "opus-4-7", "Edit", 2000, 100, 2*time.Minute))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string (RED: stub returns \"\")")
	}

	// §6.5 B6: footer label must be present.
	if !strings.Contains(out, "Total for request") {
		t.Errorf("Render() footer missing label \"Total for request\"\noutput:\n%s", out)
	}

	// Separator-split (between footer and data rows) must start with ├, end with ┤,
	// contain ┬ at col boundaries 0/1 (≥2 ┬) and ┼ at boundaries 2-5 (≥3 ┼).
	lines := splitLines(out)
	sepFound := false
	for _, l := range lines {
		if strings.HasPrefix(l, "├") && strings.Contains(l, "┬") {
			count := strings.Count(l, "┬")
			if count < 2 {
				t.Errorf("separator-split has %d ┬ glyph(s); want ≥2 for split cols 0/1\nline: %s", count, l)
			}
			sepFound = true
			break
		}
	}
	if !sepFound {
		t.Errorf("no separator-split line found (must start with ├ and contain ┬)\noutput:\n%s", out)
	}

	// The bottom border must start with └ and end with ┘.
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
	b := renderer.NewBuilder(80)
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
	b := renderer.NewBuilder(80)
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
	b := renderer.NewBuilder(80)
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
// TestBuilder_RenderForCols_NoTruncation — §4.3 T-9 cols=0 behaves identical to Render()
//
// RenderForCols(0) is the "no truncation" sentinel. The returned string must be
// bit-for-bit identical to Render() because cols=0 means "use builder default".
//
// PASS on stub: stub always delegates to Render(), so cols=0 matches by definition.
// This is a characterization invariant that must hold even after GREEN.
// -------------------------------------------------------------------
func TestBuilder_RenderForCols_NoTruncation(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	wantRender := b.Render()
	gotForCols := b.RenderForCols(0)

	// §4.3 T-9: RenderForCols(0) ≡ Render() bit-for-bit.
	if gotForCols != wantRender {
		t.Errorf("RenderForCols(0) must equal Render() bit-for-bit\nwant: %q\n got: %q", wantRender, gotForCols)
	}
}

// -------------------------------------------------------------------
// TestBuilder_RenderForCols_DropsLowPriorityNumericColumn — §4.3 T-9 cols=60 narrows table
//
// "dur" from concept §4.3 maps to the lowest-priority numeric column (cost in 7-col layout).
//
// The standard table at cols=80 is 80 runes wide. When cols=60 is requested,
// RenderForCols must drop the lowest-priority numeric column so that the
// rendered table fits within 60 columns.
// Assertion: every line of RenderForCols(60) is ≤ 60 runes wide.
// -------------------------------------------------------------------
func TestBuilder_RenderForCols_DropsLowPriorityNumericColumn(t *testing.T) {
	b := renderer.NewBuilder(80)
	// Add a representative turn.
	b.Add(makeTurn(1, "orch", "sonnet-4", "Edit", 5000, 300, 2*time.Minute))

	out := b.RenderForCols(60)
	if out == "" {
		t.Fatal("RenderForCols(60) returned empty string; want non-empty table")
	}

	// §4.3 T-9: every line must fit within the requested cols.
	for i, line := range splitLines(out) {
		w := runeWidth(stripANSI(line))
		if w > 60 {
			t.Errorf("line[%d] rune-width = %d; want ≤ 60 (dur/P3 column must be dropped)\nline: %s", i, w, line)
		}
	}
}

// -------------------------------------------------------------------
// TestBuilder_RenderForCols_MiddleTruncateToolArg — §4.3 T-9 cols=50 truncates tool/arg
//
// A Turn with a 20-rune ToolUse string is NOT truncated when rendered at the
// default 80-col builder (flex col ≥ 16 → inner ≥ 15, still fits 20 chars? No:
// inner=16-1=15 < 20, so it IS truncated at 80 cols inside the cell itself).
// We use a longer tool name (30 chars) that definitely fits at 80 cols
// (inner ≥ 27) but should be truncated when the column is squeezed at 50 cols.
//
// At cols=80: tool/arg flex = 80 - 44 - 8 = 28 → inner = 27. A 27-char tool
// fits without "…". At cols=50: the column must be squeezed → "…" appears.
//
// Stub always returns Render() (80-col full table) → no "…" for 27-char tool
// → test FAILS (RED).
// -------------------------------------------------------------------
func TestBuilder_RenderForCols_MiddleTruncateToolArg(t *testing.T) {
	// Exactly 27 runes: fits in flex=28 at cols=80 (inner=27), but must be
	// truncated at cols=50 when tool/arg column width is squeezed.
	toolName := strings.Repeat("x", 27) // "xxx...xxx" (27 chars)

	b := renderer.NewBuilder(80)
	b.Add(parser.Turn{
		Index:   1,
		Role:    "orch",
		Model:   "sonnet-4",
		Tokens:  parser.TokenCounts{Output: 100},
		ToolUse: toolName,
	})

	out := b.RenderForCols(50)
	if out == "" {
		t.Fatal("RenderForCols(50) returned empty string; want non-empty table")
	}

	// §4.3 T-9: tool/arg column must be middle-truncated with "…" at cols=50.
	if !strings.Contains(out, "…") {
		t.Errorf("RenderForCols(50) with 27-char ToolUse must produce middle-truncation (…); not found\noutput:\n%s", out)
	}

	// The full 27-char tool string must NOT appear verbatim (it was truncated).
	if strings.Contains(out, toolName) {
		t.Errorf("RenderForCols(50) must truncate 27-char ToolUse; full string found in output\noutput:\n%s", out)
	}
}

// -------------------------------------------------------------------
// TestBuilder_RenderForCols_AcceptOverflowIfTooNarrow — §4.3 T-9 cols=20 overflow accepted
//
// When cols is so small that no reasonable truncation fits (cols=20), the table
// accepts overflow rather than becoming illegible. The output must be the same as
// Render() (full table) — a "do nothing" path. This is an invariant: overflow is
// always better than illegible truncation.
//
// PASS on stub: stub returns Render() always, which is exactly the expected
// behaviour here. This is a characterization invariant.
// -------------------------------------------------------------------
func TestBuilder_RenderForCols_AcceptOverflowIfTooNarrow(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	wantFull := b.Render()
	gotNarrow := b.RenderForCols(20)

	// §4.3 T-9: at cols=20 (too narrow for any column-drop), output must equal full Render().
	if gotNarrow != wantFull {
		t.Errorf("RenderForCols(20) must accept overflow and equal Render()\nwant: %q\n got: %q", wantFull, gotNarrow)
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
	b := renderer.NewBuilder(80)
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

// -------------------------------------------------------------------
// T-21: TestRender_FooterFirst — §6.5.b6 footer appears as line[1],
// topBorder-merge has ┬ only at cols 2-5 (not 0/1), separator-split has
// ┬ at cols 0/1 and ┼ at cols 2-5.
// -------------------------------------------------------------------
func TestRender_FooterFirst(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))
	b.Add(makeTurn(2, "orch", "opus-4-7", "Edit", 2000, 100, time.Minute))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string; want non-empty table")
	}

	lines := splitLines(out)

	// New layout for 2 data rows:
	// line[0] topBorder-merge, line[1] footer, line[2] separator-split,
	// line[3] row2 (newest), line[4] row1 (oldest), line[5] bottomBorder.
	const wantLines = 6
	if len(lines) != wantLines {
		t.Errorf("Render() with 2 turns: got %d lines; want %d (new footer-first layout)\noutput:\n%s",
			len(lines), wantLines, out)
	}

	if len(lines) < 3 {
		t.Fatal("not enough lines to continue assertions")
	}

	// line[0]: topBorder-merge — '─' at col boundaries 0/1, '┬' at 2/3/4/5 → 4 ┬ total.
	topLine := lines[0]
	if !strings.HasPrefix(topLine, "┌") {
		t.Errorf("line[0] must start with '┌'; got %q", topLine)
	}
	if !strings.HasSuffix(topLine, "┐") {
		t.Errorf("line[0] must end with '┐'; got %q", topLine)
	}
	if got := strings.Count(topLine, "┬"); got != 4 {
		t.Errorf("line[0] (topBorder-merge) must have exactly 4 '┬' (cols 2-5); got %d\nline: %s", got, topLine)
	}

	// line[1]: footer row must contain "Total for request".
	if !strings.Contains(lines[1], "Total for request") {
		t.Errorf("line[1] must be the footer row containing \"Total for request\"; got %q", lines[1])
	}

	// line[2]: separator-split — '┬' at cols 0/1 (exactly 2), '┼' at cols 2-5 (≥3).
	sepLine := lines[2]
	if !strings.HasPrefix(sepLine, "├") {
		t.Errorf("line[2] (separator-split) must start with '├'; got %q", sepLine)
	}
	if !strings.HasSuffix(sepLine, "┤") {
		t.Errorf("line[2] (separator-split) must end with '┤'; got %q", sepLine)
	}
	if got := strings.Count(sepLine, "┬"); got != 2 {
		t.Errorf("line[2] (separator-split) must have exactly 2 '┬' (cols 0/1); got %d\nline: %s", got, sepLine)
	}
	if got := strings.Count(sepLine, "┼"); got < 3 {
		t.Errorf("line[2] (separator-split) must have ≥3 '┼' (cols 2-5); got %d\nline: %s", got, sepLine)
	}

	// last line: bottomBorder — starts '└', ends '┘', contains '┴' but not '┬'.
	lastLine := lines[len(lines)-1]
	if !strings.HasPrefix(lastLine, "└") {
		t.Errorf("last line must start with '└'; got %q", lastLine)
	}
	if !strings.HasSuffix(lastLine, "┘") {
		t.Errorf("last line must end with '┘'; got %q", lastLine)
	}
	if !strings.Contains(lastLine, "┴") {
		t.Errorf("bottomBorder must contain '┴'; got %q", lastLine)
	}
	if strings.Contains(lastLine, "┬") {
		t.Errorf("bottomBorder must NOT contain '┬'; got %q", lastLine)
	}
}

// -------------------------------------------------------------------
// T-22: TestRender_ReverseNoSep — §6.5.b6 data rows appear newest-first;
// no inter-row separators (┼) between data rows.
// -------------------------------------------------------------------
func TestRender_ReverseNoSep(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(10, "orch", "sonnet-4", "Read", 500, 50, 0))
	b.Add(makeTurn(20, "orch", "sonnet-4", "Write", 600, 60, 0))
	b.Add(makeTurn(30, "orch", "opus-4-7", "Edit", 700, 70, 0))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string; want non-empty table")
	}

	lines := splitLines(out)

	// New layout for 3 data rows:
	// line[0] top, line[1] footer, line[2] sep, line[3] row30, line[4] row20, line[5] row10, line[6] bottom = 7.
	const wantLines = 7
	if len(lines) != wantLines {
		t.Errorf("Render() with 3 turns: got %d lines; want %d (new footer-first layout)\noutput:\n%s",
			len(lines), wantLines, out)
	}

	// Find line indices for each data row.
	// The index cell is 3 runes wide; for 2-digit values like 10/20/30 the cell
	// is right-aligned producing "│10 │", "│20 │", "│30 │" — no leading space.
	findLine := func(marker string) int {
		for i, l := range lines {
			if strings.Contains(l, marker) {
				return i
			}
		}
		return -1
	}

	line30 := findLine("│30 ")
	line20 := findLine("│20 ")
	line10 := findLine("│10 ")

	if line30 == -1 || line20 == -1 || line10 == -1 {
		t.Fatalf("could not find all data rows: line30=%d line20=%d line10=%d\noutput:\n%s",
			line30, line20, line10, out)
	}

	// Newest first: row 30 must appear before row 20 before row 10.
	if !(line30 < line20 && line20 < line10) {
		t.Errorf("rows must appear newest-first (30 before 20 before 10); got line30=%d line20=%d line10=%d\noutput:\n%s",
			line30, line20, line10, out)
	}

	// No ┼ in data section (lines[3:] = data rows + bottom border).
	if len(lines) > 3 {
		for _, l := range lines[3:] {
			if strings.Contains(l, "┼") {
				t.Errorf("unexpected '┼' in data section (no inter-row separators expected): %q", l)
			}
		}
	}
}

// -------------------------------------------------------------------
// T-23: TestRender6Cols_NewOrder — §6.5.b6 6-col layout (RenderForCols(50))
// also uses footer-first / reverse / no-sep grammar.
// -------------------------------------------------------------------
func TestRender6Cols_NewOrder(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(5, "orch", "sonnet-4", "Read", 1000, 200, 0))
	b.Add(makeTurn(6, "orch", "opus-4-7", "Edit", 2000, 100, time.Minute))

	// 50 cols → 6-col layout: flex6 = (50-7) - 38 = 5 > 1, fits.
	out := b.RenderForCols(50)
	if out == "" {
		t.Fatal("RenderForCols(50) returned empty string; want non-empty table")
	}

	lines := splitLines(out)

	// Same line count as 7-col with 2 rows: 6 lines.
	const wantLines = 6
	if len(lines) != wantLines {
		t.Errorf("RenderForCols(50) with 2 turns: got %d lines; want %d (new footer-first layout)\noutput:\n%s",
			len(lines), wantLines, out)
	}

	if len(lines) < 4 {
		t.Fatal("not enough lines to continue assertions")
	}

	// line[1] must be footer row.
	if !strings.Contains(lines[1], "Total for request") {
		t.Errorf("line[1] must be footer row containing \"Total for request\"; got %q", lines[1])
	}

	// line[2] (separator-split) must have exactly 2 '┬' (cols 0/1).
	if got := strings.Count(lines[2], "┬"); got != 2 {
		t.Errorf("line[2] (separator-split) must have exactly 2 '┬'; got %d\nline: %s", got, lines[2])
	}

	// lines[3:] (data rows + bottom) must have no '┼'.
	for _, l := range lines[3:] {
		if strings.Contains(l, "┼") {
			t.Errorf("unexpected '┼' in data section: %q", l)
		}
	}

	// line[0] (topBorder-merge) must start with '┌' and have fewer '┬'
	// than a 7-col full-width topBorder (which has 4 '┬'); 6-col has 3 (cols 2-4, not 5).
	topLine := lines[0]
	if !strings.HasPrefix(topLine, "┌") {
		t.Errorf("line[0] must start with '┌'; got %q", topLine)
	}
	topJoins := strings.Count(topLine, "┬")
	if topJoins >= 4 {
		t.Errorf("line[0] (topBorder-merge) in 6-col layout must have fewer than 4 '┬'; got %d\nline: %s",
			topJoins, topLine)
	}
}
