// Package probes_test — black-box tests for CtxProbe.
// Covers visibility (Size=0 → hidden), rendering across Full/Compact/Minimal
// levels with a representative usage map, and different percent values.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCtx_Visible_Empty verifies that CtxProbe.Visible returns false when
// ContextWindow.Size is zero (no context-window info in stdin).
func TestCtx_Visible_Empty(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{Size: 0},
	}}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(Size=0): want false, got true")
	}
}

// TestCtx_Render_AllLevels verifies all three display levels for a representative
// context window state.
//
// Setup:
//
//	Size = 200000
//	used = cache_read_input_tokens (128000) + input_tokens (0) + cache_creation_input_tokens (0)
//	     = 128000
//	percent = 128000 / 200000 * 100 = 64%
//	bar at 64% → round(64/10)*10 = round(6.4)*10 = 60 → "███░░"   (§4.1.b granularity)
//
// Expected per level (§4.1.b concept):
//
//	Full:    "ctx ███░░ 128K/200K (64%)"
//	Compact: "███░░ 128K/200K"
//	Minimal: "128K/200K"
func TestCtx_Render_AllLevels(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{
			Size: 200000,
			CurrentUsage: map[string]int{
				"cache_read_input_tokens":       128000,
				"input_tokens":                  0,
				"cache_creation_input_tokens":   0,
				"output_tokens":                 0,
			},
		},
	}}

	tests := []struct {
		name  string
		level probes.Level
		want  string
	}{
		{"full", probes.LevelFull, "ctx ███░░ 128K/200K (64%)"},
		{"compact", probes.LevelCompact, "███░░ 128K/200K"},
		{"minimal", probes.LevelMinimal, "128K/200K"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(%v): want %q, got %q", tc.level, tc.want, got)
			}
		})
	}
}

// TestCtx_Render_Percent verifies percent calculation and bar rendering at
// two additional percentage values (15% and 95%) to ensure the bar segment
// selection and label are consistent.
//
// Case 1: 15% usage
//
//	Size=200000, used=30000 → 15% → round to 20 → "█░░░░"
//	Full: "ctx █░░░░ 30K/200K (15%)"
//
// Case 2: 95% usage
//
//	Size=200000, used=190000 → 95% → round to 100 → "█████"
//	Full: "ctx █████ 190K/200K (95%)"
func TestCtx_Render_Percent(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	tests := []struct {
		name  string
		size  int
		used  int
		level probes.Level
		want  string
	}{
		{
			name:  "15pct full",
			size:  200000,
			used:  30000,
			level: probes.LevelFull,
			want:  "ctx █░░░░ 30K/200K (15%)",
		},
		{
			name:  "15pct compact",
			size:  200000,
			used:  30000,
			level: probes.LevelCompact,
			want:  "█░░░░ 30K/200K",
		},
		{
			name:  "95pct full",
			size:  200000,
			used:  190000,
			level: probes.LevelFull,
			want:  "ctx █████ 190K/200K (95%)",
		},
		{
			name:  "95pct minimal",
			size:  200000,
			used:  190000,
			level: probes.LevelMinimal,
			want:  "190K/200K",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{
				ContextWindow: stdin.ContextWindow{
					Size: tc.size,
					CurrentUsage: map[string]int{
						"cache_read_input_tokens":     tc.used,
						"input_tokens":                0,
						"cache_creation_input_tokens": 0,
						"output_tokens":               0,
					},
				},
			}}
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(%v, used=%d, size=%d): want %q, got %q",
					tc.level, tc.used, tc.size, tc.want, got)
			}
		})
	}
}
