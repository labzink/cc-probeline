package probes

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// TimeProbe renders the total API wall-clock duration in MM:SS format.
// Priority P3: dropped at Minimal level (returns empty string).
// Visible returns true even at zero duration.
//
// Note: Render uses the 3-param signature (d, t, level) to match the Phase 4.1.a
// test contract. The 4-param Config form will be added in Phase 4.1.a GREEN.
type TimeProbe struct{}

func (p *TimeProbe) Name() string  { return "time" }
func (p *TimeProbe) Priority() int { return 3 }
func (p *TimeProbe) MinWidth() int { return 0 } // dropped at Minimal

// Visible always returns true: probe is present but renders empty at Minimal.
func (p *TimeProbe) Visible(d Data, c Config) bool { return true }

// Render formats the elapsed time:
//
//	Full:    "time: MM:SS"
//	Compact: "MM:SS"
//	Minimal: "" (block dropped; renderer removes separator)
func (p *TimeProbe) Render(d Data, t renderer.Theme, level Level) string {
	if level == LevelMinimal {
		return ""
	}
	totalSec := d.Stdin.Cost.TotalAPIDurationMS / 1000
	mins := totalSec / 60
	secs := totalSec % 60
	mmss := fmt.Sprintf("%02d:%02d", mins, secs)
	if level == LevelFull {
		return "time: " + mmss
	}
	return mmss
}
