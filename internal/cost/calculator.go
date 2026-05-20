package cost

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/parser"
)

// Compute returns the approximate USD cost for a single turn using a
// per-model pricing table. Phase 4.4.c ports the table from
// scripts/session_stats.py and fills the lookup.
//
// Phase 4.4.0 foundation: stub returns 0. Unknown models silently stay at 0
// — see project_cost_methodology memory for the long-term plan.
func Compute(t parser.Turn) float64 {
	_ = t
	return 0
}

// ComputeAggregate sums Compute over turns.
//
// Phase 4.4.0 foundation: stub returns 0.
func ComputeAggregate(turns []parser.Turn) float64 {
	_ = turns
	return 0
}

// Format renders a USD amount as "$X.XX". Used by the box-drawing table
// footer (Phase 4.2 costFor / costForAgg replacement in 4.4.c).
func Format(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}
