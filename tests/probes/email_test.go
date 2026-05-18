// Package probes_test — black-box tests for EmailProbe.
// Covers visibility (disabled config, empty email), and rendering across all
// three Levels with both short and long email addresses.
//
// Design note: EmailProbe.Render(d Data, t Theme, level Level) does not accept
// Config directly (Probe interface constraint). The email value must therefore
// be stored inside EmailProbe at construction time (EmailProbe{Cfg: cfg} or
// similar). Tests assume EmailProbe embeds or stores Config so that Render can
// read cfg.Email. If dev chooses a different approach, assertions stand as the
// behavioural contract; the construction syntax may need adjustment.
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
			p := probes.NewEmailProbe(tc.cfg)
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
	p := probes.NewEmailProbe(cfg)

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(enabled=true, email=%q): want false, got true", "")
	}
}

// TestEmail_Render_Levels verifies rendering across all three display Levels.
//
// Short email (≤ 12 chars): no truncation at any level.
// Long email: Full/Compact return the full value; Minimal applies
// middle-truncation to min 12 chars (result = 12 visible chars including "…").
func TestEmail_Render_Levels(t *testing.T) {
	th := renderer.Theme{}
	d := probes.Data{Stdin: stdin.Payload{}}

	tests := []struct {
		name  string
		email string
		level probes.Level
		want  string
	}{
		// Short email — no truncation at any level.
		{"short full", "x@y.io", probes.LevelFull, "x@y.io"},
		{"short compact", "x@y.io", probes.LevelCompact, "x@y.io"},
		{"short minimal", "x@y.io", probes.LevelMinimal, "x@y.io"},

		// Typical email (18 chars) — Full/Compact unchanged.
		{"typical full", "labzin.k@gmail.com", probes.LevelFull, "labzin.k@gmail.com"},
		{"typical compact", "labzin.k@gmail.com", probes.LevelCompact, "labzin.k@gmail.com"},

		// Typical email at Minimal — middle-truncate to min 12 visible chars.
		// "labzin.k@gmail.com" (18) → first 6 + "…" + last 5 = "labzin…l.com" (12 chars)
		{"typical minimal", "labzin.k@gmail.com", probes.LevelMinimal, "labzin…l.com"},

		// Very long email at Minimal — same rule: ≥ 12 chars with "…".
		// "averylongemail@example.com" (26) → first 6 + "…" + last 5 = "averyl…e.com"
		{"long minimal", "averylongemail@example.com", probes.LevelMinimal, "averyl…e.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := probes.Config{EmailEnabled: true, Email: tc.email}
			p := probes.NewEmailProbe(cfg)
			got := p.Render(d, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(email=%q, %v): want %q, got %q",
					tc.email, tc.level, tc.want, got)
			}
		})
	}
}
