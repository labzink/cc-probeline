// Package renderer_test — footer icon tests for Phase 4.4.c (spec-I2).
// All tests in this file are RED on the Phase 4.4.0 foundation:
//   - Icons (≡ ↗ ⏱) are absent from the current footer.
//   - Duration is not in MM:SS format.
//   - Cost is hardcoded "$0.00" regardless of model/tokens.
//
// GREEN (Phase 4.4.c) must add icons, FormatMMSS duration, and CostCalculator
// wiring to make all tests PASS.
package renderer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// footerLine extracts the footer content row from a rendered table.
// The footer is the second-to-last non-empty line (the last is the bottom border).
func footerLine(rendered string) string {
	lines := splitLines(rendered)
	if len(lines) < 2 {
		return ""
	}
	// Walk backwards: skip the bottom border (starts with └), return first
	// content line (starts with │ and contains no ─).
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if strings.HasPrefix(l, "│") && !strings.Contains(l, "─") {
			return l
		}
	}
	return ""
}

// makeFullTurn builds a parser.Turn with all fields for footer tests.
func makeFullTurn(model string, input, output, cacheRead, cacheCreate int, dur time.Duration) parser.Turn {
	return parser.Turn{
		Index:    1,
		Role:     "orch",
		Model:    model,
		Tokens:   parser.TokenCounts{Input: input, Output: output, CacheRead: cacheRead, CacheCreate: cacheCreate},
		ToolUse:  "Read",
		Duration: dur,
	}
}

// ---------------------------------------------------------------------------
// Icon presence tests — RED: current footer has no icons
// ---------------------------------------------------------------------------

// TestBuilder_Footer_ContainsCacheIcon: the rendered footer must contain the
// cache icon ≡ (U+2261, IDENTICAL TO).
func TestBuilder_Footer_ContainsCacheIcon(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeFullTurn("sonnet-4-6", 0, 1000, 500, 0, 30*time.Second))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if !strings.Contains(footer, "≡") {
		t.Errorf("footer does not contain cache icon ≡ (U+2261)\nfooter: %q\nfull output:\n%s", footer, out)
	}
}

// TestBuilder_Footer_ContainsOutIcon: the rendered footer must contain the
// output-tokens icon ↗ (U+2197, NORTH EAST ARROW).
func TestBuilder_Footer_ContainsOutIcon(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeFullTurn("sonnet-4-6", 0, 1000, 500, 0, 30*time.Second))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if !strings.Contains(footer, "↗") {
		t.Errorf("footer does not contain output icon ↗ (U+2197)\nfooter: %q\nfull output:\n%s", footer, out)
	}
}

// TestBuilder_Footer_ContainsDurationIcon: the rendered footer must contain
// the duration icon ⏱ (U+23F1, STOPWATCH).
func TestBuilder_Footer_ContainsDurationIcon(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeFullTurn("sonnet-4-6", 0, 1000, 500, 0, 30*time.Second))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if !strings.Contains(footer, "⏱") {
		t.Errorf("footer does not contain duration icon ⏱ (U+23F1)\nfooter: %q\nfull output:\n%s", footer, out)
	}
}

// ---------------------------------------------------------------------------
// Duration MM:SS test
// ---------------------------------------------------------------------------

// TestBuilder_Footer_DurationMMSS: three turns with durations 1m20s, 2m, 25s.
// Total = 80s + 120s + 25s = 225s = 3m45s.
// format.FormatMMSS(225_000 ms) = "03:45".
// The footer must contain "⏱ 03:45".
func TestBuilder_Footer_DurationMMSS(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeFullTurn("sonnet-4-6", 0, 100, 0, 0, 1*time.Minute+20*time.Second)) // 80s
	b.Add(makeFullTurn("sonnet-4-6", 0, 100, 0, 0, 2*time.Minute))                // 120s
	b.Add(makeFullTurn("sonnet-4-6", 0, 100, 0, 0, 25*time.Second))               // 25s
	// Total duration: 225s → FormatMMSS(225_000) = "03:45"

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if !strings.Contains(footer, "03:45") {
		t.Errorf("footer duration: want \"03:45\" (3m45s total); footer: %q\nfull output:\n%s", footer, out)
	}
}

// ---------------------------------------------------------------------------
// Cost wiring tests
// ---------------------------------------------------------------------------

// TestBuilder_Footer_CostKnownModel: one turn with opus-4-7 and 1M input tokens.
// Manual: 1_000_000 × $15.00/M = $15.00.
// The footer must contain "$15.00".
func TestBuilder_Footer_CostKnownModel(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(parser.Turn{
		Index:   1,
		Role:    "orch",
		Model:   "opus-4-7",
		Tokens:  parser.TokenCounts{Input: 1_000_000},
		ToolUse: "Read",
	})

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if !strings.Contains(footer, "$15.00") {
		t.Errorf("footer cost: want \"$15.00\" (opus 1M input tokens); footer: %q\nfull output:\n%s", footer, out)
	}
}

// TestBuilder_Footer_CostUnknownModel_ZeroDisplay: unknown model → Compute
// returns 0 → footer must show "$0.00" (graceful degradation).
func TestBuilder_Footer_CostUnknownModel_ZeroDisplay(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(parser.Turn{
		Index:   1,
		Role:    "orch",
		Model:   "unknown-model-xyz",
		Tokens:  parser.TokenCounts{Input: 1_000_000, Output: 500_000},
		ToolUse: "Bash",
	})

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if !strings.Contains(footer, "$0.00") {
		t.Errorf("footer cost: want \"$0.00\" for unknown model; footer: %q\nfull output:\n%s", footer, out)
	}
}

// ---------------------------------------------------------------------------
// Width constraint
// ---------------------------------------------------------------------------

// TestBuilder_Footer_SingleLine_FitsCols80: the footer line must fit within
// 80 terminal columns. Uses format.VisualLen which accounts for wide UTF-8
// glyphs (icons ≡ ⏱ ↗ are single-width, so they count as 1 each).
func TestBuilder_Footer_SingleLine_FitsCols80(t *testing.T) {
	b := renderer.NewBuilder(80)
	b.Add(makeFullTurn("sonnet-4-6", 1_000_000, 100_000, 500_000, 50_000, 2*time.Minute))

	out := b.Render()
	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	footer := footerLine(out)
	if footer == "" {
		t.Fatal("could not extract footer line from rendered output")
	}
	w := format.VisualLen(footer)
	if w > 80 {
		t.Errorf("footer visual width = %d; want <= 80\nfooter: %q", w, footer)
	}
}
