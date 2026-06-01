// Package probes_test — RED-phase tests for cache TTL behaviour (Phase 6.8.a).
//
// T-C6 (TestTTL_OrchOnly): TTL is shown only in orchestrator context
// (SubagentGapMinutes==0); when SubagentGapMinutes>0 (subagent context),
// the TTL block must be absent from all render levels.
//
// T-C7 (TestTTL_ZeroRed): when remaining ≤ 0, CacheProbe must render "0m"
// with a {{color:bold_red}} marker (not suppress the block entirely).
// Colour rules:
//
//	> 30m  → {{color:green}}⏱ Nm{{reset}}
//	≤ 30m  → {{color:yellow}}⏱ Nm{{reset}}
//	≤ 10m  → {{color:red}}⏱ Nm{{reset}}
//	≤ 0m   → {{color:bold_red}}⏱ 0m{{reset}}  (NOT hidden)
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

// newTTLData builds a probes.Data with session timestamps suitable for TTL tests.
// orchTTLMinutes drives the OrchTTLMinutes config field.
// subagentGapMinutes drives the SubagentGapMinutes config field.
func newTTLData(now time.Time, lastTS time.Time, turnCount int) probes.Data {
	return probes.Data{
		Session: &parser.SessionStats{
			Totals: parser.TokenCounts{
				CacheRead:   1000,
				CacheCreate: 2000,
				Output:      500,
			},
			LastTimestamp: lastTS,
			TurnCount:     turnCount,
		},
		Stdin: stdin.Payload{
			Cost: stdin.Cost{
				TotalCostUSD:       0.10,
				TotalAPIDurationMS: 60000,
			},
		},
		Now: now,
	}
}

// ---------------------------------------------------------------------------
// T-C6: TestTTL_OrchOnly
// Spec: T-23 — TTL shown only in orchestrator context (SubagentGapMinutes==0).
// ---------------------------------------------------------------------------

// TestTTL_OrchOnly verifies that the TTL block (⏱Nm) is present in orchestrator
// context (SubagentGapMinutes=0) and absent in subagent context (SubagentGapMinutes>0),
// even when the session and OrchTTLMinutes are identical in both cases.
func TestTTL_OrchOnly(t *testing.T) {
	// Setup: 60m window, LastTimestamp=8m ago → remaining=52 → TTL active.
	now := time.Date(2024, 6, 1, 12, 8, 0, 0, time.UTC)
	lastTS := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	d := newTTLData(now, lastTS, 3)

	p := &probes.CacheProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	// Case A: orchestrator context — SubagentGapMinutes=0, OrchTTLMinutes=60.
	// TTL block must appear at all levels.
	orchCfg := cfgAllOn()
	orchCfg.OrchTTLMinutes = 60
	orchCfg.SubagentGapMinutes = 0 // orchestrator

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, orchCfg, th, level)
		if !strings.Contains(got, "⏱") {
			t.Errorf("TestTTL_OrchOnly [orch, level=%v]: TTL block ⏱ must appear in orchestrator context, got %q", level, got)
		}
	}

	// Case B: subagent context — SubagentGapMinutes=5 (>0), OrchTTLMinutes=60.
	// TTL block must be absent at all levels.
	subCfg := cfgAllOn()
	subCfg.OrchTTLMinutes = 60
	subCfg.SubagentGapMinutes = 5 // subagent — TTL suppressed

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, subCfg, th, level)
		if strings.Contains(got, "⏱") {
			t.Errorf("TestTTL_OrchOnly [subagent, level=%v]: TTL block ⏱ must NOT appear in subagent context, got %q", level, got)
		}
	}
}

// ---------------------------------------------------------------------------
// T-C7: TestTTL_ZeroRed
// Spec: T-24 — at remaining≤0, show "0m" with bold_red (not suppress).
//             at remaining>30, show green marker.
// ---------------------------------------------------------------------------

// TestTTL_ZeroRed verifies the full colour graduation for the TTL block:
//
//	remaining > 30m  → {{color:green}}⏱ Nm{{reset}}
//	remaining ≤ 30m  → {{color:yellow}}⏱ Nm{{reset}}
//	remaining ≤ 10m  → {{color:red}}⏱ Nm{{reset}}
//	remaining ≤ 0m   → {{color:bold_red}}⏱ 0m{{reset}}  (must NOT be hidden)
func TestTTL_ZeroRed(t *testing.T) {
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		orchTTL        int    // OrchTTLMinutes
		elapsedMinutes int    // minutes since LastTimestamp
		wantContains   string // expected substring in Render output
		wantAbsent     string // must not appear (empty = no check)
	}{
		{
			// remaining = 60 - 5 = 55 → > 30m → green
			name:           "remaining=55m — green",
			orchTTL:        60,
			elapsedMinutes: 5,
			wantContains:   "{{color:green}}",
			wantAbsent:     "",
		},
		{
			// remaining = 60 - 35 = 25 → ≤ 30m → yellow
			name:           "remaining=25m — yellow",
			orchTTL:        60,
			elapsedMinutes: 35,
			wantContains:   "{{color:yellow}}",
			wantAbsent:     "{{color:green}}",
		},
		{
			// remaining = 60 - 55 = 5 → ≤ 10m → red
			name:           "remaining=5m — red",
			orchTTL:        60,
			elapsedMinutes: 55,
			wantContains:   "{{color:red}}",
			wantAbsent:     "{{color:yellow}}",
		},
		{
			// remaining = 60 - 60 = 0 → ≤ 0m → bold_red "0m" (NOT hidden)
			name:           "remaining=0m — bold_red 0m visible",
			orchTTL:        60,
			elapsedMinutes: 60,
			wantContains:   "{{color:bold_red}}",
			wantAbsent:     "",
		},
		{
			// remaining = 60 - 70 = -10 → ≤ 0m → bold_red "0m" (NOT hidden)
			name:           "remaining=-10m — bold_red 0m visible",
			orchTTL:        60,
			elapsedMinutes: 70,
			wantContains:   "{{color:bold_red}}",
			wantAbsent:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: session with LastTimestamp set to elapsedMinutes ago.
			now := base
			lastTS := base.Add(-time.Duration(tc.elapsedMinutes) * time.Minute)
			d := newTTLData(now, lastTS, 3)

			p := &probes.CacheProbe{}
			// AnsiEnabled=true so colour markers are emitted.
			th := renderer.Theme{AnsiEnabled: true}

			cfg := cfgAllOn()
			cfg.OrchTTLMinutes = tc.orchTTL
			cfg.SubagentGapMinutes = 0 // orchestrator context

			got := p.Render(d, cfg, th, probes.LevelFull)

			// Assert the expected colour marker is present.
			if !strings.Contains(got, tc.wantContains) {
				t.Errorf("Render(Full) [%s]: want %q in output, got %q",
					tc.name, tc.wantContains, got)
			}

			// For remaining ≤ 0: the TTL block must be visible (not suppressed).
			if tc.elapsedMinutes >= tc.orchTTL {
				if !strings.Contains(got, "⏱") {
					t.Errorf("Render(Full) [%s]: TTL block ⏱ must appear even at remaining≤0, got %q",
						tc.name, got)
				}
				if !strings.Contains(got, "0m") {
					t.Errorf("Render(Full) [%s]: output must contain '0m' when remaining≤0, got %q",
						tc.name, got)
				}
			}

			// Assert excluded marker is absent (where specified).
			if tc.wantAbsent != "" && strings.Contains(got, tc.wantAbsent) {
				t.Errorf("Render(Full) [%s]: must NOT contain %q, got %q",
					tc.name, tc.wantAbsent, got)
			}
		})
	}
}
