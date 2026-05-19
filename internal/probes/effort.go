package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// effortIcon maps effort level strings to Unicode circle icons.
// "off" maps to the empty string (probe hidden via Visible).
var effortIcon = map[string]string{
	"low":    "○",
	"medium": "◔",
	"high":   "◑",
	"xhigh":  "◕",
	"max":    "●",
	"off":    "",
}

// EffortProbe renders the effort level as a Unicode circle icon.
// All three display levels render the same icon (effort is P0; icon is never dropped).
type EffortProbe struct{}

func (p *EffortProbe) Name() string  { return "effort" }
func (p *EffortProbe) Priority() int { return 0 }
func (p *EffortProbe) MinWidth() int { return 1 } // single rune icon

// Visible returns false when the effort level is "off" or empty.
func (p *EffortProbe) Visible(d Data, c Config) bool {
	lvl := d.Stdin.Effort.Level
	return lvl != "" && lvl != "off"
}

// Render returns the Unicode icon for the effort level.
// Returns "" for "off" or unrecognised levels (renderer drops the separator).
func (p *EffortProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	icon, ok := effortIcon[d.Stdin.Effort.Level]
	if !ok {
		return ""
	}
	return icon
}
