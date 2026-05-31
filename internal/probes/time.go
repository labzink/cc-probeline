package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// TimeProbe renders the total API wall-clock duration in MM:SS format.
// Priority P1. Returns MM:SS at all levels including Minimal.
type TimeProbe struct{}

func (p *TimeProbe) Name() string  { return "time" }
func (p *TimeProbe) Priority() int { return 1 }
func (p *TimeProbe) MinWidth() int { return 5 } // length of "MM:SS"

// Visible returns false when TimeEnabled is false; otherwise always true.
func (p *TimeProbe) Visible(d Data, c Config) bool {
	if !c.TimeEnabled {
		return false
	}
	return true
}

// Render formats the elapsed time. When AnsiEnabled, the value is wrapped in
// {{dim}}…{{reset}} per B3 §5.
//
//	Full:    "time: MM:SS"  (or "time: {{dim}}MM:SS{{reset}}" with colour)
//	Compact: "MM:SS"        (or "{{dim}}MM:SS{{reset}}" with colour)
//	Minimal: "MM:SS"        (or "{{dim}}MM:SS{{reset}}" with colour)
func (p *TimeProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	raw := formatMMSS(d.Stdin.Cost.TotalAPIDurationMS)
	var mmss string
	if t.AnsiEnabled {
		mmss = "{{dim}}" + raw + "{{reset}}"
	} else {
		mmss = raw
	}
	if level == LevelFull {
		return "time: " + mmss
	}
	return mmss
}
