// Package probes_test — black-box tests for CostProbe.
// Covers visibility, and rendering across Full/Compact/Minimal levels with
// various cost values (zero, typical, large).
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCost_Visible verifies that CostProbe.Visible always returns true
// (cost block is always shown, even at $0.00).
func TestCost_Visible(t *testing.T) {
	p := &probes.CostProbe{}
	cfg := cfgAllOn()

	tests := []struct {
		name string
		cost float64
		want bool
	}{
		{"zero cost", 0.00, true},
		{"non-zero cost", 13.79, true},
		{"large cost", 1234.56, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: tc.cost}}}
			got := p.Visible(d, cfg)
			if got != tc.want {
				t.Errorf("Visible(cost=%v): want %v, got %v", tc.cost, tc.want, got)
			}
		})
	}
}

// TestCost_Render_Full verifies CostProbe.Render at LevelFull:
// format is "cost: $<value>" with 2 decimal places.
// CostProbe reads the official d.Stdin.Cost.TotalCostUSD (Phase 7.46).
func TestCost_Render_Full(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0.00, "cost: $0.00"},
		{"typical", 13.79, "cost: $13.79"},
		{"large", 1234.56, "cost: $1234.56"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Phase 7.46: header shows the official d.Stdin.Cost.TotalCostUSD.
			d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: tc.cost}}}
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, probes.LevelFull)
			if got != tc.want {
				t.Errorf("Render(Full, cost=%v): want %q, got %q", tc.cost, tc.want, got)
			}
		})
	}
}

// TestCost_Render_Compact verifies CostProbe.Render at LevelCompact:
// format is "$<value>" (label dropped per §A4 P1).
// CostProbe reads the official d.Stdin.Cost.TotalCostUSD (Phase 7.46).
func TestCost_Render_Compact(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0.00, "$0.00"},
		{"typical", 13.79, "$13.79"},
		{"large", 1234.56, "$1234.56"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Phase 7.46: header shows the official d.Stdin.Cost.TotalCostUSD.
			d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: tc.cost}}}
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, probes.LevelCompact)
			if got != tc.want {
				t.Errorf("Render(Compact, cost=%v): want %q, got %q", tc.cost, tc.want, got)
			}
		})
	}
}

// TestCost_Render_Minimal verifies CostProbe.Render at LevelMinimal:
// format is "$<value>" — same as Compact (value is never dropped; cost is P0).
// CostProbe reads the official d.Stdin.Cost.TotalCostUSD (Phase 7.46).
func TestCost_Render_Minimal(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0.00, "$0.00"},
		{"typical", 13.79, "$13.79"},
		{"large", 1234.56, "$1234.56"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Phase 7.46: header shows the official d.Stdin.Cost.TotalCostUSD.
			d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: tc.cost}}}
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, probes.LevelMinimal)
			if got != tc.want {
				t.Errorf("Render(Minimal, cost=%v): want %q, got %q", tc.cost, tc.want, got)
			}
		})
	}
}
