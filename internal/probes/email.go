package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// EmailProbe renders the user's email address.
// It is constructed with a Config so that Render can read cfg.Email without
// a Config parameter (3-param Render to match Phase 4.1.a test contract).
//
// Visible returns false when EmailEnabled=false or Email is empty.
//
// Note: Render uses the 3-param signature (d, t, level) to match the Phase 4.1.a
// test contract. The 4-param Config form will be added in Phase 4.1.a GREEN.
type EmailProbe struct {
	cfg Config
}

// NewEmailProbe constructs an EmailProbe that will read the email from cfg.
func NewEmailProbe(cfg Config) *EmailProbe {
	return &EmailProbe{cfg: cfg}
}

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
func (p *EmailProbe) Render(d Data, t renderer.Theme, level Level) string {
	email := p.cfg.Email
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
