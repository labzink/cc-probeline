package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// TimeProbe renders the total API wall-clock duration in MM:SS format.
// Priority P0: inviolable group, dropped at Minimal level (returns empty string).
// Visible returns true even at zero duration.
type TimeProbe struct{}

func (p *TimeProbe) Name() string  { return "time" }
func (p *TimeProbe) Priority() int { return 0 }
func (p *TimeProbe) MinWidth() int { return 0 } // dropped at Minimal

// Visible always returns true: probe is present but renders empty at Minimal.
func (p *TimeProbe) Visible(d Data, c Config) bool { return true }

// Render formats the elapsed time:
//
//	Full:    "time: MM:SS"
//	Compact: "MM:SS"
//	Minimal: "" (block dropped; renderer removes separator)
func (p *TimeProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	if level == LevelMinimal {
		return ""
	}
	mmss := formatMMSS(d.Stdin.Cost.TotalAPIDurationMS)
	if level == LevelFull {
		return "time: " + mmss
	}
	return mmss
}
