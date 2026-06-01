// Package probes_test contains black-box tests for internal/probes plain probes.
// This file covers ModelProbe: visibility, rendering across all three Levels,
// and minimum width contract.
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestModel_Visible verifies that ModelProbe.Visible returns true when
// Stdin.Model.ID is non-empty and false when it is empty.
func TestModel_Visible(t *testing.T) {
	p := &probes.ModelProbe{}
	cfg := cfgAllOn()

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"opus id set", "claude-opus-4-7-20250805", true},
		{"sonnet id set", "claude-sonnet-4-6", true},
		{"empty id", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Model: stdin.Model{ID: tc.id}}}
			got := p.Visible(d, cfg)
			if got != tc.want {
				t.Errorf("Visible(%q): want %v, got %v", tc.id, tc.want, got)
			}
		})
	}
}

// TestModel_Render_AllLevels verifies that ModelProbe.Render produces the
// canonical short model key for all three Levels (Full/Compact/Minimal are
// identical for P0 — model is never dropped).
func TestModel_Render_AllLevels(t *testing.T) {
	p := &probes.ModelProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name  string
		id    string
		level probes.Level
		want  string
	}{
		// claude-opus-4-7-20250805 → "opus-4-7"
		{"opus full", "claude-opus-4-7-20250805", probes.LevelFull, "opus-4-7"},
		{"opus compact", "claude-opus-4-7-20250805", probes.LevelCompact, "opus-4-7"},
		{"opus minimal", "claude-opus-4-7-20250805", probes.LevelMinimal, "opus-4-7"},
		// claude-sonnet-4-6 → "sonnet-4-6"
		{"sonnet full", "claude-sonnet-4-6", probes.LevelFull, "sonnet-4-6"},
		{"sonnet compact", "claude-sonnet-4-6", probes.LevelCompact, "sonnet-4-6"},
		{"sonnet minimal", "claude-sonnet-4-6", probes.LevelMinimal, "sonnet-4-6"},
		// claude-haiku-4-5 → "haiku-4-5"
		{"haiku full", "claude-haiku-4-5", probes.LevelFull, "haiku-4-5"},
		// no-prefix id → returned as-is
		{"no-prefix full", "opus-4-7", probes.LevelFull, "opus-4-7"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Model: stdin.Model{ID: tc.id}}}
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(%q, %v): want %q, got %q", tc.id, tc.level, tc.want, got)
			}
		})
	}
}

// TestModel_MinWidth verifies that ModelProbe.MinWidth returns at least 8,
// which is len("opus-4-7") — the shortest realistic canonical model name.
func TestModel_MinWidth(t *testing.T) {
	p := &probes.ModelProbe{}
	if got := p.MinWidth(); got < 8 {
		t.Errorf("MinWidth(): want >= 8, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// T-14: Model colour = effortColorMarker (Phase 6.9.c)
//
// Contract (spec-common.md §2.3):
//   ModelProbe.Render wraps the model name in effortColorMarker(effort.Level)
//   instead of {{bold}}.
//   - effort=high/xhigh/max → "{{color:magenta}}<name>{{reset}}"
//   - effort=low            → "{{dim}}<name>{{reset}}"
//   - effort=medium or none → plain "<name>" (no colour marker, no {{bold}})
// ---------------------------------------------------------------------------

// TestModel_ColourMatchesEffortHigh (T-14a) verifies that when effort.Level is
// "high"/"xhigh"/"max", the model name itself is prefixed by {{color:magenta}}
// (not just the effort glyph), and the legacy {{bold}} marker is absent.
func TestModel_ColourMatchesEffortHigh(t *testing.T) {
	p := &probes.ModelProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	efforts := []string{"high", "xhigh", "max"}
	for _, lvl := range efforts {
		t.Run("effort="+lvl, func(t *testing.T) {
			d := probes.Data{
				Stdin: stdin.Payload{
					Model:  stdin.Model{ID: "claude-sonnet-4-6"},
					Effort: stdin.Effort{Level: lvl},
				},
			}
			cfg := probes.Config{ModelEnabled: true}
			got := p.Render(d, cfg, th, probes.LevelFull)

			// The model name must be directly preceded by {{color:magenta}}.
			// Pattern: "{{color:magenta}}sonnet-4-6"
			if !strings.Contains(got, "{{color:magenta}}sonnet-4-6") {
				t.Errorf("T-14a: effort=%q: want model name wrapped as {{color:magenta}}sonnet-4-6 in output, got %q", lvl, got)
			}
			// Legacy {{bold}} must NOT appear — model colour now comes from effortColorMarker.
			if strings.Contains(got, "{{bold}}") {
				t.Errorf("T-14a: effort=%q: must NOT contain {{bold}}, got %q", lvl, got)
			}
		})
	}
}

// TestModel_ColourMatchesEffortLow (T-14b) verifies that when effort.Level is
// "low", the model name itself is directly preceded by {{dim}} (wrapping the
// model name, not just the effort glyph), and {{bold}} is absent.
func TestModel_ColourMatchesEffortLow(t *testing.T) {
	p := &probes.ModelProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	d := probes.Data{
		Stdin: stdin.Payload{
			Model:  stdin.Model{ID: "claude-opus-4-7-20250805"},
			Effort: stdin.Effort{Level: "low"},
		},
	}
	cfg := probes.Config{ModelEnabled: true}
	got := p.Render(d, cfg, th, probes.LevelFull)

	// The model name must be directly preceded by {{dim}}.
	// Pattern: "{{dim}}opus-4-7"
	if !strings.Contains(got, "{{dim}}opus-4-7") {
		t.Errorf("T-14b: effort=low: want model name wrapped as {{dim}}opus-4-7 in output, got %q", got)
	}
	// Legacy {{bold}} must NOT appear.
	if strings.Contains(got, "{{bold}}") {
		t.Errorf("T-14b: effort=low: must NOT contain {{bold}}, got %q", got)
	}
}

// TestModel_NoMarkerMedium (T-14c) verifies that for effort=medium or no effort,
// the model name is rendered plain (no colour marker, no {{bold}}).
func TestModel_NoMarkerMedium(t *testing.T) {
	p := &probes.ModelProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	tests := []struct {
		name  string
		level string
	}{
		{"effort=medium", "medium"},
		{"effort=empty", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{
				Stdin: stdin.Payload{
					Model:  stdin.Model{ID: "claude-sonnet-4-6"},
					Effort: stdin.Effort{Level: tc.level},
				},
			}
			cfg := probes.Config{ModelEnabled: true}
			got := p.Render(d, cfg, th, probes.LevelFull)

			// No colour markers expected for medium/no effort.
			if strings.Contains(got, "{{bold}}") {
				t.Errorf("T-14c: %s: must NOT contain {{bold}}, got %q", tc.name, got)
			}
			if strings.Contains(got, "{{color:magenta}}") {
				t.Errorf("T-14c: %s: must NOT contain {{color:magenta}}, got %q", tc.name, got)
			}
			if strings.Contains(got, "{{dim}}") {
				t.Errorf("T-14c: %s: must NOT contain {{dim}} on model name, got %q", tc.name, got)
			}
			if !strings.Contains(got, "sonnet-4-6") {
				t.Errorf("T-14c: %s: want model name in output, got %q", tc.name, got)
			}
		})
	}
}
