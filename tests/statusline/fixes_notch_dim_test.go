// Package statusline_test — RED tests for Phase 6.9 FIXES group G-render:
//
//   - Part A (notch boundary): the standalone full-line inter-group separator
//     (groupSep = ├─┼─┤ on its own line) is REMOVED; instead the anchor row of each
//     group carries notch dividers (├ / ┼ / ┤) at bar positions, dim.
//   - Part B (F14 whole-row dim): inner cell {{reset}} must NOT kill the enclosing
//     {{dim}} wrapper on history/dim rows — after renderer.Apply the dim escape must
//     stay active across the entire visible line.
//
// All tests go through the production path (Assembler.Render → renderer.Apply).
// Assertions are on the VISIBLE output after Apply, not on raw {{marker}} presence
// (per visual-render-test discipline — marker-string asserts were exactly the blind
// spot that hid F14).
//
// Helpers reused from assembler_test.go (same package):
//
//	orchTurn, renderWithTurns, dataRows, groupSepLines, tableLines, stripMkA,
//	makeStdAssembler.
//
// F1/F2/F9 live in fixes_f1f2f9_table_test.go (same package, not touched here).
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// ansiTheme returns a colour-enabled theme with the default palette, used for
// Apply-level assertions in F14 tests.
func ansiTheme() renderer.Theme {
	return renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.DefaultPalette(),
	}
}

// renderWithTurnsApplied renders the assembler output and applies the ANSI theme
// so assertions can be made on the visible result (ESC sequences, not markers).
func renderWithTurnsApplied(t *testing.T, turns []parser.Turn, now time.Time, orchTTLMinutes int) string {
	t.Helper()
	raw := renderWithTurns(t, turns, nil, now, orchTTLMinutes)
	return renderer.Apply(raw, ansiTheme())
}

// notchDataRows returns lines that are data rows whose first visible character
// is a notch glyph (├ rather than │ or a full dim wrapper). Notch rows are data
// rows (not horizontal separator lines, not legend, not top/bottom border).
//
// A notch row satisfies all of:
//   - its bare (marker-stripped) form starts with ├
//   - it contains ┼ (inner junction)
//   - it contains cell content — identified by containing at least one space
//     (pure horizontal lines of ─ and junction runes have no spaces)
func notchDataRows(out string) []string {
	var rows []string
	for _, l := range strings.Split(out, "\n") {
		bare := stripMkA(l)
		if !strings.HasPrefix(bare, "├") {
			continue
		}
		if !strings.Contains(bare, "┼") {
			continue
		}
		// A pure horizontal line consists entirely of ─ plus junction runes (no spaces).
		// A notch data row has padded cells (spaces present).
		if !strings.Contains(bare, " ") {
			// No spaces → pure horizontal line (groupSep or legendSep), not a notch row.
			continue
		}
		rows = append(rows, l)
	}
	return rows
}

// standaloneSepLines returns lines that are pure full-line ├─┼─┤ separators
// BETWEEN DATA ROWS (not the legend separator, not a notch data row). These are
// the old groupSep style that should be ABSENT between data rows after the notch
// redesign.
//
// A standalone separator: starts with ├, contains ┼, ends with ┤, has NO spaces.
// The legend separator (the pure horizontal ├─┼─┄ immediately before the legend
// row) is EXCLUDED — that separator is intentional and part of the table footer.
func standaloneSepLines(out string) []string {
	// Identify the legend separator line so it can be excluded.
	legSep := legendSepLine(out)
	legSepBare := ""
	if legSep != "" {
		legSepBare = stripMkA(legSep)
	}

	var seps []string
	for _, l := range strings.Split(out, "\n") {
		bare := stripMkA(l)
		if !strings.HasPrefix(bare, "├") {
			continue
		}
		if !strings.Contains(bare, "┼") {
			continue
		}
		if !strings.HasSuffix(bare, "┤") {
			continue
		}
		// Standalone separator: no spaces (only ─ and junction runes).
		if strings.Contains(bare, " ") {
			continue
		}
		// Exclude the legend separator (intentional footer line, not inter-group).
		if legSepBare != "" && bare == legSepBare {
			continue
		}
		seps = append(seps, l)
	}
	return seps
}

// legendSepLine returns the line immediately before the legend row, or "".
// The legend row is identified by containing " role " and " model ".
func legendSepLine(out string) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		bare := stripMkA(l)
		if strings.Contains(bare, " role ") && strings.Contains(bare, " model ") {
			if i > 0 {
				return lines[i-1]
			}
		}
	}
	return ""
}

// =============================================================================
// Part A — Notch boundary
// =============================================================================

// ---------------------------------------------------------------------------
// TestNotch_AnchorRowCarriesNotchDividers
//
// Contract (notch): every orch group gets exactly one notch row — its anchor
// (chronologically earliest turn = bottom of the group's block in newest-first
// output). This includes the freshest group.
//
// Fixture: 2 orch groups, 1 turn each (newest-first: GroupID=2, GroupID=1).
// Each group has only one turn, so that single turn is the anchor for its group.
// Expected: exactly 2 notch data rows (one per group), 0 standalone separators.
//
// This test FAILS on current code which emits a standalone groupSep line
// between groups instead of notch dividers on the anchor rows.
// ---------------------------------------------------------------------------
func TestNotch_AnchorRowCarriesNotchDividers(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// GroupID=2 is fresh (top); GroupID=1 is history (below).
	// Each group has one turn = that turn IS the anchor for its group.
	turnFresh := orchTurn(2, "notch-fresh", 2, base.Add(10*time.Second), "EditNotch", "")
	turnOld := orchTurn(1, "notch-old", 1, base, "ReadNotch", "")

	// Newest-first.
	raw := renderWithTurns(t, []parser.Turn{turnFresh, turnOld}, nil, base.Add(2*time.Minute), 5)

	if !strings.Contains(raw, "┌") {
		t.Fatalf("Notch-anchor: no table in raw output\nraw:\n%s", raw)
	}

	// A. Exactly 2 notch data rows (one per group, both are anchors).
	notchRows := notchDataRows(raw)
	if len(notchRows) != 2 {
		t.Errorf("Notch-anchor: expected 2 notch data rows (anchor of G2='EditNotch' + anchor of G1='ReadNotch');\n"+
			"  got %d. Current code emits a standalone groupSep line instead of notch rows.\n"+
			"  Fix: renderUnifiedDataRow must emit notch glyphs on every anchor row.\n"+
			"  notch rows found:\n%s\n  raw output:\n%s",
			len(notchRows), strings.Join(notchRows, "\n"), raw)
	}

	// B. No standalone full-line ├─┼─┤ between data rows (old groupSep removed).
	standalones := standaloneSepLines(raw)
	if len(standalones) > 0 {
		t.Errorf("Notch-anchor: found %d standalone ├─┼─┤ separator line(s) between data rows;\n"+
			"  these must be REMOVED — the notch redesign embeds boundaries into anchor rows.\n"+
			"  standalone lines:\n%s\n  raw output:\n%s",
			len(standalones), strings.Join(standalones, "\n"), raw)
	}

	// C. Both named rows must use notch dividers.
	for _, tool := range []string{"EditNotch", "ReadNotch"} {
		found := false
		for _, l := range strings.Split(raw, "\n") {
			if !strings.Contains(l, tool) {
				continue
			}
			found = true
			bare := stripMkA(l)
			if !strings.HasPrefix(bare, "├") {
				t.Errorf("Notch-anchor: anchor row (%q) must start with '├';\n"+
					"  got bare start: %q\n  raw line: %s", tool, barePrefix(bare, 5), l)
			}
			if !strings.Contains(bare, "┼") {
				t.Errorf("Notch-anchor: anchor row (%q) must contain '┼';\n  raw line: %s", tool, l)
			}
			// The anchor row's trailing border is '┤'. A TTL suffix (e.g. " ⏱ 4m")
			// may appear after the '┤' in the line, so HasSuffix would fail.
			// We verify by checking that the bare form CONTAINS '┤'.
			if !strings.Contains(bare, "┤") {
				t.Errorf("Notch-anchor: anchor row (%q) must contain trailing '┤';\n  bare: %q", tool, barePrefix(bare, 60))
			}
		}
		if !found {
			t.Errorf("Notch-anchor: anchor row %q not found in output\nraw:\n%s", tool, raw)
		}
	}
}

// ---------------------------------------------------------------------------
// TestNotch_AllGroupBoundariesHaveNotchRow
//
// Contract: every orch group gets exactly one notch row (its anchor), including
// the freshest. 3 groups → 3 notch rows.
//
// Fixture: 3 orch groups, 1 turn each (newest-first: GroupID=3, GroupID=2, GroupID=1).
// Each single-turn group has its one turn as its anchor.
// Expected: exactly 3 notch data rows, 0 standalone separators.
//
// This test FAILS on current code which emits 2 standalone groupSep lines and
// 0 notch data rows.
// ---------------------------------------------------------------------------
func TestNotch_AllGroupBoundariesHaveNotchRow(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turnG3 := orchTurn(3, "notch-g3", 3, base.Add(20*time.Second), "ToolG3", "")
	turnG2 := orchTurn(2, "notch-g2", 2, base.Add(10*time.Second), "ToolG2", "")
	turnG1 := orchTurn(1, "notch-g1", 1, base, "ToolG1", "")

	// Newest-first order.
	raw := renderWithTurns(t, []parser.Turn{turnG3, turnG2, turnG1}, nil, base.Add(3*time.Minute), 5)

	if !strings.Contains(raw, "┌") {
		t.Fatalf("Notch-allgroups: no table in output\nraw:\n%s", raw)
	}

	// Expect exactly 3 notch rows — one per group (G3, G2, G1 each have one turn = anchor).
	notchRows := notchDataRows(raw)
	if len(notchRows) != 3 {
		t.Errorf("Notch-allgroups: expected 3 notch data rows (one per group: G3, G2, G1);\n"+
			"  got %d. Every group including the freshest gets a notch on its anchor.\n"+
			"  notch rows found:\n%s\n"+
			"  raw output:\n%s",
			len(notchRows), strings.Join(notchRows, "\n"), raw)
	}

	// Verify each named tool row is a notch row.
	for _, tool := range []string{"ToolG3", "ToolG2", "ToolG1"} {
		for _, l := range strings.Split(raw, "\n") {
			if !strings.Contains(l, tool) {
				continue
			}
			bare := stripMkA(l)
			if !strings.HasPrefix(bare, "├") {
				t.Errorf("Notch-allgroups: anchor row (%q) must start with '├'; got: %q\n  raw line: %s",
					tool, barePrefix(bare, 5), l)
			}
		}
	}

	// No standalone full-line separators.
	standalones := standaloneSepLines(raw)
	if len(standalones) > 0 {
		t.Errorf("Notch-allgroups: found %d standalone ├─┼─┤ separator line(s);\n"+
			"  must be zero after notch redesign.\n  raw:\n%s",
			len(standalones), raw)
	}
}

// ---------------------------------------------------------------------------
// TestNotch_NotchGlyphsAreDim
//
// Contract: notch glyphs (├ / ┼ / ┤) on the anchor data row are dim — after
// renderer.Apply, the dim escape \x1b[2m appears before ├ ON A DATA ROW
// (a row that also contains cell content / spaces), not on a pure horizontal
// separator line.
//
// Fixture: 2 orch groups.
// After Apply: at least one line must contain dimEsc+"├" AND also contain
// a space (cell content), distinguishing it from the old standalone separator
// (which is a pure horizontal line and also carries \x1b[2m in current code).
//
// This test FAILS on current code: the old standalone ├─┼─┤ separator IS dim
// but has no spaces. A notch DATA row (├ + spaces + content, all dim) does not
// exist yet. So the specific combined condition (dimEsc+"├" on a content line)
// is not met.
// ---------------------------------------------------------------------------
func TestNotch_NotchGlyphsAreDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turnFresh := orchTurn(2, "notch-dim-fresh", 2, base.Add(10*time.Second), "EditDimCheck", "")
	turnOld := orchTurn(1, "notch-dim-old", 1, base, "ReadDimCheck", "")

	applied := renderWithTurnsApplied(t, []parser.Turn{turnFresh, turnOld}, base.Add(2*time.Minute), 5)

	if !strings.Contains(applied, "┌") {
		t.Fatalf("Notch-dim: no table in applied output\noutput:\n%s", applied)
	}

	// After Apply, dim is \x1b[2m. The anchor row's ├ must be preceded by \x1b[2m
	// AND the line must contain a space (cell content = data row, not a horizontal
	// separator line).
	const dimEsc = "\x1b[2m"
	foundDimNotchDataRow := false
	for _, l := range strings.Split(applied, "\n") {
		// Must contain dimEsc immediately before ├.
		if !strings.Contains(l, dimEsc+"├") {
			continue
		}
		// Must also be a data row (contains spaces after the ├ sequence).
		if strings.Contains(l, " ") {
			foundDimNotchDataRow = true
			break
		}
	}

	if !foundDimNotchDataRow {
		t.Errorf("Notch-dim: after Apply, expected a data row containing %q+'├' (dim notch) AND spaces;\n"+
			"  Current code has only the old standalone ├─┼─┄ separator (pure horizontal, no spaces).\n"+
			"  After the notch redesign: the anchor data row must use dimBar glyphs.\n"+
			"  Fix: anchor row dividers must use dimBar pattern ({{dim}}├{{reset}}, etc.).\n"+
			"  applied output:\n%s", dimEsc, applied)
	}
}

// ---------------------------------------------------------------------------
// TestNotch_LegendSepStaysFullLine
//
// Contract: the legend separator (the full-line ├─┼─┤ immediately above the
// legend row) is NOT removed by the notch redesign — it must still be present.
//
// Fixture: 2 orch groups.
// The legend separator must appear as a pure horizontal line (no spaces).
//
// This test verifies the notch redesign does not accidentally convert the
// legend separator into a notch data row. It is expected to PASS on current
// code (legend sep is already present) and must continue to pass after GREEN.
// ---------------------------------------------------------------------------
func TestNotch_LegendSepStaysFullLine(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turnFresh := orchTurn(2, "legend-fresh", 2, base.Add(10*time.Second), "EditLegend", "")
	turnOld := orchTurn(1, "legend-old", 1, base, "ReadLegend", "")

	raw := renderWithTurns(t, []parser.Turn{turnFresh, turnOld}, nil, base.Add(2*time.Minute), 5)

	if !strings.Contains(raw, "┌") {
		t.Fatalf("Notch-legendsep: no table in output\nraw:\n%s", raw)
	}

	// The legend separator is the full-line ├─┼─┄ immediately before the legend row.
	legSep := legendSepLine(raw)
	if legSep == "" {
		t.Errorf("Notch-legendsep: legend separator not found above legend row;\n"+
			"  the legend separator must remain a full horizontal line after the notch redesign.\n"+
			"  raw output:\n%s", raw)
		return
	}

	bareLegSep := stripMkA(legSep)
	if !strings.HasPrefix(bareLegSep, "├") {
		t.Errorf("Notch-legendsep: legend separator must start with '├'; got: %q", legSep)
	}
	if !strings.Contains(bareLegSep, "┼") {
		t.Errorf("Notch-legendsep: legend separator must contain '┼'; got: %q", legSep)
	}
	// The legend separator must be a pure horizontal line (no spaces = not a data row).
	if strings.Contains(bareLegSep, " ") {
		t.Errorf("Notch-legendsep: legend separator must be a pure horizontal line (no spaces);\n"+
			"  found spaces — it may have been incorrectly converted to a notch data row.\n"+
			"  sep line: %q", legSep)
	}
}

// ---------------------------------------------------------------------------
// TestNotch_NewerTurnsOfGroupAreNotNotched
//
// Guard: within a multi-turn group, ONLY the chronologically earliest turn
// (anchor) carries notch dividers. The newer turns of the same group use plain
// │ dividers. The fresh group also has an anchor notch row.
//
// Fixture: fresh group G2 has 2 turns: Index=4 (newest, top) and Index=3
// (oldest = anchor of G2).
//   - "tool4" row (Index=4, newer, top of G2's block): must NOT start with ├.
//   - "tool3" row (Index=3, anchor of G2): must start with ├ (has notch).
//
// This test PASSES on current code for the "tool4 plain" assertion (no notch
// glyphs at all yet), but is included as an over-application guard: after GREEN
// the implementation must not put notch on every row of a multi-turn group.
// The "tool3 is notched" assertion will RED on current code.
// ---------------------------------------------------------------------------
func TestNotch_NewerTurnsOfGroupAreNotNotched(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// G2 = fresh group, 2 turns. Index=4 is newer (top); Index=3 is anchor (bottom of G2 block).
	turnG2newer := parser.Turn{
		Index: 4, Role: "orch", Model: "claude-sonnet-4-6",
		UUID: "guard-g2-newer", GroupID: 2,
		Timestamp:   base.Add(20 * time.Second),
		Tokens:      parser.TokenCounts{Output: 80, CacheCreate: 800},
		ToolUse:     "tool4",
		IsSidechain: false,
	}
	turnG2anchor := parser.Turn{
		Index: 3, Role: "orch", Model: "claude-sonnet-4-6",
		UUID: "guard-g2-anchor", GroupID: 2,
		Timestamp:   base.Add(10 * time.Second),
		Tokens:      parser.TokenCounts{Output: 60, CacheCreate: 600},
		ToolUse:     "tool3",
		IsSidechain: false,
	}

	// Newest-first: tool4 then tool3.
	raw := renderWithTurns(t, []parser.Turn{turnG2newer, turnG2anchor}, nil, base.Add(2*time.Minute), 5)

	if !strings.Contains(raw, "┌") {
		t.Fatalf("Notch-guard: no table in output\nraw:\n%s", raw)
	}

	// "tool4" (newer turn of G2) must NOT carry notch — only the anchor does.
	for _, l := range strings.Split(raw, "\n") {
		if !strings.Contains(l, "tool4") {
			continue
		}
		bare := stripMkA(l)
		if strings.HasPrefix(bare, "├") {
			t.Errorf("Notch-guard: 'tool4' (newer, non-anchor turn of G2) must NOT start with '├';\n"+
				"  only the anchor (earliest) turn of each group carries notch dividers.\n"+
				"  bare row prefix: %q", barePrefix(bare, 20))
		}
	}

	// "tool3" (anchor of G2, the only group here) must have notch.
	anchorFound := false
	for _, l := range strings.Split(raw, "\n") {
		if !strings.Contains(l, "tool3") {
			continue
		}
		anchorFound = true
		bare := stripMkA(l)
		if !strings.HasPrefix(bare, "├") {
			t.Errorf("Notch-guard: 'tool3' (anchor of G2) must start with '├';\n"+
				"  current code emits no notch rows at all.\n"+
				"  bare row: %q\n  raw line: %s", barePrefix(bare, 8), l)
		}
	}
	if !anchorFound {
		t.Errorf("Notch-guard: 'tool3' anchor row not found in output\nraw:\n%s", raw)
	}
}

// ---------------------------------------------------------------------------
// TestNotch_AnchorRowIsEarliestTurnMultiTurnGroup
//
// PIN: in a multi-turn group the anchor row is the CHRONOLOGICALLY EARLIEST turn
// (lowest Index / Timestamp). In newest-first output that is the BOTTOM row of
// the group's block. Every group — including the freshest — has an anchor notch.
//
// Fixture: 2 orch groups, 2 turns each.
//   - Group 2 (fresh): Index=4 (ts+30s, top of G2 block) and Index=3 (ts+20s, anchor of G2).
//   - Group 1 (history): Index=2 (ts+10s) and Index=1 (ts+0s, anchor of G1).
//
// Newest-first order: tool4, tool3, tool2, tool1.
// Expected: exactly 2 notch rows — tool3 (anchor G2) and tool1 (anchor G1).
// Non-anchor turns tool4 and tool2 must use plain │.
// 0 standalone separators.
//
// This test FAILS on current code: standalone groupSep lands between tool3 and
// tool2 (the group boundary scanning top-to-bottom), whereas the correct notch
// rows are tool3 (bottom of G2 block) and tool1 (bottom of G1 block).
// ---------------------------------------------------------------------------
func TestNotch_AnchorRowIsEarliestTurnMultiTurnGroup(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Group 2 (fresh): two turns. Index=4 is newer (top); Index=3 is anchor.
	turnG2b := parser.Turn{
		Index: 4, Role: "orch", Model: "claude-sonnet-4-6",
		UUID: "mt-g2b", GroupID: 2,
		Timestamp:   base.Add(30 * time.Second),
		Tokens:      parser.TokenCounts{Output: 100, CacheCreate: 1000},
		ToolUse:     "tool4",
		IsSidechain: false,
	}
	turnG2a := parser.Turn{
		// Anchor of group 2: chronologically first in G2 = bottom of G2's block.
		Index: 3, Role: "orch", Model: "claude-sonnet-4-6",
		UUID: "mt-g2a", GroupID: 2,
		Timestamp:   base.Add(20 * time.Second),
		Tokens:      parser.TokenCounts{Output: 80, CacheCreate: 800},
		ToolUse:     "tool3",
		IsSidechain: false,
	}
	// Group 1 (history): two turns. Index=2 is newer; Index=1 is anchor.
	turnG1b := parser.Turn{
		Index: 2, Role: "orch", Model: "claude-sonnet-4-6",
		UUID: "mt-g1b", GroupID: 1,
		Timestamp:   base.Add(10 * time.Second),
		Tokens:      parser.TokenCounts{Output: 60, CacheCreate: 600},
		ToolUse:     "tool2",
		IsSidechain: false,
	}
	turnG1a := parser.Turn{
		// Anchor of group 1: chronologically first = bottom of G1's block.
		Index: 1, Role: "orch", Model: "claude-sonnet-4-6",
		UUID: "mt-g1a", GroupID: 1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 40, CacheCreate: 400},
		ToolUse:     "tool1",
		IsSidechain: false,
	}

	// Newest-first: tool4, tool3, tool2, tool1.
	raw := renderWithTurns(
		t,
		[]parser.Turn{turnG2b, turnG2a, turnG1b, turnG1a},
		nil,
		base.Add(3*time.Minute),
		5,
	)

	if !strings.Contains(raw, "┌") {
		t.Fatalf("Notch-multiturn: no table in output\nraw:\n%s", raw)
	}

	// Contract A: exactly 2 notch data rows (anchor of G2="tool3" + anchor of G1="tool1").
	notchRows := notchDataRows(raw)
	if len(notchRows) != 2 {
		t.Errorf("Notch-multiturn: expected 2 notch data rows (G2 anchor='tool3', G1 anchor='tool1');\n"+
			"  got %d.\n"+
			"  notch rows:\n%s\n"+
			"  raw output:\n%s",
			len(notchRows), strings.Join(notchRows, "\n"), raw)
	}

	// Contract B: anchor rows must start with ├.
	for _, tool := range []string{"tool3", "tool1"} {
		found := false
		for _, l := range strings.Split(raw, "\n") {
			if !strings.Contains(l, tool) {
				continue
			}
			found = true
			bare := stripMkA(l)
			if !strings.HasPrefix(bare, "├") {
				t.Errorf("Notch-multiturn: anchor row (%q) must start with '├';\n"+
					"  got bare: %q.\n"+
					"  Current code emits standalone groupSep before tool2, tool3/tool1 rows stay plain.\n"+
					"  raw line: %s", tool, barePrefix(bare, 8), l)
			}
		}
		if !found {
			t.Errorf("Notch-multiturn: anchor row %q not found in output\nraw:\n%s", tool, raw)
		}
	}

	// Contract C: non-anchor turns must use plain dividers (NOT start with ├).
	for _, tool := range []string{"tool4", "tool2"} {
		for _, l := range strings.Split(raw, "\n") {
			if !strings.Contains(l, tool) {
				continue
			}
			bare := stripMkA(l)
			if strings.HasPrefix(bare, "├") {
				t.Errorf("Notch-multiturn: non-anchor row (%q) must NOT start with '├';\n"+
					"  only the earliest turn of each group carries notch dividers.\n"+
					"  raw line: %s", tool, l)
			}
		}
	}

	// Contract D: no standalone full-line separators.
	standalones := standaloneSepLines(raw)
	if len(standalones) > 0 {
		t.Errorf("Notch-multiturn: found %d standalone ├─┼─┤ line(s); must be 0 after notch redesign.\nraw:\n%s",
			len(standalones), raw)
	}
}

// =============================================================================
// Part B — F14: whole-row dim survives inner cell {{reset}}
// =============================================================================

// ---------------------------------------------------------------------------
// TestF14_DimRowStaysDimAcrossWholeLineOrch
//
// F14 bug: renderUnifiedDataRow wraps history rows as:
//
//	"{{dim}}" + content + "{{reset}}"
//
// where content contains per-cell markers like {{color:cyan}}role{{reset}}.
// After renderer.Apply the inner {{reset}} → \x1b[0m which kills the outer dim
// (\x1b[2m). The rest of the row renders undimmed.
//
// Contract after fix: a dim orchestrator row (Dim=true, history group) must,
// after renderer.Apply, have dim (\x1b[2m) still active at every point in the
// line. Equivalently: no \x1b[0m (reset) may appear unless immediately followed
// by \x1b[2m (re-dim), except for the very last reset at end-of-row.
//
// Fixture:
//   - GroupID=2 (fresh, maxOrchGroup=2)
//   - GroupID=1 (history, Dim=true) — role cell emits {{color:cyan}}orch{{reset}};
//     after Apply the inner \x1b[0m kills the outer \x1b[2m.
//
// Assertion: on the APPLIED output (visible result), not on raw markers.
//
// Fails on current code because the inner {{reset}} drops the outer {{dim}}.
// ---------------------------------------------------------------------------
func TestF14_DimRowStaysDimAcrossWholeLineOrch(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// GroupID=2 fresh (max), GroupID=1 history (Dim=true).
	turnFresh := orchTurn(2, "f14-fresh", 2, base.Add(10*time.Second), "EditF14", "")
	turnHist := orchTurn(1, "f14-hist", 1, base, "ReadF14", "")

	// Newest-first.
	applied := renderWithTurnsApplied(t, []parser.Turn{turnFresh, turnHist}, base.Add(2*time.Minute), 5)

	if !strings.Contains(applied, "┌") {
		t.Fatalf("F14-orch: no table in applied output\napplied:\n%s", applied)
	}

	// Find the history (dim) row — it contains "ReadF14".
	histRowFound := false
	for _, l := range strings.Split(applied, "\n") {
		if !strings.Contains(l, "ReadF14") {
			continue
		}
		histRowFound = true

		const dimEsc = "\x1b[2m"
		const resetEsc = "\x1b[0m"

		// The history row must begin with the dim escape.
		if !strings.HasPrefix(l, dimEsc) {
			t.Errorf("F14-orch: history (dim) row must begin with dim escape %q after Apply;\n"+
				"  got row start: %q\n  full line: %s", dimEsc, barePrefix(l, 20), l)
		}

		// No bare reset that kills dim mid-line.
		checkDimContinuity(t, "F14-orch", l, dimEsc, resetEsc)
	}

	if !histRowFound {
		t.Fatalf("F14-orch: history row 'ReadF14' not found in applied output\napplied:\n%s", applied)
	}
}

// ---------------------------------------------------------------------------
// TestF14_DimRowStaysDimAcrossWholeLineSubagent
//
// RENDERER-LEVEL test for F14 on a subagent (IsSidechain=true) row.
//
// This test is intentionally isolated from F15 (assembler Dim-assignment for
// old subagents). It builds a UnifiedRow{Dim:true, IsSidechain:true} directly
// and calls RenderUnifiedRows + renderer.Apply, so:
//   - It exercises the RENDERER path for sidechain dim rows (F14 scope).
//   - It does NOT depend on Assembler wiring of F15.
//   - It turns GREEN when G-render (table_unified.go) is fixed, regardless of
//     whether assembler/F15 is done.
//
// Subagent role = "code-reviewer" → yellow cell ({{color:yellow}}…{{reset}}).
// After Apply the inner \x1b[0m must NOT kill the outer \x1b[2m dim.
//
// Fails on current code: renderUnifiedDataRow wraps dim rows as
// {{dim}}content{{reset}}; the inner {{reset}} of the yellow cell resolves to
// \x1b[0m and drops dim for the remainder of the line.
// ---------------------------------------------------------------------------
func TestF14_DimRowStaysDimAcrossWholeLineSubagent(t *testing.T) {
	b := renderer.NewBuilder(80)

	rows := []renderer.UnifiedRow{
		{
			// Non-dim fresh orch row (table needs ≥1 row).
			HashCell:    "2",
			Role:        "orch",
			Model:       "sonnet-4-6",
			CacheRead:   100,
			CacheCreate: 1000,
			Out:         50,
			CostCell:    "$0.01",
			Tool:        "EditDirectSA",
			IsSidechain: false,
			Dim:         false,
			GroupID:     2,
		},
		{
			// Dim subagent row. IsSidechain=true → role rendered yellow
			// ({{color:yellow}}code-reviewer{{reset}}).
			// After Apply: \x1b[2m (outer dim) ... \x1b[33m (yellow) ... \x1b[0m (kills dim).
			HashCell:    "↳1",
			Role:        "code-reviewer",
			Model:       "sonnet-4-6",
			CacheRead:   50,
			CacheCreate: 500,
			Out:         30,
			CostCell:    "Σ $0.00",
			Tool:        "ReadDirectSA",
			IsSidechain: true,
			Dim:         true, // F14 path: whole-row dim with coloured sidechain role
			GroupID:     0,    // subagent rows carry GroupID=0
			SkipSeparator: true,
		},
	}

	raw := b.RenderUnifiedRows(rows)
	if raw == "" {
		t.Fatal("F14-subagent-direct: RenderUnifiedRows returned empty string")
	}

	applied := renderer.Apply(raw, ansiTheme())

	const dimEsc = "\x1b[2m"
	const resetEsc = "\x1b[0m"

	dimRowFound := false
	for _, l := range strings.Split(applied, "\n") {
		if !strings.Contains(l, "ReadDirectSA") {
			continue
		}
		dimRowFound = true

		// Dim sidechain row must start with dim escape.
		if !strings.HasPrefix(l, dimEsc) {
			t.Errorf("F14-subagent-direct: dim sidechain row must begin with %q after Apply;\n"+
				"  got start: %q\n  full line: %s", dimEsc, barePrefix(l, 20), l)
		}

		// No bare reset kills dim mid-line (the yellow role cell fires this bug).
		checkDimContinuity(t, "F14-subagent-direct", l, dimEsc, resetEsc)
	}

	if !dimRowFound {
		t.Errorf("F14-subagent-direct: sidechain dim row 'ReadDirectSA' not found in applied output\napplied:\n%s", applied)
	}
}

// ---------------------------------------------------------------------------
// TestF14_DimRowNoBareResetBeforeLineEnd
//
// Renderer-level test: build UnifiedRows directly via RenderUnifiedRows
// (bypassing assembler) with Dim=true and a coloured role cell. After Apply,
// verify dim continuity.
//
// This tests the renderer API in isolation so the fix can be verified at the
// correct level independently of assembler wiring.
//
// Fails on current code: renderUnifiedDataRow wraps as {{dim}}...{{reset}};
// inner {{reset}} of the cyan cell resolves to \x1b[0m and kills dim.
// ---------------------------------------------------------------------------
func TestF14_DimRowNoBareResetBeforeLineEnd(t *testing.T) {
	b := renderer.NewBuilder(80)

	rows := []renderer.UnifiedRow{
		{
			// Non-dim fresh row (needed so the table is non-empty).
			HashCell:    "2",
			Role:        "orch",
			Model:       "sonnet-4-6",
			CacheRead:   100,
			CacheCreate: 1000,
			Out:         50,
			CostCell:    "$0.01",
			Tool:        "EditDirect",
			IsSidechain: false,
			Dim:         false,
			GroupID:     2,
		},
		{
			// History (dim) row with coloured role cell.
			// Role "orch" → unifiedRoleColour → {{color:cyan}}orch{{reset}}.
			// After Apply: \x1b[2m ... \x1b[36m ... \x1b[0m (kills dim).
			HashCell:    "1",
			Role:        "orch",
			Model:       "sonnet-4-6",
			CacheRead:   50,
			CacheCreate: 500,
			Out:         30,
			CostCell:    "$0.00",
			Tool:        "ReadDirect",
			IsSidechain: false,
			Dim:         true,
			GroupID:     1,
		},
	}

	raw := b.RenderUnifiedRows(rows)
	if raw == "" {
		t.Fatal("F14-direct: RenderUnifiedRows returned empty string")
	}

	applied := renderer.Apply(raw, ansiTheme())

	const dimEsc = "\x1b[2m"
	const resetEsc = "\x1b[0m"

	dimRowFound := false
	for _, l := range strings.Split(applied, "\n") {
		if !strings.Contains(l, "ReadDirect") {
			continue
		}
		dimRowFound = true

		// Must start with dim escape.
		if !strings.HasPrefix(l, dimEsc) {
			t.Errorf("F14-direct: dim row must start with %q after Apply;\n  got: %q",
				dimEsc, barePrefix(l, 20))
		}

		checkDimContinuity(t, "F14-direct", l, dimEsc, resetEsc)
	}

	if !dimRowFound {
		t.Errorf("F14-direct: dim row 'ReadDirect' not found in applied output\napplied:\n%s", applied)
	}
}

// =============================================================================
// Helper utilities (local to this file)
// =============================================================================

// checkDimContinuity verifies that on a dim-wrapped row (started with dimEsc),
// every occurrence of resetEsc is immediately followed by dimEsc (re-dim) OR
// is the very last escape sequence in the line (the closing outer reset).
//
// A bare resetEsc not followed by dimEsc means dim was killed mid-line,
// leaving subsequent content undimmed — the F14 bug.
func checkDimContinuity(t *testing.T, label string, line string, dimEsc string, resetEsc string) {
	t.Helper()

	pos := 0
	for {
		idx := strings.Index(line[pos:], resetEsc)
		if idx < 0 {
			break
		}
		absIdx := pos + idx
		after := line[absIdx+len(resetEsc):]

		if after == "" {
			// Trailing reset at end of row — this is the closing reset of the outer dim.
			break
		}

		// If after starts with dimEsc, dim is immediately restored — acceptable.
		if strings.HasPrefix(after, dimEsc) {
			pos = absIdx + len(resetEsc)
			continue
		}

		// Bare reset: dim was killed and not immediately restored.
		t.Errorf("%s: dim killed mid-line — %q not followed by %q;\n"+
			"  Inner cell {{reset}} dropped outer {{dim}}.\n"+
			"  Fix: renderer must re-emit {{dim}} after each inner reset in a dim row,\n"+
			"  or use a dim-aware reset that returns to dim state instead of full reset.\n"+
			"  segment after reset: %q\n  full line: %s",
			label, resetEsc, dimEsc, barePrefix(after, 40), line)

		pos = absIdx + len(resetEsc)
	}
}

// barePrefix returns the first n bytes of s (or all of s if shorter).
// Used for error message truncation.
func barePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
