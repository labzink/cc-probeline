// Package cost_test — coverage for compat.go legacy shims.
// Compute and ComputeAggregate are deprecated (Phase 6.8.d will remove them)
// but must compile and return correct values for renderer/table.go compatibility.
package cost_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
)

// TestCompute_KnownModel verifies Compute returns the expected USD amount
// for a known model (sonnet-4-6, 1M output tokens = $15.00).
func TestCompute_KnownModel(t *testing.T) {
	turn := parser.Turn{
		Model:  "opus-4-7",
		Tokens: parser.TokenCounts{Input: 1_000_000},
	}
	got := cost.Compute(turn)
	// opus-4-7: input $15/M → 1M input = $15.00.
	want := 15.00
	if !approxEqual(got, want) {
		t.Errorf("Compute(opus-4-7, 1M input) = %.6f; want %.6f", got, want)
	}
}

// TestCompute_UnknownModel verifies Compute returns 0 for unknown models.
func TestCompute_UnknownModel(t *testing.T) {
	turn := parser.Turn{
		Model:  "unknown-model-xyz",
		Tokens: parser.TokenCounts{Input: 1_000_000, Output: 500_000},
	}
	got := cost.Compute(turn)
	if got != 0 {
		t.Errorf("Compute(unknown model) = %.6f; want 0 (graceful degradation)", got)
	}
}

// TestComputeAggregate_Sum verifies ComputeAggregate sums Compute across turns.
func TestComputeAggregate_Sum(t *testing.T) {
	turns := []parser.Turn{
		{Model: "opus-4-7", Tokens: parser.TokenCounts{Input: 1_000_000}},   // $15.00
		{Model: "sonnet-4-6", Tokens: parser.TokenCounts{Output: 1_000_000}}, // $15.00
	}
	got := cost.ComputeAggregate(turns)
	want := 30.00
	if !approxEqual(got, want) {
		t.Errorf("ComputeAggregate = %.6f; want %.6f", got, want)
	}
}

// TestComputeAggregate_EmptySlice verifies ComputeAggregate returns 0 for empty input.
func TestComputeAggregate_EmptySlice(t *testing.T) {
	got := cost.ComputeAggregate(nil)
	if got != 0 {
		t.Errorf("ComputeAggregate(nil) = %.6f; want 0", got)
	}
}
