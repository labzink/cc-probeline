// Package statusline_test — RED tests for Phase 6.9 FIXES group G-render.
//
// Covers F1 (leading border of fresh data rows must be dim), F2 (legendSep right
// corner must be ┤ not ┐), and F9 (cache r/w cell: read pinned left, write
// pinned right within the column width).
//
// All tests go through the production path: Assembler.Render → perTurnTable →
// RenderUnifiedRows. No direct calls to RenderUnified / RenderUnifiedRows.
//
// Helpers reused from assembler_test.go (same package):
//
//	orchTurn, renderWithTurns, dataRows, groupSepLines, tableLines, stripMk,
//	makeStdAssembler.
//
// New helpers are prefixed f1/f2/f9 to avoid name collisions.
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// F1: TestF1_FreshRowLeadingBarIsDim
//
// Spec §2.3 T-6 (reinforced by F1 review): ALL vertical bars of data rows —
// including the LEADING (leftmost) border — must use the dimBar pattern
// ({{dim}}│{{reset}}), not a plain │ rune.
//
// Current code (renderUnifiedDataRow, table_unified.go:150):
//
//	sb.WriteRune('│')   ← plain rune, not dimBar
//
// The existing TestAssemble_DataBarsDim has a blind spot: it passes when ≥1
// {{dim}}│{{reset}} appears anywhere in the row, so it accepts rows that start
// with a plain │ followed by dim inner bars. This test explicitly checks the
// LEADING position.
//
// Contract:
//   - After stripping any whole-row {{dim}}…{{reset}} wrapper (dim/history rows),
//     a fresh (non-dim) data row must NOT start with a plain │ — its first rune
//     after stripping row-level markers must still be a {{dim}}│{{reset}} sequence.
//   - Equivalently: the raw (un-stripped) fresh row must start with "{{dim}}│{{reset}}"
//     (i.e. the dimBar) rather than with "│".
//
// This test FAILS on current code (plain │ at position 0 of fresh row).
// ---------------------------------------------------------------------------
func TestF1_FreshRowLeadingBarIsDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Single orchestrator turn in the freshest group (maxGroupID=1).
	// No history → the row is a fresh (non-dim) row.
	turn := orchTurn(1, "f1-fresh", 1, base, "BashF1", "")

	out := renderWithTurns(t, []parser.Turn{turn}, nil, base.Add(time.Minute), 5)

	rows := dataRows(out)
	if len(rows) == 0 {
		t.Fatalf("F1: no data rows in output\noutput:\n%s", out)
	}

	// Identify the fresh (non-dim) row. A dim/history row starts with "{{dim}}".
	// The fresh row must NOT start with "{{dim}}" at the row level.
	freshRowFound := false
	for _, row := range rows {
		if strings.HasPrefix(row, "{{dim}}") {
			// History/whole-dim row — skip (its leading │ is inside the dim wrapper,
			// which is the correct treatment for history rows).
			continue
		}
		freshRowFound = true

		// F1 contract: the leading border of a fresh data row must be
		// {{dim}}│{{reset}}, not a bare │.
		// A bare │ at position 0 means the leading border is un-dimmed.
		if strings.HasPrefix(row, "│") {
			t.Errorf("F1: fresh data row starts with plain '│' (leading border not dim);\n"+
				"  expected the row to begin with '{{dim}}│{{reset}}' (dimBar).\n"+
				"  Fix: renderUnifiedDataRow must write dimBar for the leading border.\n"+
				"  row: %s", row)
		}

		// Additionally verify that the row DOES contain "{{dim}}│{{reset}}" somewhere
		// (inner bars are already dim per existing GREEN).
		if !strings.Contains(row, "{{dim}}│{{reset}}") {
			t.Errorf("F1: fresh data row contains no {{dim}}│{{reset}} bar at all;\n"+
				"  row: %s", row)
		}
	}

	if !freshRowFound {
		t.Fatalf("F1: no non-dim (fresh) data rows found; all rows are history-wrapped\noutput:\n%s", out)
	}
}

// f1MultiGroupFreshRow returns the fresh (topmost, non-dim) data row from a
// two-group output. The second group is the fresh one (maxGroupID=2).
func f1MultiGroupFreshRow(t *testing.T, out string) string {
	t.Helper()
	rows := dataRows(out)
	for _, row := range rows {
		if !strings.HasPrefix(row, "{{dim}}") {
			return row
		}
	}
	return ""
}

// TestF1_FreshRowLeadingBarIsDim_MultiGroup exercises F1 with two orch groups so
// the code path that decides "this is the fresh group" is exercised (maxGroupID
// detection). The fresh row (GroupID=2) must have a dim leading border; the
// history row (GroupID=1) is wrapped whole in {{dim}} and is not the subject of
// this test.
func TestF1_FreshRowLeadingBarIsDim_MultiGroup(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// GroupID=2 is current (max), GroupID=1 is history.
	turnCur := orchTurn(2, "f1-cur", 2, base.Add(10*time.Second), "EditF1", "")
	turnHist := orchTurn(1, "f1-hist", 1, base, "ReadF1", "")

	// Newest-first order.
	out := renderWithTurns(t, []parser.Turn{turnCur, turnHist}, nil, base.Add(2*time.Minute), 5)

	if !strings.Contains(out, "┌") {
		t.Fatalf("F1-multi: no table in output\noutput:\n%s", out)
	}

	freshRow := f1MultiGroupFreshRow(t, out)
	if freshRow == "" {
		t.Fatalf("F1-multi: no fresh (non-dim) data row found\noutput:\n%s", out)
	}

	// The fresh row (containing "EditF1") must NOT start with plain │.
	if strings.HasPrefix(freshRow, "│") {
		t.Errorf("F1-multi: fresh data row (GroupID=2) starts with plain '│';\n"+
			"  expected '{{dim}}│{{reset}}' (dimBar) as leading border.\n"+
			"  row: %s", freshRow)
	}

	// It must also contain "EditF1" (sanity: we have the right row).
	if !strings.Contains(freshRow, "EditF1") {
		t.Errorf("F1-multi: fresh row does not contain 'EditF1'; got: %s", freshRow)
	}
}

// ---------------------------------------------------------------------------
// F2: TestF2_LegendSepEndsWithCornerTee
//
// Spec §2.3 T-5: the separator immediately before the legend row must be a
// continuous ├─┼─┤ line (right corner = ┤, "corner tee"), not ├─┼─┐ (right
// corner = ┐, which creates a visual gap in the border).
//
// Current code (RenderUnifiedRows, table_unified.go:88):
//
//	legendSep := hlineSlice(colWidths, '├', '┼', '┐', '─', nil)  ← ┐ is wrong
//
// After the fix '┐' must be replaced with '┤'.
//
// The existing TestAssemble_LegendSeparator only checks that the line before the
// legend starts with ├ and contains ┼ — it does NOT check the right corner rune.
// This test explicitly asserts the right corner.
//
// This test FAILS on current code (right corner is ┐, not ┤).
// ---------------------------------------------------------------------------
func TestF2_LegendSepEndsWithCornerTee(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turn := orchTurn(1, "f2-t1", 1, base, "BashF2", "")
	out := renderWithTurns(t, []parser.Turn{turn}, nil, base.Add(time.Minute), 5)

	if !strings.Contains(out, "┌") {
		t.Fatalf("F2: no table in output\noutput:\n%s", out)
	}

	lines := strings.Split(out, "\n")

	// Find the legend row (contains "role" and "model" labels).
	legendIdx := -1
	for i, l := range lines {
		bare := stripMk(l)
		if strings.Contains(bare, " role ") && strings.Contains(bare, " model ") {
			legendIdx = i
			break
		}
	}
	if legendIdx < 0 {
		t.Fatalf("F2: legend row not found\noutput:\n%s", out)
	}
	if legendIdx == 0 {
		t.Fatalf("F2: legend row is first line, no room for separator above")
	}

	// The line immediately before the legend must be the legendSep.
	legendSepLine := lines[legendIdx-1]
	bareLegendSep := stripMk(legendSepLine)

	// Must start with ├ and contain ┼ (as existing TestAssemble_LegendSeparator checks).
	if !strings.HasPrefix(bareLegendSep, "├") {
		t.Errorf("F2: line before legend must start with '├'; got: %s", legendSepLine)
	}
	if !strings.Contains(bareLegendSep, "┼") {
		t.Errorf("F2: line before legend must contain '┼'; got: %s", legendSepLine)
	}

	// F2 contract: the last non-space rune of the legendSep (inside any {{reset}}
	// wrapper) must be ┤, NOT ┐.
	//
	// After stripping markers the legendSep bare form must end with ┤.
	if !strings.HasSuffix(bareLegendSep, "┤") {
		t.Errorf("F2: legendSep right corner must be '┤' (continuous border), got trailing: %q\n"+
			"  Current code emits '┐' which creates a visual gap in the table border.\n"+
			"  Fix: change hlineSlice rightCorner arg from '┐' to '┤' (table_unified.go:88).\n"+
			"  legendSep line: %s", bareLegendSep[max(0, len(bareLegendSep)-4):], legendSepLine)
	}

	// Also verify the LAST rune of the full legendSep line (raw, before strip).
	// It may be wrapped with {{reset}} but the rune before {{reset}} or at end
	// must be ┤ in the visible content.
	// Already covered by the bare check above; add as belt+suspenders.
	// The group separators (├─┼─┤) already end with ┤ — the legend sep is the
	// only one with the wrong corner in current code.

	// Sanity (notch contract): after the notch redesign, standalone inter-group
	// ├─┼─┤ separator lines between data rows are REMOVED. The only remaining
	// pure horizontal ├…┼…┤ line is the legend separator itself (checked above).
	// groupSepLines returns lines that start with ├, contain ┼, end with ┤, and
	// have no spaces (pure horizontal). After the redesign, that set is exactly
	// {legendSep}. So groupSepLines must return exactly 1 line (the legend sep
	// we already found), not 2+ (old code emitted groupSep + legendSep).
	//
	// NOTE: legendSep currently ends with ┐ (F2 bug), so groupSepLines (which
	// checks for ┤ suffix) may return 0 until F2 is also fixed. This sanity
	// check documents the combined F2 + notch contract.
	groupSeps := groupSepLines(out)
	// After both F2 and notch are fixed: exactly 1 pure horizontal ├…┼…┤ line
	// (the legend sep), no standalone inter-group separators.
	// Before F2: legendSep ends with ┐ so groupSepLines returns 0.
	// Before notch: groupSepLines returns the inter-group sep count.
	// Either way, after fix: exactly 1 (legend sep only).
	if len(groupSeps) > 1 {
		t.Errorf("F2 sanity (notch contract): found %d standalone ├─┼─┤ lines;\n"+
			"  after notch redesign only the legend separator should remain (1 line).\n"+
			"  Old code emits an inter-group separator per boundary — those must be removed.\n"+
			"  lines:\n%s", len(groupSeps), strings.Join(groupSeps, "\n"))
	}
	// Also verify each remaining line ends with ┤ (covers F2 legend corner fix).
	for _, gs := range groupSeps {
		bareGs := stripMk(gs)
		if !strings.HasSuffix(bareGs, "┤") {
			t.Errorf("F2 sanity: remaining ├…┼…┤ line must end with '┤'; got: %s", gs)
		}
	}
}

// ---------------------------------------------------------------------------
// F9: TestF9_CacheRWColumnAlignment
//
// Spec §2.3 / F9 review: in the cache r/w column (width=13, inner=12):
//   - read sub-token is pinned to the LEFT (1-space left margin, as in padCell).
//   - write sub-token is pinned to the RIGHT (1-space margin from right edge).
//
// Current code (cacheRWCell, table_unified.go:175-182):
//
//	return readStr + "/" + writeStr  ← joined by "/" and left-aligned (no split)
//
// After the fix the visible content of the cell (as extracted from the row)
// must show readStr near the left edge and writeStr near the right edge of the
// 13-wide column.
//
// Column layout (T-32): cache is column index 3 with width=13.
//   - Inner usable width = 12 (padCell adds 1-space margin on each side).
//   - Cell visible chars = 13, of which positions 0..11 are usable content
//     (0=leading space, 11=content), position 12 is trailing space.
//   - With read="100" (3 chars), write="2K" (2 chars), spaces = 12-3-2 = 7:
//     visible cell = " 100       2K " (total 14? No — AlignLeft = " "+content+pad).
//
// padCell(AlignLeft, 13) returns " " + content + pad where len(content)+pad = 12.
// New cell content (12 visible) = readStr + spaces(12-3-2) + writeStr
//
//	= "100" + "       " + "2K" = "100       2K" (12 chars).
//
// After padCell(AlignLeft, 13): " 100       2K" = 13 visible (no trailing space because
// inner=12 and content already fills all 12 chars... wait, pad = 12-12 = 0):
//
//	" " + "100       2K" + "" = " 100       2K" (13 chars visible).
//
// Actually padCell(AlignLeft, w=13): inner=12; content visible len = 12; pad = 0;
// result = " " + "100       2K" = 13 chars. Trailing space = 0 — that's fine.
//
// The test strategy:
//  1. Build a turn with CacheRead=100 (→"100") and CacheCreate=2000 (→"2K").
//  2. Find the fresh data row containing the orch turn.
//  3. Extract the cache cell substring from the row (between the 3rd and 4th │).
//  4. Verify that the visible (marker-stripped) cell has "100" at position 1
//     (1-space left margin) and "2K" at the rightmost position before the
//     trailing-space margin.
//
// This test FAILS on current code which produces "100/2K" left-aligned (read
// and write joined by "/" with no right-justification of write).
// ---------------------------------------------------------------------------
func TestF9_CacheRWColumnAlignment(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// CacheRead=100 → FormatK="100" (3 chars)
	// CacheCreate=2000 → FormatK="2K" (2 chars)
	// Cache column width=13, inner=12.
	// Expected visible cell (13 chars): " 100       2K" (read=left, write=right, no trailing space
	// because content fills inner exactly).
	// Or with trailing space: " 100      2K " if inner > content.
	// Let's compute: inner=12, readLen=3, writeLen=2, spacesNeeded=12-3-2=7.
	// content = "100" + 7spaces + "2K" = 12 chars. padCell adds leading " " = 13.
	//
	// The test checks positions, not the exact spacing, to be resilient to minor
	// format tweaks: read at [1] and write ending at [12] (0-indexed, before last char).

	turn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "f9-t1",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{CacheRead: 100, CacheCreate: 2000, Output: 50},
		ToolUse:     "ReadF9",
		IsSidechain: false,
	}

	out := renderWithTurns(t, []parser.Turn{turn}, nil, base.Add(time.Minute), 5)

	rows := dataRows(out)
	if len(rows) == 0 {
		t.Fatalf("F9: no data rows in output\noutput:\n%s", out)
	}

	// Find the fresh row (non-dim).
	freshRow := ""
	for _, row := range rows {
		if !strings.HasPrefix(row, "{{dim}}") {
			freshRow = row
			break
		}
	}
	if freshRow == "" {
		t.Fatalf("F9: no fresh data row found\noutput:\n%s", out)
	}

	// Extract the cache cell. The row format (stripped) is:
	//   │ hashCell │ roleCell │ modelCell │ cacheCell │ outCell │ costCell │ toolCell │
	// Split bare row on │ to get individual cells (index 3 = cache cell, 0-based after
	// leading │).
	bareRow := stripMk(freshRow)
	// Remove any dim/reset wrappers first (fresh rows shouldn't have row-level dim,
	// but guard anyway).
	cells := strings.Split(bareRow, "│")
	// cells[0] is empty (before leading │), cells[1]=hash, [2]=role, [3]=model,
	// [4]=cache, [5]=out, [6]=cost, [7]=tool, [8]=empty (after trailing │).
	if len(cells) < 5 {
		t.Fatalf("F9: expected at least 5 │-separated segments in row, got %d\n"+
			"  bare row: %q\n  raw row: %s", len(cells), bareRow, freshRow)
	}

	cacheCell := cells[4] // index 4 = cache column (0=empty, 1=hash, 2=role, 3=model, 4=cache)

	// F9 contract: readStr is at the left of the cache cell (position 1, after 1-space margin).
	// writeStr is at the right of the cache cell (before the trailing 1-space margin or at end).
	readStr := "100" // FormatK(100)
	writeStr := "2K" // FormatK(2000)

	// Check read is near the left (starts at position 1 in the cell).
	readPos := strings.Index(cacheCell, readStr)
	if readPos < 0 {
		t.Errorf("F9: readStr %q not found in cache cell %q", readStr, cacheCell)
	} else if readPos > 2 {
		// Allow position 1 (AlignLeft margin) — if it's at 0 or > 2 it's wrong.
		t.Errorf("F9: readStr %q must be near left of cache cell (position ≤ 2), got position %d\n"+
			"  cache cell: %q", readStr, readPos, cacheCell)
	}

	// Check write is near the right of the cache cell.
	// The cell is 13 visible chars wide. writeStr ends at position 12 or 13 (0-indexed).
	writePos := strings.LastIndex(cacheCell, writeStr)
	if writePos < 0 {
		t.Errorf("F9: writeStr %q not found in cache cell %q", writeStr, cacheCell)
	} else {
		// write must be right-justified: it must end at a position close to the right
		// edge of the cell (within 1 char of end for 1-space margin).
		// Cell width = 13, writeStr len = 2.
		// Right-justified: writePos + len(writeStr) should be ≥ cellLen-2 (allowing 1-space margin).
		cellVisLen := len([]rune(cacheCell))
		writeEnd := writePos + len([]rune(writeStr))
		if writeEnd < cellVisLen-2 {
			t.Errorf("F9: writeStr %q is NOT right-justified in cache cell;\n"+
				"  writeEnd=%d but cellLen=%d (expected writeEnd ≥ cellLen-2=%d).\n"+
				"  Current code: read/write joined by '/' with no right-alignment.\n"+
				"  Fix: read=left (1-space indent), write=right (1-space from right edge).\n"+
				"  cache cell: %q", writeStr, writeEnd, cellVisLen, cellVisLen-2, cacheCell)
		}
	}

	// Also verify that the current "/" join is gone: the cache cell must NOT contain
	// "100/2K" (the current left-aligned joined form).
	if strings.Contains(cacheCell, readStr+"/"+writeStr) {
		t.Errorf("F9: cache cell must NOT use '/' join between read and write;\n"+
			"  found %q in cache cell %q.\n"+
			"  After fix: read is left-pinned and write is right-pinned (split layout).",
			readStr+"/"+writeStr, cacheCell)
	}
}

// TestF9_CacheRWColumnAlignment_WithRedWrite verifies F9 alignment when
// redWrite is active (cache_create wrapped in {{color:red}}…{{reset}}). The
// {{color:red}} marker is zero-width for visible-length purposes, so the
// positional logic must measure VISIBLE width (via format.VisualLen / stripMk)
// and not byte length.
//
// A turn with a gap ≥ OrchTTLMinutes triggers redWrite (T-34).
// OrchTTLMinutes=5, gap=5m → redWrite on the current turn.
//
// This test also FAILS on current code (wrong cell layout).
func TestF9_CacheRWColumnAlignment_WithRedWrite(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	const orchTTL = 5

	// Two orch turns: gap=5m → redWrite on cur.
	// CacheRead=50 → FormatK="50", CacheCreate=3000 → FormatK="3K".
	prevTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "f9-prev",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{CacheRead: 50, CacheCreate: 1000, Output: 20},
		ToolUse:     "ToolPrevF9",
		IsSidechain: false,
	}
	curTurn := parser.Turn{
		Index:       2,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "f9-cur",
		GroupID:     2,
		Timestamp:   base.Add(5 * time.Minute), // gap=5m → redWrite
		Tokens:      parser.TokenCounts{CacheRead: 50, CacheCreate: 3000, Output: 30},
		ToolUse:     "ToolCurF9",
		IsSidechain: false,
	}

	// Newest-first.
	out := renderWithTurns(t, []parser.Turn{curTurn, prevTurn}, nil, base.Add(5*time.Minute+time.Second), orchTTL)

	// Find the cur row (fresh, contains "ToolCurF9").
	curRowRaw := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "ToolCurF9") {
			curRowRaw = l
			break
		}
	}
	if curRowRaw == "" {
		t.Fatalf("F9-redwrite: cur row 'ToolCurF9' not found\noutput:\n%s", out)
	}

	// Extract bare row to get cache cell.
	bareRow := stripMk(curRowRaw)
	cells := strings.Split(bareRow, "│")
	if len(cells) < 5 {
		t.Fatalf("F9-redwrite: expected ≥5 │-segments, got %d; bare row: %q", len(cells), bareRow)
	}
	cacheCell := cells[4] // cache column

	readStr := "50"  // FormatK(50)
	writeStr := "3K" // FormatK(3000)

	// Read must be near left.
	readPos := strings.Index(cacheCell, readStr)
	if readPos < 0 {
		t.Errorf("F9-redwrite: readStr %q not found in cache cell %q (bare)", readStr, cacheCell)
	} else if readPos > 2 {
		t.Errorf("F9-redwrite: readStr %q must be near left (pos ≤ 2), got %d; cell %q", readStr, readPos, cacheCell)
	}

	// Write must be near right (measured on bare/stripped cell).
	writePos := strings.LastIndex(cacheCell, writeStr)
	if writePos < 0 {
		t.Errorf("F9-redwrite: writeStr %q not found in cache cell %q (bare)", writeStr, cacheCell)
	} else {
		cellVisLen := len([]rune(cacheCell))
		writeEnd := writePos + len([]rune(writeStr))
		if writeEnd < cellVisLen-2 {
			t.Errorf("F9-redwrite: writeStr %q not right-justified;\n"+
				"  writeEnd=%d cellLen=%d (want ≥ %d).\n"+
				"  With {{color:red}} marker the code must measure VISIBLE width, not bytes.\n"+
				"  cache cell (bare): %q", writeStr, writeEnd, cellVisLen, cellVisLen-2, cacheCell)
		}
	}

	// Also verify that {{color:red}} appears in the raw row on the cur row
	// (precondition: T-34 is wired, redWrite fires). If not — the test was
	// misconfigured, not an F9 failure.
	if !strings.Contains(curRowRaw, "{{color:red}}") {
		t.Logf("F9-redwrite: NOTE — cur row does not contain {{color:red}}; "+
			"T-34 may not be wired yet. F9 alignment assertion runs on bare cell regardless.\n"+
			"  row: %s", curRowRaw)
	}

	// Verify that "/"-join does not appear in the cache cell (bare form).
	if strings.Contains(cacheCell, readStr+"/"+writeStr) {
		t.Errorf("F9-redwrite: cache cell must NOT contain '%s/%s' join; cell: %q",
			readStr, writeStr, cacheCell)
	}
}
