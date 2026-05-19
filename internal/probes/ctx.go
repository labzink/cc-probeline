package probes

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CtxProbe renders the context window usage as a 5-segment progress bar.
//
// Used tokens = sum of three input-side keys from CurrentUsage:
// cache_read_input_tokens + cache_creation_input_tokens + input_tokens.
// Percentage = used / Size * 100 (clamped to [0, 100]).
//
// Display:
//
//	Full:    "ctx <bar> <usedK>/<sizeK> (<pct>%)"
//	Compact: "<bar> <usedK>/<sizeK>"
//	Minimal: "<usedK>/<sizeK>"
type CtxProbe struct{}

func (p *CtxProbe) Name() string  { return "ctx" }
func (p *CtxProbe) Priority() int { return 1 }
func (p *CtxProbe) MinWidth() int { return len("128K/200K") }

// Visible returns false when ContextWindow.Size is zero.
func (p *CtxProbe) Visible(d Data, c Config) bool {
	return d.Stdin.ContextWindow.Size > 0
}

// Render formats the context window usage with a progress bar.
func (p *CtxProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	size := d.Stdin.ContextWindow.Size
	if size == 0 {
		return ""
	}

	// Sum only the three input-side keys per concept §4.1.b line 467.
	cu := d.Stdin.ContextWindow.CurrentUsage
	used := cu["cache_read_input_tokens"] + cu["cache_creation_input_tokens"] + cu["input_tokens"]

	pct := float64(used) / float64(size) * 100.0
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	usedK := formatK(used)
	sizeK := formatK(size)
	label := usedK + "/" + sizeK

	if level == LevelMinimal {
		return label
	}

	// Round pct to nearest 10 before passing to ProgressBar, so that the bar
	// reflects the semantic step (§4.1.b concept). ProgressBar itself floors
	// to the nearest 10 (for granularity stability), so we pre-round here to
	// avoid the floor-only bias at percentages like 15% or 95%.
	pctRounded := roundNearest10(pct)
	bar := renderer.ProgressBar(pctRounded)

	if level == LevelCompact {
		return bar + " " + label
	}

	// LevelFull: show bar + label + percentage (raw pct, not rounded).
	pctInt := int(pct)
	return fmt.Sprintf("ctx %s %s (%d%%)", bar, label, pctInt)
}

// roundNearest10 rounds v to the nearest multiple of 10 using standard rounding
// (0.5 rounds up). Used by CtxProbe before passing to renderer.ProgressBar.
func roundNearest10(v float64) float64 {
	r := int((v/10.0)+0.5) * 10
	if r < 0 {
		r = 0
	}
	if r > 100 {
		r = 100
	}
	return float64(r)
}
