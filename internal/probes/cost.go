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
// (d.Stdin.Cost.TotalCostUSD), clipped to this session via the /clear baseline,
// not our table-driven estimate (Phase 7.46).
//
// Rationale: the header must show the single most authoritative figure, even at a
// small lag, rather than our reconstructed estimate (which carries a known ~10%
// reasoning-billing gap). But TotalCostUSD is cumulative across /clear, so we
// subtract st.BaselineCost — the ccTotal captured on the first Reconcile of this
// session_id — to show only the current session's spend (a /clear starts a new
// session_id with a fresh baseline). On a session begun from zero baseline=0, so
// this equals the raw total. The per-turn table keeps our estimate (labelled
// "~cost"); a small header↔table mismatch is expected. This does not reintroduce
// the 7.45 dancing-number bug — that came from redistributing the lagging ccTotal
// across turns, not from a monotonic session-clipped header.
//
// Budget signal: when c.CostBudgetUSD > 0 and the session cost has reached or
// exceeded it, the figure is painted bold_red (config-honesty, Phase 7.47).
// A budget of 0 (the default) disables the check and the figure stays plain.
//
//	Full:              "cost: $<value>"
//	Compact/Minimal:   "$<value>"
func (p *CostProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	cost := officialSessionCost(d)
	value := fmt.Sprintf("$%.2f", cost)
	if c.CostBudgetUSD > 0 && cost >= c.CostBudgetUSD && t.AnsiEnabled {
		value = "{{color:bold_red}}" + value + "{{reset}}"
	}
	if level == LevelFull {
		return "cost: " + value
	}
	return value
}

// officialSessionCost is the official CC meter clipped to this session: the raw
// cumulative TotalCostUSD minus the per-session baseline (ccTotal at the first
// Reconcile of this session_id). A /clear creates a new session_id whose fresh
// baseline subtracts everything spent before it. Falls back to the raw total when
// no state is available (baseline 0). Never negative.
func officialSessionCost(d Data) float64 {
	c := d.Stdin.Cost.TotalCostUSD
	if d.State != nil {
		c -= d.State.BaselineCost
	}
	if c < 0 {
		c = 0
	}
	return c
}
