package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// EmailProbe renders the user's email address.
// Zero-state: all data comes from Config passed per-call to Visible and Render.
//
// Visible returns false when EmailEnabled=false or Email is empty.
//
// Display:
//
//	Full/Compact: full email unchanged.
//	Minimal:      middle-truncate to at least 12 visible runes.
type EmailProbe struct{}

func (p *EmailProbe) Name() string  { return "email" }
func (p *EmailProbe) Priority() int { return 1 }
func (p *EmailProbe) MinWidth() int { return 12 }

// Visible returns true only when EmailEnabled=true and Email is non-empty.
func (p *EmailProbe) Visible(d Data, c Config) bool {
	return c.EmailEnabled && c.Email != ""
}

// Render formats the email address:
//
//	Full/Compact: full email unchanged.
//	Minimal:      middle-truncate to at least 12 visible runes.
func (p *EmailProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	email := c.Email
	if level == LevelMinimal {
		return middleTruncate(email, 12)
	}
	return email
}
