// Package statusline_test — RED tests for Phase 6.8 FIXES: C1 table redesign wired.
//
// Root cause (from review-consolidated.md C1):
//   perTurnTable() calls the old Builder.Add/AddSubagents/RenderForCols path.
//   RenderUnified (the new redesigned table) is never called from assembler.
//   The old path produces "Total for request" footer and no ├┼┤ separators.
//
// Fix vector: rewrite perTurnTable to merge orch+subagent Turns by timestamp,
//   pass *state.Session, and call RenderUnified instead of old Builder path.
//
// Production path verified: Assembler.Render(d) → perTurnTable(d, cols) → RenderUnified.
//
// RED: all sub-tests below FAIL until perTurnTable calls RenderUnified.
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// makeC1Assembler creates a Standard-mode Assembler with wide cols and swapped
// line0/line1/line2 registries (fakes) so the table is the only variable.
func makeC1Assembler() *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   mode.Standard,
		Theme:  renderer.Theme{AnsiEnabled: false},
		Cols:   120,
		Config: probes.Config{},
	}
}

// makeC1Data builds a probes.Data with orchestrator Turns in two GroupIDs
// (GroupID 1=older/history, GroupID 2=current), plus one sidechain Turn
// (IsSidechain=true) interleaved between them by timestamp.
//
// Also includes a Thinking=true turn to verify glyph rendering.
//
// Timestamps (base = 2026-06-01T12:00:00Z):
//
//	base+0s:  orchTurn1 (GroupID=1, ToolA, oldest)
//	base+10s: subTurn   (IsSidechain=true, SubTool)
//	base+20s: orchTurn2 (GroupID=2, ToolB)
//	base+25s: thinkingTurn (GroupID=2, Thinking=true, newest)
//
// Turns are listed newest-first in Session.Turns for perTurnTable.
func makeC1Data() probes.Data {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchTurn1 := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-orch1",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 100},
		ToolUse:     "ToolA",
		IsSidechain: false,
	}

	subTurn := parser.Turn{
		Role:        "sub",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-sub",
		GroupID:     1,
		Timestamp:   base.Add(10 * time.Second),
		Tokens:      parser.TokenCounts{Output: 50},
		ToolUse:     "SubTool",
		IsSidechain: true,
	}

	orchTurn2 := parser.Turn{
		Index:       2,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-orch2",
		GroupID:     2,
		Timestamp:   base.Add(20 * time.Second),
		Tokens:      parser.TokenCounts{Output: 200},
		ToolUse:     "ToolB",
		IsSidechain: false,
	}

	thinkingTurn := parser.Turn{
		Index:       3,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-think",
		GroupID:     2,
		Timestamp:   base.Add(25 * time.Second),
		Tokens:      parser.TokenCounts{Output: 150},
		ToolUse:     "",
		IsSidechain: false,
		Thinking:    true,
	}

	perTurnCosts := map[string]float64{
		"uuid-orch1": 0.10,
		"uuid-orch2": 0.20,
		"uuid-think": 0.05,
	}
	costFn := func(uuid string) (float64, bool) {
		v, ok := perTurnCosts[uuid]
		return v, ok
	}

	return probes.Data{
		Session: &parser.SessionStats{
			// Newest-first ordering — matches how perTurnTable must pass them to RenderUnified.
			Turns: []parser.Turn{thinkingTurn, orchTurn2, subTurn, orchTurn1},
			Totals: parser.TokenCounts{
				CacheRead:   1000,
				CacheCreate: 500,
				Output:      500,
			},
			TurnCount: 4,
		},
		Subagents: []parser.SubagentStats{
			{
				AgentID:   "agent-x",
				AgentType: "code-reviewer",
				Model:     "claude-sonnet-4-6",
				Tokens:    parser.TokenCounts{Output: 50},
				LastTool:  "Read",
			},
		},
		PerTurnCostFn: costFn,
		Now:           time.Date(2026, 6, 1, 12, 0, 30, 0, time.UTC),
		TerminalCols:  120,
	}
}

// ---------------------------------------------------------------------------
// C1-a: TestAssembler_Table_GroupSeparator
//
// Spec T-16: the first row of each new GroupID (scanning top-to-bottom) uses
// ├┼┤ as cell dividers. With GroupID=1 and GroupID=2, exactly one ├┼┤ group-
// separator line must appear BETWEEN data rows (not only as part of header/footer).
//
// Distinguishing feature of the new group-separator vs old header line:
//   - New: starts with ├ and ends with ┤ and contains ┼ (all same rune style, full-width dashes).
//   - Old: the old Builder header is ├────┬──...──┤ (contains ┬ not ┼).
//
// Spec says the new separator uses ┼ as inner junction (not ┬).
//
// RED: old Builder path emits ├┬┤ (has ┬) not ├┼┤ (all ┼) for its separator row.
// After fix: exactly one ├┼┤ line exists (the inter-group separator).
// ---------------------------------------------------------------------------

func TestAssembler_Table_GroupSeparator(t *testing.T) {
	// Replace registries so only table matters in the output.
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeC1Assembler()
	out := a.Render(makeC1Data())

	// Count lines that match the new group-separator pattern:
	//   starts with ├, ends with ┤, contains ┼, does NOT contain ┬.
	// The old Builder separator uses ┬ (not ┼) as inner junction → does not match.
	newGroupSepCount := 0
	for _, l := range strings.Split(out, "\n") {
		bare := strings.TrimSpace(l)
		// Strip colour/marker tokens for comparison.
		stripped := strings.NewReplacer(
			"{{dim}}", "", "{{reset}}", "",
		).Replace(bare)
		if strings.HasPrefix(stripped, "├") && strings.HasSuffix(stripped, "┤") &&
			strings.Contains(stripped, "┼") && !strings.Contains(stripped, "┬") {
			newGroupSepCount++
		}
	}

	if newGroupSepCount == 0 {
		t.Errorf("C1 T-16: expected a group-separator line (├──┼──┤, no ┬) in Assembler output;\n"+
			"  got 0 — perTurnTable must call RenderUnified which uses ┼ not ┬ as inner junction\n"+
			"  (Old Builder emits ├──┬──┤ header which does NOT satisfy this check)\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// C1-b: TestAssembler_Table_LegendNotTotalFooter
//
// Spec T-18: footer = legend row with column labels; "Total for request" absent.
//
// RED: old Builder path emits "Total for request" footer.
// ---------------------------------------------------------------------------

func TestAssembler_Table_LegendNotTotalFooter(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeC1Assembler()
	out := a.Render(makeC1Data())

	// Old footer must be gone.
	if strings.Contains(out, "Total for request") {
		t.Errorf("C1 T-18: Assembler output must NOT contain 'Total for request' footer;\n"+
			"  found it — perTurnTable must use RenderUnified which has legend-row footer\noutput:\n%s", out)
	}

	// New legend keywords must be present.
	for _, kw := range []string{"role", "model", "cache", "out", "cost", "tool"} {
		if !strings.Contains(out, kw) {
			t.Errorf("C1 T-18: legend keyword %q not found in Assembler output\noutput:\n%s", kw, out)
		}
	}
}

// ---------------------------------------------------------------------------
// C1-c: TestAssembler_Table_SubagentInterleaved
//
// Spec T-15: subagent turns woven into the main stream by timestamp.
// SubTool (sidechain) must appear between ToolB (newer) and ToolA (older)
// in the output — NOT appended at the end as in the old AddSubagents path.
//
// RED: old path appends subagent rows after all orch rows via AddSubagents.
// ---------------------------------------------------------------------------

func TestAssembler_Table_SubagentInterleaved(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeC1Assembler()
	out := a.Render(makeC1Data())

	// Find positions of the three tool markers.
	posToolB := strings.Index(out, "ToolB")
	posSubTool := strings.Index(out, "SubTool")
	posToolA := strings.Index(out, "ToolA")

	if posToolB < 0 || posSubTool < 0 || posToolA < 0 {
		t.Fatalf("C1 T-15: one or more tool markers not found (posToolB=%d, posSubTool=%d, posToolA=%d)\noutput:\n%s",
			posToolB, posSubTool, posToolA, out)
	}

	// Newest-first: ToolB (newest) < SubTool (middle) < ToolA (oldest).
	// If old path: ToolB < ToolA < SubTool (subagent appended last) → test FAILS.
	if !(posToolB < posSubTool && posSubTool < posToolA) {
		t.Errorf("C1 T-15: subagent rows must be interleaved by timestamp;\n"+
			"  want ToolB(%d) < SubTool(%d) < ToolA(%d) — posSubTool must be between orch rows\n"+
			"  FIX: perTurnTable must merge turns+sidechain by timestamp, not AddSubagents at end\noutput:\n%s",
			posToolB, posSubTool, posToolA, out)
	}
}

// ---------------------------------------------------------------------------
// C1-d: TestAssembler_Table_ThinkingGlyph
//
// Spec T-19: Turn.Thinking=true → tool column shows thinking glyph (non-empty,
// not the turn's ToolUse name which is "").
//
// RED: old Builder path ignores Turn.Thinking; new RenderUnified uses thinkingGlyph.
// ---------------------------------------------------------------------------

func TestAssembler_Table_ThinkingGlyph(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeC1Assembler()
	out := a.Render(makeC1Data())

	// The table must contain a table border (at least ┌ from RenderUnified).
	if !strings.Contains(out, "┌") {
		t.Fatalf("C1 T-19: no table border '┌' in output — table not rendered\noutput:\n%s", out)
	}

	// The thinking turn (Thinking=true, ToolUse="") should produce a non-empty
	// tool cell — the glyph. We can't know the exact glyph, but we know ToolUse=""
	// so the old Builder would produce an empty tool cell. New RenderUnified shows 💭.
	// Detect presence of any multi-byte glyph character that isn't a box-draw char.
	// Simpler: assert that the output contains "💭" (the thinkingGlyph constant).
	if !strings.Contains(out, "💭") {
		t.Errorf("C1 T-19: thinking turn (Thinking=true, ToolUse='') must show thinking glyph '💭' in tool column;\n"+
			"  not found in Assembler output\n"+
			"  FIX: perTurnTable must call RenderUnified which renders thinkingGlyph\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// C1-e: TestAssembler_Table_SidechainDash
//
// Spec T-20: sidechain turns must show "—" in the cost column.
//
// RED: old Builder path uses cost for sidechain rows too (no dash logic).
// ---------------------------------------------------------------------------

func TestAssembler_Table_SidechainDash(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeC1Assembler()
	out := a.Render(makeC1Data())

	// Find the line containing SubTool (sidechain turn).
	subToolLineFound := false
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "SubTool") {
			subToolLineFound = true
			// Sidechain row must contain "—" for cost.
			if !strings.Contains(l, "—") {
				t.Errorf("C1 T-20: sidechain row (SubTool) must show '—' in cost column; row:\n%s", l)
			}
		}
	}

	if !subToolLineFound {
		// If SubTool not found, this is a T-15 failure — C1-c will catch it.
		t.Logf("C1 T-20: SubTool row not found — likely T-15 interleave failure (see C1-c test)")
	}
}
