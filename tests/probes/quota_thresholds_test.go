// Package probes_test — tests for the per-window configurable quota colour
// thresholds (Phase 7.47 config-honesty). Each rate-limit window (5h, 7d) has its
// own notice/warn/critical trio driving the green→yellow→orange→red gradient,
// with a fixed bold_red cap above 95%. A zero Config falls back to the baked
// defaults 0.50/0.70/0.90. Asserted at Minimal level, where the percentage number
// carries the colour marker directly.
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

func quotaData(pct5h, pct7d float64) probes.Data {
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: pct5h},
		SevenDay: stdin.RateWindow{UsedPercentage: pct7d},
	}
	return probes.Data{Stdin: stdin.Payload{RateLimits: rl}}
}

// TestQuota_Thresholds_DefaultBands verifies the default fallback bands on both
// windows (same trio, so identical markers at the same percentage).
func TestQuota_Thresholds_DefaultBands(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir()) // empty → no snapshot → use RateLimits
	p := &probes.QuotaProbe{}
	th := renderer.Theme{AnsiEnabled: true}
	cfg := probes.Config{QuotaEnabled: true} // zero ratios → default fallback

	cases := []struct {
		pct  float64
		want string
	}{
		{40, "{{color:green}}"},
		{55, "{{color:yellow}}"},
		{75, "{{color:orange}}"},
		{92, "{{color:red}}"},
		{97, "{{color:bold_red}}"},
	}
	for _, c := range cases {
		got := p.Render(quotaData(c.pct, c.pct), cfg, th, probes.LevelMinimal)
		if !strings.Contains(got, c.want) {
			t.Errorf("default quota bands pct=%.0f: want %q, got %q", c.pct, c.want, got)
		}
	}
}

// TestQuota_Thresholds_PerWindowIndependent proves the two windows use separate
// trios: at the same fill, the 5h window (low thresholds) reds out while the 7d
// window (high thresholds) stays green.
func TestQuota_Thresholds_PerWindowIndependent(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())
	p := &probes.QuotaProbe{}
	th := renderer.Theme{AnsiEnabled: true}
	cfg := probes.Config{
		QuotaEnabled:         true,
		Quota5hNoticeRatio:   0.10,
		Quota5hWarnRatio:     0.20,
		Quota5hCriticalRatio: 0.30,
		Quota7dNoticeRatio:   0.60,
		Quota7dWarnRatio:     0.70,
		Quota7dCriticalRatio: 0.80,
	}

	got := p.Render(quotaData(55, 55), cfg, th, probes.LevelMinimal)
	if !strings.Contains(got, "{{color:red}}") {
		t.Errorf("5h window at 55%% with critical=0.30 must be red: %q", got)
	}
	if !strings.Contains(got, "{{color:green}}") {
		t.Errorf("7d window at 55%% with notice=0.60 must be green: %q", got)
	}
}
