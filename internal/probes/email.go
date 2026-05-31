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
//	Full:    full email unchanged.
//	Compact: middle-truncate to 16 runes.
//	Minimal: middle-truncate to 12 runes.
type EmailProbe struct{}

func (p *EmailProbe) Name() string  { return "email" }
func (p *EmailProbe) Priority() int { return 2 }
func (p *EmailProbe) MinWidth() int { return 12 }

// Visible returns true only when EmailEnabled=true and Email is non-empty.
func (p *EmailProbe) Visible(d Data, c Config) bool {
	return c.EmailEnabled && c.Email != ""
}

// Render formats the email address. When AnsiEnabled, the value is wrapped in
// {{dim}}…{{reset}} per B3 §5.
//
//	Full:    full email (or {{dim}}full email{{reset}} with colour)
//	Compact: middle-truncate(16)
//	Minimal: middle-truncate(12)
func (p *EmailProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	email := c.Email
	var value string
	switch level {
	case LevelCompact:
		value = middleTruncate(email, 16)
	case LevelMinimal:
		value = middleTruncate(email, 12)
	default:
		value = email
	}
	if t.AnsiEnabled {
		return "{{dim}}" + value + "{{reset}}"
	}
	return value
}
