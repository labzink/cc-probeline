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
// on /clear together with the cost counter.
//
// The fallback to raw TotalAPIDurationMS keys on d.State == nil ("state not yet
// loaded") rather than on durMS <= 0. After /clear the state IS loaded and
// SessionDurMS is legitimately 0 (baseline == current) — falling back there would
// resurrect the stale cumulative total (bug: cost reset to $0.00 but time kept
// 238:17). Negative deltas (transient ccTotal dip) are clamped to 0.
//
//	Full:    "time: MM:SS"
//	Compact: "MM:SS"
//	Minimal: "MM:SS"
func (p *TimeProbe) Render(d Data, _ Config, _ renderer.Theme, level Level) string {
	durMS := d.SessionDurMS
	if d.State == nil {
		durMS = d.Stdin.Cost.TotalAPIDurationMS
	}
	if durMS < 0 {
		durMS = 0
	}
	mmss := formatMMSS(durMS)
	if level == LevelFull {
		return "time: " + mmss
	}
	return mmss
}
