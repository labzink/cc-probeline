// Package probes_test — tests for the three configurable ctx colour thresholds
// (Phase 7.47 config-honesty). The Ctx probe escalates the Full-level bar colour
// green → yellow → orange → red across notice/warn/critical, with a fixed
// bold_red cap above 95%. A zero Config must fall back to the baked defaults
// 0.50/0.70/0.90 (mirrors config.ApplyRangeFix in the production path).
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// ctxAtPct builds a Data whose context window is filled to approximately pct%.
func ctxAtPct(pct int) probes.Data {
	const size = 200000
	used := size * pct / 100
	return probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{
			Size:         size,
			CurrentUsage: map[string]int{"cache_read_input_tokens": used},
		},
	}}
}

// TestCtx_Thresholds_DefaultBands verifies that a zero Config falls back to the
// baked defaults and reproduces the historical colour bands at their boundaries.
func TestCtx_Thresholds_DefaultBands(t *testing.T) {
	p := &probes.CtxProbe{}
	th := renderer.Theme{AnsiEnabled: true}
	cfg := probes.Config{CtxEnabled: true} // zero ratios → default fallback 0.50/0.70/0.90

	cases := []struct {
		pct  int
		want string
	}{
		{40, "{{color:green}}"},    // < notice (50)
		{55, "{{color:yellow}}"},   // notice..warn
		{75, "{{color:orange}}"},   // warn..critical
		{92, "{{color:red}}"},      // critical..95
		{97, "{{color:bold_red}}"}, // > 95 fixed cap
	}
	for _, c := range cases {
		got := p.Render(ctxAtPct(c.pct), cfg, th, probes.LevelFull)
		if !strings.Contains(got, c.want) {
			t.Errorf("default bands pct=%d: want marker %q, got %q", c.pct, c.want, got)
		}
	}
}

// TestCtx_Thresholds_CustomConfig proves the three keys are actually wired: with
// custom thresholds the colour onsets move accordingly.
func TestCtx_Thresholds_CustomConfig(t *testing.T) {
	p := &probes.CtxProbe{}
	th := renderer.Theme{AnsiEnabled: true}
	cfg := probes.Config{
		CtxEnabled:       true,
		CtxNoticeRatio:   0.30,
		CtxWarnRatio:     0.60,
		CtxCriticalRatio: 0.80,
	}

	cases := []struct {
		pct  int
		want string
	}{
		{25, "{{color:green}}"},  // < notice (30)
		{45, "{{color:yellow}}"}, // notice..warn
		{70, "{{color:orange}}"}, // warn..critical
		{85, "{{color:red}}"},    // >= critical (80), still < 95
	}
	for _, c := range cases {
		got := p.Render(ctxAtPct(c.pct), cfg, th, probes.LevelFull)
		if !strings.Contains(got, c.want) {
			t.Errorf("custom bands pct=%d: want marker %q, got %q", c.pct, c.want, got)
		}
	}
}
