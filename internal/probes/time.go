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

// Render formats the elapsed time:
//
//	Full:    "time: MM:SS"
//	Compact: "MM:SS"
//	Minimal: "MM:SS"
func (p *TimeProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	mmss := formatMMSS(d.Stdin.Cost.TotalAPIDurationMS)
	if level == LevelFull {
		return "time: " + mmss
	}
	return mmss
}
