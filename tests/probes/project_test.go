// Package probes_test — black-box tests for ProjectProbe.
// Covers visibility (empty Cwd), short-name rendering (no truncation),
// and middle-truncation for Minimal level.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestProject_Visible_EmptyCwd verifies ProjectProbe visibility rules.
// Per §4.1.a: source is basename(data.Stdin.Cwd); empty Cwd → "?".
// The probe is always Visible (it falls back to "?"), consistent with the
// concept statement "if empty — '?'" with no mention of hiding.
func TestProject_Visible_EmptyCwd(t *testing.T) {
	p := &probes.ProjectProbe{}
	cfg := cfgAllOn()

	tests := []struct {
		name string
		cwd  string
		want bool
	}{
		{"non-empty cwd", "/Users/abc/Projects/cc-probeline", true},
		// Empty Cwd → "?" but probe remains Visible (concept: "if empty — '?'").
		{"empty cwd", "", true},
		// Root "/" → basename is "" → "?"
		{"root cwd", "/", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Cwd: tc.cwd}}
			got := p.Visible(d, cfg)
			if got != tc.want {
				t.Errorf("Visible(cwd=%q): want %v, got %v", tc.cwd, tc.want, got)
			}
		})
	}
}

// TestProject_Render_EmptyCwd verifies that an empty Cwd renders as "?".
func TestProject_Render_EmptyCwd(t *testing.T) {
	p := &probes.ProjectProbe{}
	th := renderer.Theme{}
	d := probes.Data{Stdin: stdin.Payload{Cwd: ""}}

	for _, lvl := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		t.Run(lvl.String(), func(t *testing.T) {
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, lvl)
			if got != "?" {
				t.Errorf("Render(empty cwd, %v): want %q, got %q", lvl, "?", got)
			}
		})
	}
}

// TestProject_Render_Levels verifies rendering across all three levels.
//
// Short name (≤ 8 chars): no truncation at any level.
// Long name: Full/Compact return the full basename; Minimal applies
// middle-truncate to min 8 chars.
func TestProject_Render_Levels(t *testing.T) {
	p := &probes.ProjectProbe{}
	th := renderer.Theme{}

	tests := []struct {
		name  string
		cwd   string
		level probes.Level
		want  string
	}{
		// Short project name — identical across all levels.
		{"short full", "/home/user/foo", probes.LevelFull, "foo"},
		{"short compact", "/home/user/foo", probes.LevelCompact, "foo"},
		{"short minimal", "/home/user/foo", probes.LevelMinimal, "foo"},

		// Long project name — Full/Compact return full basename.
		{"long full", "/Users/abc/Projects/cc-probeline", probes.LevelFull, "cc-probeline"},
		{"long compact", "/Users/abc/Projects/cc-probeline", probes.LevelCompact, "cc-probeline"},

		// Long project name at Minimal — middle-truncate to min 8 chars.
		// "cc-probeline" (12 chars) → middle-truncate → "cc-pro…ne" (9 chars with …)
		{"long minimal", "/Users/abc/Projects/cc-probeline", probes.LevelMinimal, "cc-pro…ne"},

		// Very long name at Minimal — middle-truncate result must be ≥ 8 chars + ellipsis.
		// "my-super-long-project-name" → first 4 + … + last 3 = "my-s…ame" (8 chars with …)
		{"very long minimal", "/home/user/my-super-long-project-name", probes.LevelMinimal, "my-s…ame"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Cwd: tc.cwd}}
			cfg := probes.Config{}
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(cwd=%q, %v): want %q, got %q", tc.cwd, tc.level, tc.want, got)
			}
		})
	}
}
