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

	// Count lines that start with "│" (data rows) — excludes top/bottom borders
	// that start with "┌" / "└" and separators that start with "├".
	// Also excludes the legend footer row ("# role model cache out cost tool").
	dataRowCount := 0
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(stripMk(line))
		if strings.HasPrefix(trimmed, "│") &&
			!strings.HasPrefix(trimmed, "├") &&
			!strings.HasPrefix(trimmed, "┌") &&
			!strings.HasPrefix(trimmed, "└") {
			// C1: exclude the legend footer row (contains column header labels).
			// Old footer check: "Total for request" (removed by C1).
			// New legend row check: contains "role" as a cell label.
			bareStripped := stripMk(line)
			if !strings.Contains(bareStripped, "Total for request") &&
				!(strings.Contains(bareStripped, " role ") && strings.Contains(bareStripped, " model ")) {
				dataRowCount++
			}
		}
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
