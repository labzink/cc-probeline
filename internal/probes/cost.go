package probes

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CostProbe renders the total session cost in USD.
// Always visible (even at $0.00). Priority P2.
type CostProbe struct{}

func (p *CostProbe) Name() string  { return "cost" }
func (p *CostProbe) Priority() int { return 2 }
func (p *CostProbe) MinWidth() int { return len("$0.00") }

// Visible always returns true: cost block is shown even at $0.00.
func (p *CostProbe) Visible(d Data, c Config) bool { return true }

// Render formats the cost:
//
//	Full:              "cost: $<value>"
//	Compact/Minimal:   "$<value>"
func (p *CostProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	cost := d.Stdin.Cost.TotalCostUSD
	value := fmt.Sprintf("$%.2f", cost)
	if level == LevelFull {
		return "cost: " + value
	}
	return value
}
