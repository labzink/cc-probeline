// Package renderer_test — Phase 6.6.c RED tests for new table layout.
//
// T-13..T-18: column widths updated in 6.6.d to {4,13,12,13,7,8,15}, new drop-order
// #→cost→trunc, AddSubagents row mapping, and footer exclusion of agent tokens.
//
// Threshold derivation for RenderForCols (§2.4, updated for 6.6.d):
//
//	New cols (6.6.d):  {4, 13, 12, 13, 7, 8, 15}
//	Content sum:        4+13+12+13+7+8+15 = 72
//	Full table width:   72 + 8 borders = 80
//
//	Step 1 — drop col "#" (width 4):
//	  6-col content: 72-4 = 68; borders: 7 → table width = 75
//	  Threshold: cols < 80 triggers the drop; result (75) fits when cols >= 75.
//	  T-14 test cols = 78 → "#" dropped, cost present.
//
//	Step 2 — also drop "cost" (width 8):
//	  5-col content: 68-8 = 60; borders: 6 → table width = 66
//	  Threshold: triggered when 75-wide result doesn't fit, i.e. cols < 75.
//	  T-15 test cols = 70 → "#" and "cost" dropped.
//
//	Step 3 — middle-truncate tool/arg:
//	  Triggered when 66-wide result doesn't fit, i.e. cols < 66.
//	  T-16 test cols = 50 → tool/arg truncated.
package renderer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// makeAgentStats builds a minimal parser.SubagentStats for use in subagent row tests.
func makeAgentStats(agentID, agentType, model, lastTool string, cacheRead, cacheCreate, output int) parser.SubagentStats {
	return parser.SubagentStats{
		AgentID:   agentID,
		AgentType: agentType,
		Model:     model,
		Tokens: parser.TokenCounts{
			CacheRead:   cacheRead,
			CacheCreate: cacheCreate,
			Output:      output,
		},
		LastTool: lastTool,
	}
}

// stripMk removes {{marker}} tokens so structural border/row detection can run
// against the bare box-drawing runes (borders are now wrapped in {{dim}}…{{reset}}).
func stripMk(s string) string { return format.StripMarkers(s) }

// maxVisualLen returns the maximum format.VisualLen across all non-empty lines.
func maxVisualLen(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		if w := format.VisualLen(line); w > max {
			max = w
		}
	}
	return max
}

// -------------------------------------------------------------------
// T-21: TestTable66_Widths (updated from T-13 for 6.6.d)
//
// NewBuilder(80).cols must equal {4,13,12,13,7,8,15} (6.6.d widths).
// Full table (Render()) width must equal 80 visual columns.
//
// Source of truth: spec-common.md §2.4.
// -------------------------------------------------------------------
func TestTable66_Widths(t *testing.T) {
	b := renderer.NewBuilder(80)
	// Add one turn so Render() produces a non-empty table.
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string; want non-empty table (RED: NewBuilder has old widths {4,7,12,13,7,9,16})")
	}

	// 6.6.d revised widths: role 7→13, cost 9→8, tool 16→15.
	wantCols := [7]int{4, 13, 12, 13, 7, 8, 15}
	// Full table: 72 content + 8 borders = 80.
	const wantWidth = 80 // was 76 in 6.6.c

	lines := splitLines(out)
	if len(lines) == 0 {
		t.Fatal("Render() produced no lines")
	}

	// Every line in the table must be exactly wantWidth visual columns wide.
	for i, l := range lines {
		w := format.VisualLen(l)
		if w != wantWidth {
			t.Errorf("line[%d] visual width = %d; want %d (6.6.d widths {4,13,12,13,7,8,15})\nline: %s",
				i, w, wantWidth, l)
		}
	}

	// Verify column content widths by inspecting cells of the first content row.
	// The content row (not a border/separator) has the form: │cell0│cell1│...│cell6│
	var contentRow string
	for _, l := range lines {
		if strings.HasPrefix(stripMk(l), "│") && !strings.Contains(l, "─") && !strings.Contains(l, "Total for request") {
			contentRow = l
			break
		}
	}
	if contentRow == "" {
		t.Fatal("could not find content row in Render() output")
	}

	// Split by │ to get individual cells. Expected cell widths match wantCols.
	parts := strings.Split(contentRow, "│")
	// parts[0]="" (before first │), parts[1..7]=cells, parts[8]="" (after last │)
	if len(parts) != 9 {
		t.Fatalf("expected 9 parts splitting by │; got %d\nrow: %s", len(parts), contentRow)
	}
	for i := 0; i < 7; i++ {
		cell := parts[i+1]
		got := format.VisualLen(cell)
		if got != wantCols[i] {
			t.Errorf("col[%d] cell width = %d; want %d (6.6.d widths)\ncell: %q", i, got, wantCols[i], cell)
		}
	}
}

// -------------------------------------------------------------------
// T-22 (part 1): TestTable66_DropHashFirst (updated for 6.6.d)
//
// RenderForCols(78): cols=78 ∈ (75,80) → col "#" must be dropped.
// Result must NOT contain the "#" column (first column; col index 0),
// but "cost" column must still be present.
//
// Threshold (6.6.d §2.4):
//
//	full table = 80. Drop "#" (4) → 6-col width = 68+7 = 75.
//	At cols=78 the 75-wide result fits, so only "#" is dropped and cost remains.
//
// -------------------------------------------------------------------
func TestTable66_DropHashFirst(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, time.Minute))

	// cols=78: below 80 (triggers drop of "#") but ≥ 75 (6-col result 75 fits).
	out := b.RenderForCols(78)
	if out == "" {
		t.Fatal("RenderForCols(78) returned empty string; want non-empty table")
	}

	// Width of every line must be <= 78.
	for i, line := range splitLines(out) {
		w := format.VisualLen(line)
		if w > 78 {
			t.Errorf("line[%d] visual width = %d; want ≤ 78\nline: %s", i, w, line)
		}
	}

	// The "#" column (turn index "1") must NOT appear as a stand-alone cell
	// in the first content row at position 0. After drop the first column is "role".
	// In a 4-wide "#" col the content is right-aligned: "  1 " (4 chars).
	// After drop the first col is "role" (13-wide, left-aligned in 6.6.d).
	var contentRow string
	for _, l := range splitLines(out) {
		if strings.HasPrefix(stripMk(l), "│") && !strings.Contains(l, "─") && !strings.Contains(l, "Total for request") {
			contentRow = l
			break
		}
	}
	if contentRow == "" {
		t.Fatal("could not find content row in RenderForCols(78) output")
	}

	// After dropping "#", the content row has 6 cells (not 7).
	parts := strings.Split(contentRow, "│")
	// parts[0]="" (before first │), parts[1..6]=cells, parts[7]="" (after last │)
	if len(parts) != 8 {
		t.Errorf("expected 8 parts (6 cells + 2 empty ends) in 6-col layout; got %d\nrow: %s", len(parts), contentRow)
	}

	// Cost column must still be present: cost cell contains "$" or "0.00" style string.
	// In 6-col layout cost is at position index 5 (0-based among content cells).
	if !strings.Contains(out, "$") && !strings.Contains(out, "0.00") {
		t.Errorf("RenderForCols(78) must keep cost column (drop only #); cost marker not found\noutput:\n%s", out)
	}
}

// -------------------------------------------------------------------
// T-22 (part 2): TestTable66_DropHashAndCost (updated for 6.6.d)
//
// RenderForCols(72): cols=72 ∈ (66,75) → both "#" AND "cost" must be
// dropped → 5-col table (width 66).
//
// Threshold (6.6.d §2.4):
//
//	6-col width = 75. At cols=72 the 6-col result (75) overflows,
//	so cost is additionally dropped; 5-col width = 66 which fits in 72.
//
// Distinguishes from old 6.6.c thresholds: with old full=76/sixColWidth=71,
// cols=72 falls in the range (71,76) → only "#" dropped, cost retained.
// Under 6.6.d cols=72 < 75 → both dropped. This test is RED on old code.
// -------------------------------------------------------------------
func TestTable66_DropHashAndCost(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Edit", 5000, 300, 2*time.Minute))

	// cols=72: below 75 (6-col doesn't fit) but above 66 (5-col fits).
	// On old code (sixColWidth=71): 72 ≥ 71 → only "#" dropped, cost kept → RED.
	out := b.RenderForCols(72)
	if out == "" {
		t.Fatal("RenderForCols(72) returned empty string; want non-empty table")
	}

	// All lines must be <= 72 visual columns.
	for i, line := range splitLines(out) {
		w := format.VisualLen(line)
		if w > 72 {
			t.Errorf("line[%d] visual width = %d; want ≤ 72\nline: %s", i, w, line)
		}
	}

	// Content row must have 5 cells (not 6 or 7).
	var contentRow string
	for _, l := range splitLines(out) {
		if strings.HasPrefix(stripMk(l), "│") && !strings.Contains(l, "─") && !strings.Contains(l, "Total for request") {
			contentRow = l
			break
		}
	}
	if contentRow == "" {
		t.Fatal("could not find content row in RenderForCols(72) output")
	}

	parts := strings.Split(contentRow, "│")
	// 5-col: parts[0]="" parts[1..5]=cells parts[6]="" → 7 parts
	if len(parts) != 7 {
		t.Errorf("expected 7 parts (5 cells + 2 empty ends) in 5-col layout; got %d\nrow: %s", len(parts), contentRow)
	}

	// Cost ("$") must NOT appear in a content row (dropped).
	// On old code: cost is retained at cols=72 (6-col layout) → contains "$" → RED.
	if strings.Contains(contentRow, "$") {
		t.Errorf("RenderForCols(72) content row must NOT contain cost '$' (both # and cost dropped at cols=72 in 6.6.d); got:\n%s", contentRow)
	}
}

// -------------------------------------------------------------------
// T-22 (part 3): TestTable66_TruncTool (updated for 6.6.d)
//
// RenderForCols(60): cols=60 < 66 (fiveColWidth) → tool/arg column is
// middle-truncated. Every line must fit in 60 cols.
// The "…" ellipsis marker must appear in output.
//
// Calculation (6.6.d §2.4): fixed5 = role13+model12+cache13+out7 = 45.
// At cols=60: flex = 60 − 6 borders − 45 = 9 ≥ 1 → trunc step runs.
// 5-col width = 45 + 9 + 6 = 60. tool inner = 8 → 30-char tool → "…".
// fiveColMinTotal = 53 (floor: flex=1 → 45+1+6=52+1=53).
// -------------------------------------------------------------------
func TestTable66_TruncTool(t *testing.T) {
	// Use a tool name long enough to require truncation.
	longTool := strings.Repeat("x", 30) // 30 runes — will not fit in tight tool/arg cell

	b := renderer.NewBuilder(80)
	b.Add(parser.Turn{
		Index:   1,
		Role:    "orch",
		Model:   "sonnet-4",
		Tokens:  parser.TokenCounts{Output: 100},
		ToolUse: longTool,
	})

	// cols=60: below fiveColWidth=66 → trunc step; flex=9, table fits in 60.
	out := b.RenderForCols(60)
	if out == "" {
		t.Fatal("RenderForCols(60) returned empty string; want non-empty table")
	}

	// All lines must fit within 60 cols.
	for i, line := range splitLines(out) {
		w := format.VisualLen(line)
		if w > 60 {
			t.Errorf("line[%d] visual width = %d; want ≤ 60\nline: %s", i, w, line)
		}
	}

	// "…" must appear (middle-truncation indicator).
	if !strings.Contains(out, "…") {
		t.Errorf("RenderForCols(60) with 30-char tool must emit '…' (middle-truncation); not found\noutput:\n%s", out)
	}

	// Full tool string must NOT appear verbatim.
	if strings.Contains(out, longTool) {
		t.Errorf("RenderForCols(60) must truncate 30-char ToolUse; full string found\noutput:\n%s", out)
	}
}

// -------------------------------------------------------------------
// T-17: TestTable66_SubagentRow
//
// AddSubagents([SubagentStats{...}]) must produce a row where:
//   - "#" cell = "↳"
//   - role cell = AgentType (truncated to 13-wide col by padCell, 6.6.d)
//   - model cell = SubagentStats.Model
//   - cache cell = "<CacheRead>/<CacheCreate>" (FormatK)
//   - out cell = SubagentStats.Tokens.Output (FormatK)
//   - cost cell = "—" (no source, BL-7)
//   - tool cell = LastTool (empty → "—")
//
// This test will compile-fail (RED) until Builder.AddSubagents is defined.
// -------------------------------------------------------------------
func TestTable66_SubagentRow(t *testing.T) {
	// Build a turn so we have at least one regular row.
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	// Build subagent stats manually.
	sub := makeAgentStats("abc123", "code-reviewer", "opus-4", "Bash", 2000, 500, 300)

	// AddSubagents is the new method (does not exist yet → compile-fail RED).
	b.AddSubagents([]parser.SubagentStats{sub})

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty after AddSubagents; want non-empty table")
	}

	// Verify "↳" marker is present (subagent row indicator in "#" column).
	if !strings.Contains(out, "↳") {
		t.Errorf("AddSubagents: output must contain '↳' in # column; not found\noutput:\n%s", out)
	}

	// Verify AgentType appears in output (truncated by padCell to 13-wide col in 6.6.d).
	// "code-reviewer" is 13 chars → padCell inner=12 → fits exactly without truncation (6.6.d).
	// The full string "code-reviewer" (13 chars) fits in inner=12? No: 13 > 12 → must be truncated.
	// padCell inner = col_width - 1 (margin) = 13 - 1 = 12. 13 chars > 12 → truncated.
	if strings.Contains(out, "code-reviewer") {
		// 13 chars, inner=12 → must be middle-truncated; full string should NOT appear.
		t.Errorf("AddSubagents: AgentType 'code-reviewer' (13 chars) must be truncated in 13-wide col (inner=12); full string found\noutput:\n%s", out)
	}
	// The truncated form must contain "…" for middle-truncation.
	// code-reviewer (13) → MiddleTruncate("code-reviewer", 12) → "code-r…iewer" with "…".
	// The only over-width cell here is role (model "opus-4" and tool "Bash" are short),
	// so "…" must originate from the truncated AgentType.
	if !strings.Contains(out, "…") {
		t.Errorf("AddSubagents: truncated AgentType 'code-reviewer' (13>12) must emit '…'; not found\noutput:\n%s", out)
	}

	// Verify cost cell is "—" (not a dollar amount).
	// Cost column for subagent must show "—" as per spec (BL-7, no source).
	if !strings.Contains(out, "—") {
		t.Errorf("AddSubagents: cost cell must contain '—' (no cost source for subagent); not found\noutput:\n%s", out)
	}

	// Verify model appears (short form without "claude-" prefix if stripped by Add logic).
	if !strings.Contains(out, "opus-4") {
		t.Errorf("AddSubagents: model 'opus-4' must appear in subagent row; not found\noutput:\n%s", out)
	}

	// Verify LastTool appears.
	if !strings.Contains(out, "Bash") {
		t.Errorf("AddSubagents: LastTool 'Bash' must appear in subagent row; not found\noutput:\n%s", out)
	}

	// Verify empty LastTool maps to "—".
	b2 := renderer.NewBuilder(80)
	b2.Add(makeTurn(1, "orch", "sonnet-4", "Read", 500, 100, 0))
	subNoTool := makeAgentStats("xyz", "general-purpose", "sonnet-4", "", 100, 50, 80)
	b2.AddSubagents([]parser.SubagentStats{subNoTool})
	out2 := b2.Render()
	if !strings.Contains(out2, "—") {
		t.Errorf("AddSubagents with empty LastTool: tool cell must be '—'; not found\noutput:\n%s", out2)
	}
}

// -------------------------------------------------------------------
// T-18: TestTable66_TotalExcludesAgents
//
// After b.Add(turns) + b.AddSubagents(subs), the footer "Total for request"
// aggregates (cache/out) must equal ONLY the sum of the turns passed via Add(),
// NOT including the subagent tokens.
//
// Verify: sum of turn tokens matches what appears in footer; subagent tokens
// are strictly additive to rows but NOT to aggregates.
// -------------------------------------------------------------------
func TestTable66_TotalExcludesAgents(t *testing.T) {
	// Turn with known token counts.
	b := renderer.NewBuilder(80)
	turn1 := makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0)
	turn2 := makeTurn(2, "orch", "opus-4", "Edit", 3000, 100, 0)
	b.Add(turn1)
	b.Add(turn2)

	// Subagent with different token counts that must NOT appear in footer totals.
	sub := makeAgentStats("sa1", "code-reviewer", "sonnet-4", "Bash", 5000, 2000, 800)
	b.AddSubagents([]parser.SubagentStats{sub})

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty after Add+AddSubagents; want non-empty table")
	}

	// Expected footer aggregates (turns only):
	//   aggCacheRead  = 1000 + 3000 = 4000 → FormatK = "4K"
	//   aggCacheCreate = 200 + 100  = 300  → FormatK = "300"
	//   aggOut         = 0 (Turn.Tokens.Output from makeTurn = value in 'out' param)
	//
	// makeTurn passes cache=cacheRead, out=output as param 5/6.
	// makeTurn(1, ..., cache=1000, out=200, ...) → CacheRead=1000 Output=200
	// makeTurn(2, ..., cache=3000, out=100, ...) → CacheRead=3000 Output=100
	//   aggOut = 200 + 100 = 300 → FormatK = "300"
	//   aggCacheRead = 1000+3000=4000 → "4K"
	//   aggCacheCreate = 0+0=0 → "0"
	//
	// Subagent tokens (5000/2000 cache, 800 out) must NOT appear in footer.
	// If they leak, aggCacheRead = 9000 → "9K" instead of "4K".

	// Extract the footer row (contains "Total for request").
	var footerRow string
	for _, l := range splitLines(out) {
		if strings.Contains(l, "Total for request") {
			footerRow = l
			break
		}
	}
	if footerRow == "" {
		t.Fatal("footer row 'Total for request' not found in output")
	}

	// The footer must show aggregates from turns only.
	// aggOut from turns = 200+100 = 300 → FormatK("300") = "300".
	// Subagent out=800 → if leaked, total = 1100 → FormatK("1.1K") or "1100".
	// Subagent cacheRead=5000 → if leaked, total cacheRead = 4000+5000=9000 → "9K".

	// Assert "4K" appears in footer (cacheRead sum of turns).
	if !strings.Contains(footerRow, "4K") {
		t.Errorf("footer must contain '4K' (aggCacheRead=4000 from turns only); got:\n%s", footerRow)
	}

	// Assert subagent cacheRead ("5K" or "9K") does NOT bleed into footer.
	// If subagent cacheRead leaked, total = 9000 → "9K".
	if strings.Contains(footerRow, "9K") {
		t.Errorf("footer must NOT contain '9K' (subagent cacheRead=5K leaked into Total); got:\n%s", footerRow)
	}

	// Assert subagent output ("800") does NOT appear in footer output cell.
	// Turn out total = 300 → "300". Subagent out = 800 → if leaked total = 1100 → "1.1K" or "1100".
	if strings.Contains(footerRow, "1100") || strings.Contains(footerRow, "1.1K") {
		t.Errorf("footer must NOT contain subagent output tokens (1100 / 1.1K); got:\n%s", footerRow)
	}
}

// -------------------------------------------------------------------
// T-23: TestTable66_RoleWidth
//
// 6.6.d widens role column to 13 (inner = 12 = col_width - 1 margin).
//
//	Case A: AgentType exactly 12 runes → padCell fits without truncation.
//	        All 12 runes must appear verbatim in the content row.
//
//	Case B: AgentType exactly 13 runes → padCell inner=12 → middle-truncated
//	        to 12 runes total (11 chars + "…"). The full 13-rune string
//	        must NOT appear verbatim; "…" must be present.
//
// Source: spec-common.md §2.4 (role col 13, inner=12).
// -------------------------------------------------------------------
func TestTable66_RoleWidth(t *testing.T) {
	// Case A: 12-rune AgentType — must fit without truncation.
	// "code-review." is exactly 12 ASCII runes.
	roleExact12 := "code-review." // len=12
	if len([]rune(roleExact12)) != 12 {
		// Sanity: ensure fixture length is correct.
		t.Fatalf("test fixture roleExact12 has %d runes, want 12", len([]rune(roleExact12)))
	}

	b12 := renderer.NewBuilder(80)
	b12.Add(makeTurn(1, "orch", "sonnet-4", "Read", 500, 100, 0))
	b12.AddSubagents([]parser.SubagentStats{
		makeAgentStats("a1", roleExact12, "sonnet-4", "Read", 100, 50, 80),
	})

	out12 := b12.Render()
	if out12 == "" {
		t.Fatal("Case A: Render() returned empty string")
	}

	// The full 12-rune role value must appear verbatim in the output (no truncation).
	if !strings.Contains(out12, roleExact12) {
		t.Errorf("Case A: role value %q (12 runes) must appear verbatim in 13-wide col (inner=12); not found\noutput:\n%s",
			roleExact12, out12)
	}

	// "…" must NOT appear in the role cell for a 12-rune value.
	// Note: "…" may appear in tool/arg cell from other turns, so we check the
	// content row that carries our subagent (the one containing "↳").
	var subRow string
	for _, l := range splitLines(out12) {
		if strings.Contains(l, "↳") {
			subRow = l
			break
		}
	}
	if subRow == "" {
		t.Fatal("Case A: could not find subagent row (↳) in output")
	}
	// The role cell is the second cell (index 1, after "#"="↳").
	// Split and check that cell does not contain "…".
	parts12 := strings.Split(subRow, "│")
	if len(parts12) >= 3 {
		roleCell := parts12[2] // col index 1 = role, after "#" and before model
		if strings.Contains(roleCell, "…") {
			t.Errorf("Case A: role cell must NOT contain '…' for 12-rune value; got cell=%q\nrow: %s",
				roleCell, subRow)
		}
		// Verify the full value appears in the role cell.
		if !strings.Contains(roleCell, roleExact12) {
			t.Errorf("Case A: role cell must contain %q verbatim; got cell=%q\nrow: %s",
				roleExact12, roleCell, subRow)
		}
	} else {
		t.Fatalf("Case A: could not split subagent row into cells; got %d parts\nrow: %s",
			len(parts12), subRow)
	}

	// Case B: 13-rune AgentType — must be middle-truncated.
	// "code-reviewer" is exactly 13 ASCII runes.
	roleOver12 := "code-reviewer" // len=13
	if len([]rune(roleOver12)) != 13 {
		t.Fatalf("test fixture roleOver12 has %d runes, want 13", len([]rune(roleOver12)))
	}

	b13 := renderer.NewBuilder(80)
	b13.Add(makeTurn(1, "orch", "sonnet-4", "Read", 500, 100, 0))
	b13.AddSubagents([]parser.SubagentStats{
		makeAgentStats("b1", roleOver12, "sonnet-4", "Read", 100, 50, 80),
	})

	out13 := b13.Render()
	if out13 == "" {
		t.Fatal("Case B: Render() returned empty string")
	}

	// The full 13-rune string must NOT appear verbatim (it was truncated).
	if strings.Contains(out13, roleOver12) {
		t.Errorf("Case B: role value %q (13 runes) must be truncated in 13-wide col (inner=12); full string found\noutput:\n%s",
			roleOver12, out13)
	}

	// "…" must appear in the subagent row (middle-truncation indicator in role cell).
	var subRow13 string
	for _, l := range splitLines(out13) {
		if strings.Contains(l, "↳") {
			subRow13 = l
			break
		}
	}
	if subRow13 == "" {
		t.Fatal("Case B: could not find subagent row (↳) in output")
	}
	parts13 := strings.Split(subRow13, "│")
	if len(parts13) >= 3 {
		roleCell13 := parts13[2] // col index 1 = role
		if !strings.Contains(roleCell13, "…") {
			t.Errorf("Case B: role cell must contain '…' (middle-truncation) for 13-rune value; got cell=%q\nrow: %s",
				roleCell13, subRow13)
		}
		// The truncated role cell must still be exactly 13 visual runes wide (col width=13).
		cellWidth := format.VisualLen(roleCell13)
		if cellWidth != 13 {
			t.Errorf("Case B: role cell visual width = %d; want 13 (6.6.d col width)\ncell: %q",
				cellWidth, roleCell13)
		}
	} else {
		t.Fatalf("Case B: could not split subagent row into cells; got %d parts\nrow: %s",
			len(parts13), subRow13)
	}
}
