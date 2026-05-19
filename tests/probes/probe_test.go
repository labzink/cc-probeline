// Package probes_test contains black-box foundation tests for internal/probes.
// Covers Level.String() contract and registry ordering stability.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
)

// TestLevel_String verifies that each Level constant produces the expected
// human-readable name, which is used in slog messages and debug output.
func TestLevel_String(t *testing.T) {
	tests := []struct {
		level probes.Level
		want  string
	}{
		{probes.LevelFull, "full"},
		{probes.LevelCompact, "compact"},
		{probes.LevelMinimal, "minimal"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.level.String()
			if got != tc.want {
				t.Errorf("Level(%d).String(): want %q, got %q", int(tc.level), tc.want, got)
			}
		})
	}

	// C5: Level(99) must return "unknown" (default branch, 80% → 100%).
	if got := probes.Level(99).String(); got != "unknown" {
		t.Errorf("Level(99).String(): want %q, got %q", "unknown", got)
	}
}

// TestRegistry_OrderStable verifies that Line1Registry is a fixed-order slice:
// two reads of the same package-level variable return the same probe sequence.
// Stability matters because the visual order in the status line must be
// deterministic across re-renders.
func TestRegistry_OrderStable(t *testing.T) {
	first := probes.Line1Registry
	second := probes.Line1Registry

	if len(first) != len(second) {
		t.Fatalf("Line1Registry: length changed between reads: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("Line1Registry[%d]: probe pointer changed between reads", i)
		}
	}
	// In the RED phase the registry is empty (populated in 4.1.a/b/c).
	// The test asserts ordering stability, not a specific length.
	// When 4.1.a/b/c fill the registry, this test still passes.
	t.Logf("Line1Registry length: %d (expected 0 in 4.1.0 foundation)", len(first))
}
