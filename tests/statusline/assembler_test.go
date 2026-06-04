// Package statusline_test contains RED tests for Phase 4.2.d — Multi-line assembler.
// All tests target internal/statusline.Assembler.Render, which currently returns "".
// Tests asserting non-empty output FAIL on the stub; tests asserting absence of
// something in "" PASS trivially — those are marked with
// "// PASS on stub: intentional, becomes meaningful after GREEN".
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// fakeProbe is a minimal Probe implementation that returns fixed strings,
// used to isolate assembler logic from real probe implementations.
// Compact / minimal default to out / "" so existing Phase 4.2 tests that
// only set `out` keep their previous behaviour (Full == Compact == out).
// Phase 4.3 integration tests can populate compact and minimal explicitly to
// exercise FitLine downgrade behaviour.
type fakeProbe struct {
	name     string
	priority int
	minWidth int
	visible  bool
	out      string
	compact  string // optional: rendered at LevelCompact; if "" falls back to out
	minimal  string // optional: rendered at LevelMinimal; "" means dropped by assemble
}

func (f *fakeProbe) Name() string                                { return f.name }
func (f *fakeProbe) Priority() int                               { return f.priority }
func (f *fakeProbe) MinWidth() int                               { return f.minWidth }
func (f *fakeProbe) Visible(_ probes.Data, _ probes.Config) bool { return f.visible }
func (f *fakeProbe) Render(_ probes.Data, _ probes.Config, _ renderer.Theme, l probes.Level) string {
	switch l {
	case probes.LevelCompact:
		// Empty compact means "not set" — falls back to out (Full).
		// To express "invisible at Compact", use compactSet bool or a sentinel.
		if f.compact != "" {
			return f.compact
		}
		return f.out
	case probes.LevelMinimal:
		return f.minimal
	default: // LevelFull
		return f.out
	}
}

// makeTurns constructs a slice of n minimal parser.Turn values for use in test
// SessionStats.Turns.
func makeTurns(n int) []parser.Turn {
	turns := make([]parser.Turn, n)
	for i := range turns {
		turns[i] = parser.Turn{
			Index: i + 1,
			Role:  "orch",
			Model: "sonnet-4-6",
		}
	}
	return turns
}

// makeData constructs a probes.Data with the given number of turns in the session.
func makeData(turns int) probes.Data {
	var session *parser.SessionStats
	if turns >= 0 {
		s := &parser.SessionStats{
			TurnCount: turns,
			Turns:     makeTurns(turns),
		}
		session = s
	}
	return probes.Data{
		Session:      session,
		Now:          time.Now(),
		TerminalCols: 80,
	}
}

// makeAssembler returns an Assembler configured with the given mode and a
// default zero Theme (AnsiEnabled: false — no ANSI escape codes).
func makeAssembler(m mode.Mode) *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   m,
		Theme:  renderer.Theme{},
		Cols:   80,
		Config: probes.Config{},
	}
}

// swapLine0 replaces Line0Registry for the duration of the test and restores
// it via t.Cleanup. Returns the backup for manual inspection if needed.
func swapLine0(t *testing.T, ps []probes.Probe) {
	t.Helper()
	old := probes.Line0Registry
	probes.Line0Registry = ps
	t.Cleanup(func() { probes.Line0Registry = old })
}

// swapLine1 replaces Line1Registry for the duration of the test.
func swapLine1(t *testing.T, ps []probes.Probe) {
	t.Helper()
	old := probes.Line1Registry
	probes.Line1Registry = ps
	t.Cleanup(func() { probes.Line1Registry = old })
}

// swapLine2 replaces Line2Registry for the duration of the test.
func swapLine2(t *testing.T, ps []probes.Probe) {
	t.Helper()
	old := probes.Line2Registry
	probes.Line2Registry = ps
	t.Cleanup(func() { probes.Line2Registry = old })
}

// ---------------------------------------------------------------------------
// TestAssembler_SuperCompact_3Lines
// §4.2 concept: SuperCompact mode emits line0 + line1 + line2 — no table, no
// footer. The hint widget may add one more line (Phase 4.4), so the result has
// ≥2 "\n" (≥3 lines). No table border "┌" must appear.
// ---------------------------------------------------------------------------
func TestAssembler_SuperCompact_3Lines(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeAssembler(mode.SuperCompact)
	out := a.Render(makeData(0))

	// §4.2 concept line 638-643: SuperCompact path emits 3 header lines; hint
	// widget (Phase 4.4) may add a 4th line. Assert ≥2 newlines (≥3 lines).
	if got := strings.Count(out, "\n"); got < 2 {
		t.Errorf("SuperCompact: expected at least 2 newlines (≥3 lines), got %d; output: %q", got, out)
	}

	// No table border in SuperCompact.
	if strings.Contains(out, "┌") {
		t.Errorf("SuperCompact: unexpected table top-border in output: %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Standard_WithTable
// §4.2 concept / C1 update: Standard mode appends perTurnTable.
// With 3 turns the table top-border "┌" must appear. C1 (Phase 6.8 FIXES)
// replaced the "Total for request" footer with a legend row ("role","model",...).
// ---------------------------------------------------------------------------
func TestAssembler_Standard_WithTable(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeAssembler(mode.Standard)
	out := a.Render(makeData(3))

	// perTurnTable rendered for Standard mode: top-border must appear.
	if !strings.Contains(out, "┌") {
		t.Errorf("Standard+3turns: expected table top-border '┌' in output; got %q", out)
	}

	// C1: footer is now a legend row (not "Total for request").
	if strings.Contains(out, "Total for request") {
		t.Errorf("Standard+3turns: 'Total for request' footer must be absent (C1 redesign); got %q", out)
	}
	for _, kw := range []string{"role", "model", "cost"} {
		if !strings.Contains(out, kw) {
			t.Errorf("Standard+3turns: legend keyword %q missing from table output; got %q", kw, out)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Standard_Cap20Turns
// §4.2 concept line 712-718 + C-6: when Session.Turns > 20 the assembler caps
// at the last 20 turns. Count row-content lines (lines containing "│" that are
// NOT top/bottom border) = 20.
// ---------------------------------------------------------------------------
func TestAssembler_Standard_Cap20Turns(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "c"}})

	a := makeAssembler(mode.Standard)
	out := a.Render(makeData(30))

	// Count data rows: lines that start with "│" OR "├" (notch anchor rows),
	// excluding pure horizontal separator lines (contain "─"), excluding the
	// legend row, and excluding top/bottom borders ("┌"/"└").
	// Notch redesign: anchor rows start with "├" and contain cell content
	// (spaces present, no "─"), so they are data rows, not separators.
	dataRowCount := 0
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(stripMk(line))
		// Accept rows starting with │ (regular) or ├ (notch anchor data rows).
		isDataRow := strings.HasPrefix(trimmed, "│") || strings.HasPrefix(trimmed, "├")
		if !isDataRow {
			continue
		}
		// Exclude pure horizontal lines (separators, borders): contain "─".
		if strings.Contains(trimmed, "─") {
			continue
		}
		// Exclude top/bottom borders.
		if strings.HasPrefix(trimmed, "┌") || strings.HasPrefix(trimmed, "└") {
			continue
		}
		// C1: exclude the legend footer row (contains column header labels).
		bareStripped := stripMk(line)
		if strings.Contains(bareStripped, "Total for request") ||
			(strings.Contains(bareStripped, " role ") && strings.Contains(bareStripped, " model ")) {
			continue
		}
		dataRowCount++
	}

	// §4.2 concept C-6: cap is 20, so data rows must equal 20.
	if dataRowCount != 20 {
		t.Errorf("Cap20: expected 20 data-row lines, got %d; output:\n%s", dataRowCount, out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Standard_NoTurns_NoTable
// §4.2 concept line 639: if Builder.Render() returns "" (no rows added), the
// assembler skips the table block entirely. No "┌" border, no orphan footer.
// ---------------------------------------------------------------------------
func TestAssembler_Standard_NoTurns_NoTable(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeAssembler(mode.Standard)
	out := a.Render(makeData(0))

	// §4.2 TestAssembler_Standard_NoTurns_NoTable: no table when 0 turns.
	// PASS on stub: intentional, becomes meaningful after GREEN.
	if strings.Contains(out, "┌") {
		t.Errorf("NoTurns: unexpected table top-border in output; got %q", out)
	}
	if strings.Contains(out, "Total for request") {
		t.Errorf("NoTurns: unexpected footer label 'Total for request'; got %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_HintEmpty_NoExtraLine
// §4.2 C-12: when hint widget returns "" (all hints shown, no alert), the
// assembler must NOT append an extra blank line beyond line2.
// We force the "all shown" scenario by pre-saving a fully-exhausted state.
// ---------------------------------------------------------------------------
func TestAssembler_HintEmpty_NoExtraLine(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	// Pre-save fully-exhausted hint state so widget returns "".
	shown := make([]int, len(hint.DefaultHints))
	for i := range shown {
		shown[i] = i
	}
	state := hint.State{
		ShownIndices: shown,
		CurrentIndex: len(hint.DefaultHints) - 1,
		LastSwitch:   time.Now(),
	}
	const sid = "exhausted-session"
	if err := hint.Save(sid, state); err != nil {
		t.Fatalf("hint.Save: %v", err)
	}

	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeAssembler(mode.SuperCompact)
	d := probes.Data{
		Session:      &parser.SessionStats{},
		SessionID:    sid,
		Now:          time.Now(),
		TerminalCols: 80,
	}
	out := a.Render(d)

	// C-12: empty hint → exactly 3 lines (2 newlines). No blank trailing line.
	if got := strings.Count(out, "\n"); got != 2 {
		t.Errorf("HintEmpty: expected 2 newlines (3 lines, no trailing hint), got %d; output: %q", got, out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_NoAnsiInOutput
// §4.2 C-10: Render must NOT emit ANSI escape codes. Only marker tokens
// ({{dim}}, {{reset}}, etc.) are allowed. Assert for both modes.
// PASS on stub: intentional (stub returns ""), becomes meaningful after GREEN.
// ---------------------------------------------------------------------------
func TestAssembler_NoAnsiInOutput(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	for _, m := range []mode.Mode{mode.SuperCompact, mode.Standard} {
		a := makeAssembler(m)
		out := a.Render(makeData(3))
		// §4.2 C-10: ANSI escape codes must be absent; only markers present.
		// PASS on stub: intentional, becomes meaningful after GREEN.
		if strings.Contains(out, "\x1b") {
			t.Errorf("NoAnsi[%s]: output contains ANSI escape code; output: %q", m, out)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_MarkersPresent
// §4.2 C-9: separators between probes on line0/line1 use {{dim}} and {{reset}}
// marker tokens. Assert output contains both substrings.
// ---------------------------------------------------------------------------
func TestAssembler_MarkersPresent(t *testing.T) {
	// Two probes on each line forces a separator to be emitted between them.
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "e", visible: true, out: "email@x"},
		&fakeProbe{name: "p", visible: true, out: "myproject"},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m", visible: true, out: "sonnet"},
		&fakeProbe{name: "t", visible: true, out: "12:34"},
	})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := makeAssembler(mode.SuperCompact)
	out := a.Render(makeData(0))

	// §4.2 C-9: separator tokens {{dim}} • {{reset}} must appear in output.
	if !strings.Contains(out, "{{dim}}") {
		t.Errorf("Markers: expected '{{dim}}' in output; got %q", out)
	}
	if !strings.Contains(out, "{{reset}}") {
		t.Errorf("Markers: expected '{{reset}}' in output; got %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Line2_PriorityOrder
// §4.2 test-gap: probes with different Priority() values must appear in
// ascending priority order (lower number = higher importance = leftmost).
// ---------------------------------------------------------------------------
func TestAssembler_Line2_PriorityOrder(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})

	// probeA has lower priority number → appears first (leftmost).
	probeA := &fakeProbe{name: "a", priority: 2, visible: true, out: "AAA"}
	// probeB has higher priority number → appears second (rightmost).
	probeB := &fakeProbe{name: "b", priority: 5, visible: true, out: "BBB"}
	// Register in reverse order to verify sorting works.
	swapLine2(t, []probes.Probe{probeB, probeA})

	a := makeAssembler(mode.SuperCompact)
	out := a.Render(makeData(0))

	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("PriorityOrder: expected at least 3 lines, got %d; output: %q", len(lines), out)
	}
	line2 := lines[2]

	// AAA (priority=2) must appear before BBB (priority=5).
	posA := strings.Index(line2, "AAA")
	posB := strings.Index(line2, "BBB")
	if posA < 0 || posB < 0 {
		t.Fatalf("PriorityOrder: line2=%q — expected both 'AAA' and 'BBB'", line2)
	}
	if posA >= posB {
		t.Errorf("PriorityOrder: AAA (priority=2) must appear before BBB (priority=5); line2=%q", line2)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Line2_PipeSeparator
// §4.2 C-9 + concept line 652: line2 uses " | " separator (NOT "•").
// Use a 2-probe Line2Registry swap to force the separator to appear.
// ---------------------------------------------------------------------------
func TestAssembler_Line2_PipeSeparator(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})

	// Two visible probes on line2 to force a separator between them.
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c1", visible: true, out: "cache-in:100"},
		&fakeProbe{name: "c2", visible: true, out: "cache-out:50"},
	})

	a := makeAssembler(mode.SuperCompact)
	out := a.Render(makeData(0))

	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("Line2Sep: expected at least 3 lines, got %d; output: %q", len(lines), out)
	}
	line2 := lines[2]

	// §4.2 C-9: line2 separator is " | ", not "•".
	if !strings.Contains(line2, " | ") {
		t.Errorf("Line2Sep: expected ' | ' separator in line2; got %q", line2)
	}
	// Ensure bullet separator is NOT used on line2.
	if strings.Contains(line2, " • ") {
		t.Errorf("Line2Sep: unexpected '•' separator in line2; got %q", line2)
	}
}

// =============================================================================
// Phase 6.9.e RED tests — T-3..T-13, T-26, T-30, T-33..T-36
// Tests go through Assembler.Render (production path per Integration AC).
// =============================================================================

// ---------------------------------------------------------------------------
// Helpers for Phase 6.9.e tests
// ---------------------------------------------------------------------------

// orchTurn builds an orchestrator parser.Turn for 6.9.e table tests.
// model defaults to "claude-sonnet-4-6" when empty.
func orchTurn(idx int, uuid string, groupID int, ts time.Time, tool string, model string) parser.Turn {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return parser.Turn{
		Index:       idx,
		Role:        "orch",
		Model:       model,
		UUID:        uuid,
		GroupID:     groupID,
		Timestamp:   ts,
		Tokens:      parser.TokenCounts{Output: 100, CacheCreate: 1000},
		ToolUse:     tool,
		IsSidechain: false,
	}
}

// sidechainTurn builds a sidechain (subagent) parser.Turn for 6.9.e tests.
func sidechainTurn(uuid string, ts time.Time, tool string) parser.Turn {
	return parser.Turn{
		Role:        "agent",
		Model:       "claude-sonnet-4-6",
		UUID:        uuid,
		GroupID:     0, // sidechain: 0
		Timestamp:   ts,
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     tool,
		IsSidechain: true,
	}
}

// makeStdAssembler returns a Standard-mode Assembler with given OrchTTLMinutes.
func makeStdAssembler(orchTTLMinutes int, cols int) *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   mode.Standard,
		Theme:  renderer.Theme{AnsiEnabled: false},
		Cols:   cols,
		Config: probes.Config{OrchTTLMinutes: orchTTLMinutes},
	}
}

// renderWithTurns builds probes.Data with given turns + optional subagents and
// runs Assembler.Render in Standard mode. Returns raw marker-token output.
func renderWithTurns(t *testing.T, turns []parser.Turn, subagents []parser.SubagentStats, now time.Time, orchTTLMinutes int) string {
	t.Helper()
	a := makeStdAssembler(orchTTLMinutes, 80)
	ss := &parser.SessionStats{TurnCount: len(turns), Turns: turns}
	if len(turns) > 0 {
		ss.LastTimestamp = turns[len(turns)-1].Timestamp
	}
	d := probes.Data{
		Session:      ss,
		Subagents:    subagents,
		Now:          now,
		TerminalCols: 80,
	}
	return a.Render(d)
}

// tableLines extracts non-empty lines from the table portion of Assembler output
// (lines containing "│" or box-drawing runes).
func tableLines(out string) []string {
	var result []string
	for _, l := range strings.Split(out, "\n") {
		if strings.ContainsAny(l, "│┌└├") || strings.Contains(stripMkA(l), "─") {
			result = append(result, l)
		}
	}
	return result
}

// stripMkA removes {{marker}} tokens for assertion purposes (alias of stripMk
// to avoid collision with the renderer_test package's stripMk).
func stripMkA(s string) string {
	result := s
	for strings.Contains(result, "{{") {
		start := strings.Index(result, "{{")
		end := strings.Index(result, "}}")
		if end < start || end < 0 {
			break
		}
		result = result[:start] + result[end+2:]
	}
	return result
}

// dataRows returns lines that are data rows (notch redesign aware).
//
// A data row is a line whose bare (marker-stripped) form satisfies:
//   - starts with │ (regular data row) OR starts with ├ (notch anchor row), AND
//   - contains no ─ (excludes pure horizontal separator/border lines), AND
//   - is not the legend row (identified by containing " role " and " model ").
//
// Notch anchor rows start with ├ and contain cell content (spaces), unlike
// pure horizontal lines (├─┼─┤) which have no spaces.
func dataRows(out string) []string {
	var rows []string
	for _, l := range strings.Split(out, "\n") {
		bare := stripMkA(l)
		// Accept both regular rows (│) and notch anchor rows (├).
		if !strings.HasPrefix(bare, "│") && !strings.HasPrefix(bare, "├") {
			continue
		}
		// Pure horizontal lines (separators, borders) contain ─.
		if strings.Contains(bare, "─") {
			continue
		}
		// Exclude legend row (contains column header labels).
		if strings.Contains(bare, " role ") && strings.Contains(bare, " model ") {
			continue
		}
		rows = append(rows, l)
	}
	return rows
}

// groupSepLines returns lines that are STANDALONE pure full-line ├─┼─┄ separators.
//
// After the notch redesign, inter-group boundaries are expressed as notch anchor
// data rows (├ + spaces + content) rather than standalone separator lines. The only
// remaining pure horizontal ├…┼…┤ lines are the legend separator and (if any)
// legacy inter-group separators. A standalone separator has no spaces (only ─ and
// junction runes). Notch data rows (which have spaces) are NOT returned.
//
// Callers that counted standalone separators to detect group boundaries should now
// assert on anchor notch rows via dataRows() or standaloneSepLines() from
// fixes_notch_dim_test.go. This helper's return value should be exactly 0
// between data rows and exactly 1 for the legend separator after the notch redesign.
func groupSepLines(out string) []string {
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
		// Notch data rows contain spaces (padded cells) and are excluded.
		if !strings.Contains(bare, " ") {
			seps = append(seps, l)
		}
	}
	return seps
}

// ---------------------------------------------------------------------------
// T-3: TestAssemble_SepOnlyOrchBoundary
//
// Notch redesign contract (replaces old "exactly 1 standalone sep per boundary"):
//   - ZERO standalone full-line ├─┼─┄ separators appear between data rows.
//     (The legend separator above the legend row is the only remaining pure
//     horizontal line and is excluded from the count below.)
//   - Instead, each orchestrator group has exactly one anchor notch row
//     (its chronologically earliest turn). Sidechain turns never become anchors.
//
// Table: wantStdSep = 0 always. wantAnchors = number of distinct orch groups.
// ---------------------------------------------------------------------------
func TestAssemble_SepOnlyOrchBoundary(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		turns       []parser.Turn
		wantAnchors int // one notch anchor row per orch group
	}{
		{
			// Two orch groups, one turn each → 2 anchor rows, 0 standalone seps.
			name: "two_orch_groups_one_sep",
			turns: []parser.Turn{
				orchTurn(2, "g2", 2, base.Add(10*time.Second), "Edit", ""),
				orchTurn(1, "g1", 1, base, "Read", ""),
			},
			wantAnchors: 2,
		},
		{
			// Sidechain between two orch groups: 2 anchor rows, 0 standalone seps.
			// Sidechain rows carry SkipSeparator=true — never become anchors.
			name: "sidechain_between_orch_no_extra_sep",
			turns: []parser.Turn{
				orchTurn(2, "g2", 2, base.Add(10*time.Second), "Edit", ""),
				sidechainTurn("sc1", base.Add(5*time.Second), "BashSub"),
				orchTurn(1, "g1", 1, base, "Read", ""),
			},
			wantAnchors: 2,
		},
		{
			// Single orch group, 1 turn: 1 anchor row (the single turn is the
			// anchor for G1), 0 standalone seps.
			name: "single_orch_group_no_sep",
			turns: []parser.Turn{
				orchTurn(1, "g1", 1, base, "Bash", ""),
			},
			wantAnchors: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Turns are already newest-first as perTurnTable sorts them.
			out := renderWithTurns(t, tc.turns, nil, base.Add(time.Minute), 5)

			// Verify zero standalone ├─┼─┄ separators between data rows.
			// groupSepLines now only finds pure horizontal lines (no spaces).
			// After the notch redesign the legend separator is the only such line,
			// so we only count lines that are NOT immediately before the legend row.
			lines := strings.Split(out, "\n")
			standaloneSeps := 0
			for i, l := range lines {
				bare := stripMkA(l)
				if !strings.HasPrefix(bare, "├") || !strings.Contains(bare, "┼") ||
					strings.Contains(bare, " ") {
					continue
				}
				// Pure horizontal line. Skip the legend separator (next content line
				// is the legend row).
				isLegendSep := false
				for j := i + 1; j < len(lines); j++ {
					nb := stripMkA(lines[j])
					if nb == "" {
						continue
					}
					if strings.Contains(nb, " role ") && strings.Contains(nb, " model ") {
						isLegendSep = true
					}
					break
				}
				if !isLegendSep {
					standaloneSeps++
				}
			}
			if standaloneSeps != 0 {
				t.Errorf("T-3 %s: got %d standalone inter-group separators (want 0);\n"+
					"  notch redesign: inter-group boundaries are notch anchor rows, not ├─┼─┄ lines.\n"+
					"  output:\n%s",
					tc.name, standaloneSeps, out)
			}

			// Verify expected notch anchor row count.
			notchRows := notchDataRows(out)
			if len(notchRows) != tc.wantAnchors {
				t.Errorf("T-3 %s: got %d notch anchor rows (want %d);\n"+
					"  each orch group must have exactly one anchor row with ├/┼/┤ dividers.\n"+
					"  notch rows:\n%s\n  output:\n%s",
					tc.name, len(notchRows), tc.wantAnchors,
					strings.Join(notchRows, "\n"), out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-4: TestAssemble_DimOlderGroups
//
// Spec §2.3 «Dim»: freshest orch group (maxGroupID) → no whole-row {{dim}} wrapper.
// All turns from older groups → wrapped in {{dim}}…{{reset}} (whole-row dim).
//
// Notch anchor rows (├/┼/┤) of fresh groups start with per-border dim markers
// like "{{dim}}├{{reset}}" but NOT with a whole-row dim that wraps the entire
// content. A whole-row dim row has pattern "{{dim}}<box-rune><content>{{reset}}"
// where the content is NOT interspersed with early {{reset}} markers — the row
// ends with {{reset}}.
//
// Detection heuristic:
//   - History row (whole-row dim): starts with "{{dim}}" AND ends with "{{reset}}"
//     (the outer closing reset). May have a TTL suffix after "{{reset}}", but the
//     core row content ends with "{{reset}}".
//   - Fresh row (per-border dim): starts with "{{dim}}" but does NOT end with
//     "{{reset}}" — the per-border "{{dim}}├{{reset}}" immediately releases dim.
//
// ---------------------------------------------------------------------------
func TestAssemble_DimOlderGroups(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// GroupID=2 is current (max); GroupID=1 is history.
	turnCur := orchTurn(2, "cur", 2, base.Add(10*time.Second), "ToolCurrent", "")
	turnHist := orchTurn(1, "hist", 1, base, "ToolHistory", "")

	// Newest-first.
	out := renderWithTurns(t, []parser.Turn{turnCur, turnHist}, nil, base.Add(2*time.Minute), 5)

	if !strings.Contains(out, "┌") {
		t.Fatalf("T-4: no table in output; output:\n%s", out)
	}

	histRowFound := false
	curRowFound := false

	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "ToolHistory") {
			histRowFound = true
			// History row (GroupID=1 < maxGroupID=2) must be whole-row dim.
			// A whole-row dim row starts with "{{dim}}" AND ends with "{{reset}}"
			// (the outer closing wrapper — possibly with trailing TTL suffix that
			// we strip to check the base row).
			baseRow := strings.TrimSpace(l)
			// Strip trailing TTL suffix (e.g. " ⏱ 0m") after the closing {{reset}}.
			closeIdx := strings.LastIndex(baseRow, "{{reset}}")
			if closeIdx < 0 || !strings.HasPrefix(baseRow, "{{dim}}") {
				t.Errorf("T-4: history row (GroupID=1) must be whole-row dim-wrapped\n"+
					"  (starts with '{{dim}}' and ends with '{{reset}}');\n"+
					"  line:\n%s", l)
			}
		}
		if strings.Contains(l, "ToolCurrent") {
			curRowFound = true
			// Current row (maxGroupID=2) uses per-border dim, NOT whole-row dim.
			// It must NOT be whole-row wrapped: it may start with "{{dim}}├{{reset}}"
			// (notch border) but must NOT follow the pattern {{dim}}<all content>{{reset}}.
			// Detection: if the line starts with "{{dim}}" AND the very last marker is
			// "{{reset}}" at the very end (ignoring TTL suffix), it is a whole-row wrap.
			// Fresh notch rows start with "{{dim}}├{{reset}}" — the ├{{reset}} releases
			// dim early, so the row has many individual resets, not one final outer reset.
			//
			// Simplified check: a fresh row must NOT start with "{{dim}}" followed by
			// a box-drawing char that is NOT immediately followed by "{{reset}}" at position 0.
			// Equivalently: if line starts with "{{dim}}" check that after stripping
			// the very first "{{dim}}├{{reset}}" or "{{dim}}│{{reset}}" prefix, the
			// remainder does NOT start with "{{reset}}" (which would close a whole-row wrap).
			//
			// In practice: whole-row dim always starts "{{dim}}├<content without {{reset}}>…{{reset}}"
			// (the ├ or │ is NOT wrapped in its own reset); per-border dim starts
			// "{{dim}}├{{reset}}" (the ├ is enclosed in its own dim block).
			// So: a whole-row dim row starts "{{dim}}├" where ├ is NOT followed by "{{reset}}".
			// A per-border fresh row starts "{{dim}}├{{reset}}" (├ immediately followed by {{reset}}).
			if strings.HasPrefix(l, "{{dim}}") {
				// Accept only if the first box-rune is immediately followed by {{reset}}.
				// i.e. starts with "{{dim}}├{{reset}}" or "{{dim}}│{{reset}}".
				isPerBorderDim := strings.HasPrefix(l, "{{dim}}├{{reset}}") ||
					strings.HasPrefix(l, "{{dim}}│{{reset}}")
				if !isPerBorderDim {
					t.Errorf("T-4: current row (GroupID=2=max) must NOT be whole-row dim;\n"+
						"  expected per-border dim ({{dim}}├{{reset}}…) not whole-row wrap.\n"+
						"  line:\n%s", l)
				}
			}
		}
	}

	if !histRowFound {
		t.Errorf("T-4: history row 'ToolHistory' not found in output\noutput:\n%s", out)
	}
	if !curRowFound {
		t.Errorf("T-4: current row 'ToolCurrent' not found in output\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-5: TestAssemble_LegendSeparator
//
// Spec §2.3: a ├─┼─┤ separator line must appear immediately before the legend row.
// Legend row contains column header labels ("role", "model", "cache r/w").
// ---------------------------------------------------------------------------
func TestAssemble_LegendSeparator(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turn := orchTurn(1, "t1", 1, base, "Bash", "")
	out := renderWithTurns(t, []parser.Turn{turn}, nil, base.Add(time.Minute), 5)

	if !strings.Contains(out, "┌") {
		t.Fatalf("T-5: no table; output:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	legendIdx := -1
	for i, l := range lines {
		bare := stripMkA(l)
		if strings.Contains(bare, "role") && strings.Contains(bare, "model") {
			legendIdx = i
			break
		}
	}

	if legendIdx < 0 {
		t.Fatalf("T-5: legend row not found\noutput:\n%s", out)
	}
	if legendIdx == 0 {
		t.Fatalf("T-5: legend row is first line, no room for separator before it")
	}

	// Line immediately before the legend must be a ├─┼─┤ separator.
	prevBare := stripMkA(lines[legendIdx-1])
	if !strings.HasPrefix(prevBare, "├") || !strings.Contains(prevBare, "┼") {
		t.Errorf("T-5: line before legend must be ├─┼─┤ separator; got: %s", lines[legendIdx-1])
	}
}

// ---------------------------------------------------------------------------
// T-6: TestAssemble_DataBarsDim
//
// Spec §2.3: all vertical bars of data rows use a dim-bar pattern.
// (Borders are dim, not plain │.)
// This applies to ALL data rows — both fresh-group and history.
//
// Notch redesign: anchor rows use dim notch glyphs ({{dim}}├{{reset}},
// {{dim}}┼{{reset}}, {{dim}}┤{{reset}}) instead of {{dim}}│{{reset}}.
// Both patterns satisfy "dim borders".
//
// Rules after notch redesign:
//
//	(a) History rows: whole-row wrapped in {{dim}}…{{reset}} — dividers inside
//	    the wrapper are plain box runes (outer dim applies).
//	(b) Fresh non-anchor rows: must contain "{{dim}}│{{reset}}" for cell dividers.
//	(c) Fresh anchor rows: must contain "{{dim}}├{{reset}}" (leading notch dim bar).
//
// ---------------------------------------------------------------------------
func TestAssemble_DataBarsDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	turn := orchTurn(1, "t1", 1, base, "Bash", "")
	out := renderWithTurns(t, []parser.Turn{turn}, nil, base.Add(time.Minute), 5)

	rows := dataRows(out)
	if len(rows) == 0 {
		t.Fatalf("T-6: no data rows found\noutput:\n%s", out)
	}

	for _, row := range rows {
		bare := stripMkA(row)
		_ = bare

		// History rows: whole-row dim wrapper (starts "{{dim}}" and ends "{{reset}}").
		// Their internal dividers are plain box runes inside the outer dim — OK.
		isHistoryRow := strings.HasPrefix(row, "{{dim}}") &&
			!strings.HasPrefix(row, "{{dim}}├{{reset}}") &&
			!strings.HasPrefix(row, "{{dim}}│{{reset}}")
		if isHistoryRow {
			continue
		}

		// Fresh anchor rows (notch): must use dim notch glyphs ({{dim}}├{{reset}}).
		isAnchorRow := strings.HasPrefix(row, "{{dim}}├{{reset}}")
		if isAnchorRow {
			// Anchor rows must contain dim notch inner junction ({{dim}}┼{{reset}}).
			if !strings.Contains(row, "{{dim}}┼{{reset}}") {
				t.Errorf("T-6: anchor (notch) data row must contain '{{dim}}┼{{reset}}' inner junction;\n"+
					"  Fix: notch inner dividers must use dim pattern.\n  row: %s", row)
			}
			continue
		}

		// Fresh regular rows (non-anchor, start with {{dim}}│{{reset}}):
		// must contain "{{dim}}│{{reset}}" for all cell dividers.
		if strings.Contains(row, "│") && !strings.Contains(row, "{{dim}}│{{reset}}") {
			t.Errorf("T-6: fresh data row must use '{{dim}}│{{reset}}' dim bars (or be history-wrapped or anchor);\n"+
				"  Found plain '│' not enclosed in dim markers.\n  row: %s", row)
		}
	}
}

// ---------------------------------------------------------------------------
// T-10: TestAssemble_SubagentOneRowLastTurn
//
// Spec §2.3 (subagent row): one row per subagent; cache/out = last turn;
// cost = "Σ $" cumulative.
// ---------------------------------------------------------------------------
func TestAssemble_SubagentOneRowLastTurn(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchT := orchTurn(1, "orch1", 1, base, "Bash", "")

	// Subagent has 2 turns; last turn has distinctive token values.
	turn1 := parser.Turn{
		Index:       1,
		Role:        "agent",
		UUID:        "sub-t1",
		GroupID:     0,
		Timestamp:   base.Add(time.Second),
		Tokens:      parser.TokenCounts{Output: 111, CacheCreate: 222},
		IsSidechain: true,
	}
	turn2 := parser.Turn{
		Index:       2,
		Role:        "agent",
		UUID:        "sub-t2",
		GroupID:     0,
		Timestamp:   base.Add(2 * time.Second),
		Tokens:      parser.TokenCounts{Output: 333, CacheCreate: 444},
		ToolUse:     "WriteTool",
		IsSidechain: true,
	}

	subStats := parser.SubagentStats{
		AgentID:         "agent-xyz",
		AgentType:       "code-writer",
		Model:           "claude-sonnet-4-6",
		CurrentTurnNum:  2,
		ActivationStart: base.Add(time.Second),
		LastTimestamp:   base.Add(2 * time.Second),
		TurnCount:       2,
		Turns:           []parser.Turn{turn1, turn2},
		Tokens:          parser.TokenCounts{Output: 444, CacheCreate: 666},
	}

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 1,
			Turns:     []parser.Turn{orchT},
		},
		Subagents:    []parser.SubagentStats{subStats},
		Now:          base.Add(3 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	// Exactly ONE row for the subagent (↳ appears once).
	arrowCount := strings.Count(out, "↳")
	if arrowCount != 1 {
		t.Errorf("T-10: expected 1 subagent row (1 '↳'), got %d\noutput:\n%s", arrowCount, out)
	}

	// Subagent row must show data from last turn.
	// Last turn: out=333 tokens → "333" or format.FormatK(333) in the row.
	// format.FormatK(333) = "333" (< 1000).
	if !strings.Contains(out, "WriteTool") {
		t.Errorf("T-10: subagent row must show last turn's tool 'WriteTool'; not found\noutput:\n%s", out)
	}

	// Cost must be "Σ $" prefixed (cumulative, not per-turn).
	if !strings.Contains(out, "Σ $") {
		t.Errorf("T-10: subagent row cost must be 'Σ $' (cumulative format); not found\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-11: TestAssemble_SubagentSortByActivation
//
// Spec §2.3: subagent row position determined by ActivationStart, not LastTimestamp.
// A new turn (later LastTimestamp) must NOT move the subagent row upward if its
// ActivationStart is earlier than another subagent's ActivationStart.
// ---------------------------------------------------------------------------
func TestAssemble_SubagentSortByActivation(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchT := orchTurn(1, "orch1", 1, base, "Bash", "")

	// SubA was activated first (ActivationStart earlier), but has a recent LastTimestamp.
	subA := parser.SubagentStats{
		AgentID:         "agent-A",
		AgentType:       "writer",
		ActivationStart: base.Add(1 * time.Second),  // EARLIER activation
		LastTimestamp:   base.Add(10 * time.Second), // but latest last-turn
		CurrentTurnNum:  2,
		TurnCount:       2,
		Turns: []parser.Turn{
			sidechainTurn("subA-t1", base.Add(1*time.Second), "BashA"),
			sidechainTurn("subA-t2", base.Add(10*time.Second), "ReadA"),
		},
	}

	// SubB was activated later (ActivationStart later).
	subB := parser.SubagentStats{
		AgentID:         "agent-B",
		AgentType:       "reviewer",
		ActivationStart: base.Add(5 * time.Second), // LATER activation
		LastTimestamp:   base.Add(6 * time.Second), // earlier last-turn than subA
		CurrentTurnNum:  1,
		TurnCount:       1,
		Turns: []parser.Turn{
			sidechainTurn("subB-t1", base.Add(5*time.Second), "BashB"),
		},
	}

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 1,
			Turns:     []parser.Turn{orchT},
		},
		Subagents:    []parser.SubagentStats{subA, subB},
		Now:          base.Add(3 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	// Find positions of subA ("ReadA" = last tool) and subB ("BashB" = last tool).
	posA := strings.Index(out, "ReadA")
	posB := strings.Index(out, "BashB")

	if posA < 0 || posB < 0 {
		t.Fatalf("T-11: markers not found (posA=%d posB=%d)\noutput:\n%s", posA, posB, out)
	}

	// SubB has later ActivationStart → appears first (newest activation first).
	// Rows are sorted newest-activation-first (like orch turns are newest-timestamp-first).
	// SubB (ActivationStart=+5s) before SubA (ActivationStart=+1s).
	if posB >= posA {
		t.Errorf("T-11: SubB (ActivationStart=+5s) must appear before SubA (ActivationStart=+1s); posB=%d posA=%d\noutput:\n%s",
			posB, posA, out)
	}
}

// ---------------------------------------------------------------------------
// T-12: TestAssemble_TTLRightOfRow
//
// Spec §2.3 (TTL right of row): TTL appears as suffix after the right border of:
//   - freshest orch turn row
//   - each subagent row
//
// TTL format: "⏱ Nm" (or coloured variant with markers when near expiry).
// TTL suffix appears OUTSIDE the table cell (after "│" right border).
// ---------------------------------------------------------------------------
func TestAssemble_TTLRightOfRow(t *testing.T) {
	// OrchTTLMinutes=5, elapsed=2min → remaining=3 → "⏱ 3m" suffix on fresh orch row.
	// Remaining=3 > 10m? No, 3 ≤ 10 → {{color:red}}⏱ 3m{{reset}} (ansiEnabled=false → plain).
	// With AnsiEnabled=false: marker tokens present but not ANSI codes.
	// We check for "⏱" glyph presence on the fresh orch row.

	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	orchT := orchTurn(1, "orch1", 1, base, "Bash", "")
	// now = base+2min → elapsed=2min, remaining=5-2=3m → "⏱ 3m"
	now := base.Add(2 * time.Minute)

	subTurnT := parser.Turn{
		Role:        "agent",
		UUID:        "sub-t1",
		GroupID:     0,
		Timestamp:   base.Add(30 * time.Second),
		Tokens:      parser.TokenCounts{Output: 50},
		ToolUse:     "SubTool",
		IsSidechain: true,
	}
	subStats := parser.SubagentStats{
		AgentID:         "agent-S",
		AgentType:       "agent",
		ActivationStart: base.Add(30 * time.Second),
		LastTimestamp:   base.Add(30 * time.Second),
		CurrentTurnNum:  1,
		TurnCount:       1,
		Turns:           []parser.Turn{subTurnT},
	}

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount:     1,
			Turns:         []parser.Turn{orchT},
			LastTimestamp: base,
		},
		Subagents:    []parser.SubagentStats{subStats},
		Now:          now,
		TerminalCols: 80,
	}
	out := a.Render(d)

	// TTL suffix "⏱" must appear at least once (on the fresh orch row) and once
	// more for the subagent row — total ≥ 2 occurrences.
	// If TTL feature not yet implemented → 0 occurrences → RED.
	ttlCount := strings.Count(out, "⏱")
	if ttlCount < 2 {
		t.Errorf("T-12: expected at least 2 '⏱' TTL suffixes (orch + subagent), got %d\noutput:\n%s",
			ttlCount, out)
	}

	// TTL must appear AFTER the right border (│) on at least one row.
	// We check that some line has "│" followed by "⏱" in the remainder.
	ttlAfterBorder := false
	for _, l := range strings.Split(out, "\n") {
		bare := stripMkA(l)
		idx := strings.LastIndex(bare, "│")
		if idx >= 0 && strings.Contains(bare[idx:], "⏱") {
			ttlAfterBorder = true
			break
		}
	}
	if !ttlAfterBorder {
		t.Errorf("T-12: TTL suffix '⏱' must appear after last '│' (outside table cell)\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-13: TestAssemble_NoLine2
//
// Spec §2.3: the cache-aggregate row (line2) is removed in Phase 6.9.e.
// The output must NOT contain "cache" as a line2-type aggregate string.
// We verify by checking there is no third non-table line with "cache" content
// between line1 and the table top border.
// ---------------------------------------------------------------------------
func TestAssemble_NoLine2(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Use a real CacheProbe via Line2Registry swap to test that it is NOT rendered.
	// We keep Line2Registry with a visible cache probe, then assert its output
	// does not appear in the assembler output.
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "CACHE_LINE2_MARKER"}})

	a := &statusline.Assembler{
		Mode:   mode.Standard,
		Theme:  renderer.Theme{AnsiEnabled: false},
		Cols:   80,
		Config: probes.Config{OrchTTLMinutes: 5},
	}

	turn := orchTurn(1, "t1", 1, base, "Bash", "")
	d := probes.Data{
		Session:      &parser.SessionStats{TurnCount: 1, Turns: []parser.Turn{turn}},
		Now:          base.Add(time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	// Line2Registry is registered but 6.9.e must NOT render it.
	if strings.Contains(out, "CACHE_LINE2_MARKER") {
		t.Errorf("T-13: line2 (cache-aggregate row) must be absent from output; found 'CACHE_LINE2_MARKER'\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// T-30: TestAssemble_SubagentInstanceName
//
// Spec §2.3: at cols>100, tool cell of subagent row = "<name≤16>: <last-tool>",
// where name is joined from stdin Tasks[].Name by task.ID==AgentID.
// At cols≤100, tool cell = only last-tool (no name prefix).
// ---------------------------------------------------------------------------
func TestAssemble_SubagentInstanceName(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchT := orchTurn(1, "orch1", 1, base, "BashOrch", "")

	subTurnT := parser.Turn{
		Role:        "agent",
		UUID:        "sub-t1",
		GroupID:     0,
		Timestamp:   base.Add(time.Second),
		Tokens:      parser.TokenCounts{Output: 50},
		ToolUse:     "ReadSubFile",
		IsSidechain: true,
	}
	subStats := parser.SubagentStats{
		AgentID:         "task-abc-123",
		AgentType:       "general-purpose",
		Description:     "my-red-agent",
		ActivationStart: base.Add(time.Second),
		LastTimestamp:   base.Add(time.Second),
		CurrentTurnNum:  1,
		TurnCount:       1,
		Turns:           []parser.Turn{subTurnT},
		LastTool:        "ReadSubFile",
	}

	t.Run("cols_gt_100_has_name_prefix", func(t *testing.T) {
		// meta.json description "my-red-agent" is the instance name (F7: CC sends
		// no tasks[] to the status line). At cols>100, assembler must show
		// "<name≤16>: <last-tool>" in tool cell.
		a := makeStdAssembler(5, 120) // cols > 100
		d := probes.Data{
			Session: &parser.SessionStats{
				TurnCount: 1,
				Turns:     []parser.Turn{orchT},
			},
			Subagents:    []parser.SubagentStats{subStats},
			Now:          base.Add(2 * time.Minute),
			TerminalCols: 120,
		}
		out := a.Render(d)
		// Tool cell must contain "my-red-agent: ReadSubFile" (name+colon+tool).
		if !strings.Contains(out, "my-red-agent: ReadSubFile") {
			t.Errorf("T-30 cols>100: tool cell must be '<name>: <tool>'; not found\noutput:\n%s", out)
		}
	})

	t.Run("cols_le_100_no_name_prefix", func(t *testing.T) {
		a := makeStdAssembler(5, 80) // cols ≤ 100
		d := probes.Data{
			Session: &parser.SessionStats{
				TurnCount: 1,
				Turns:     []parser.Turn{orchT},
			},
			Subagents:    []parser.SubagentStats{subStats},
			Now:          base.Add(2 * time.Minute),
			TerminalCols: 80,
		}
		out := a.Render(d)
		// Must NOT contain "my-red-agent:" prefix in tool cell (cols ≤ 100).
		if strings.Contains(out, "my-red-agent:") {
			t.Errorf("T-30 cols≤100: must NOT have name prefix; got:\n%s", out)
		}
		// Must still contain the tool name somewhere.
		if !strings.Contains(out, "ReadSubFile") {
			t.Errorf("T-30 cols≤100: tool name 'ReadSubFile' must be present; got:\n%s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// T-33: TestAssemble_TTLFreezeOnExpiredOrch
//
// Spec §2.3 «TTL freeze»: for a pair of neighbouring orch turns (prev=older,
// cur=newer), if floor((cur.Timestamp − prev.Timestamp).Minutes()) ≥ OrchTTLMinutes,
// the prev row renders "⏱ 0m" frozen (bold_red marker, ansiEnabled=false → plain).
// If the gap < OrchTTLMinutes → prev row has NO TTL suffix.
// Sidechain turns do not participate.
//
// Manual calculation (OrchTTLMinutes=5):
//
//	Scenario A (freeze):    gap = 5m → remaining = 5 - 5 = 0 ≤ 0 → "⏱ 0m" on prev.
//	Scenario B (no freeze): gap = 3m → remaining = 5 - 3 = 2 > 0 → no TTL on prev.
//	Sidechain between:      should not trigger freeze between orchs.
//
// ---------------------------------------------------------------------------
func TestAssemble_TTLFreezeOnExpiredOrch(t *testing.T) {
	const orchTTL = 5 // minutes
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		prevTS        time.Time
		curTS         time.Time
		sidechain     bool // insert a sidechain turn between prev and cur
		wantFreeze    bool // expect "⏱ 0m" on prev row
		wantNonFreeze bool // expect no "⏱" on prev row (when !wantFreeze)
	}{
		{
			// gap=5m, orchTTL=5 → remaining=5-5=0 ≤ 0 → FREEZE prev.
			name:       "gap_eq_ttl_freeze",
			prevTS:     base,
			curTS:      base.Add(5 * time.Minute),
			wantFreeze: true,
		},
		{
			// gap=6m > orchTTL=5 → remaining=5-6=-1 ≤ 0 → FREEZE prev.
			name:       "gap_gt_ttl_freeze",
			prevTS:     base,
			curTS:      base.Add(6 * time.Minute),
			wantFreeze: true,
		},
		{
			// gap=3m < orchTTL=5 → remaining=5-3=2 > 0 → NO freeze on prev.
			name:          "gap_lt_ttl_no_freeze",
			prevTS:        base,
			curTS:         base.Add(3 * time.Minute),
			wantNonFreeze: true,
		},
		{
			// Sidechain turn between prev and cur: gap between orchs still =5m → freeze.
			name:       "sidechain_between_still_freezes",
			prevTS:     base,
			curTS:      base.Add(5 * time.Minute),
			sidechain:  true,
			wantFreeze: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build turns newest-first: cur (newer) then prev (older).
			turns := []parser.Turn{
				orchTurn(2, "cur", 2, tc.curTS, "ToolCur", ""),
				orchTurn(1, "prev", 1, tc.prevTS, "ToolPrev", ""),
			}
			if tc.sidechain {
				// Insert sidechain between cur and prev by timestamp.
				scTS := tc.prevTS.Add(tc.curTS.Sub(tc.prevTS) / 2)
				turns = []parser.Turn{
					orchTurn(2, "cur", 2, tc.curTS, "ToolCur", ""),
					sidechainTurn("sc1", scTS, "ScTool"),
					orchTurn(1, "prev", 1, tc.prevTS, "ToolPrev", ""),
				}
			}

			// now = curTS + 1s (so fresh row gets a live TTL, not expired).
			now := tc.curTS.Add(time.Second)
			out := renderWithTurns(t, turns, nil, now, orchTTL)

			// Identify the prev row (contains "ToolPrev").
			prevRowLine := ""
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, "ToolPrev") {
					prevRowLine = l
					break
				}
			}
			if prevRowLine == "" {
				t.Fatalf("T-33 %s: 'ToolPrev' row not found\noutput:\n%s", tc.name, out)
			}

			if tc.wantFreeze {
				// Prev row must contain "⏱ 0m" (frozen TTL).
				if !strings.Contains(prevRowLine, "⏱ 0m") {
					t.Errorf("T-33 %s: prev row must contain '⏱ 0m' (frozen); gap=%v orchTTL=%d\nrow: %s\nfull output:\n%s",
						tc.name, tc.curTS.Sub(tc.prevTS), orchTTL, prevRowLine, out)
				}
			}
			if tc.wantNonFreeze {
				// Prev row must NOT contain any "⏱" (no TTL at all).
				if strings.Contains(prevRowLine, "⏱") {
					t.Errorf("T-33 %s: prev row must have NO '⏱' TTL (gap<orchTTL); gap=%v orchTTL=%d\nrow: %s\nfull output:\n%s",
						tc.name, tc.curTS.Sub(tc.prevTS), orchTTL, prevRowLine, out)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-34: TestAssemble_RedCacheWriteAfterExpiry
//
// Spec §2.3 (cache-write red, orchestrator): when the gap from the previous
// orch turn ≥ OrchTTLMinutes, the current orch turn's cache_create cell renders
// in {{color:red}} (NOT bold_red). Read-part is unchanged.
//
// Manual calculation (OrchTTLMinutes=5):
//
//	gap=5m → remaining=5-5=0 ≤ 0 → prev cache is expired → cur cache_create RED.
//	gap=3m → remaining=5-3=2 > 0 → prev cache alive → cur cache_create NOT red.
//	Only orchestrator turns (IsSidechain=false).
//
// ---------------------------------------------------------------------------
func TestAssemble_RedCacheWriteAfterExpiry(t *testing.T) {
	const orchTTL = 5
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		gapMin  int
		wantRed bool
	}{
		// gap=5m ≥ orchTTL=5 → cache_create of cur → RED.
		{name: "gap_5m_eq_ttl_red", gapMin: 5, wantRed: true},
		// gap=6m > orchTTL=5 → RED.
		{name: "gap_6m_gt_ttl_red", gapMin: 6, wantRed: true},
		// gap=3m < orchTTL=5 → NOT red.
		{name: "gap_3m_lt_ttl_no_red", gapMin: 3, wantRed: false},
		// Single orch turn (no previous) → NOT red.
		{name: "single_turn_no_red", gapMin: -1, wantRed: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var turns []parser.Turn
			if tc.gapMin < 0 {
				// Single turn only (no prev).
				turns = []parser.Turn{orchTurn(1, "cur", 1, base, "ToolX", "")}
			} else {
				prevTS := base
				curTS := base.Add(time.Duration(tc.gapMin) * time.Minute)
				// Newest-first: cur then prev.
				turns = []parser.Turn{
					orchTurn(2, "cur", 2, curTS, "ToolCur", ""),
					orchTurn(1, "prev", 1, prevTS, "ToolPrev", ""),
				}
			}

			now := base.Add(time.Duration(max(tc.gapMin, 0))*time.Minute + time.Second)
			out := renderWithTurns(t, turns, nil, now, orchTTL)

			// Identify the cur row (contains "ToolCur" or "ToolX" for single).
			curMarker := "ToolCur"
			if tc.gapMin < 0 {
				curMarker = "ToolX"
			}
			curRowLine := ""
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, curMarker) {
					curRowLine = l
					break
				}
			}
			if curRowLine == "" {
				t.Fatalf("T-34 %s: cur row '%s' not found\noutput:\n%s", tc.name, curMarker, out)
			}

			if tc.wantRed {
				// cur row must contain "{{color:red}}" (not bold_red) near cache_create value.
				if !strings.Contains(curRowLine, "{{color:red}}") {
					t.Errorf("T-34 %s: cur orch turn after expired prev must have '{{color:red}}' on cache_create; gap=%dm orchTTL=%d\nrow: %s\nfull:\n%s",
						tc.name, tc.gapMin, orchTTL, curRowLine, out)
				}
				// Must NOT use bold_red for cache_create (that's reserved for "⏱ 0m").
				if strings.Contains(curRowLine, "{{color:bold_red}}") {
					t.Errorf("T-34 %s: cache_create red must use '{{color:red}}', not '{{color:bold_red}}'; row: %s",
						tc.name, curRowLine)
				}
			} else {
				// cur row must NOT have {{color:red}} from cache-write logic.
				// (It may have red from TTL if near expiry, but we use gap<TTL so no TTL red either.)
				if strings.Contains(curRowLine, "{{color:red}}") {
					t.Errorf("T-34 %s: cur orch turn must NOT have '{{color:red}}' (gap<orchTTL); row: %s\nfull:\n%s",
						tc.name, curRowLine, out)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-35: TestAssemble_RedCacheWriteOnModelSwitch
//
// Spec §2.3 (cache-write red, orchestrator), condition (b): orch turn with
// model ≠ previous orch turn model → cache_create RED (OR with T-34).
// Same model + gap<orchTTL → no red.
// A model switch with gap<orchTTL (only model trigger) → still RED.
//
// Manual scenario:
//
//	Scenario A: modelA→modelB, gap=2m < orchTTL=5 → only model switch → RED.
//	Scenario B: modelA→modelA, gap=2m < orchTTL=5 → no switch, no gap → NOT red.
//	Scenario C: modelA→modelB, gap=5m ≥ orchTTL=5 → both triggers → RED (OR).
//
// ---------------------------------------------------------------------------
func TestAssemble_RedCacheWriteOnModelSwitch(t *testing.T) {
	const orchTTL = 5
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		prevModel string
		curModel  string
		gapMin    int
		wantRed   bool
	}{
		{
			// Model switch only (gap < orchTTL) → RED.
			// gap=2m < 5m, models differ.
			name:      "model_switch_gap_lt_ttl_red",
			prevModel: "claude-sonnet-4-6",
			curModel:  "claude-opus-4-5",
			gapMin:    2,
			wantRed:   true,
		},
		{
			// Same model, gap < orchTTL → NOT red.
			name:      "same_model_gap_lt_ttl_no_red",
			prevModel: "claude-sonnet-4-6",
			curModel:  "claude-sonnet-4-6",
			gapMin:    2,
			wantRed:   false,
		},
		{
			// Model switch + gap ≥ orchTTL → RED (OR of both conditions).
			name:      "model_switch_gap_ge_ttl_red",
			prevModel: "claude-sonnet-4-6",
			curModel:  "claude-opus-4-5",
			gapMin:    5,
			wantRed:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prevTS := base
			curTS := base.Add(time.Duration(tc.gapMin) * time.Minute)
			// Newest-first: cur then prev.
			turns := []parser.Turn{
				orchTurn(2, "cur", 2, curTS, "ToolCur", tc.curModel),
				orchTurn(1, "prev", 1, prevTS, "ToolPrev", tc.prevModel),
			}

			now := curTS.Add(time.Second)
			out := renderWithTurns(t, turns, nil, now, orchTTL)

			// Find cur row ("ToolCur").
			curRowLine := ""
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, "ToolCur") {
					curRowLine = l
					break
				}
			}
			if curRowLine == "" {
				t.Fatalf("T-35 %s: 'ToolCur' row not found\noutput:\n%s", tc.name, out)
			}

			if tc.wantRed {
				if !strings.Contains(curRowLine, "{{color:red}}") {
					t.Errorf("T-35 %s: cur turn must have '{{color:red}}' on cache_create (model switch or gap); prevModel=%q curModel=%q gap=%dm\nrow: %s\nfull:\n%s",
						tc.name, tc.prevModel, tc.curModel, tc.gapMin, curRowLine, out)
				}
			} else {
				if strings.Contains(curRowLine, "{{color:red}}") {
					t.Errorf("T-35 %s: cur turn must NOT have '{{color:red}}' (same model, gap<orchTTL); row: %s\nfull:\n%s",
						tc.name, curRowLine, out)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-36: TestAssemble_SubagentRedCacheWriteOnCollapse
//
// Spec §2.3 (cache-write red, subagent, no freeze):
// If gap between last two subagent turns (Turns[len-1].Timestamp − Turns[len-2].Timestamp)
// ≥ OrchTTLMinutes → cache_create of subagent row in {{color:red}}.
// TTL of subagent row must NOT be "⏱ 0m" (no freeze, stays live).
// Model switch within subagent does NOT trigger (only orchestrator model switch).
// Single subagent turn (len<2) → no red.
//
// Manual calculation (OrchTTLMinutes=5):
//
//	Scenario A (collapse): gap between last 2 sub turns = 5m → RED cache_create.
//	Scenario B (no collapse): gap = 3m → NOT red.
//	Scenario C (single turn): len(Turns)=1 → NOT red.
//	Scenario D (model switch sub): same-gap<orchTTL but different models → NOT red.
//
// ---------------------------------------------------------------------------
func TestAssemble_SubagentRedCacheWriteOnCollapse(t *testing.T) {
	const orchTTL = 5
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	orchT := orchTurn(1, "orch1", 1, base, "BashOrch", "")

	tests := []struct {
		name       string
		turns      []parser.Turn // subagent Turns (subset, last 2 matter)
		wantRed    bool
		wantFreeze bool // if true, also check NO "⏱ 0m" (TTL not frozen)
	}{
		{
			// gap[len-1 - len-2] = 5m ≥ orchTTL → RED. No freeze.
			// turn1 at base+0s, turn2 at base+5m.
			// gap = 5m, remaining = orchTTL - 5 = 0 ≤ 0 → RED.
			name: "collapse_gap_eq_ttl_red",
			turns: []parser.Turn{
				sidechainTurn("s1", base, "ToolA"),
				sidechainTurn("s2", base.Add(5*time.Minute), "ToolB"),
			},
			wantRed:    true,
			wantFreeze: false, // TTL should NOT be "⏱ 0m"
		},
		{
			// gap = 3m < orchTTL → NOT red.
			name: "no_collapse_gap_lt_ttl",
			turns: []parser.Turn{
				sidechainTurn("s1", base, "ToolA"),
				sidechainTurn("s2", base.Add(3*time.Minute), "ToolB"),
			},
			wantRed: false,
		},
		{
			// Single subagent turn → NOT red.
			name: "single_turn_no_red",
			turns: []parser.Turn{
				sidechainTurn("s1", base, "ToolA"),
			},
			wantRed: false,
		},
		{
			// gap=2m < orchTTL, model switch within subagent → NOT red (model
			// switch only applies to orchestrator per spec §2.3).
			name: "model_switch_sub_no_red",
			turns: []parser.Turn{
				{
					Role:        "agent",
					UUID:        "s1",
					GroupID:     0,
					Timestamp:   base,
					Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
					Model:       "claude-sonnet-4-6",
					IsSidechain: true,
				},
				{
					Role:        "agent",
					UUID:        "s2",
					GroupID:     0,
					Timestamp:   base.Add(2 * time.Minute),
					Tokens:      parser.TokenCounts{Output: 75, CacheCreate: 750},
					Model:       "claude-opus-4-5", // different model
					ToolUse:     "ToolB",
					IsSidechain: true,
				},
			},
			wantRed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lastTurn := tc.turns[len(tc.turns)-1]
			subStats := parser.SubagentStats{
				AgentID:         "agent-sub",
				AgentType:       "general-purpose",
				ActivationStart: tc.turns[0].Timestamp,
				LastTimestamp:   lastTurn.Timestamp,
				CurrentTurnNum:  len(tc.turns),
				TurnCount:       len(tc.turns),
				Turns:           tc.turns,
			}

			// now = lastTurn.Timestamp + 1min (sub TTL = orchTTL - 1m = 4m, alive).
			now := lastTurn.Timestamp.Add(time.Minute)

			a := makeStdAssembler(orchTTL, 80)
			d := probes.Data{
				Session: &parser.SessionStats{
					TurnCount: 1,
					Turns:     []parser.Turn{orchT},
				},
				Subagents:    []parser.SubagentStats{subStats},
				Now:          now,
				TerminalCols: 80,
			}
			out := a.Render(d)

			// Find the subagent panel row (6.9.e: one row per subagent with "↳N" in # cell).
			// After 6.9.e implementation, subagent panel row will contain "↳" in # column.
			// Pre-6.9.e: subagent turns appear as individual interleaved rows with yellow role.
			// We detect the "freshest" subagent row (last subagent turn = the panel row after 6.9.e).
			// For now we look for the row that would be the panel row: the one with the last tool.
			lastTool := tc.turns[len(tc.turns)-1].ToolUse
			subRowLine := ""
			// First try "↳" marker (post-6.9.e), then fall back to last-tool in yellow row.
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, "↳") {
					subRowLine = l
					break
				}
			}
			if subRowLine == "" && lastTool != "" {
				// Pre-6.9.e fallback: find the row with the last-turn tool.
				for _, l := range strings.Split(out, "\n") {
					if strings.Contains(l, lastTool) && strings.Contains(l, "{{color:yellow}}") {
						subRowLine = l
						break
					}
				}
			}
			if subRowLine == "" {
				// Any yellow-role row (subagent) as last resort.
				lines := strings.Split(out, "\n")
				// We want the LAST yellow-role row (most recent subagent turn).
				for i := len(lines) - 1; i >= 0; i-- {
					if strings.Contains(lines[i], "{{color:yellow}}") {
						subRowLine = lines[i]
						break
					}
				}
			}
			if subRowLine == "" {
				t.Fatalf("T-36 %s: subagent row not found (↳ or yellow-role)\noutput:\n%s", tc.name, out)
			}

			if tc.wantRed {
				// Subagent row must contain "{{color:red}}" (cache_create red).
				if !strings.Contains(subRowLine, "{{color:red}}") {
					t.Errorf("T-36 %s: subagent row must have '{{color:red}}' on cache_create; gap≥orchTTL\nrow: %s\nfull:\n%s",
						tc.name, subRowLine, out)
				}
				// TTL of subagent row must NOT be "⏱ 0m" (no freeze for subagents).
				// "⏱ 0m" = bold_red freeze, only for orchestrator rows.
				if strings.Contains(subRowLine, "⏱ 0m") {
					t.Errorf("T-36 %s: subagent row must NOT have frozen '⏱ 0m' (no freeze for subagents); row: %s",
						tc.name, subRowLine)
				}
			} else {
				// No red cache_create.
				if strings.Contains(subRowLine, "{{color:red}}") {
					t.Errorf("T-36 %s: subagent row must NOT have '{{color:red}}' (gap<orchTTL); row: %s\nfull:\n%s",
						tc.name, subRowLine, out)
				}
			}
		})
	}
}
