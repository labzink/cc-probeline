// Package probes_test — RED tests for Phase 6.8 FIXES: C3 TTL visible.
//
// Root cause (from review-consolidated.md C3):
//   cacheTTL() suppresses TTL when subagentGapMinutes > 0.
//   The problem: config.SubagentGapMinutes default = 5 (non-zero) is passed as
//   subagentGapMinutes to cacheTTL(). So TTL is always hidden even in orchestrator
//   context, because the threshold is used as a runtime flag instead of checking
//   "is this actually a subagent context?".
//
// Fix vector: pass 0 for orchestrator render (or use an explicit isSubagent bool).
//   The config threshold SubagentGapMinutes must only be used in subagent context.
//
// This test uses CacheProbe.Render with a default-like config (OrchTTLMinutes=60,
// SubagentGapMinutes=5 as stored in config but session is orchestrator context),
// and asserts that TTL "⏱" IS visible.
//
// RED: with current code, SubagentGapMinutes=5 → cacheTTL returns "" → ⏱ absent.
package probes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCacheProbe_TTL_OrchContext_DefaultConfig (C3 / T-23, T-24) verifies that
// CacheProbe shows the TTL block ("⏱") when the context is orchestrator, even
// when Config.SubagentGapMinutes > 0 (the threshold is a config value, not a
// runtime "is subagent" flag).
//
// Setup:
//   - OrchTTLMinutes = 60 (default from config)
//   - SubagentGapMinutes = 5 (default from config — this is the THRESHOLD, not a flag)
//   - Session has turns and a recent LastTimestamp (20m ago → remaining=40m → green)
//   - AnsiEnabled = true so colour markers are emitted
//
// Expected: output at LevelFull contains "⏱" with a positive minute count.
//
// RED: CacheProbe currently passes c.SubagentGapMinutes to cacheTTL(), which
// treats any value > 0 as "subagent context" → TTL is always suppressed.
// After fix, the subagentGapMinutes arg to cacheTTL must be 0 for orch context.
func TestCacheProbe_TTL_OrchContext_DefaultConfig(t *testing.T) {
	p := &probes.CacheProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	// Default-like config: both fields populated as a real config would look.
	// SubagentGapMinutes=5 is the threshold value, NOT a "is-subagent" flag.
	cfg := probes.Config{
		CacheEnabled:       true,
		CostEnabled:        true,
		OrchTTLMinutes:     60,
		SubagentGapMinutes: 5, // default config value — must NOT suppress TTL in orch context
	}

	now := time.Date(2026, 6, 1, 12, 20, 0, 0, time.UTC)
	lastTS := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) // 20m ago → remaining=40m

	d := probes.Data{
		Session: &parser.SessionStats{
			Totals: parser.TokenCounts{
				CacheRead:   3000,
				CacheCreate: 500,
				Output:      1000,
			},
			LastTimestamp: lastTS,
			TurnCount:     3,
		},
		Stdin: stdin.Payload{
			Cost: stdin.Cost{
				TotalCostUSD:       5.00,
				TotalAPIDurationMS: 120000,
			},
		},
		Now: now,
	}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// TTL block must be present: ⏱ with positive minutes.
	if !strings.Contains(got, "⏱") {
		t.Errorf("C3 CacheProbe TTL in orch context: want '⏱' in output, got %q\n"+
			"  FIX: SubagentGapMinutes config value must not suppress TTL in orchestrator context;\n"+
			"  cacheTTL() must receive subagentGapMinutes=0 when rendering for orchestrator", got)
	}

	// At 40m remaining → green marker.
	if !strings.Contains(got, "{{color:green}}") {
		t.Errorf("C3 CacheProbe TTL colour: 40m remaining should use green marker, got %q", got)
	}

	// Verify a positive minute count appears (not "0m").
	if strings.Contains(got, "⏱ 0m") {
		t.Errorf("C3 CacheProbe TTL: got '⏱ 0m' but remaining=40m should show positive count, got %q", got)
	}
}
