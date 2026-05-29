// Package renderer_test — Phase 6.6.c RED tests for new table layout.
//
// T-13..T-18: new column widths {4,7,12,13,7,9,16}, new drop-order #→cost→trunc,
// AddSubagents row mapping, and footer exclusion of agent tokens.
//
// Threshold derivation for RenderForCols (§2.3):
//
//	New cols:         {4, 7, 12, 13, 7, 9, 16}
//	Content sum:       4+7+12+13+7+9+16 = 68
//	Full table width:  68 + 8 borders = 76
//
//	Step 1 — drop col "#" (width 4):
//	  6-col content: 68-4 = 64; borders: 7 → table width = 71
//	  Threshold: cols < 76 triggers the drop; result (71) fits when cols >= 71.
//	  T-14 test cols = 72 → "#" dropped, cost present.
//
//	Step 2 — also drop "cost" (width 9):
//	  5-col content: 64-9 = 55; borders: 6 → table width = 61
//	  Threshold: triggered when 71-wide result doesn't fit, i.e. cols < 71.
//	  T-15 test cols = 65 → "#" and "cost" dropped.
//
//	Step 3 — middle-truncate tool/arg:
//	  Triggered when 61-wide result doesn't fit, i.e. cols < 61.
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
// T-13: TestTable66_Widths
//
// NewBuilder(80).cols must equal {4,7,12,13,7,9,16}.
// Full table (Render()) width must equal 76 visual columns.
// -------------------------------------------------------------------
func TestTable66_Widths(t *testing.T) {
	b := renderer.NewBuilder(80)
	// Add one turn so Render() produces a non-empty table.
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string; want non-empty table (RED: NewBuilder stub has old widths)")
	}

	// Check fixed column widths via the exported Cols() accessor.
	// AddSubagents is undefined → this test will compile-fail until GREEN adds the method.
	// But the width assertion alone is enough to be RED on old widths.
	wantCols := [7]int{4, 7, 12, 13, 7, 9, 16}
	// NewBuilder must expose cols for white-box verification. Access via a small
	// shim: render a single-row table and measure each cell via visual width of
	// first content line. Alternatively, use the exported Cols() method once added.
	// For now, measure the top-border line to verify total width = 76.
	const wantWidth = 76 // 68 content + 8 borders

	lines := splitLines(out)
	if len(lines) == 0 {
		t.Fatal("Render() produced no lines")
	}

	// Every line in the table must be exactly wantWidth visual columns wide.
	for i, l := range lines {
		w := format.VisualLen(l)
		if w != wantWidth {
			t.Errorf("line[%d] visual width = %d; want %d (new widths {4,7,12,13,7,9,16})\nline: %s",
				i, w, wantWidth, l)
		}
	}

	// Verify column content widths by inspecting cells of the first content row.
	// The content row (not a border/separator) has the form: │cell0│cell1│...│cell6│
	var contentRow string
	for _, l := range lines {
		if strings.HasPrefix(l, "│") && !strings.Contains(l, "─") && !strings.Contains(l, "Total for request") {
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
			t.Errorf("col[%d] cell width = %d; want %d\ncell: %q", i, got, wantCols[i], cell)
		}
	}
}

// -------------------------------------------------------------------
// T-14: TestTable66_DropHashFirst
//
// RenderForCols(72): cols=72 < 76 (full) → col "#" must be dropped.
// Result must NOT contain the "#" column (first column; col index 0),
// but "cost" column must still be present.
//
// Threshold: full table = 76. Drop "#" → 71. At cols=72 the 71-wide
// result fits, so only "#" is dropped and cost remains.
// -------------------------------------------------------------------
func TestTable66_DropHashFirst(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, time.Minute))

	// cols=72: below 76 (triggers drop of "#") but above 71 (6-col result fits).
	out := b.RenderForCols(72)
	if out == "" {
		t.Fatal("RenderForCols(72) returned empty string; want non-empty table")
	}

	// Width of every line must be <= 72.
	for i, line := range splitLines(out) {
		w := format.VisualLen(line)
		if w > 72 {
			t.Errorf("line[%d] visual width = %d; want ≤ 72\nline: %s", i, w, line)
		}
	}

	// The "#" column (turn index "1") must NOT appear as a stand-alone cell
	// in the first content row at position 0. After drop the first column is "role".
	// We verify by checking that the content row does NOT begin with │ + right-aligned
	// single-digit number (which would mean # col is still present).
	// Concretely: in a 4-wide "#" col the content is right-aligned: "  1 " (4 chars).
	// After drop the first col is "role" (7-wide, left-aligned: " orch   ").
	var contentRow string
	for _, l := range splitLines(out) {
		if strings.HasPrefix(l, "│") && !strings.Contains(l, "─") && !strings.Contains(l, "Total for request") {
			contentRow = l
			break
		}
	}
	if contentRow == "" {
		t.Fatal("could not find content row in RenderForCols(72) output")
	}

	// After dropping "#", the content row starts with │ followed by the role cell
	// (7 wide, left-aligned: " orch   ").
	// The old first cell " #=1  " (4 wide, right-aligned) must NOT be the first cell.
	// Verify: row has 6 cells (not 7).
	parts := strings.Split(contentRow, "│")
	// parts[0]="" (before first │), parts[1..6]=cells, parts[7]="" (after last │)
	if len(parts) != 8 {
		t.Errorf("expected 8 parts (6 cells + 2 empty ends) in 6-col layout; got %d\nrow: %s", len(parts), contentRow)
	}

	// Cost column must still be present: cost cell contains "$" or "0.00" style string.
	// In 6-col layout cost is at position index 5 (0-based among content cells).
	if !strings.Contains(out, "$") && !strings.Contains(out, "0.00") {
		t.Errorf("RenderForCols(72) must keep cost column (drop only #); cost marker not found\noutput:\n%s", out)
	}
}

// -------------------------------------------------------------------
// T-15: TestTable66_DropHashAndCost
//
// RenderForCols(65): cols=65 < 71 (6-col result would be 71 wide, does
// not fit) → both "#" AND "cost" must be dropped → 5-col table (width 61).
//
// Threshold: 6-col width = 71. At cols=65 the 6-col result (71) overflows,
// so cost is additionally dropped; 5-col width = 61 which fits in 65.
// -------------------------------------------------------------------
func TestTable66_DropHashAndCost(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Edit", 5000, 300, 2*time.Minute))

	// cols=65: below 71 (6-col doesn't fit) but above 61 (5-col fits).
	out := b.RenderForCols(65)
	if out == "" {
		t.Fatal("RenderForCols(65) returned empty string; want non-empty table")
	}

	// All lines must be <= 65 visual columns.
	for i, line := range splitLines(out) {
		w := format.VisualLen(line)
		if w > 65 {
			t.Errorf("line[%d] visual width = %d; want ≤ 65\nline: %s", i, w, line)
		}
	}

	// Content row must have 5 cells (not 6 or 7).
	var contentRow string
	for _, l := range splitLines(out) {
		if strings.HasPrefix(l, "│") && !strings.Contains(l, "─") && !strings.Contains(l, "Total for request") {
			contentRow = l
			break
		}
	}
	if contentRow == "" {
		t.Fatal("could not find content row in RenderForCols(65) output")
	}

	parts := strings.Split(contentRow, "│")
	// 5-col: parts[0]="" parts[1..5]=cells parts[6]="" → 7 parts
	if len(parts) != 7 {
		t.Errorf("expected 7 parts (5 cells + 2 empty ends) in 5-col layout; got %d\nrow: %s", len(parts), contentRow)
	}

	// Cost ("$") must NOT appear in a content row (dropped).
	if strings.Contains(contentRow, "$") {
		t.Errorf("RenderForCols(65) content row must NOT contain cost '$'; got:\n%s", contentRow)
	}
}

// -------------------------------------------------------------------
// T-16: TestTable66_TruncTool
//
// RenderForCols(50): cols=50 < 61 (5-col result would be 61 wide) →
// tool/arg column is middle-truncated. Every line must fit in 50 cols.
// The "…" ellipsis marker must appear in output.
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

	// cols=50: below 61 (5-col doesn't fit) → tool/arg truncated.
	out := b.RenderForCols(50)
	if out == "" {
		t.Fatal("RenderForCols(50) returned empty string; want non-empty table")
	}

	// All lines must fit within 50 cols.
	for i, line := range splitLines(out) {
		w := format.VisualLen(line)
		if w > 50 {
			t.Errorf("line[%d] visual width = %d; want ≤ 50\nline: %s", i, w, line)
		}
	}

	// "…" must appear (middle-truncation indicator).
	if !strings.Contains(out, "…") {
		t.Errorf("RenderForCols(50) with 30-char tool must emit '…' (middle-truncation); not found\noutput:\n%s", out)
	}

	// Full tool string must NOT appear verbatim.
	if strings.Contains(out, longTool) {
		t.Errorf("RenderForCols(50) must truncate 30-char ToolUse; full string found\noutput:\n%s", out)
	}
}

// -------------------------------------------------------------------
// T-17: TestTable66_SubagentRow
//
// AddSubagents([SubagentStats{...}]) must produce a row where:
//   - "#" cell = "↳"
//   - role cell = AgentType (truncated to 7-wide col by padCell)
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

	// Verify AgentType appears in output (truncated by padCell to 7-wide col).
	// "code-reviewer" is 13 chars → padCell inner=6 → middle-truncated to ≤6 runes.
	// Verify that some prefix/truncation of "code-reviewer" appears (not the full string).
	if strings.Contains(out, "code-reviewer") {
		// It's 13 chars, inner=6 chars → must be truncated; full string should NOT appear.
		t.Errorf("AddSubagents: long AgentType 'code-reviewer' (13 chars) must be truncated in 7-wide col; full string found\noutput:\n%s", out)
	}
	// The truncated form must contain "…" for middle-truncation.
	// code-reviewer → MiddleTruncate("code-reviewer", 6) → "co…er" or similar with "…".

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
