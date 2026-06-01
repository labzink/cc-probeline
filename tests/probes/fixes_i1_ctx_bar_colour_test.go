// Package probes_test — RED tests for Phase 6.8 FIXES: I1 ctx bar colour.
//
// Root cause (from review-consolidated.md I1):
//   The ANSI Full-path of CtxProbe.Render renders the progress bar WITHOUT colour.
//   Only the number (usedK) gets the colour marker.
//   Legacy path (AnsiEnabled=false) DID colour the bar via ProgressBarColor + bar.
//   The ANSI Full path (T-22 branch) lost ProgressBarColor on the bar.
//
// Fix vector: in the t.AnsiEnabled branch of Render (LevelFull), wrap the bar
// with ProgressBarColor(pct, t) + bar + {{reset}}, not just the number.
//
// RED: CtxProbe.Render Full with AnsiEnabled=true currently returns
//   "ctx <plain bar> <colour>usedK{{reset}}/sizeK"
// instead of
//   "ctx <colour><bar>{{reset}} <colour>usedK{{reset}}/sizeK"
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCtxProbe_BarColoured_AnsiEnabled (I1 / T-22 regression) verifies that
// CtxProbe.Render at LevelFull with AnsiEnabled=true applies a colour marker
// BEFORE the progress bar string, not only before the number.
//
// Setup:
//   - Size = 200000
//   - used = 150000 (cache_read_input_tokens), pct = 75%
//   - 75% → orange zone (70–89%)
//   - AnsiEnabled = true
//
// Expected:
//   - Output contains "{{color:orange}}" BEFORE the bar characters (█/░/▒).
//   - The colour marker is left of the bar in the string (posColour < posBar).
//
// RED: current code produces "ctx <plainbar> {{color:orange}}150K{{reset}}/200K"
// (colour marker only on the number, bar is plain).
func TestCtxProbe_BarColoured_AnsiEnabled(t *testing.T) {
	p := &probes.CtxProbe{}
	th := renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.DefaultPalette(),
	}
	cfg := probes.Config{CtxEnabled: true}

	// 75% → orange zone.
	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{
			Size: 200000,
			CurrentUsage: map[string]int{
				"cache_read_input_tokens":     150000, // 75%
				"input_tokens":                0,
				"cache_creation_input_tokens": 0,
			},
		},
	}}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// Colour marker must be present.
	const colourMarker = "{{color:orange}}"
	if !strings.Contains(got, colourMarker) {
		t.Errorf("I1 CtxProbe bar colour: want %q in Full ANSI output, got %q"+
			"\n  FIX: ProgressBarColor must be applied to the bar string, not only the number", colourMarker, got)
	}

	// The colour marker must appear BEFORE any bar character (█ ░ ▒).
	// This proves the bar is wrapped, not just the number after the bar.
	posColour := strings.Index(got, colourMarker)
	// First bar character position (find the earliest of the three).
	posBar := -1
	for _, ch := range []string{"█", "░", "▒"} {
		if idx := strings.Index(got, ch); idx >= 0 {
			if posBar < 0 || idx < posBar {
				posBar = idx
			}
		}
	}

	if posBar < 0 {
		t.Fatalf("I1 CtxProbe bar colour: no bar character (█/░/▒) found in output %q", got)
	}

	if posColour >= posBar {
		t.Errorf("I1 CtxProbe bar colour: colour marker must appear BEFORE bar characters;\n"+
			"  posColour=%d, posBar=%d, output=%q\n"+
			"  FIX: apply ProgressBarColor to the bar string in the AnsiEnabled Full path",
			posColour, posBar, got)
	}
}
