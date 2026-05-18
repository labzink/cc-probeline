// Package probes_test contains black-box tests for internal/probes plain probes.
// This file covers ModelProbe: visibility, rendering across all three Levels,
// and minimum width contract.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestModel_Visible verifies that ModelProbe.Visible returns true when
// Stdin.Model.ID is non-empty and false when it is empty.
func TestModel_Visible(t *testing.T) {
	p := &probes.ModelProbe{}
	cfg := probes.Config{}

	tests := []struct {
		name  string
		id    string
		want  bool
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
			got := p.Render(d, th, tc.level)
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
