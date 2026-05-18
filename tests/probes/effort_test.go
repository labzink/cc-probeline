// Package probes_test — black-box tests for EffortProbe.
// Covers visibility (off level → hidden), icon rendering across all 6 effort
// levels × 3 display levels.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestEffort_Visible_Off verifies that EffortProbe.Visible returns false when
// effort level is "off" (or empty), and true for any active effort level.
func TestEffort_Visible_Off(t *testing.T) {
	p := &probes.EffortProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	tests := []struct {
		name  string
		level string
		want  bool
	}{
		{"off explicit", "off", false},
		{"empty string", "", false},
		{"low", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"xhigh", "xhigh", true},
		{"max", "max", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: tc.level}}}
			got := p.Visible(d, cfg)
			if got != tc.want {
				t.Errorf("Visible(%q): want %v, got %v", tc.level, tc.want, got)
			}
			// When off, Render must also return empty string (renderer drops the separator).
			if !tc.want {
				rendered := p.Render(d, cfg, th, probes.LevelFull)
				if rendered != "" {
					t.Errorf("Render(%q, Full): want %q, got %q", tc.level, "", rendered)
				}
			}
		})
	}
}

// TestEffort_Render_Icons verifies the exact Unicode icon for each effort level
// across all three display Levels (icons are identical — effort is P0).
//
// Icon mapping (from §4.1.a concept):
//
//	low=○  medium=◔  high=◑  xhigh=◕  max=●  off=""
func TestEffort_Render_Icons(t *testing.T) {
	p := &probes.EffortProbe{}
	th := renderer.Theme{}

	tests := []struct {
		effortLevel string
		displayLvl  probes.Level
		want        string
	}{
		// low
		{"low", probes.LevelFull, "○"},
		{"low", probes.LevelCompact, "○"},
		{"low", probes.LevelMinimal, "○"},
		// medium
		{"medium", probes.LevelFull, "◔"},
		{"medium", probes.LevelCompact, "◔"},
		{"medium", probes.LevelMinimal, "◔"},
		// high
		{"high", probes.LevelFull, "◑"},
		{"high", probes.LevelCompact, "◑"},
		{"high", probes.LevelMinimal, "◑"},
		// xhigh
		{"xhigh", probes.LevelFull, "◕"},
		{"xhigh", probes.LevelCompact, "◕"},
		{"xhigh", probes.LevelMinimal, "◕"},
		// max
		{"max", probes.LevelFull, "●"},
		{"max", probes.LevelCompact, "●"},
		{"max", probes.LevelMinimal, "●"},
		// off — already tested in TestEffort_Visible_Off; included here for completeness
		{"off", probes.LevelFull, ""},
		{"off", probes.LevelCompact, ""},
		{"off", probes.LevelMinimal, ""},
	}

	for _, tc := range tests {
		name := tc.effortLevel + "/" + tc.displayLvl.String()
		t.Run(name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: tc.effortLevel}}}
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, tc.displayLvl)
			if got != tc.want {
				t.Errorf("Render(effort=%q, level=%v): want %q, got %q",
					tc.effortLevel, tc.displayLvl, tc.want, got)
			}
		})
	}
}
