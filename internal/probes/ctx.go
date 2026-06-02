package probes

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CtxProbe renders the context window usage as a progress bar.
//
// Used tokens = sum of three input-side keys from CurrentUsage:
// cache_read_input_tokens + cache_creation_input_tokens + input_tokens.
// Percentage = used / Size * 100 (clamped to [0, 100]).
//
// Display (AnsiEnabled=false / legacy):
//
//	Full:    "ctx <bar10> <usedK>/<sizeK> (<pct>%)"
//	Compact: "<bar5> <usedK>/<sizeK>"   (no %)
//	Minimal: "<usedK>/<sizeK>"           (no bar, no %)
//
// Display (AnsiEnabled=true / T-22):
//
//	Full:    "ctx <bar10> <coloured-usedK>/<sizeK>"  (no %; usedK colour by fill)
//	Compact: "<bar5> <usedK>/<sizeK>"               (unchanged)
//	Minimal: "<usedK>/<sizeK>"                       (no colour markers)
//
// Colour rules for usedK (AnsiEnabled=true, > 95% → bold_red; < 50% → green;
// otherwise marker from ProgressBarColor threshold):
//
//	> 95% → {{color:bold_red}}
//	< 50% → {{color:green}}
//	50–69% → {{color:yellow}}
//	70–89% → {{color:orange}}
//	≥ 90%  → {{color:red}}
type CtxProbe struct{}

func (p *CtxProbe) Name() string  { return "ctx" }
func (p *CtxProbe) Priority() int { return 1 }
func (p *CtxProbe) MinWidth() int { return len("128K/200K") }

// Visible returns false when CtxEnabled is false or ContextWindow.Size is zero.
func (p *CtxProbe) Visible(d Data, c Config) bool {
	if !c.CtxEnabled {
		return false
	}
	return d.Stdin.ContextWindow.Size > 0
}

// ctxNumberMarker returns the semantic colour marker token for the usedK number
// when AnsiEnabled=true. Uses bold_red above 95%, otherwise falls back to the
// ProgressBarColor threshold (green/yellow/orange/red).
func ctxNumberMarker(pct float64, t renderer.Theme) string {
	if !t.AnsiEnabled {
		return ""
	}
	if pct > 95 {
		return "{{color:bold_red}}"
	}
	// Reuse ProgressBarColor thresholds for the remaining bands.
	// ProgressBarColor returns ANSI codes, but we need marker tokens.
	// Map the same thresholds manually.
	switch {
	case pct < 50:
		return "{{color:green}}"
	case pct < 70:
		return "{{color:yellow}}"
	case pct < 90:
		return "{{color:orange}}"
	default:
		return "{{color:red}}"
	}
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

	if level == LevelMinimal {
		// Minimal: bare numbers, no colour markers (T-22).
		return usedK + "/" + sizeK
	}

	// colourReset is only emitted when AnsiEnabled is true.
	colourReset := ""
	if t.AnsiEnabled {
		colourReset = "{{reset}}"
	}

	if level == LevelCompact {
		// 5-segment bar; no percentage display.
		// Round to nearest 10 before passing to ProgressBar for visual stability.
		bar := renderer.ProgressBar(roundNearest10(pct))
		colorCode := renderer.ProgressBarColor(pct, t)
		// ProgressBarColor returns "" when AnsiEnabled=false → bar without colour.
		return colorCode + bar + colourReset + " " + usedK + "/" + sizeK
	}

	// LevelFull path.
	bar := renderer.ProgressBar10(pct)

	if t.AnsiEnabled {
		// T-15: colour ONLY the bar; usedK number is rendered plain (no marker).
		// ctxNumberMarker returns a {{color:X}} token for the bar colour band.
		marker := ctxNumberMarker(pct, t)
		return fmt.Sprintf("ctx %s%s{{reset}} %s/%s", marker, bar, usedK, sizeK)
	}

	// Legacy (AnsiEnabled=false): include percentage for existing tests.
	barColor := renderer.ProgressBarColor(pct, t) // returns "" when disabled
	pctInt := int(pct)
	return fmt.Sprintf("ctx %s%s%s %s/%s (%d%%)", barColor, bar, colourReset, usedK, sizeK, pctInt)
}

// roundNearest10 rounds v to the nearest multiple of 10 using standard rounding
// (0.5 rounds up). Kept for any callers outside this probe.
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
