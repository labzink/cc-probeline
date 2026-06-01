// Package probes_test — RED tests for Phase 6.8 FIXES: S1 time not wrapped in dim.
//
// Root cause (from review-consolidated.md S1):
//   TimeProbe.Render wraps the value in {{dim}}…{{reset}} when AnsiEnabled=true.
//   CostProbe.Render does NOT wrap in dim. Spec requires consistent treatment:
//   time should be styled like cost (no dim wrapper).
//
// Fix vector: remove {{dim}} wrapper from TimeProbe.Render.
//
// RED: TimeProbe.Render with AnsiEnabled=true currently produces
//   "{{dim}}MM:SS{{reset}}" — must instead produce "MM:SS" (no dim).
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestTimeProbe_NoDim_AnsiEnabled (S1) verifies that TimeProbe.Render does NOT
// wrap the time value in {{dim}}…{{reset}} when AnsiEnabled=true.
//
// Spec: time display should be consistent with CostProbe (no dim).
// The dim wrapper makes time visually lighter than cost, which is inconsistent.
//
// Setup: AnsiEnabled=true, TotalAPIDurationMS=90000 (1:30).
//
// Expected: output does NOT contain "{{dim}}".
// RED: current code always wraps with "{{dim}}" + raw + "{{reset}}" → fails.
func TestTimeProbe_NoDim_AnsiEnabled(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.DefaultPalette(),
	}
	cfg := probes.Config{TimeEnabled: true}

	d := probes.Data{
		Stdin: stdin.Payload{
			Cost: stdin.Cost{TotalAPIDurationMS: 90000}, // 1:30
		},
	}

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, cfg, th, level)

		if strings.Contains(got, "{{dim}}") {
			t.Errorf("S1 TimeProbe.Render(level=%v, AnsiEnabled=true): must NOT contain '{{dim}}', got %q\n"+
				"  FIX: remove {{dim}} wrapper from TimeProbe.Render; style like CostProbe (no dim)", level, got)
		}
		if strings.Contains(got, "{{reset}}") && !strings.Contains(got, "{{color:") {
			// A {{reset}} without a preceding colour marker is a leftover from the dim wrapper.
			t.Errorf("S1 TimeProbe.Render(level=%v): orphan '{{reset}}' without colour marker suggests leftover dim wrapper, got %q",
				level, got)
		}
	}
}
