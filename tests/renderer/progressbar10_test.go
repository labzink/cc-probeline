// Package renderer_test — black-box tests for ProgressBar10 (10-segment bar, 5% precision).
// RED phase: ProgressBar10 does not exist yet; these tests must fail to compile.
// Spec reference: spec-common.md §2.1.
package renderer_test

import (
	"fmt"
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// TestProgressBar10_Length verifies that ProgressBar10 returns exactly 10 runes
// for a representative set of inputs. Length is measured via len([]rune(s)) to
// correctly count multi-byte glyphs (█▒░ are 3-byte UTF-8 codepoints).
func TestProgressBar10_Length(t *testing.T) {
	inputs := []float64{0, 5, 15, 50, 100}
	for _, pct := range inputs {
		got := renderer.ProgressBar10(pct)
		if runeLen := len([]rune(got)); runeLen != 10 {
			t.Errorf("ProgressBar10(%.0f%%): rune length = %d, want 10 (got %q)",
				pct, runeLen, got)
		}
	}
}

// TestProgressBar10_Precision5 verifies the exact glyph mapping for key boundary
// values defined in spec-common.md §2.1.
//
// Algorithm (per spec):
//   - clamp input to [0,100]
//   - round DOWN to nearest multiple of 5%  (e.g. 14 → 10, 15 → 15, 16 → 15)
//   - 10 cells; cell i covers percentage band [i*10, (i+1)*10)
//   - val = clamp(rounded − i*10, 0, 10)
//   - val >= 10 → '█'; val == 5 → '▒'; otherwise → '░'
//
// Manual verification of expected values:
//
//	0%   → rounded=0;   all cells val=0 → ░░░░░░░░░░
//	5%   → rounded=5;   i=0 val=5→▒, rest=0→░  → ▒░░░░░░░░░
//	10%  → rounded=10;  i=0 val=10→█, rest=0→░  → █░░░░░░░░░
//	15%  → rounded=15;  i=0 val=10→█, i=1 val=5→▒, rest→░ → █▒░░░░░░░░
//	50%  → rounded=50;  i=0..4 val=10→█, i=5 val=0→░, rest=0→░ → █████░░░░░
//	100% → rounded=100; all cells val=10→█ → ██████████
func TestProgressBar10_Precision5(t *testing.T) {
	tests := []struct {
		percent float64
		want    string
	}{
		{0, "░░░░░░░░░░"},
		{5, "▒░░░░░░░░░"},
		{10, "█░░░░░░░░░"},
		{15, "█▒░░░░░░░░"},
		{50, "█████░░░░░"},
		{100, "██████████"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("pct_%.0f", tc.percent), func(t *testing.T) {
			got := renderer.ProgressBar10(tc.percent)
			if got != tc.want {
				t.Errorf("ProgressBar10(%.0f%%): want %q (len %d runes), got %q (len %d runes)",
					tc.percent, tc.want, len([]rune(tc.want)), got, len([]rune(got)))
			}
		})
	}
}

// TestProgressBar10_Clamp verifies that values outside [0,100] are clamped
// (not panicked) and produce the same result as the boundary values 0 and 100.
func TestProgressBar10_Clamp(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		want    string
	}{
		// Negative value must behave identically to 0%.
		{"negative -20 treated as 0", -20, "░░░░░░░░░░"},
		// Overflow must behave identically to 100%.
		{"overflow 150 treated as 100", 150, "██████████"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Must not panic; result must equal the clamped boundary.
			got := renderer.ProgressBar10(tc.percent)
			if got != tc.want {
				t.Errorf("ProgressBar10(%.0f%%): want %q, got %q", tc.percent, tc.want, got)
			}
		})
	}
}
