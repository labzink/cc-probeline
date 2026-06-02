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

// Render formats the elapsed time relative to the session baseline (phase 6.9.a).
// Uses d.SessionDurMS (TotalAPIDurationMS − BaselineDurMS) so the counter resets
// on /clear together with the cost counter. Falls back to raw TotalAPIDurationMS
// when SessionDurMS is zero (state not yet loaded).
//
//	Full:    "time: MM:SS"
//	Compact: "MM:SS"
//	Minimal: "MM:SS"
func (p *TimeProbe) Render(d Data, _ Config, _ renderer.Theme, level Level) string {
	durMS := d.SessionDurMS
	if durMS <= 0 {
		durMS = d.Stdin.Cost.TotalAPIDurationMS
	}
	mmss := formatMMSS(durMS)
	if level == LevelFull {
		return "time: " + mmss
	}
	return mmss
}
