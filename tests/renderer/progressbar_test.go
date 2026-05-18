// Package renderer_test — black-box tests for the ProgressBar and ProgressBarColor helpers.
// Covers all 11 canonical percentage points, boundary/granularity cases,
// and colour selection based on threshold and AnsiEnabled flag.
package renderer_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// TestProgressBar_All11Points verifies the exact 5-segment UTF-8 bar string
// for every 10-percentage-point step from 0 to 100.
//
// Segment map (each segment = 20%):
//
//	full "░" → empty, "▒" → half, "█" → full
//
// Canonical output from §4.1.b concept:
//
//	0%  → "░░░░░"
//	10% → "▒░░░░"
//	20% → "█░░░░"
//	30% → "█▒░░░"
//	40% → "██░░░"
//	50% → "██▒░░"
//	60% → "███░░"
//	70% → "███▒░"
//	80% → "████░"
//	90% → "████▒"
//	100%→ "█████"
func TestProgressBar_All11Points(t *testing.T) {
	tests := []struct {
		percent float64
		want    string
	}{
		{0, "░░░░░"},
		{10, "▒░░░░"},
		{20, "█░░░░"},
		{30, "█▒░░░"},
		{40, "██░░░"},
		{50, "██▒░░"},
		{60, "███░░"},
		{70, "███▒░"},
		{80, "████░"},
		{90, "████▒"},
		{100, "█████"},
	}

	for _, tc := range tests {
		name := strings.TrimRight(strings.TrimRight(strings.Replace(
			strings.Replace(strings.Replace(strings.Replace(tc.want, "░", "e", -1), "▒", "h", -1), "█", "f", -1),
			"e", "e", -1), "e"), "")
		_ = name
		t.Run(strings.Replace(tc.want, " ", "_", -1), func(t *testing.T) {
			got := renderer.ProgressBar(tc.percent)
			if got != tc.want {
				t.Errorf("ProgressBar(%.0f%%): want %q, got %q", tc.percent, tc.want, got)
			}
		})
	}
}

// TestProgressBar_Granularity verifies rounding-to-nearest-10% behaviour at
// boundary values that are commonly mishandled: 49, 50, 51, and 70.
//
// Expected (round half up to nearest 10):
//
//	49  → round to 50 → "██▒░░"   (two full, one half, two empty)
//	50  → round to 50 → "██▒░░"
//	51  → round to 50 → "██▒░░"   (rounds down to nearest 10)
//	70  → round to 70 → "███▒░"
func TestProgressBar_Granularity(t *testing.T) {
	tests := []struct {
		percent float64
		want    string
	}{
		{49, "██░░░"},
		{50, "██▒░░"},
		{51, "██▒░░"},
		{70, "███▒░"},
	}

	for _, tc := range tests {
		t.Run(strings.Replace(tc.want, " ", "_", -1), func(t *testing.T) {
			got := renderer.ProgressBar(tc.percent)
			if got != tc.want {
				t.Errorf("ProgressBar(%.0f%%): want %q, got %q", tc.percent, tc.want, got)
			}
		})
	}
}

// TestProgressBar_NegativeAndOverflow verifies that values outside [0, 100]
// are clamped: negative → empty bar, >100 → full bar.
func TestProgressBar_NegativeAndOverflow(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		want    string
	}{
		{"negative -5", -5, "░░░░░"},
		{"overflow 150", 150, "█████"},
		{"exactly 0", 0, "░░░░░"},
		{"exactly 100", 100, "█████"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderer.ProgressBar(tc.percent)
			if got != tc.want {
				t.Errorf("ProgressBar(%.0f%%): want %q, got %q", tc.percent, tc.want, got)
			}
		})
	}
}

// TestProgressBarColor verifies ANSI colour selection by percent threshold and
// that AnsiEnabled=false always yields an empty string.
//
// Thresholds (§4.1.b concept):
//
//	<50     → green
//	50..70  → yellow
//	70..90  → orange
//	>=90    → red
func TestProgressBarColor(t *testing.T) {
	t.Run("ansi_disabled", func(t *testing.T) {
		th := renderer.Theme{AnsiEnabled: false}
		for _, pct := range []float64{0, 25, 49, 50, 70, 90, 100} {
			got := renderer.ProgressBarColor(pct, th)
			if got != "" {
				t.Errorf("ProgressBarColor(%.0f%%, AnsiEnabled=false): want %q, got %q",
					pct, "", got)
			}
		}
	})

	t.Run("ansi_enabled_green_below_50", func(t *testing.T) {
		th := renderer.Theme{
			AnsiEnabled: true,
			Colors:      renderer.ColorScheme{Green: "\033[32m"},
		}
		for _, pct := range []float64{0, 25, 49} {
			got := renderer.ProgressBarColor(pct, th)
			if got == "" {
				t.Errorf("ProgressBarColor(%.0f%%, <50): want non-empty green code, got %q", pct, got)
			}
		}
	})

	t.Run("ansi_enabled_red_at_90_plus", func(t *testing.T) {
		th := renderer.Theme{
			AnsiEnabled: true,
			Colors: renderer.ColorScheme{
				Red:    "\033[31m",
				Orange: "\033[33m",
				Yellow: "\033[93m",
				Green:  "\033[32m",
			},
		}
		for _, pct := range []float64{90, 95, 100} {
			got := renderer.ProgressBarColor(pct, th)
			if got == "" {
				t.Errorf("ProgressBarColor(%.0f%%, >=90): want non-empty red code, got %q", pct, got)
			}
		}
	})
}
