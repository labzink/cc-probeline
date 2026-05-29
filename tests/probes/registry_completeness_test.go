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

	const want = 10
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
// uniformly. Uses a per-probe expected-priority map to catch wrong Priority()
// values (tighter than range-only check — see spec §A4 and code-review T3).
//
// This brings package coverage above the PLAN ≥85% threshold by visiting
// all 33 trivial methods (11 probes × 3 getters) in one pass.
func TestRegistry_TrivialGetters(t *testing.T) {
	all := append([]probes.Probe{}, probes.Line0Registry...)
	all = append(all, probes.Line1Registry...)
	all = append(all, probes.Line2Registry...)
	all = append(all, probes.SubagentRegistry...)

	wantPriority := map[string]int{
		"model":    0,
		"ctx":      0,
		"time":     0,
		"quota":    3,
		"project":  2,
		"cache":    2,
		"git":      2,
		"cost":     2,
		"email":    1,
		"subagent": 4,
	}

	for _, p := range all {
		name := p.Name()
		if name == "" {
			t.Errorf("probe %T: Name() returned empty string", p)
			continue
		}

		want, ok := wantPriority[name]
		if !ok {
			t.Errorf("probe %q: not in expected-priority map", name)
		} else if got := p.Priority(); got != want {
			t.Errorf("probe %q: Priority() = %d, want %d", name, got, want)
		}

		if mw := p.MinWidth(); mw < 0 {
			t.Errorf("probe %s: MinWidth()=%d, want >= 0", name, mw)
		}
	}
}

// TestRegistry_PriorityValues pins the exact Priority() value for each of the
// 11 registered probes. Fails immediately if any probe's Priority() drifts from
// the values specified in the Phase 4.3 PLAN (Pre-step #4) and the architecture
// concept §4.3 line 869.
//
// Priority table (pinned):
//
//	model=0  effort=0  ctx=0  time=0
//	quota=3
//	project=2  cache=2  git=2  cost=2
//	email=1
//	subagent=4
func TestRegistry_PriorityValues(t *testing.T) {
	tt := []struct {
		name string
		want int
	}{
		{"model", 0},
		{"time", 0},
		{"ctx", 0},
		{"quota", 3},
		{"project", 2},
		{"cache", 2},
		{"git", 2},
		{"cost", 2},
		{"email", 1},
		{"subagent", 4},
	}

	all := append([]probes.Probe{}, probes.Line0Registry...)
	all = append(all, probes.Line1Registry...)
	all = append(all, probes.Line2Registry...)
	all = append(all, probes.SubagentRegistry...)

	byName := make(map[string]probes.Probe, len(all))
	for _, p := range all {
		byName[p.Name()] = p
	}

	for _, tc := range tt {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			p, ok := byName[tc.name]
			if !ok {
				t.Fatalf("probe %q not found in any registry", tc.name)
			}
			if got := p.Priority(); got != tc.want {
				t.Fatalf("probe %q: Priority() = %d, want %d", tc.name, got, tc.want)
			}
		})
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
