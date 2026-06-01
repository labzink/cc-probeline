// Package renderer_test — RED tests for Phase 6.8.d table redesign (T-T1..T-T6).
//
// These tests verify the new per-turn table layout:
//   - T-T1: orch + subagent turns interleaved in one stream, newest-first by Timestamp.
//   - T-T2: group-separator ├┼┤ on the first row of each new GroupID (no extra blank lines).
//   - T-T3: current group (max GroupID) plain; history groups wrapped in {{dim}}…{{reset}}.
//   - T-T4: footer = legend row "# role model cache out cost tool"; no "Total for request".
//   - T-T5: Turn.Thinking → glyph in tool column; ToolUse name when Thinking=false.
//   - T-T6: IsSidechain=true → cost cell "—"; orchestrator → cost.PerTurn value.
//
// All tests are expected to FAIL (RED) until internal/renderer/table.go is rewritten
// per Phase 6.8.d spec.
package renderer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
)

// ----------------------------------------------------------------------------
// Helpers used only in this file
// ----------------------------------------------------------------------------

// makeOrchTurn builds a minimal orchestrator Turn for redesign tests.
// uuid is used as Turn.UUID for per-turn cost lookups.
// groupID sets the Turn.GroupID (1-based request group).
// ts sets the Timestamp for interleave ordering.
func makeOrchTurn(idx int, uuid string, groupID int, ts time.Time, tool string, outTokens int) parser.Turn {
	return parser.Turn{
		Index:       idx,
		Role:        "orch",
		Model:       "sonnet-4",
		UUID:        uuid,
		GroupID:     groupID,
		Timestamp:   ts,
		Tokens:      parser.TokenCounts{Output: outTokens},
		ToolUse:     tool,
		IsSidechain: false,
	}
}

// makeSubTurn builds a minimal sidechain Turn (IsSidechain=true, GroupID=0 pre-merge).
// GroupID will be assigned during interleave merge, so it is set here to the expected
// post-merge value for assertion purposes.
func makeSubTurn(uuid string, groupID int, ts time.Time, tool string) parser.Turn {
	return parser.Turn{
		Role:        "sub",
		Model:       "sonnet-4",
		UUID:        uuid,
		GroupID:     groupID,
		Timestamp:   ts,
		ToolUse:     tool,
		IsSidechain: true,
		Tokens:      parser.TokenCounts{Output: 50},
	}
}

// newBuilderRedesign constructs a renderer.Builder and calls RenderUnified, which
// is the new API expected in Phase 6.8.d. The builder receives a merged list of
// orch+subagent turns pre-sorted by Timestamp DESC. Cost lookups use the provided
// state.Session.
//
// NOTE: RenderUnified is the expected new entry-point for the redesigned table.
// It does not exist yet — calls will fail to compile until GREEN implements it.
// That compile failure is the RED signal for T-T1..T-T6.
func renderRedesign(turns []parser.Turn, st *state.Session, termCols int) string {
	b := renderer.NewBuilder(termCols)
	return b.RenderUnified(turns, st)
}

// lineContains returns true if any line in output contains all given substrings.
func lineContains(output string, subs ...string) bool {
	for _, l := range strings.Split(output, "\n") {
		found := true
		for _, s := range subs {
			if !strings.Contains(l, s) {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}

// collectLines returns non-empty lines (trailing empty stripped).
func collectLines(s string) []string {
	all := strings.Split(s, "\n")
	for len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	return all
}

// uuidOrder extracts the UUID of each data row from the output by inspecting
// a simple marker comment that the new Render puts in each row (UUID printed
// in data cells is not reliable without knowing column positions). Instead we
// rely on the tool column content matching the given uuid-keyed map.
//
// For T-T1 we use the ToolUse field as a unique row marker (each turn has a
// distinct tool name: "Read_A", "Read_B", "Read_Sub").
func rowOrder(output string, markers []string) []int {
	// Returns the line index (among collectLines) of each marker's first appearance.
	lines := collectLines(output)
	order := make([]int, len(markers))
	for i := range order {
		order[i] = -1
	}
	for li, l := range lines {
		stripped := stripMk(l) // remove {{marker}} tokens
		for mi, m := range markers {
			if order[mi] == -1 && strings.Contains(stripped, m) {
				order[mi] = li
			}
		}
	}
	return order
}

// ----------------------------------------------------------------------------
// T-T1: TestTable_InterleaveByTimestamp
//
// Spec T-15: orch turns + subagent turns merged into one stream, sorted by
// Timestamp DESC (newest row at top / smallest line index in the output).
//
// Fixture:
//   - orchTurn1: ts=base+0s  (oldest)   tool="ToolA"
//   - subTurn:   ts=base+5s  (middle)   tool="ToolSub"
//   - orchTurn2: ts=base+10s (newest)   tool="ToolB"
//
// Expected output order top-to-bottom: ToolB, ToolSub, ToolA.
// ----------------------------------------------------------------------------
func TestTable_InterleaveByTimestamp(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchTurn1 := makeOrchTurn(1, "uuid-orch1", 1, base, "ToolA", 100)
	subTurn := makeSubTurn("uuid-sub1", 1, base.Add(5*time.Second), "ToolSub")
	orchTurn2 := makeOrchTurn(2, "uuid-orch2", 2, base.Add(10*time.Second), "ToolB", 200)

	// Merged list sorted newest-first (as the assembler will prepare it).
	turns := []parser.Turn{orchTurn2, subTurn, orchTurn1}

	st := &state.Session{}
	out := renderRedesign(turns, st, 80)

	if out == "" {
		t.Fatal("RenderUnified() returned empty string; want non-empty table (RED: method not yet implemented)")
	}

	// Find line positions of each tool marker.
	positions := rowOrder(out, []string{"ToolB", "ToolSub", "ToolA"})
	posB, posSub, posA := positions[0], positions[1], positions[2]

	if posB == -1 || posSub == -1 || posA == -1 {
		t.Fatalf("T-T1: could not locate all tool markers in output (posB=%d, posSub=%d, posA=%d)\noutput:\n%s",
			posB, posSub, posA, out)
	}

	// Newest-first: ToolB before ToolSub before ToolA.
	if !(posB < posSub && posSub < posA) {
		t.Errorf("T-T1: rows must appear newest-first (ToolB < ToolSub < ToolA); got posB=%d posSub=%d posA=%d\noutput:\n%s",
			posB, posSub, posA, out)
	}
}

// ----------------------------------------------------------------------------
// T-T2: TestTable_GroupSeparator
//
// Spec T-16: the first row of each new GroupID (scanning top-to-bottom) must
// use ├┼┤ as cell dividers instead of plain │. No additional blank lines
// between groups.
//
// Fixture: 2 orch turns — GroupID=2 (newest, current) and GroupID=1 (older).
// Output order (newest-first): GroupID=2 row first, then GroupID=1 row.
// The ├┼┤ separator must appear exactly once: on the first row of GroupID=1
// (i.e. on the second data row, which is the first row of the old group).
// ----------------------------------------------------------------------------
func TestTable_GroupSeparator(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// GroupID=2 is newer (current), GroupID=1 is older.
	turnGroup2 := makeOrchTurn(2, "uuid-g2", 2, base.Add(10*time.Second), "EditFile", 200)
	turnGroup1 := makeOrchTurn(1, "uuid-g1", 1, base, "ReadFile", 100)

	// Merged newest-first.
	turns := []parser.Turn{turnGroup2, turnGroup1}

	st := &state.Session{}
	out := renderRedesign(turns, st, 80)

	if out == "" {
		t.Fatal("RenderUnified() returned empty string; want non-empty table (RED)")
	}

	lines := collectLines(out)

	// Count lines that contain ├ followed by ┼ or ┤ — the group separator pattern.
	// A group separator line starts with ├ and contains ┼ (inner) and ends with ┤.
	groupSepCount := 0
	for _, l := range lines {
		bare := stripMk(l)
		if strings.HasPrefix(bare, "├") && strings.Contains(bare, "┼") && strings.HasSuffix(bare, "┤") {
			groupSepCount++
		}
	}

	// Exactly one group-separator line (between GroupID=2 and GroupID=1).
	if groupSepCount != 1 {
		t.Errorf("T-T2: expected exactly 1 group-separator line (├…┼…┤); got %d\noutput:\n%s",
			groupSepCount, out)
	}

	// No blank lines between data rows (groups are separated by ├┼┤ line, not empty lines).
	dataSection := false
	blankBetweenRows := false
	for _, l := range lines {
		bare := stripMk(l)
		// Data section: lines that start with │ or ├ (data rows and group separators).
		if strings.HasPrefix(bare, "│") || strings.HasPrefix(bare, "├") {
			dataSection = true
		}
		if dataSection && bare == "" {
			blankBetweenRows = true
			break
		}
	}
	if blankBetweenRows {
		t.Errorf("T-T2: blank line found between data rows; groups must be separated by ├┼┤ only\noutput:\n%s", out)
	}
}

// ----------------------------------------------------------------------------
// T-T3: TestTable_DimHistory
//
// Spec T-17: rows belonging to the current group (max GroupID) must NOT be
// wrapped in {{dim}}; rows from older groups must be wrapped in {{dim}}…{{reset}}.
//
// Fixture: 2 groups — GroupID=2 (current), GroupID=1 (history).
// After rendering, the line containing the GroupID=1 turn must contain "{{dim}}"
// while the line for GroupID=2 must not.
//
// Note: the test inspects raw Render() output (before Apply) to check markers.
// ----------------------------------------------------------------------------
func TestTable_DimHistory(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turnCurrent := makeOrchTurn(2, "uuid-cur", 2, base.Add(10*time.Second), "ToolCurrent", 200)
	turnHistory := makeOrchTurn(1, "uuid-hist", 1, base, "ToolHistory", 100)

	// Merged newest-first: current first, history second.
	turns := []parser.Turn{turnCurrent, turnHistory}

	st := &state.Session{}
	out := renderRedesign(turns, st, 80)

	if out == "" {
		t.Fatal("RenderUnified() returned empty string; want non-empty table (RED)")
	}

	// Find the line containing the history turn's tool marker.
	historyLineDim := false
	currentLineNoDim := true
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "ToolHistory") {
			// History row must contain {{dim}}.
			if strings.Contains(l, "{{dim}}") {
				historyLineDim = true
			} else {
				t.Errorf("T-T3: history row (GroupID=1) must contain {{dim}} wrapper; line:\n%s", l)
			}
		}
		if strings.Contains(l, "ToolCurrent") {
			// Current row must NOT contain {{dim}}.
			if strings.Contains(l, "{{dim}}") {
				currentLineNoDim = false
				t.Errorf("T-T3: current row (GroupID=2, max) must NOT contain {{dim}}; line:\n%s", l)
			}
		}
	}

	if !historyLineDim {
		t.Errorf("T-T3: history row marker 'ToolHistory' not found or not dim-wrapped in output:\n%s", out)
	}
	_ = currentLineNoDim // already reported above if violated
}

// ----------------------------------------------------------------------------
// T-T4: TestTable_LegendBottom
//
// Spec T-18: the footer area must contain a legend row with column labels
// (e.g. "# role model cache out cost tool") instead of the "Total for request"
// aggregation row. The string "Total for request" must be absent.
//
// The legend row must appear as a content line before the bottom border (└…┘).
// ----------------------------------------------------------------------------
func TestTable_LegendBottom(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turn := makeOrchTurn(1, "uuid-leg", 1, base, "BashCmd", 100)
	turns := []parser.Turn{turn}

	st := &state.Session{}
	out := renderRedesign(turns, st, 80)

	if out == "" {
		t.Fatal("RenderUnified() returned empty string; want non-empty table (RED)")
	}

	// "Total for request" must be absent from the new design.
	if strings.Contains(out, "Total for request") {
		t.Errorf("T-T4: new table must NOT contain \"Total for request\" footer label\noutput:\n%s", out)
	}

	// Legend keywords must all appear somewhere in the output (column labels).
	// The legend line contains column header names.
	legendKeywords := []string{"role", "model", "cache", "out", "cost", "tool"}
	for _, kw := range legendKeywords {
		if !strings.Contains(out, kw) {
			t.Errorf("T-T4: legend row must contain column label %q; not found\noutput:\n%s", kw, out)
		}
	}

	// The legend line must appear before the bottom border.
	lines := collectLines(out)
	if len(lines) == 0 {
		t.Fatal("T-T4: no lines in output")
	}
	bottomBorder := lines[len(lines)-1]
	bareBottom := stripMk(bottomBorder)
	if !strings.HasPrefix(bareBottom, "└") {
		t.Errorf("T-T4: last line must be bottom border starting with '└'; got: %s", bottomBorder)
	}

	// Legend row must precede the bottom border — find it above the last line.
	legendLineFound := false
	for _, l := range lines[:len(lines)-1] {
		bare := stripMk(l)
		// Legend line is a content row (starts with │) containing "role" and "model".
		if strings.HasPrefix(bare, "│") && strings.Contains(bare, "role") && strings.Contains(bare, "model") {
			legendLineFound = true
			break
		}
	}
	if !legendLineFound {
		t.Errorf("T-T4: no legend row (│…role…model…) found before bottom border\noutput:\n%s", out)
	}
}

// ----------------------------------------------------------------------------
// T-T5: TestTable_ThinkingGlyph
//
// Spec T-19: when Turn.Thinking=true, the tool column must contain the thinking
// glyph (a constant defined by the dev — we verify with Contains, not exact match).
// When Turn.Thinking=false and ToolUse is set, the tool column must contain the
// tool name. The two cases must be mutually exclusive.
// ----------------------------------------------------------------------------
func TestTable_ThinkingGlyph(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Turn with Thinking=true, no ToolUse (spec: thinking AND no tool_use).
	thinkingTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "sonnet-4",
		UUID:        "uuid-think",
		GroupID:     2,
		Timestamp:   base.Add(5 * time.Second),
		Tokens:      parser.TokenCounts{Output: 150},
		ToolUse:     "", // no tool during thinking
		Thinking:    true,
		IsSidechain: false,
	}

	// Turn with Thinking=false and ToolUse set.
	toolTurn := parser.Turn{
		Index:       2,
		Role:        "orch",
		Model:       "sonnet-4",
		UUID:        "uuid-tool",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 200},
		ToolUse:     "WriteFile",
		Thinking:    false,
		IsSidechain: false,
	}

	turns := []parser.Turn{thinkingTurn, toolTurn}

	st := &state.Session{}
	out := renderRedesign(turns, st, 80)

	if out == "" {
		t.Fatal("RenderUnified() returned empty string; want non-empty table (RED)")
	}

	// Locate the thinking turn's row (UUID or row position: thinkingTurn is newest).
	// We identify it by finding the row that does NOT contain "WriteFile".
	// The thinking glyph must be present in that row's tool cell.
	//
	// The spec does not fix the glyph value (dev decides 💭 vs "…" vs "⟳").
	// We only require that the tool cell is NOT empty and NOT "WriteFile" for the
	// thinking row.
	lines := collectLines(out)
	var thinkingRowLine, toolRowLine string
	for _, l := range lines {
		bare := stripMk(l)
		if !strings.HasPrefix(bare, "│") || strings.Contains(bare, "─") {
			continue // skip borders
		}
		if strings.Contains(bare, "WriteFile") {
			toolRowLine = l
		}
		// The thinking row is a data row that does NOT contain "WriteFile".
		// We distinguish it by its Timestamp position (it should be the first data row).
		// Since both rows lack a unique text marker, we rely on the fact that thinkingTurn
		// is newest-first → it appears before toolTurn.
	}

	// Find both data rows by their position: thinkingTurn (index 1 of data rows) and toolTurn.
	dataRows := []string{}
	for _, l := range lines {
		bare := stripMk(l)
		if strings.HasPrefix(bare, "│") && !strings.Contains(bare, "─") &&
			!strings.Contains(bare, "role") { // exclude legend row
			dataRows = append(dataRows, l)
		}
	}

	if len(dataRows) < 2 {
		t.Fatalf("T-T5: expected at least 2 data rows; got %d\noutput:\n%s", len(dataRows), out)
	}

	// First data row = thinkingTurn (newest, GroupID=2).
	thinkingRowLine = dataRows[0]
	if toolRowLine == "" && len(dataRows) >= 2 {
		toolRowLine = dataRows[1]
	}

	// Thinking row: tool cell must be non-empty (glyph present) and must NOT equal "WriteFile".
	thinkingRowBare := stripMk(thinkingRowLine)
	if strings.Contains(thinkingRowBare, "WriteFile") {
		t.Errorf("T-T5: thinking row must not contain tool name 'WriteFile' (should show glyph instead)\nrow: %s", thinkingRowLine)
	}
	// The thinking glyph must make the tool cell non-trivially populated.
	// We verify by checking the raw output contains some non-space content in the
	// last column of the thinking row (the glyph occupies the tool cell).
	// Minimally: the tool cell must not be all spaces after stripping markers.
	toolCellContent := extractLastCell(thinkingRowBare)
	if strings.TrimSpace(toolCellContent) == "" {
		t.Errorf("T-T5: thinking row tool cell must contain a glyph (non-empty); got empty cell\nrow: %s", thinkingRowLine)
	}

	// Tool row: must contain "WriteFile" in the tool cell.
	if !strings.Contains(stripMk(toolRowLine), "WriteFile") {
		t.Errorf("T-T5: tool row must contain 'WriteFile' in tool column; got:\n%s", toolRowLine)
	}
}

// extractLastCell returns the content of the last cell in a │-delimited row.
func extractLastCell(row string) string {
	parts := strings.Split(row, "│")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] // last non-empty segment before trailing │
}

// ----------------------------------------------------------------------------
// T-T6: TestTable_SubagentDash
//
// Spec T-20: a sidechain turn (IsSidechain=true) must show "—" in the cost
// column. An orchestrator turn must show the cost.PerTurn value when available,
// formatted as "$X.XX".
//
// Fixture:
//   - orchTurn: uuid="uuid-orch", IsSidechain=false. cost.PerTurn(st, "uuid-orch") = $0.42.
//   - subTurn:  uuid="uuid-sub",  IsSidechain=true.  cost must render "—".
//
// We pre-populate st.PerTurnCost so that cost.PerTurn returns the known value.
// ----------------------------------------------------------------------------
func TestTable_SubagentDash(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "sonnet-4",
		UUID:        "uuid-orch",
		GroupID:     1,
		Timestamp:   base.Add(10 * time.Second),
		Tokens:      parser.TokenCounts{Output: 200},
		ToolUse:     "BashOrch",
		IsSidechain: false,
	}

	subTurn := parser.Turn{
		Role:        "sub",
		Model:       "sonnet-4",
		UUID:        "uuid-sub",
		GroupID:     1,
		Timestamp:   base.Add(5 * time.Second),
		Tokens:      parser.TokenCounts{Output: 50},
		ToolUse:     "BashSub",
		IsSidechain: true,
	}

	// Pre-populate PerTurnCost so cost.PerTurn("uuid-orch") returns $0.42.
	st := &state.Session{
		Initialized: true,
		PerTurnCost: map[string]float64{
			"uuid-orch": 0.42,
		},
	}

	// Sanity-check: cost.PerTurn must work as expected before invoking render.
	if v, ok := cost.PerTurn(st, "uuid-orch"); !ok || v != 0.42 {
		t.Fatalf("fixture sanity: cost.PerTurn(st, 'uuid-orch') = (%v, %v); want (0.42, true)", v, ok)
	}
	if _, ok := cost.PerTurn(st, "uuid-sub"); ok {
		t.Fatalf("fixture sanity: cost.PerTurn(st, 'uuid-sub') must return ok=false for sidechain; got ok=true")
	}

	// Merged newest-first: orchTurn (ts+10s) then subTurn (ts+5s).
	turns := []parser.Turn{orchTurn, subTurn}

	out := renderRedesign(turns, st, 80)

	if out == "" {
		t.Fatal("RenderUnified() returned empty string; want non-empty table (RED)")
	}

	// Locate the orch row (contains "BashOrch") and sub row (contains "BashSub").
	orchRowFound := false
	subRowFound := false

	for _, l := range strings.Split(out, "\n") {
		bare := stripMk(l)
		if !strings.HasPrefix(bare, "│") || strings.Contains(bare, "─") {
			continue
		}
		if strings.Contains(bare, "BashOrch") {
			orchRowFound = true
			// Orch row must contain "$0.42".
			expected := cost.Format(0.42) // "$0.42"
			if !strings.Contains(bare, expected) {
				t.Errorf("T-T6: orch row must contain cost %q; row:\n%s", expected, l)
			}
			// Must NOT contain "—" in the cost position (i.e., not use the dash fallback).
			// We only assert $0.42 presence above.
		}
		if strings.Contains(bare, "BashSub") {
			subRowFound = true
			// Sub row must contain "—" for cost (IsSidechain=true).
			if !strings.Contains(bare, "—") {
				t.Errorf("T-T6: subagent row must contain '—' for cost; row:\n%s", l)
			}
		}
	}

	if !orchRowFound {
		t.Errorf("T-T6: orch row marker 'BashOrch' not found in output:\n%s", out)
	}
	if !subRowFound {
		t.Errorf("T-T6: sub row marker 'BashSub' not found in output:\n%s", out)
	}
}
