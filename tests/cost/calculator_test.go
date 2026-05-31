// Package cost_test — unit tests for the CostCalculator (Phase 4.4.c).
// Format tests (TestFormat_*) PASS on the current stub because Format() is
// already implemented. Compute/ComputeAggregate tests are RED: the stub
// returns 0 unconditionally; GREEN fills in the pricing table.
package cost_test

import (
	"math"
	"testing"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
)

// approxEqual returns true when a and b differ by at most 1e-9.
// Used for floating-point USD comparisons where exact bit-equality
// is not guaranteed across platforms.
func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---------------------------------------------------------------------------
// Compute — per-turn cost
// ---------------------------------------------------------------------------

// TestCompute_KnownModel_Opus: Turn{Model:"opus-4-7", Tokens:{Input:1_000_000}}
// Manual: 1_000_000 tokens × $15.00/M = $15.00
func TestCompute_KnownModel_Opus(t *testing.T) {
	turn := parser.Turn{
		Model:  "opus-4-7",
		Tokens: parser.TokenCounts{Input: 1_000_000},
	}
	got := cost.Compute(turn)
	want := 15.00
	if !approxEqual(got, want) {
		t.Errorf("Compute(opus-4-7, Input=1M) = %.6f; want %.6f", got, want)
	}
}

// TestCompute_KnownModel_Opus48: opus-4-8 is a stopgap entry mirroring opus-4-7
// pricing so a freshly-released model is not silently costed at $0 (BL-10).
// Manual: 1_000_000 tokens × $15.00/M = $15.00 (same as opus-4-7).
func TestCompute_KnownModel_Opus48(t *testing.T) {
	turn := parser.Turn{
		Model:  "opus-4-8",
		Tokens: parser.TokenCounts{Input: 1_000_000},
	}
	got := cost.Compute(turn)
	want := 15.00
	if !approxEqual(got, want) {
		t.Errorf("Compute(opus-4-8, Input=1M) = %.6f; want %.6f", got, want)
	}
}

// TestCompute_KnownModel_Sonnet: Turn{Model:"sonnet-4-6", Tokens:{Output:100_000}}
// Manual: 100_000 tokens × $15.00/M = 0.1 × 15 = $1.50
func TestCompute_KnownModel_Sonnet(t *testing.T) {
	turn := parser.Turn{
		Model:  "sonnet-4-6",
		Tokens: parser.TokenCounts{Output: 100_000},
	}
	got := cost.Compute(turn)
	want := 1.50
	if !approxEqual(got, want) {
		t.Errorf("Compute(sonnet-4-6, Output=100K) = %.6f; want %.6f", got, want)
	}
}

// TestCompute_UnknownModel_ReturnsZero: unlisted model must return 0.0 (silent
// graceful degradation — see project_cost_methodology memory).
func TestCompute_UnknownModel_ReturnsZero(t *testing.T) {
	turn := parser.Turn{
		Model:  "unknown-x",
		Tokens: parser.TokenCounts{Input: 1_000_000, Output: 500_000},
	}
	got := cost.Compute(turn)
	if got != 0.0 {
		t.Errorf("Compute(unknown-x) = %.6f; want 0.0", got)
	}
}

// TestCompute_AllTokenTypes_Sum: all four token types contribute to the total.
// Model=sonnet-4-6, Input=1_000_000, Output=100_000, CacheRead=500_000, CacheCreate=200_000.
// Manual (sonnet-4-6 pricing: Input=$3/M, Output=$15/M, CacheRead=$0.30/M, CacheCreate=$3.75/M):
//
//	Input:       1_000_000 × 3.00   / 1_000_000 = 3.00
//	Output:        100_000 × 15.00  / 1_000_000 = 1.50
//	CacheRead:     500_000 × 0.30   / 1_000_000 = 0.15
//	CacheCreate:   200_000 × 3.75   / 1_000_000 = 0.75
//	Total = 5.40
func TestCompute_AllTokenTypes_Sum(t *testing.T) {
	turn := parser.Turn{
		Model: "sonnet-4-6",
		Tokens: parser.TokenCounts{
			Input:       1_000_000,
			Output:      100_000,
			CacheRead:   500_000,
			CacheCreate: 200_000,
		},
	}
	got := cost.Compute(turn)
	want := 5.40
	if !approxEqual(got, want) {
		t.Errorf("Compute(sonnet-4-6, all token types) = %.6f; want %.6f (5.40)", got, want)
	}
}

// TestCompute_CacheCreateSplit_NotDoubleCounted: when CacheCreate5m and
// CacheCreate1h are provided, CacheCreate == CacheCreate5m + CacheCreate1h
// (they are a breakdown of the same total, not additive).
// Only CacheCreate (the total) must be used in the cost formula.
// Model=opus-4-7, CacheCreate=10_000, CacheCreate5m=4_000, CacheCreate1h=6_000.
// Manual: 10_000 × $18.75/M = 10_000/1_000_000 × 18.75 = 0.0001875 × 10 = $0.1875
func TestCompute_CacheCreateSplit_NotDoubleCounted(t *testing.T) {
	turn := parser.Turn{
		Model: "opus-4-7",
		Tokens: parser.TokenCounts{
			CacheCreate:   10_000,
			CacheCreate5m: 4_000,
			CacheCreate1h: 6_000,
		},
	}
	got := cost.Compute(turn)
	// If split fields were summed (4K+6K+10K=20K) the cost would double:
	// 20_000 × 18.75/M = $0.375. Only the total (10K) must be used → $0.1875.
	want := 0.1875
	if !approxEqual(got, want) {
		t.Errorf("Compute(opus-4-7, CacheCreate=10K, split 4K+6K) = %.6f; want %.6f (split must not be double-counted)", got, want)
	}
}

// TestCompute_EmptyTurn_ReturnsZero: Turn{} has empty Model string → unknown
// model → must return 0.
func TestCompute_EmptyTurn_ReturnsZero(t *testing.T) {
	got := cost.Compute(parser.Turn{})
	if got != 0.0 {
		t.Errorf("Compute(Turn{}) = %.6f; want 0.0", got)
	}
}

// ---------------------------------------------------------------------------
// ComputeAggregate
// ---------------------------------------------------------------------------

// TestComputeAggregate_MultipleTurns: sum of Compute over 3 turns.
// Turn1: opus-4-7,   Input=1_000_000         → $15.00
// Turn2: sonnet-4-6, Output=100_000           → $1.50
// Turn3: haiku-4-5,  Input=1_000_000          → $1.00
//
// Manual: haiku-4-5 Input pricing = $1.00/M → 1_000_000 × 1.00/M = $1.00
// Total: 15.00 + 1.50 + 1.00 = $17.50
func TestComputeAggregate_MultipleTurns(t *testing.T) {
	turns := []parser.Turn{
		{Model: "opus-4-7", Tokens: parser.TokenCounts{Input: 1_000_000}},
		{Model: "sonnet-4-6", Tokens: parser.TokenCounts{Output: 100_000}},
		{Model: "haiku-4-5", Tokens: parser.TokenCounts{Input: 1_000_000}},
	}
	got := cost.ComputeAggregate(turns)
	want := 17.50
	if !approxEqual(got, want) {
		t.Errorf("ComputeAggregate(3 turns) = %.6f; want %.6f", got, want)
	}
}

// TestComputeAggregate_Empty: nil slice must return 0.0 without panic.
func TestComputeAggregate_Empty(t *testing.T) {
	got := cost.ComputeAggregate(nil)
	if got != 0.0 {
		t.Errorf("ComputeAggregate(nil) = %.6f; want 0.0", got)
	}
}

// ---------------------------------------------------------------------------
// Format — already implemented in foundation; all four tests are PASS
// ---------------------------------------------------------------------------

func TestFormat_Cents(t *testing.T) {
	got := cost.Format(0.57)
	want := "$0.57"
	if got != want {
		t.Errorf("Format(0.57) = %q; want %q", got, want)
	}
}

func TestFormat_Dollars(t *testing.T) {
	got := cost.Format(13.79)
	want := "$13.79"
	if got != want {
		t.Errorf("Format(13.79) = %q; want %q", got, want)
	}
}

func TestFormat_ZeroDollars(t *testing.T) {
	got := cost.Format(0)
	want := "$0.00"
	if got != want {
		t.Errorf("Format(0) = %q; want %q", got, want)
	}
}

func TestFormat_LargeAmount(t *testing.T) {
	got := cost.Format(1234.56)
	want := "$1234.56"
	if got != want {
		t.Errorf("Format(1234.56) = %q; want %q", got, want)
	}
}
