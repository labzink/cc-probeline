// Package probes_test — black-box tests for CtxProbe.
// Covers visibility (Size=0 → hidden), rendering across Full/Compact/Minimal
// levels with a representative usage map, and different percent values.
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestCtx_Visible_Empty verifies that CtxProbe.Visible returns false when
// ContextWindow.Size is zero (no context-window info in stdin).
func TestCtx_Visible_Empty(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{Size: 0},
	}}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(Size=0): want false, got true")
	}
}

// TestCtx_Render_AllLevels verifies all three display levels for a representative
// context window state.
//
// Setup:
//
//	Size = 200000
//	used = cache_read_input_tokens (128000) + input_tokens (0) + cache_creation_input_tokens (0)
//	     = 128000
//	percent = 128000 / 200000 * 100 = 64%
//
// Phase 6.6 expected per level (§2.2):
//
//	Full:    "ctx ██████░░░░ 128K/200K (64%)"  (ProgressBar10: floor(64/5)*5=60% → 6 full + 4 empty)
//	Compact: "███░░ 128K/200K"                 (ProgressBar: roundNearest10(64%)=60% → ███░░)
//	Minimal: "128K/200K"
func TestCtx_Render_AllLevels(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{
			Size: 200000,
			CurrentUsage: map[string]int{
				"cache_read_input_tokens":     128000,
				"input_tokens":                0,
				"cache_creation_input_tokens": 0,
				"output_tokens":               0,
			},
		},
	}}

	tests := []struct {
		name  string
		level probes.Level
		want  string
	}{
		// Phase 6.6: Full uses ProgressBar10 (10 runes); Compact uses ProgressBar (5 runes).
		{"full", probes.LevelFull, "ctx ██████░░░░ 128K/200K (64%)"},
		{"compact", probes.LevelCompact, "███░░ 128K/200K"},
		{"minimal", probes.LevelMinimal, "128K/200K"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(%v): want %q, got %q", tc.level, tc.want, got)
			}
		})
	}
}

// TestCtx_Render_Percent verifies percent calculation and bar rendering at
// two additional percentage values (15% and 95%) to ensure the bar segment
// selection and label are consistent.
//
// Phase 6.6 Full uses ProgressBar10 (10 runes, 5% precision):
//
//	Case 1: 15% usage
//	  Size=200000, used=30000 → 15% → floor(15/5)*5=15% → "█▒░░░░░░░░" (10 runes)
//	  Full: "ctx █▒░░░░░░░░ 30K/200K (15%)"
//
//	Case 2: 95% usage
//	  Size=200000, used=190000 → 95% → floor(95/5)*5=95% → "█████████▒" (10 runes)
//	  Full: "ctx █████████▒ 190K/200K (95%)"
//
// Compact still uses ProgressBar (5 runes) with roundNearest10:
//
//	15% → roundNearest10=20% → "█░░░░"
func TestCtx_Render_Percent(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	tests := []struct {
		name  string
		size  int
		used  int
		level probes.Level
		want  string
	}{
		{
			// Phase 6.6: Full uses ProgressBar10; 15% floors to 15% → █▒░░░░░░░░
			name:  "15pct full",
			size:  200000,
			used:  30000,
			level: probes.LevelFull,
			want:  "ctx █▒░░░░░░░░ 30K/200K (15%)",
		},
		{
			// Compact uses ProgressBar with roundNearest10: 15% → 20% → █░░░░
			name:  "15pct compact",
			size:  200000,
			used:  30000,
			level: probes.LevelCompact,
			want:  "█░░░░ 30K/200K",
		},
		{
			// Phase 6.6: Full uses ProgressBar10; 95% floors to 95% → █████████▒
			name:  "95pct full",
			size:  200000,
			used:  190000,
			level: probes.LevelFull,
			want:  "ctx █████████▒ 190K/200K (95%)",
		},
		{
			name:  "95pct minimal",
			size:  200000,
			used:  190000,
			level: probes.LevelMinimal,
			want:  "190K/200K",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{
				ContextWindow: stdin.ContextWindow{
					Size: tc.size,
					CurrentUsage: map[string]int{
						"cache_read_input_tokens":     tc.used,
						"input_tokens":                0,
						"cache_creation_input_tokens": 0,
						"output_tokens":               0,
					},
				},
			}}
			got := p.Render(d, cfg, th, tc.level)
			if got != tc.want {
				t.Errorf("Render(%v, used=%d, size=%d): want %q, got %q",
					tc.level, tc.used, tc.size, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-E2: TestCtx_NumbersColoured (Phase 6.8.e)
//
// Spec T-22: Full output = "ctx <bar> <usedK>/<sizeK>" where usedK is
// wrapped with a semantic colour marker; no "%" character; bar preserved.
//
// Colour rules (AnsiEnabled=true, markers resolved later by renderer.Apply):
//   > 95% fill → {{color:bold_red}} on usedK number
//   < 50% fill → {{color:green}} on usedK number
//
// Minimal output = bare "usedK/sizeK" — no colour markers, no "%".
// ---------------------------------------------------------------------------

// TestCtx_NumbersColoured verifies that CtxProbe.Render (Full) wraps the used-K
// number with the correct colour marker based on fill percentage, emits no "%"
// character, and preserves the progress bar glyph.
func TestCtx_NumbersColoured(t *testing.T) {
	p := &probes.CtxProbe{}
	th := renderer.Theme{AnsiEnabled: true}

	tests := []struct {
		name          string
		size          int
		usedTokens    int
		level         probes.Level
		wantContains  []string // all must appear in output
		wantAbsent    []string // none may appear in output
	}{
		{
			// >95% → usedK must carry {{color:bold_red}}.
			// size=200000, used=192000 → 96% → bold_red.
			name:       "full_over95pct_bold_red",
			size:       200000,
			usedTokens: 192000,
			level:      probes.LevelFull,
			wantContains: []string{
				"{{color:bold_red}}", // colour marker must be present on used-K
				"192K",              // the number itself
				"200K",              // size
				"ctx",               // label preserved
			},
			wantAbsent: []string{
				"%", // no percent sign in Full output (T-22)
			},
		},
		{
			// <50% → usedK must carry {{color:green}}.
			// size=200000, used=60000 → 30% → green.
			name:       "full_under50pct_green",
			size:       200000,
			usedTokens: 60000,
			level:      probes.LevelFull,
			wantContains: []string{
				"{{color:green}}", // colour marker must be present on used-K
				"60K",            // the number itself
				"200K",           // size
				"ctx",            // label preserved
			},
			wantAbsent: []string{
				"%", // no percent sign in Full output (T-22)
			},
		},
		{
			// Full output must contain a progress bar glyph (T-22: bar preserved).
			// size=200000, used=128000 → 64% → bar exists but between 50% and 95%.
			name:       "full_bar_preserved",
			size:       200000,
			usedTokens: 128000,
			level:      probes.LevelFull,
			wantContains: []string{
				"█",   // at least one full block glyph proves bar is present
				"ctx", // label
			},
			wantAbsent: []string{
				"%", // no percent sign (T-22)
			},
		},
		{
			// Minimal level: bare "usedK/sizeK", no colour markers, no "%".
			name:       "minimal_bare_numbers",
			size:       200000,
			usedTokens: 192000,
			level:      probes.LevelMinimal,
			wantContains: []string{
				"192K/200K",
			},
			wantAbsent: []string{
				"%",            // no percent sign
				"{{color:",     // no colour markers at Minimal
				"ctx",          // no label at Minimal
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{
				ContextWindow: stdin.ContextWindow{
					Size: tc.size,
					CurrentUsage: map[string]int{
						"cache_read_input_tokens":     tc.usedTokens,
						"input_tokens":                0,
						"cache_creation_input_tokens": 0,
						"output_tokens":               0,
					},
				},
			}}
			cfg := probes.Config{CtxEnabled: true}
			got := p.Render(d, cfg, th, tc.level)

			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Render(%v, used=%d, size=%d): expected %q in output, got %q",
						tc.level, tc.usedTokens, tc.size, want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Render(%v, used=%d, size=%d): must NOT contain %q, got %q",
						tc.level, tc.usedTokens, tc.size, absent, got)
				}
			}
		})
	}
}

// TestCtx_RoundNearest10_ClampAbove100 verifies that roundNearest10 clamps
// values above 100 to exactly 100 (the r > 100 branch, ctx.go:89-92).
// This exercises the branch that existing tests do not cover (66.7% → 100%).
func TestCtx_RoundNearest10_ClampAbove100(t *testing.T) {
	p := &probes.CtxProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	// Feed used > size so raw pct > 100 before clamping.
	// size=100000, used=110000 → raw pct=110 → clamped to 100 → bar "█████".
	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{
			Size: 100000,
			CurrentUsage: map[string]int{
				"cache_read_input_tokens": 110000,
			},
		},
	}}

	got := p.Render(d, cfg, th, probes.LevelFull)
	const wantBar = "█████"
	if !strings.Contains(got, wantBar) {
		t.Errorf("Render(used>size): want bar %q in output, got %q", wantBar, got)
	}
}
