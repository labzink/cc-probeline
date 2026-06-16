// Package probes_test — tests for the cost-budget colour signal (Phase 7.47
// config-honesty). CostProbe paints the session figure bold_red once the cost
// reaches or exceeds a positive CostBudgetUSD; a budget of 0 disables the check,
// and no colour tokens are emitted when ANSI is disabled.
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

func costData(cost float64) probes.Data {
	return probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: cost}}}
}

func TestCost_Budget_Signal(t *testing.T) {
	p := &probes.CostProbe{}
	ansi := renderer.Theme{AnsiEnabled: true}
	plain := renderer.Theme{}

	tests := []struct {
		name       string
		cost       float64
		budget     float64
		theme      renderer.Theme
		wantBold   bool
		wantTokens bool // whether any {{ token may appear
	}{
		{"budget 0 disables even at high cost", 100, 0, ansi, false, false},
		{"under budget is plain", 4.99, 5, ansi, false, false},
		{"at budget turns bold_red", 5.00, 5, ansi, true, true},
		{"over budget turns bold_red", 6.00, 5, ansi, true, true},
		{"over budget but no-color emits no tokens", 6.00, 5, plain, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := probes.Config{CostEnabled: true, CostBudgetUSD: tc.budget}
			got := p.Render(costData(tc.cost), cfg, tc.theme, probes.LevelFull)

			hasBold := strings.Contains(got, "{{color:bold_red}}")
			if hasBold != tc.wantBold {
				t.Errorf("%s: bold_red=%v, want %v (got %q)", tc.name, hasBold, tc.wantBold, got)
			}
			hasTokens := strings.Contains(got, "{{")
			if hasTokens != tc.wantTokens {
				t.Errorf("%s: tokens=%v, want %v (got %q)", tc.name, hasTokens, tc.wantTokens, got)
			}
		})
	}
}
