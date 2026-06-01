package probes

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CostProbe renders the total session cost in USD.
// Always visible (even at $0.00). Priority P2.
type CostProbe struct{}

func (p *CostProbe) Name() string  { return "cost" }
func (p *CostProbe) Priority() int { return 1 }
func (p *CostProbe) MinWidth() int { return len("$0.00") }

// Visible returns false when CostEnabled is false; otherwise always true (even at $0.00).
func (p *CostProbe) Visible(d Data, c Config) bool {
	if !c.CostEnabled {
		return false
	}
	return true
}

// Render formats the cost using d.SessionTotal (delta from session baseline),
// not the raw cumulative d.Stdin.Cost.TotalCostUSD (C2 / T-6).
//
//	Full:              "cost: $<value>"
//	Compact/Minimal:   "$<value>"
func (p *CostProbe) Render(d Data, _ Config, _ renderer.Theme, level Level) string {
	// C2: use session-delta cost, not raw cumulative total.
	value := fmt.Sprintf("$%.2f", d.SessionTotal)
	if level == LevelFull {
		return "cost: " + value
	}
	return value
}
