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
func (p *EmailProbe) Priority() int { return 2 }
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
	if email == "" {
		return ""
	}
	if level == LevelMinimal {
		return emailMiddleTruncate(email, 12)
	}
	return email
}

// emailMiddleTruncate applies middle truncation with "…" targeting minWidth runes.
// head = ceil((minWidth-1)/2), tail = floor((minWidth-1)/2).
// Result is exactly minWidth runes when len(s) > minWidth.
func emailMiddleTruncate(s string, minWidth int) string {
	runes := []rune(s)
	if len(runes) <= minWidth {
		return s
	}
	tail := (minWidth - 1) / 2  // floor
	head := minWidth - 1 - tail // ceil; head >= tail
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}
