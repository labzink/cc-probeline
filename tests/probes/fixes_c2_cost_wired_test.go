// Package probes_test — header cost source.
//
// Phase 6.8 C2 once required the header to show d.SessionTotal (our session-delta
// estimate) instead of the raw official total. Phase 7.46 reverses that decision:
// the header must show the single authoritative figure — the official Claude Code
// meter d.Stdin.Cost.TotalCostUSD — even at a small lag, because our table-driven
// estimate carries a known reasoning-billing gap. The per-turn table keeps our
// estimate (labelled "~cost"); a small header↔table mismatch is expected.
//
// This test goes through CostProbe.Render directly (legitimate: CostProbe is a
// leaf probe; the Assembler just calls p.Render).
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCostProbe_UsesOfficialTotal (Phase 7.46) verifies that CostProbe.Render
// shows the official d.Stdin.Cost.TotalCostUSD, NOT our d.SessionTotal estimate.
//
// Setup:
//   - d.Stdin.Cost.TotalCostUSD = 10.00  (official meter — must appear)
//   - d.SessionTotal            = 3.50   (our estimate — must NOT drive the header)
//
// Expected: output contains "$10.00", NOT "$3.50".
func TestCostProbe_UsesOfficialTotal(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{CostEnabled: true}

	d := probes.Data{
		Stdin: stdin.Payload{
			Cost: stdin.Cost{TotalCostUSD: 10.00}, // official meter — must appear in output
		},
		SessionTotal: 3.50, // our estimate — must NOT drive the header
	}

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, cfg, th, level)

		// Must show the official total, not our estimate.
		if !strings.Contains(got, "$10.00") {
			t.Errorf("Phase 7.46 CostProbe.Render(level=%v): want '$10.00' (official TotalCostUSD) in output, got %q"+
				"\n  FIX: CostProbe header must read d.Stdin.Cost.TotalCostUSD", level, got)
		}
		// Must NOT show our session-delta estimate.
		if strings.Contains(got, "$3.50") {
			t.Errorf("Phase 7.46 CostProbe.Render(level=%v): must NOT show our '$3.50' (SessionTotal), got %q"+
				"\n  FIX: CostProbe header must read the official total", level, got)
		}
	}
}

// TestCostProbe_ClipsToSessionBaseline (Phase 7.46) verifies the header clips the
// official cumulative meter to the current session via st.BaselineCost. After a
// /clear the meter keeps climbing (TotalCostUSD is cumulative), but a fresh
// session_id captures a new baseline; the header must show only ccTotal − baseline.
//
// Setup (post-/clear session, mirrors live state 9ed7c7ea):
//   - d.Stdin.Cost.TotalCostUSD = 25.90  (cumulative, includes pre-/clear spend)
//   - d.State.BaselineCost      = 21.94  (captured at this session's first Reconcile)
//
// Expected: "$3.96" (session-only), NOT "$25.90" (cumulative).
func TestCostProbe_ClipsToSessionBaseline(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{CostEnabled: true}

	d := probes.Data{
		Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: 25.90}},
		State: &state.Session{BaselineCost: 21.94},
	}
	got := p.Render(d, cfg, th, probes.LevelCompact)
	if !strings.Contains(got, "$3.96") {
		t.Errorf("header must clip to session via baseline: want '$3.96', got %q", got)
	}
	if strings.Contains(got, "$25.90") {
		t.Errorf("header must NOT show pre-/clear cumulative '$25.90', got %q", got)
	}
}

// TestCacheProbe_UsesLastRequestCost (C2+I3 / T-8) was deleted in Phase 7 (BL-33):
// CacheProbe was removed. The property it tested (last-request cost in the row-2
// aggregate) is no longer relevant — the per-turn cost column in the unified
// table is covered by tests in tests/renderer/table_redesign_test.go (T-T6).
