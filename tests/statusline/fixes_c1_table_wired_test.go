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
// Notch redesign contract (N-notch, 2026-06-03):
//   - The standalone full-line ├─┼─┤ inter-group separator is REMOVED.
//   - Instead the anchor row (first row of each new GroupID scanning top-to-bottom)
//     carries notch dividers (├ leading, ┼ inner, ┤ trailing).
//   - A notch row is a data row: it starts with ├ AND contains cell content (spaces).
//
// Fixture: makeC1Data() has GroupID=1 (older) and GroupID=2 (current).
// Output (newest-first): GroupID=2 rows first, GroupID=1 row(s) below.
// The GroupID=1 row is the anchor and must use notch dividers.
//
// RED: current code (old Builder path) emits ├──┬──┤ horizontal separator; does
// not call RenderUnifiedRows at all → no notch rows, no ├ data rows.
// After fix: ≥1 notch data row (anchor for GroupID=1 boundary); 0 standalone
// inter-group ├─┼─┤ lines.
// ---------------------------------------------------------------------------
func TestAssembler_Table_GroupSeparator(t *testing.T) {
	// Replace registries so only table matters in the output.
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeC1Assembler()
	out := a.Render(makeC1Data())

	// Helper: strip marker tokens (brute-force, avoids importing format).
	stripTokens := func(s string) string {
		return strings.NewReplacer("{{dim}}", "", "{{reset}}", "").Replace(s)
	}

	// A. Notch data rows: lines starting with ├, containing ┼, AND containing spaces
	// (cell content). The old Builder emits no such lines (only ├──┬──┤ separators).
	notchCount := 0
	for _, l := range strings.Split(out, "\n") {
		bare := stripTokens(strings.TrimSpace(l))
		if strings.HasPrefix(bare, "├") &&
			strings.Contains(bare, "┼") &&
			strings.Contains(bare, " ") {
			notchCount++
		}
	}
	if notchCount == 0 {
		t.Errorf("C1 T-16 (notch): expected ≥1 notch data row (├ leading, ┼ inner, spaces present)\n"+
			"  in Assembler output; got 0.\n"+
			"  Old Builder path emits ├──┬──┤ header (no spaces, uses ┬) — RenderUnifiedRows\n"+
			"  must be called and must produce notch rows for group boundaries.\n"+
			"  output:\n%s", out)
	}

	// B. No standalone inter-group separator (old groupSep pattern: ├…┼…┤, no spaces).
	// The legend separator (pure horizontal ├…┼…┤) is the only allowed pure-horizontal
	// ├ line; additional standalone separators indicate the old groupSep was not removed.
	standaloneCount := 0
	for _, l := range strings.Split(out, "\n") {
		bare := stripTokens(strings.TrimSpace(l))
		if strings.HasPrefix(bare, "├") &&
			strings.Contains(bare, "┼") &&
			strings.HasSuffix(bare, "┤") &&
			!strings.Contains(bare, " ") {
			standaloneCount++
		}
	}
	// After fix: standaloneCount == 1 (legend sep only). 0 means legend sep absent
	// (separate F2 issue). ≥2 means old groupSep still present.
	if standaloneCount >= 2 {
		t.Errorf("C1 T-16 (notch): found %d standalone ├─┼─┤ lines;\n"+
			"  after notch redesign only the legend separator should remain (≤1).\n"+
			"  Old code emits inter-group ├─┼─┤ lines — those must be removed.\n"+
			"  output:\n%s", standaloneCount, out)
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
// C1-c: subagent interleave test REMOVED in Phase 6.9.e.
// Per-turn interleaving of subagent rows is superseded by the collapsed
// one-row-per-subagent design (contract tests T-10/T-11 in assembler_test.go).
// ---------------------------------------------------------------------------

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
	// tool cell. Phase 6.9.e (T-7) replaced the old 💭 glyph with the text
	// "thinking..." in the tool column; assert the new text is present.
	if !strings.Contains(out, "thinking...") {
		t.Errorf("C1 T-19: thinking turn (Thinking=true, ToolUse='') must show 'thinking...' in tool column;\n"+
			"  not found in Assembler output\n"+
			"  FIX: perTurnTable must call RenderUnified which renders the thinking text\noutput:\n%s", out)
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
