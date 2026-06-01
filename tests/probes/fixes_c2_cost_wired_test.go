// Package probes_test — RED tests for Phase 6.8 FIXES: C2+I3 cost wired.
//
// Root cause (from review-consolidated.md):
//   - CostProbe.Render uses d.Stdin.Cost.TotalCostUSD (raw cumulative total)
//     instead of d.SessionTotal (delta from baseline, computed by cost.SessionTotal).
//   - CacheProbe cost segment also uses raw TotalCostUSD instead of d.LastRequestCost.
//
// Production path: Assembler.Render(d) → CostProbe.Render(d,...) → must read d.SessionTotal.
//
// These tests go through CostProbe.Render directly (legitimate: CostProbe is
// a leaf probe, not an internal helper; Assembler just calls p.Render).
// The key fix: the probe must use d.SessionTotal, not d.Stdin.Cost.TotalCostUSD.
//
// RED: both tests FAIL until CostProbe/CacheProbe are updated to read the
// delta fields instead of the raw total.
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCostProbe_UsesSessionTotal (C2 / T-6) verifies that CostProbe.Render
// shows d.SessionTotal (ccTotal − BaselineCost), NOT d.Stdin.Cost.TotalCostUSD.
//
// Setup:
//   - d.Stdin.Cost.TotalCostUSD = 10.00  (raw cumulative from CC)
//   - d.SessionTotal             = 3.50   (delta since /clear baseline)
//
// Expected: output contains "$3.50", NOT "$10.00".
//
// RED: CostProbe currently reads d.Stdin.Cost.TotalCostUSD → outputs "$10.00".
func TestCostProbe_UsesSessionTotal(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{CostEnabled: true}

	d := probes.Data{
		Stdin: stdin.Payload{
			Cost: stdin.Cost{TotalCostUSD: 10.00}, // raw cumulative — must NOT appear in output
		},
		SessionTotal: 3.50, // delta cost for this session — must appear in output
	}

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, cfg, th, level)

		// Must show the session delta, not the raw total.
		if !strings.Contains(got, "$3.50") {
			t.Errorf("C2 CostProbe.Render(level=%v): want '$3.50' (SessionTotal) in output, got %q"+
				"\n  FIX: CostProbe must read d.SessionTotal, not d.Stdin.Cost.TotalCostUSD", level, got)
		}
		// Must NOT show the raw total when it differs from SessionTotal.
		if strings.Contains(got, "$10.00") {
			t.Errorf("C2 CostProbe.Render(level=%v): must NOT show raw '$10.00' (TotalCostUSD), got %q"+
				"\n  FIX: CostProbe must read d.SessionTotal", level, got)
		}
	}
}

// TestCacheProbe_UsesLastRequestCost (C2+I3 / T-8) verifies that CacheProbe's
// cost segment shows d.LastRequestCost (cost of the most recent prompt group),
// NOT d.Stdin.Cost.TotalCostUSD.
//
// Setup:
//   - d.Stdin.Cost.TotalCostUSD = 10.00  (raw cumulative)
//   - d.LastRequestCost          = 1.25   (cost of latest prompt group)
//
// Expected: cost segment in output contains "$1.25", NOT "$10.00".
//
// RED: CacheProbe currently reads d.Stdin.Cost.TotalCostUSD → outputs "$10.00".
func TestCacheProbe_UsesLastRequestCost(t *testing.T) {
	p := &probes.CacheProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{CacheEnabled: true, CostEnabled: true}

	d := probes.Data{
		Session: &parser.SessionStats{
			Totals: parser.TokenCounts{
				CacheRead:   5000,
				CacheCreate: 1000,
				Output:      2000,
			},
		},
		Stdin: stdin.Payload{
			Cost: stdin.Cost{
				TotalCostUSD:       10.00, // raw cumulative — must NOT appear as cost in output
				TotalAPIDurationMS: 60000,
			},
		},
		LastRequestCost: 1.25, // last-request delta — must appear in output
	}

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, cfg, th, level)

		// Cost segment must show LastRequestCost.
		if !strings.Contains(got, "$1.25") {
			t.Errorf("C2+I3 CacheProbe.Render(level=%v): want '$1.25' (LastRequestCost) in cost segment, got %q"+
				"\n  FIX: CacheProbe must read d.LastRequestCost, not d.Stdin.Cost.TotalCostUSD", level, got)
		}
		// Must NOT show the raw total.
		if strings.Contains(got, "$10.00") {
			t.Errorf("C2+I3 CacheProbe.Render(level=%v): must NOT show raw '$10.00' (TotalCostUSD), got %q"+
				"\n  FIX: CacheProbe must read d.LastRequestCost", level, got)
		}
	}
}
