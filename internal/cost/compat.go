// Package cost — backward-compatibility shims for the old pricing-table API.
//
// Compute and ComputeAggregate are retained so that internal/renderer/table.go
// (Phase 6.8.d scope) compiles without modification.
// Phase 6.8.d will replace the table renderer with per-turn delta costs from
// state.Session.PerTurnCost and these functions will be removed.
package cost

import "github.com/labzink/cc-probeline/internal/parser"

// pricing holds per-million-token USD rates for one model.
// Used only by the legacy Compute shim (table renderer compat until Phase 6.8.d).
type pricing struct {
	Input, Output, CacheRead, CacheCreate float64
}

// legacyPricing is the retained pricing table for backward compatibility.
// Renamed from the old public symbol (removed per T-9 / delta methodology);
// this private table serves only the renderer compat shim (Phase 6.8.d will delete it).
var legacyPricing = map[string]pricing{
	"opus-4-8":   {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheCreate: 18.75},
	"opus-4-7":   {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheCreate: 18.75},
	"sonnet-4-6": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheCreate: 3.75},
	"haiku-4-5":  {Input: 1.00, Output: 5.00, CacheRead: 0.10, CacheCreate: 1.25},
}

const perMillion = 1_000_000.0

// Compute returns the approximate USD cost for a single turn using the
// legacy per-model pricing table. Unknown models return 0.
//
// Deprecated: replaced by Reconcile/PerTurn (Phase 6.8.a). Kept for
// renderer/table.go compatibility until Phase 6.8.d.
func Compute(t parser.Turn) float64 {
	p, ok := legacyPricing[t.Model]
	if !ok {
		return 0
	}
	return (float64(t.Tokens.Input)*p.Input +
		float64(t.Tokens.Output)*p.Output +
		float64(t.Tokens.CacheRead)*p.CacheRead +
		float64(t.Tokens.CacheCreate)*p.CacheCreate) / perMillion
}

// ComputeAggregate sums Compute over all turns.
//
// Deprecated: replaced by SessionTotal (Phase 6.8.a). Kept for
// renderer/table.go compatibility until Phase 6.8.d.
func ComputeAggregate(turns []parser.Turn) float64 {
	var sum float64
	for _, t := range turns {
		sum += Compute(t)
	}
	return sum
}
