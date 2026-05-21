package cost

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/parser"
)

// Pricing holds per-million-token USD rates for one model.
type Pricing struct {
	Input, Output, CacheRead, CacheCreate float64
}

// modelPricing is the canonical pricing table (USD per million tokens).
// Ported from PLAN constants (scripts/session_stats.py has no pricing table —
// see project_cost_methodology memory for delta methodology).
// Update here when Anthropic changes public pricing.
var modelPricing = map[string]Pricing{
	"opus-4-7":   {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheCreate: 18.75},
	"sonnet-4-6": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheCreate: 3.75},
	"haiku-4-5":  {Input: 1.00, Output: 5.00, CacheRead: 0.10, CacheCreate: 1.25},
}

const perMillion = 1_000_000.0

// Compute returns the approximate USD cost for a single turn using the
// per-model pricing table. Unknown models return 0 (graceful degradation —
// see project_cost_methodology memory).
// Only TokenCounts.CacheCreate (the total) is used; CacheCreate5m/1h are a
// breakdown of that total and must not be summed again.
func Compute(t parser.Turn) float64 {
	p, ok := modelPricing[t.Model]
	if !ok {
		return 0
	}
	return (float64(t.Tokens.Input)*p.Input +
		float64(t.Tokens.Output)*p.Output +
		float64(t.Tokens.CacheRead)*p.CacheRead +
		float64(t.Tokens.CacheCreate)*p.CacheCreate) / perMillion
}

// ComputeAggregate sums Compute over all turns. Returns 0 for nil/empty slice.
func ComputeAggregate(turns []parser.Turn) float64 {
	var sum float64
	for _, t := range turns {
		sum += Compute(t)
	}
	return sum
}

// Format renders a USD amount as "$X.XX". Used by the renderer footer.
func Format(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}
