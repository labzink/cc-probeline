// Package probes_test — black-box tests for EmailProbe.
// Covers visibility (disabled config, empty email), and rendering across all
// three Levels with both short and long email addresses.
//
// EmailProbe is zero-state: Config is passed per-call to Visible and Render.
// Construction is simply &probes.EmailProbe{}.
package probes_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestEmail_Visible_Disabled verifies that EmailProbe.Visible returns false
// when Config.EmailEnabled is false, regardless of the Email value.
func TestEmail_Visible_Disabled(t *testing.T) {
	d := probes.Data{Stdin: stdin.Payload{}}
	p := &probes.EmailProbe{}

	tests := []struct {
		name string
		cfg  probes.Config
		want bool
	}{
		{"disabled empty email", probes.Config{EmailEnabled: false, Email: ""}, false},
		{"disabled non-empty email", probes.Config{EmailEnabled: false, Email: "labzin.k@gmail.com"}, false},
		{"enabled non-empty email", probes.Config{EmailEnabled: true, Email: "labzin.k@gmail.com"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Visible(d, tc.cfg)
			if got != tc.want {
				t.Errorf("Visible(enabled=%v, email=%q): want %v, got %v",
					tc.cfg.EmailEnabled, tc.cfg.Email, tc.want, got)
			}
		})
	}
}

// TestEmail_Visible_EmptyEmail verifies that EmailProbe.Visible returns false
// when EmailEnabled is true but Email string is empty.
func TestEmail_Visible_EmptyEmail(t *testing.T) {
	d := probes.Data{Stdin: stdin.Payload{}}
	cfg := probes.Config{EmailEnabled: true, Email: ""}
	p := &probes.EmailProbe{}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(enabled=true, email=%q): want false, got true", "")
	}
}

// TestEmail_Render_Levels verifies rendering across all three display Levels.
//
// Short email (≤ 12 chars): no truncation at any level.
// Phase 6.6: Compact applies middle-truncation to 16 runes; Minimal to 12 runes.
func TestEmail_Render_Levels(t *testing.T) {
	th := renderer.Theme{}
	d := probes.Data{Stdin: stdin.Payload{}}
	p := &probes.EmailProbe{}

	tests := []struct {
		name  string
		email string
		level probes.Level
		want  string
	}{
		// Short email — no truncation at any level (≤ 12 runes).
		{"short full", "x@y.io", probes.LevelFull, "x@y.io"},
		{"short compact", "x@y.io", probes.LevelCompact, "x@y.io"},
		{"short minimal", "x@y.io", probes.LevelMinimal, "x@y.io"},

		// Typical email (18 chars) — Full unchanged.
		{"typical full", "labzin.k@gmail.com", probes.LevelFull, "labzin.k@gmail.com"},

		// Typical email at Compact (Phase 6.6) — middle-truncate to 16.
		// "labzin.k@gmail.com" (18) → regime 1: half=9 < 15; head=9, tail=7 → "labzin.k@…ail.com" (17 runes)
		{"typical compact", "labzin.k@gmail.com", probes.LevelCompact, "labzin.k@…ail.com"},

		// Typical email at Minimal — middle-truncate to min 12 visible chars.
		// "labzin.k@gmail.com" (18) → regime 1: head=9, tail=3 → "labzin.k@…com" (13 chars)
		{"typical minimal", "labzin.k@gmail.com", probes.LevelMinimal, "labzin.k@…com"},

		// Very long email at Minimal — regime 2: head=6, tail=5 → "averyl…e.com" (12 chars)
		// "averylongemail@example.com" (26): half=13 >= minWidth-1=11 → regime 2
		{"long minimal", "averylongemail@example.com", probes.LevelMinimal, "averyl…e.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := probes.Config{EmailEnabled: true, Email: tc.email}
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(email=%q, %v): want %q, got %q",
					tc.email, tc.level, tc.want, got)
			}
		})
	}
}
