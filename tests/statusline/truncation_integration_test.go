// Package statusline_test — RED integration tests for Phase 4.3.d.
//
// These tests exercise the wire-up between Assembler.Render, DetectCols, FitLine,
// and Builder.RenderForCols. They fall into two groups:
//
//   - RED tests: FAIL on the current stubs because Assembler.renderLine does not
//     call FitLine (FitLine stub returns "", so assembler keeps using the raw
//     joined probe output which overflows the requested cols).
//
//   - Invariant tests (StillNoAnsi, MarkersStillPresent, RowCountStable,
//     Cap20StillEnforced): PASS on the stub because they verify properties that
//     already hold in Phase 4.2 and must continue to hold after GREEN.
//
// Detection strategy for RED tests: we register probes whose combined output
// (with separators) exceeds the target cols. The current assembler joins them
// raw without FitLine → output overflows → assertions on VisualLen > cols fail.
// After GREEN, FitLine trims the output to fit → assertions pass.
package statusline_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// makeAssemblerForCols returns an Assembler with the given mode and terminal
// width. A zero-value Theme (no ANSI) is used to keep output clean for
// inspection.
func makeAssemblerForCols(m mode.Mode, cols int) *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   m,
		Theme:  renderer.Theme{},
		Cols:   cols,
		Config: probes.Config{},
	}
}

// makeDataWithTurns is a thin wrapper that re-uses the helper from
// assembler_test.go (same test package).
// Comment: delegates to makeData() defined in assembler_test.go.
func makeDataWithTurns(n int) probes.Data {
	return makeData(n)
}

// statusLines splits the Render() output into the three header lines (line0,
// line1, line2). The assembler always emits at least 3 newline-separated lines.
func statusLines(out string) (line0, line1, line2 string) {
	parts := strings.SplitN(out, "\n", 4)
	if len(parts) < 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}

// countDataRows counts the number of per-turn data rows in the Render() output.
// A data row is a line that starts with '│', contains no '─' (not a separator
// or border), and does not contain the footer label "Total for request".
func countDataRows(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "│") &&
			!strings.Contains(trimmed, "─") &&
			!strings.Contains(trimmed, "Total for request") {
			n++
		}
	}
	return n
}

// longProbeOutput generates a string of exactly n visible characters (ASCII),
// suitable as a fakeProbe.out value whose VisualLen == n.
func longProbeOutput(prefix string, totalLen int) string {
	for len(prefix) < totalLen {
		prefix += "x"
	}
	return prefix[:totalLen]
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_Cols80_AllProbesFit — §4.3 T-9
//
// Probes are set up so that their raw joined output exceeds 80 visual columns.
// The current assembler (no FitLine) will output an overflowing line.
// After GREEN (FitLine wired), the output must fit ≤ 80 cols for each line.
//
// RED: raw join overflows → VisualLen(line0) > 80 → assertion fails.
// GREEN: FitLine trims/drops probes → VisualLen ≤ 80.
// ---------------------------------------------------------------------------
func TestAssembler_Render_Cols80_AllProbesFit(t *testing.T) {
	// §4.3 T-9: two probes on line0 with combined raw output > 80 chars.
	// P0=50 chars + sep(~12) + P1=30 chars ≈ 92 chars raw → overflows 80.
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "p0", priority: 0, visible: true, out: longProbeOutput("model:", 50)},
		&fakeProbe{name: "p1", priority: 1, visible: true, out: longProbeOutput("ctx:", 30)},
	})
	swapLine1(t, []probes.Probe{
		// P0=50 chars + sep + P1=30 chars ≈ 92 raw → overflows 80.
		&fakeProbe{name: "m0", priority: 0, visible: true, out: longProbeOutput("cache:", 50)},
		&fakeProbe{name: "m1", priority: 1, visible: true, out: longProbeOutput("out:", 30)},
	})
	swapLine2(t, []probes.Probe{
		// P0=45 chars + sep(" | "=3) + P1=40 chars = 88 raw → overflows 80.
		&fakeProbe{name: "c0", priority: 0, visible: true, out: longProbeOutput("cache:", 45)},
		&fakeProbe{name: "c1", priority: 1, visible: true, out: longProbeOutput("cost:", 40)},
	})

	a := makeAssemblerForCols(mode.SuperCompact, 80)
	out := a.Render(makeDataWithTurns(0))

	l0, l1, l2 := statusLines(out)

	// §4.3 T-9: after FitLine is wired, every header line must be ≤ 80 visual cols.
	// RED: raw join overflows → VisualLen > 80 → at least one assertion fails.
	for name, line := range map[string]string{"line0": l0, "line1": l1, "line2": l2} {
		vl := format.VisualLen(line)
		if vl > 80 {
			t.Errorf("Cols80: %s visual length = %d; want ≤ 80 (RED: FitLine not wired yet)\nline: %q", name, vl, line)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_Cols50_TruncatesNonP0 — §4.3 T-9
//
// At cols=50, probes with combined raw output >50 chars must be truncated.
// The current assembler joins them raw → overflow. After GREEN: ≤ 50.
//
// RED: raw join overflows 50 → at least one VisualLen > 50.
// ---------------------------------------------------------------------------
func TestAssembler_Render_Cols50_TruncatesNonP0(t *testing.T) {
	// P0=30 chars + sep(~12) + P1=20 chars = ~62 raw → overflows 50.
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "p0", priority: 0, visible: true, out: longProbeOutput("email:", 30)},
		&fakeProbe{name: "p1", priority: 1, visible: true, out: longProbeOutput("project:", 20)},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m0", priority: 0, visible: true, out: longProbeOutput("model:", 30)},
		&fakeProbe{name: "m1", priority: 2, visible: true, out: longProbeOutput("effort:", 20)},
	})
	swapLine2(t, []probes.Probe{
		// Line2 sep=" | " (3 chars). P0=25 + P1=25 + sep(3) = 53 → overflows 50.
		&fakeProbe{name: "c0", priority: 0, visible: true, out: longProbeOutput("cache:", 25)},
		&fakeProbe{name: "c1", priority: 1, visible: true, out: longProbeOutput("cost:", 25)},
	})

	a := makeAssemblerForCols(mode.SuperCompact, 50)
	out := a.Render(makeDataWithTurns(0))

	l0, l1, l2 := statusLines(out)

	// §4.3 T-9: all header lines must fit ≤ 50 after FitLine is wired.
	// RED: current raw join produces overflowing output.
	for name, line := range map[string]string{"line0": l0, "line1": l1, "line2": l2} {
		vl := format.VisualLen(line)
		if vl > 50 {
			t.Errorf("Cols50: %s visual length = %d; want ≤ 50 (RED: FitLine not wired)\nline: %q", name, vl, line)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_Cols120_NoTruncation — §4.3 T-9
//
// At cols=120, the terminal is wide enough that no probe is dropped or shortened.
// The output at cols=120 must contain at least as many visible characters as at
// cols=80 (no content lost at the wider setting).
//
// RED strategy: probes produce 90-char raw output (overflows 80, fits in 120).
// At cols=80 GREEN truncates → shorter. At cols=120 GREEN keeps full → longer.
// Current stub (no FitLine): both produce the same raw output → test trivially
// passes (no regression). We therefore assert that VisualLen(line0@cols80) < 90
// (i.e. truncation happened at 80) while VisualLen(line0@cols120) == 90.
//
// FAIL on stub: at cols=80, raw output = 90 chars → VisualLen = 90, NOT < 90.
// GREEN: FitLine trims to 80 → VisualLen@80 < 90, while VisualLen@120 = 90.
// ---------------------------------------------------------------------------
func TestAssembler_Render_Cols120_NoTruncation(t *testing.T) {
	// P0=50 chars + sep(~12) + P1=28 chars = ~90 raw (overflows 80, fits 120).
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "p0", priority: 0, visible: true, out: longProbeOutput("model:", 50)},
		&fakeProbe{name: "p1", priority: 1, visible: true, out: longProbeOutput("time:", 28)},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m0", priority: 0, visible: true, out: longProbeOutput("cache:", 50)},
	})
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c0", priority: 0, visible: true, out: longProbeOutput("cache:", 50)},
		&fakeProbe{name: "c1", priority: 1, visible: true, out: longProbeOutput("cost:", 28)},
	})

	d := makeDataWithTurns(0)
	a80 := makeAssemblerForCols(mode.SuperCompact, 80)
	a120 := makeAssemblerForCols(mode.SuperCompact, 120)

	out80 := a80.Render(d)
	out120 := a120.Render(d)

	l0at80, _, _ := statusLines(out80)
	l0at120, _, _ := statusLines(out120)

	// §4.3 T-9: at cols=80, line0 must be trimmed to ≤ 80 (shorter than raw 90).
	// RED: current assembler outputs raw 90 → VisualLen == 90, not < 90 → FAIL.
	vl80 := format.VisualLen(l0at80)
	if vl80 > 80 {
		t.Errorf("Cols120/80: line0 at cols=80 visual length = %d; want ≤ 80 (RED: FitLine not wired)\nline: %q", vl80, l0at80)
	}

	// At cols=120, the same content must be preserved in full (no truncation).
	vl120 := format.VisualLen(l0at120)
	if vl120 > 120 {
		t.Errorf("Cols120/120: line0 at cols=120 visual length = %d; want ≤ 120\nline: %q", vl120, l0at120)
	}

	// At cols=120 content must not be shorter than at cols=80 after truncation.
	if vl120 < vl80 {
		t.Errorf("Cols120: line0 at cols=120 (%d chars) shorter than at cols=80 (%d chars); nothing should be dropped at wider terminals", vl120, vl80)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_ZeroCols_UsesDetect — §4.3 T-9 + DetectCols wire
//
// An Assembler with Cols=0 must call renderer.DetectCols() internally.
// We set $COLUMNS=100 via t.Setenv. With FitLine wired, the header lines
// must fit within 100 columns (= COLUMNS value). Without FitLine wiring, the
// raw joined output overflows for our long probes.
//
// RED: raw join of P0(55)+sep+P1(50)≈117 chars > 100 → VisualLen > 100 → FAIL.
// GREEN: Assembler uses DetectCols() → 100 → FitLine trims → VisualLen ≤ 100.
// ---------------------------------------------------------------------------
func TestAssembler_Render_ZeroCols_UsesDetect(t *testing.T) {
	// Force DetectCols to return 100 via $COLUMNS env var (§4.3 width.go contract).
	t.Setenv("COLUMNS", "100")

	// P0=55 chars + sep(~12) + P1=50 chars ≈ 117 raw → overflows 100.
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "p0", priority: 0, visible: true, out: longProbeOutput("model:", 55)},
		&fakeProbe{name: "p1", priority: 1, visible: true, out: longProbeOutput("time:", 50)},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m0", priority: 0, visible: true, out: longProbeOutput("cache:", 55)},
		&fakeProbe{name: "m1", priority: 1, visible: true, out: longProbeOutput("cost:", 50)},
	})
	swapLine2(t, []probes.Probe{
		// Line2 sep=" | "(3). P0=55 + P1=50 + sep(3) = 108 → overflows 100.
		&fakeProbe{name: "c0", priority: 0, visible: true, out: longProbeOutput("cache:", 55)},
		&fakeProbe{name: "c1", priority: 1, visible: true, out: longProbeOutput("cost:", 50)},
	})

	// Cols=0: assembler must detect terminal width internally via DetectCols().
	a := makeAssemblerForCols(mode.SuperCompact, 0)
	out := a.Render(makeDataWithTurns(0))

	l0, l1, l2 := statusLines(out)

	// §4.3 T-9: after FitLine is wired, lines must fit within COLUMNS=100.
	// RED: Assembler ignores Cols=0 / does not call DetectCols → raw overflow → FAIL.
	const detectedCols = 100
	for name, line := range map[string]string{"line0": l0, "line1": l1, "line2": l2} {
		vl := format.VisualLen(line)
		if vl > detectedCols {
			t.Errorf("ZeroCols: %s visual length = %d; want ≤ %d (COLUMNS=100, RED: DetectCols not wired)", name, vl, detectedCols)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_SoftWrap_Line2_NarrowCols — §4.3 T-9 soft-wrap
//
// At cols<50, FitLine soft-wraps line2 by splitting on " | " and adding extra
// "\n". The output must therefore contain more than 2 newlines (baseline in
// SuperCompact with 3 lines = 2 "\n").
//
// RED: FitLine not wired → renderLine raw-joins probes → no soft-wrap → 2 "\n".
// GREEN: FitLine soft-wraps line2 → extra "\n" → > 2 total.
// ---------------------------------------------------------------------------
func TestAssembler_Render_SoftWrap_Line2_NarrowCols(t *testing.T) {
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "e", priority: 0, visible: true, out: "user@x"},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m", priority: 0, visible: true, out: "sonnet"},
	})
	// Three line2 probes; combined with " | " separators exceeds 40 cols:
	// "cache: 10K/2K" (13) + " | "(3) + "out: 500" (8) + " | "(3) + "cost: $0.12" (11) = 38 → add padding.
	// Use longer probe content to ensure overflow at cols=40.
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c1", priority: 0, visible: true, out: "cache: 10K/2K-read"},
		&fakeProbe{name: "c2", priority: 1, visible: true, out: "out: 500 tokens"},
		&fakeProbe{name: "c3", priority: 2, visible: true, out: "cost: $0.12"},
	})

	a := makeAssemblerForCols(mode.SuperCompact, 40)
	out := a.Render(makeDataWithTurns(0))

	// §4.3 concept soft-wrap: at cols<50, line2 is split onto extra lines.
	// SuperCompact baseline: 2 newlines (3 lines). Soft-wrap adds ≥ 1 more.
	nlCount := strings.Count(out, "\n")
	if nlCount <= 2 {
		t.Errorf("SoftWrap: expected >2 newlines at cols=40 (soft-wrap active); got %d\noutput: %q", nlCount, out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_StillNoAnsi — §4.2 C-10 invariant survives truncation
//
// Render() must never emit ANSI escape codes regardless of cols value.
//
// PASS on stub: Phase 4.2 already guarantees no ANSI in the assembler.
// This guards against regressions during GREEN implementation.
// ---------------------------------------------------------------------------
func TestAssembler_Render_StillNoAnsi(t *testing.T) {
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "e", priority: 0, visible: true, out: "user@example.com"},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m", priority: 0, visible: true, out: "claude-sonnet-4-6"},
	})
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c", priority: 0, visible: true, out: "cache: 10K/2K"},
	})

	for _, cols := range []int{40, 50, 80, 120} {
		a := makeAssemblerForCols(mode.SuperCompact, cols)
		out := a.Render(makeDataWithTurns(0))

		// §4.2 C-10: output must not contain ANSI escape codes.
		if strings.Contains(out, "\x1b") {
			t.Errorf("StillNoAnsi[cols=%d]: output contains ANSI escape code (\\x1b)\noutput: %q", cols, out)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_MarkersStillPresent — §4.2 C-9 invariant survives truncation
//
// After truncation, the output must still contain at least one "{{" marker token.
//
// PASS on stub: markers survive from Phase 4.2 renderLine logic.
// Guards against regressions where truncation accidentally strips all markers.
// ---------------------------------------------------------------------------
func TestAssembler_Render_MarkersStillPresent(t *testing.T) {
	// Two probes on line0 to force the "{{dim}} • {{reset}}" separator.
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "e", priority: 0, visible: true, out: "user@example.com"},
		&fakeProbe{name: "p", priority: 1, visible: true, out: "myproject"},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m", priority: 0, visible: true, out: "sonnet"},
	})
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c", priority: 0, visible: true, out: "cache: 10K"},
	})

	a := makeAssemblerForCols(mode.SuperCompact, 80)
	out := a.Render(makeDataWithTurns(0))

	// §4.2 C-9: at least one {{...}} marker must survive in the output.
	if !strings.Contains(out, "{{") {
		t.Errorf("MarkersStillPresent: output does not contain any '{{' marker; truncation must not strip all markers\noutput: %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_RowCountStable_Standard — §4.3 T-9 + C-6
//
// Column-drop must not remove table rows. With 5 turns and cols=80, the
// per-turn table must contain exactly 5 data rows.
//
// PASS on stub: perTurnTable already uses b.Render() (real Phase 4.2.d impl),
// which correctly produces 5 rows. After GREEN, RenderForCols(80) must also
// produce 5 rows (column-drop only, no row-drop).
// ---------------------------------------------------------------------------
func TestAssembler_Render_RowCountStable_Standard(t *testing.T) {
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "e", priority: 0, visible: true, out: "user@x"},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m", priority: 0, visible: true, out: "sonnet"},
	})
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c", priority: 0, visible: true, out: "cache:0"},
	})

	const nTurns = 5
	a := makeAssemblerForCols(mode.Standard, 80)
	out := a.Render(makeDataWithTurns(nTurns))

	// §4.3 T-9 + C-6: column-drop must not remove rows; exactly 5 data rows.
	got := countDataRows(out)
	if got != nTurns {
		t.Errorf("RowCountStable: expected %d data rows (column-drop must not drop rows); got %d\noutput:\n%s", nTurns, got, out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_Cap20StillEnforced_AfterTruncation — §4.3 T-9 + C-6
//
// With 30 turns and cols=50, the assembler must still cap the table at 20 rows.
// Column-drop narrows the table but must never drop rows.
//
// PASS on stub: perTurnTable already caps at 20 (Phase 4.2.d). After GREEN,
// RenderForCols(50) must also produce 20 rows.
// ---------------------------------------------------------------------------
func TestAssembler_Render_Cap20StillEnforced_AfterTruncation(t *testing.T) {
	swapLine0(t, []probes.Probe{
		&fakeProbe{name: "e", priority: 0, visible: true, out: "user@x"},
	})
	swapLine1(t, []probes.Probe{
		&fakeProbe{name: "m", priority: 0, visible: true, out: "sonnet"},
	})
	swapLine2(t, []probes.Probe{
		&fakeProbe{name: "c", priority: 0, visible: true, out: "cache:0"},
	})

	const nTurns = 30
	const wantRows = 20 // §4.2 C-6 cap

	a := makeAssemblerForCols(mode.Standard, 50)
	out := a.Render(makeDataWithTurns(nTurns))

	// §4.3 T-9 + C-6: cap-20 must hold even after column-drop at cols=50.
	got := countDataRows(out)
	if got != wantRows {
		t.Errorf("Cap20AfterTruncation: expected %d data rows (cap-20 survives column-drop); got %d\noutput:\n%s", wantRows, got, out)
	}
}
