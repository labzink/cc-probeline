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

// Visible returns false when EffortEnabled is false or the effort level is "off" or empty.
func (p *EffortProbe) Visible(d Data, c Config) bool {
	if !c.EffortEnabled {
		return false
	}
	lvl := d.Stdin.Effort.Level
	return lvl != "" && lvl != "off"
}

// Render returns the Unicode icon for the effort level, colour-wrapped per
// B3 §5 (see effortGlyph). Returns "" for "off" or unrecognised levels.
func (p *EffortProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	return effortGlyph(d.Stdin.Effort.Level, t.AnsiEnabled)
}

// effortGlyph returns the effort icon for lvl, colour-wrapped per B3 §5 when
// ansiEnabled. It is the single source of truth for effort colouring, shared by
// EffortProbe.Render and ModelProbe.Render (which appends the glyph inline):
//
//	low            → {{dim}}…{{reset}}
//	medium         → no marker (default colour)
//	high/xhigh/max → {{color:magenta}}…{{reset}}
//
// Returns "" for "off" or unrecognised levels (caller drops the segment).
func effortGlyph(lvl string, ansiEnabled bool) string {
	icon, ok := effortIcon[lvl]
	if !ok || icon == "" {
		return ""
	}
	if !ansiEnabled {
		return icon
	}
	switch lvl {
	case "low":
		return "{{dim}}" + icon + "{{reset}}"
	case "high", "xhigh", "max":
		return "{{color:magenta}}" + icon + "{{reset}}"
	default:
		// medium and any future default levels — no colour marker.
		return icon
	}
}
