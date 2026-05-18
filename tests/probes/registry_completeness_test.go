// Package probes_test — meta-tests that walk the 4 probe registries and
// exercise trivial getters (Name/Priority/MinWidth). These tests also act
// as the registry-completeness gate from PLAN AC: the sum of all 4
// registries must equal 11 (the total probe count for Phase 4.1).
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
)

// TestRegistry_Complete asserts that all 11 probes are wired into one of the
// four registries. Mirrors PLAN §Cross-step Acceptance line 567-568.
func TestRegistry_Complete(t *testing.T) {
	total := len(probes.Line0Registry) +
		len(probes.Line1Registry) +
		len(probes.Line2Registry) +
		len(probes.SubagentRegistry)

	const want = 11
	if total != want {
		t.Errorf("registry total: want %d probes registered, got %d "+
			"(Line0=%d Line1=%d Line2=%d Subagent=%d)",
			want, total,
			len(probes.Line0Registry),
			len(probes.Line1Registry),
			len(probes.Line2Registry),
			len(probes.SubagentRegistry))
	}
}

// TestRegistry_TrivialGetters iterates every registered probe and calls
// Name/Priority/MinWidth so that the trivial-getter coverage is exercised
// uniformly. Asserts a few invariants:
//
//   - Name() is non-empty.
//   - Priority() is in [0, 4] (per spec §A4 priority table).
//   - MinWidth() is non-negative.
//
// This brings package coverage above the PLAN ≥85% threshold by visiting
// all 33 trivial methods (11 probes × 3 getters) in one pass.
func TestRegistry_TrivialGetters(t *testing.T) {
	all := append([]probes.Probe{}, probes.Line0Registry...)
	all = append(all, probes.Line1Registry...)
	all = append(all, probes.Line2Registry...)
	all = append(all, probes.SubagentRegistry...)

	for _, p := range all {
		name := p.Name()
		if name == "" {
			t.Errorf("probe %T: Name() returned empty string", p)
		}

		prio := p.Priority()
		if prio < 0 || prio > 4 {
			t.Errorf("probe %s: Priority()=%d, want [0,4]", name, prio)
		}

		if mw := p.MinWidth(); mw < 0 {
			t.Errorf("probe %s: MinWidth()=%d, want >= 0", name, mw)
		}
	}
}

// TestRegistry_UniqueNames asserts that every registered probe has a unique
// Name() — otherwise downstream code (slog tagging, registry filtering)
// would be ambiguous.
func TestRegistry_UniqueNames(t *testing.T) {
	all := append([]probes.Probe{}, probes.Line0Registry...)
	all = append(all, probes.Line1Registry...)
	all = append(all, probes.Line2Registry...)
	all = append(all, probes.SubagentRegistry...)

	seen := make(map[string]bool, len(all))
	for _, p := range all {
		n := p.Name()
		if seen[n] {
			t.Errorf("probe Name %q registered more than once", n)
		}
		seen[n] = true
	}
}
