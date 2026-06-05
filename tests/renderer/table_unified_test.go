// Package renderer_test — RED tests for Phase 6.9.e unified table redesign.
//
// Test IDs covered:
//   - T-7:  Turn.Thinking → tool column = "thinking..." (text, not glyph)
//   - T-8:  ToolUse=="" && !Thinking → tool column = "thinking..."
//   - T-9:  sidechain # column = "↳N" (N=CurrentTurnNum), not bare "↳"
//   - T-26: orch cost cell = "$X.XX"; subagent cost cell = "Σ $X.XX"
//   - T-31: legend column cache label = "cache r/w"
//   - T-32: base column widths [4,14,12,13,7,9,13], table sum = 80
//
// Tests go through Assembler.Render (production path) per Integration AC.
// Direct RenderUnified calls are used only for low-level structural assertions
// that cannot be observed at the Assembler level (column widths, legend label).
package renderer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// ---------------------------------------------------------------------------
// Helpers shared across this file
// ---------------------------------------------------------------------------

// makeAssemblerUnified returns a Standard-mode assembler (renders table) with
// AnsiEnabled=false (marker tokens in output) and a fixed 80-col width.
func makeAssemblerUnified(cols int) *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   mode.Standard,
		Theme:  renderer.Theme{AnsiEnabled: false},
		Cols:   cols,
		Config: probes.Config{OrchTTLMinutes: 5},
	}
}

// makeDataWithTurns constructs probes.Data carrying the given turns as the
// session. now is used as d.Now for deterministic TTL computation.
func makeDataWithTurns(turns []parser.Turn, now time.Time, cols int) probes.Data {
	ss := &parser.SessionStats{
		TurnCount: len(turns),
		Turns:     turns,
	}
	if len(turns) > 0 {
		ss.LastTimestamp = turns[len(turns)-1].Timestamp
	}
	return probes.Data{
		Session:      ss,
		Now:          now,
		TerminalCols: cols,
	}
}

// makeDataWithState constructs probes.Data with a state.Session for per-turn cost.
func makeDataWithState(turns []parser.Turn, st *state.Session, now time.Time, cols int) probes.Data {
	d := makeDataWithTurns(turns, now, cols)
	d.State = st
	return d
}

// assembleStdOutput runs Assembler.Render in Standard mode and returns the raw
// marker-token output (no ANSI codes, AnsiEnabled=false).
func assembleStdOutput(t *testing.T, turns []parser.Turn, st *state.Session, now time.Time, cols int) string {
	t.Helper()
	a := makeAssemblerUnified(cols)
	d := makeDataWithState(turns, st, now, cols)
	return a.Render(d)
}

// collectDataLines returns lines from the table output that are data rows
// (notch redesign aware).
//
// A data row is a line whose bare (marker-stripped) form satisfies:
//   - starts with │ (regular data row) OR starts with ├ (notch anchor row), AND
//   - contains no ─ (excludes pure horizontal separator/border lines), AND
//   - is not the legend row (identified by " role " and " model " labels).
//
// Notch anchor rows start with ├ and carry cell content (spaces present),
// unlike pure horizontal lines (├─┼─┤) which have no spaces and contain ─.
func collectDataLines(out string) []string {
	var rows []string
	for _, l := range strings.Split(out, "\n") {
		bare := stripMk(l)
		// Accept both regular rows (│) and notch anchor rows (├).
		if !strings.HasPrefix(bare, "│") && !strings.HasPrefix(bare, "├") {
			continue
		}
		if strings.Contains(bare, "─") {
			continue // horizontal border line or pure separator
		}
		// Exclude legend row.
		if strings.Contains(bare, " role ") && strings.Contains(bare, " model ") {
			continue
		}
		rows = append(rows, l)
	}
	return rows
}

// ---------------------------------------------------------------------------
// T-7: TestRender_ThinkingText
//
// Spec §2.3: Turn.Thinking=true → tool column = "thinking..."
// Current impl uses thinkingGlyph="💭" — spec 6.9.e changes it to "thinking..."
// Test goes through Assembler.Render (production path).
// ---------------------------------------------------------------------------
func TestRender_ThinkingText(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Orchestrator turn with Thinking=true, no ToolUse (per spec §2.3).
	thinkingTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-think",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 100},
		ToolUse:     "", // no tool_use during thinking
		Thinking:    true,
		IsSidechain: false,
	}

	out := assembleStdOutput(t, []parser.Turn{thinkingTurn}, nil, base.Add(time.Minute), 80)

	if !strings.Contains(out, "┌") {
		t.Fatalf("T-7: no table in output (Standard mode should produce table); output:\n%s", out)
	}

	// Find the data row containing the thinking turn.
	// The tool cell must contain "thinking..." (not emoji glyph).
	foundThinkingText := false
	for _, l := range collectDataLines(out) {
		bare := stripMk(l)
		if strings.Contains(bare, "thinking...") {
			foundThinkingText = true
		}
		// Must NOT contain the old glyph.
		if strings.Contains(bare, "💭") {
			t.Errorf("T-7: tool cell must be 'thinking...' text, not emoji glyph '💭'; row: %s", l)
		}
	}

	if !foundThinkingText {
		t.Errorf("T-7: Turn.Thinking=true must render 'thinking...' in tool column; not found\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-8: TestRender_EmptyToolThinking
//
// Spec §2.3: ToolUse=="" && !Thinking → tool column = "thinking..."
// This is the "no activity yet" state — also renders "thinking...".
// Test goes through Assembler.Render.
// ---------------------------------------------------------------------------
func TestRender_EmptyToolThinking(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Orchestrator turn with no tool and Thinking=false.
	emptyToolTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-notool",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 50},
		ToolUse:     "", // empty tool
		Thinking:    false,
		IsSidechain: false,
	}

	out := assembleStdOutput(t, []parser.Turn{emptyToolTurn}, nil, base.Add(time.Minute), 80)

	if !strings.Contains(out, "┌") {
		t.Fatalf("T-8: no table in output; output:\n%s", out)
	}

	// The data row with no tool should render "thinking..." in tool cell.
	foundThinkingText := false
	for _, l := range collectDataLines(out) {
		bare := stripMk(l)
		if strings.Contains(bare, "thinking...") {
			foundThinkingText = true
		}
	}

	if !foundThinkingText {
		t.Errorf("T-8: ToolUse=='' && !Thinking must render 'thinking...' in tool column; not found\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-9: TestRender_SubagentArrowAndNum
//
// Spec §2.3: sidechain # column = "↳N" where N = SubagentStats.CurrentTurnNum.
// The # cell is NOT bare "↳" — it must include the activation count as a digit.
// Test goes through Assembler.Render with a subagent in d.Subagents.
// ---------------------------------------------------------------------------
func TestRender_SubagentArrowAndNum(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Orchestrator turn (anchor for GroupID).
	orchTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-orch",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 100},
		ToolUse:     "Bash",
		IsSidechain: false,
	}

	// Subagent with CurrentTurnNum=3 — the # column must show "↳3".
	subTurn := parser.Turn{
		Index:       1,
		Role:        "agent",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-sub-t1",
		GroupID:     0,
		Timestamp:   base.Add(2 * time.Second),
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 1000},
		ToolUse:     "Read",
		IsSidechain: true,
	}

	subStats := parser.SubagentStats{
		AgentID:         "agent-abc",
		AgentType:       "code-reviewer",
		Model:           "claude-sonnet-4-6",
		CurrentTurnNum:  3, // activation has 3 turns
		ActivationStart: base.Add(time.Second),
		LastTimestamp:   base.Add(2 * time.Second),
		TurnCount:       3,
		Turns:           []parser.Turn{subTurn},
	}

	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 1,
			Turns:     []parser.Turn{orchTurn},
		},
		Subagents:    []parser.SubagentStats{subStats},
		Now:          base.Add(3 * time.Minute),
		TerminalCols: 80,
	}

	a := makeAssemblerUnified(80)
	out := a.Render(d)

	if !strings.Contains(out, "┌") {
		t.Fatalf("T-9: no table in output; output:\n%s", out)
	}

	// The subagent # column must show ASCII "↳3" (normal digit); the old Unicode
	// subscript "↳₃" was dropped per user request (2026-06-05).
	foundArrowNum := false
	for _, l := range collectDataLines(out) {
		bare := stripMk(l)
		if strings.Contains(bare, "↳3") {
			foundArrowNum = true
		}
		// Unicode subscript "↳₃" is the old format — must NOT appear.
		if strings.Contains(bare, "↳₃") {
			t.Errorf("subagent #: subscript '↳₃' found; want ASCII '↳3'; row: %s", l)
		}
	}

	if !foundArrowNum {
		t.Errorf("subagent # column must show ASCII '↳3' (CurrentTurnNum=3); not found\noutput:\n%s", out)
	}
}

// TestRender_SubagentArrowFlushLeft verifies the subagent "↳N" arrow always sits
// flush against the left border (first position) for both single- and multi-digit
// N — the # column is flush-left for sidechain rows, not right-aligned.
func TestRender_SubagentArrowFlushLeft(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	orchTurn := parser.Turn{
		Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "uuid-orch",
		GroupID: 1, Timestamp: base, Tokens: parser.TokenCounts{Output: 100}, ToolUse: "Bash",
	}
	mkSub := func(id string, num int) parser.SubagentStats {
		turn := parser.Turn{
			Role: "agent", Model: "claude-haiku-4-5", UUID: "u-" + id, GroupID: 0,
			Timestamp: base.Add(time.Second), Tokens: parser.TokenCounts{Output: 50},
			ToolUse: "Read", IsSidechain: true,
		}
		return parser.SubagentStats{
			AgentID: id, AgentType: "agent", Model: "claude-haiku-4-5",
			CurrentTurnNum: num, ActivationStart: base.Add(time.Second),
			LastTimestamp: base.Add(time.Second), TurnCount: num,
			Turns: []parser.Turn{turn},
		}
	}
	d := probes.Data{
		Session:      &parser.SessionStats{TurnCount: 1, Turns: []parser.Turn{orchTurn}},
		Subagents:    []parser.SubagentStats{mkSub("two", 18), mkSub("one", 2)},
		Now:          base.Add(time.Minute),
		TerminalCols: 80,
	}
	out := makeAssemblerUnified(80).Render(d)

	subRows := 0
	for _, l := range collectDataLines(out) {
		bare := stripMk(l)
		if !strings.Contains(bare, "↳") {
			continue
		}
		subRows++
		// Flush-left: the row's first border is immediately followed by "↳"
		// (no leading space), regardless of single- vs multi-digit number.
		if !strings.HasPrefix(bare, "│↳") {
			t.Errorf("subagent arrow must be flush against the left bar (│↳…); got: %q", bare)
		}
	}
	if subRows != 2 {
		t.Errorf("expected 2 subagent rows (↳18 and ↳2), got %d\noutput:\n%s", subRows, out)
	}
}

// ---------------------------------------------------------------------------
// T-26: TestRender_CostCellFormat
//
// Spec §2.3: orch cost cell = "$X.XX"; subagent cost cell = "Σ $X.XX"
// Test goes through Assembler.Render with a state populated for per-turn cost.
// ---------------------------------------------------------------------------
func TestRender_CostCellFormat(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchTurn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-orch-cost",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 200, CacheCreate: 5000},
		ToolUse:     "BashOrch",
		IsSidechain: false,
	}

	st := &state.Session{
		Initialized: true,
		PerTurnCost: map[string]float64{
			"uuid-orch-cost": 0.42,
		},
	}

	subTurn := parser.Turn{
		Index:       1,
		Role:        "agent",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-sub-cost",
		GroupID:     0,
		Timestamp:   base.Add(time.Second),
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     "Read",
		IsSidechain: true,
	}

	// SubagentTotal for "uuid-agent-1" should be $0.15 via st.SubagentCost.
	// We set it in SubagentCost map (expected new field added by 6.9.a).
	// If field doesn't exist yet → test compiles but assertion fails (RED).

	subStats := parser.SubagentStats{
		AgentID:         "uuid-agent-1",
		AgentType:       "general-purpose",
		Model:           "claude-sonnet-4-6",
		CurrentTurnNum:  1,
		ActivationStart: base.Add(time.Second),
		LastTimestamp:   base.Add(time.Second),
		TurnCount:       1,
		Turns:           []parser.Turn{subTurn},
		Tokens:          parser.TokenCounts{Output: 50, CacheCreate: 500},
	}

	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 1,
			Turns:     []parser.Turn{orchTurn},
		},
		Subagents:    []parser.SubagentStats{subStats},
		State:        st,
		Now:          base.Add(2 * time.Minute),
		TerminalCols: 80,
	}

	a := makeAssemblerUnified(80)
	out := a.Render(d)

	if !strings.Contains(out, "┌") {
		t.Fatalf("T-26: no table in output; output:\n%s", out)
	}

	// Orchestrator row must contain "$0.42" (plain dollar format, no Σ).
	orchCostFound := false
	subCostFound := false
	for _, l := range collectDataLines(out) {
		bare := stripMk(l)
		if strings.Contains(bare, "BashOrch") {
			// Orch row: must contain "$0.42" and must NOT start with "Σ".
			if strings.Contains(bare, "$0.42") {
				orchCostFound = true
			}
			if strings.Contains(bare, "Σ $") {
				t.Errorf("T-26: orch cost cell must NOT contain 'Σ $'; got row: %s", l)
			}
		}
		if strings.Contains(bare, "Read") && strings.Contains(bare, "↳") {
			// Subagent row: must contain "Σ $" prefix.
			if strings.Contains(bare, "Σ $") {
				subCostFound = true
			}
		}
	}

	if !orchCostFound {
		t.Errorf("T-26: orch cost cell must be '$0.42'; not found\noutput:\n%s", out)
	}
	if !subCostFound {
		t.Errorf("T-26: subagent cost cell must contain 'Σ $'; not found\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-31: TestRender_LegendCacheLabel
//
// Spec §2.3: legend column cache label = "cache r/w"
// Current impl uses "cache" — spec 6.9.e changes to "cache r/w".
// Goes through RenderUnified directly (legend is structural, not production-path-specific).
// ---------------------------------------------------------------------------
func TestRender_LegendCacheLabel(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-leg",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 100},
		ToolUse:     "Bash",
		IsSidechain: false,
	}

	b := renderer.NewBuilder(80)
	out := b.RenderUnified([]parser.Turn{turn}, nil)

	// Legend row (contains "role" and "model") must also contain "cache r/w".
	legendFound := false
	for _, l := range strings.Split(out, "\n") {
		bare := stripMk(l)
		if strings.Contains(bare, "role") && strings.Contains(bare, "model") {
			legendFound = true
			if !strings.Contains(bare, "cache r/w") {
				t.Errorf("T-31: legend row must contain 'cache r/w' as cache column label; got row: %s", l)
			}
			// Must NOT have standalone "cache" without "r/w" in cache column position.
			// (Checking for exact old label "cache" that is NOT "cache r/w".)
			// We rely on Contains("cache r/w") above as positive assertion.
		}
	}

	if !legendFound {
		t.Errorf("T-31: legend row (containing 'role' and 'model') not found\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-32: TestRender_BaseColumnWidths
//
// Spec §2.3: base widths [#=4, role=14, model=12, cache=13, out=7, cost=9, tool=13].
// Content sum = 4+14+12+13+7+9+13 = 72; total with 8 borders = 80.
// Current impl uses [4,13,12,13,7,8,15] = 72 too, but different per-column values.
// The spec-6.9.e tweak: role+1 (13→14), cost+1 (8→9), tool−2 (15→13).
// We verify by rendering a single turn and measuring the visual width of each line.
// ---------------------------------------------------------------------------
func TestRender_BaseColumnWidths(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turn := parser.Turn{
		Index:       1,
		Role:        "orch",
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-width",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 100},
		ToolUse:     "Bash",
		IsSidechain: false,
	}

	b := renderer.NewBuilder(80)
	out := b.RenderUnified([]parser.Turn{turn}, nil)

	// Every non-empty line in the table (before marker stripping for visual len)
	// must have VisualLen == 80 when markers are stripped.
	// (Borders, data rows, legend row all span the full width.)
	const wantWidth = 80
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if l == "" {
			continue
		}
		bare := stripMk(l)
		vlen := visualLen(bare)
		if vlen != wantWidth {
			t.Errorf("T-32: table line visual width = %d, want %d; line: %q", vlen, wantWidth, bare)
		}
	}

	// Also verify the legend row has role col = 14 characters (including padding).
	// We identify the legend row and check the role cell width.
	// Cell width = colWidth + 1 (for │), but role starts after first │.
	// The role label is "role" padded to visual width 14 (inner=13, 1 space margin).
	// legend: │ role         │ model       │ cache r/w   │ ...
	//          ↑ 1sp "role" 9sp ↑
	// With role colWidth=14: inner=13, " role" + 9 spaces = 14 chars → correct.
	// With old colWidth=13: inner=12, " role" + 8 spaces = 13 chars → wrong.
	for _, l := range strings.Split(out, "\n") {
		bare := stripMk(l)
		if !strings.Contains(bare, "role") || !strings.Contains(bare, "model") {
			continue
		}
		// Split by │ to extract cells.
		cells := strings.Split(bare, "│")
		// cells[0]="" cells[1]=# cells[2]=role cells[3]=model ...
		if len(cells) < 4 {
			t.Errorf("T-32: legend row has unexpected cell count; row: %q", bare)
			continue
		}
		roleCell := cells[2] // role column content (with padding)
		if len(roleCell) != 14 {
			t.Errorf("T-32: role column width = %d, want 14 (spec 6.9.e role+1); cell: %q", len(roleCell), roleCell)
		}
		costCell := cells[6] // cost column content
		if len(cells) > 6 && len(costCell) != 9 {
			t.Errorf("T-32: cost column width = %d, want 9 (spec 6.9.e cost+1); cell: %q", len(costCell), costCell)
		}
		toolCell := cells[7] // tool column content
		if len(cells) > 7 && len(toolCell) != 13 {
			t.Errorf("T-32: tool column width = %d, want 13 (spec 6.9.e tool−2); cell: %q", len(toolCell), toolCell)
		}
	}
}

// visualLen returns the visual length of a string (rune count, for ASCII-only
// test assertions where format.VisualLen is overkill).
func visualLen(s string) int {
	return len([]rune(s))
}
