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

// Render formats the header cost from the official Claude Code meter
// (d.Stdin.Cost.TotalCostUSD), not our table-driven estimate (Phase 7.46).
//
// Rationale: the header must show the single most authoritative figure, even at a
// small lag, rather than our reconstructed estimate (which carries a known ~10%
// reasoning-billing gap). The per-turn table keeps our estimate (labelled "~cost")
// for attribution; a small header↔table mismatch is expected and accepted. This
// does not reintroduce the 7.45 dancing-number bug — that came from redistributing
// the lagging ccTotal across turns, not from showing the official total verbatim.
//
//	Full:              "cost: $<value>"
//	Compact/Minimal:   "$<value>"
func (p *CostProbe) Render(d Data, _ Config, _ renderer.Theme, level Level) string {
	value := fmt.Sprintf("$%.2f", d.Stdin.Cost.TotalCostUSD)
	if level == LevelFull {
		return "cost: " + value
	}
	return value
}
