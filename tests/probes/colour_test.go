// Package probes_test — RED tests for Phase 6.7.b semantic colour markers.
//
// All tests exercise probe.Render → renderer.Apply → strings.Contains(ANSI code).
// Theme with colour: renderer.Theme{AnsiEnabled: true, Colors: renderer.DefaultPalette()}.
// Theme without colour: renderer.Theme{} (zero value, AnsiEnabled=false).
//
// Tests in this file: T-3..T-10, T-13, T-14.
// T-11, T-12 are in 6.7.c (table/subagent scope).
package probes_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// colourTheme returns a Theme with ANSI enabled and the default palette.
func colourTheme() renderer.Theme {
	return renderer.Theme{AnsiEnabled: true, Colors: renderer.DefaultPalette()}
}

// ----------------------------------------------------------------------------
// T-3: ModelProbe — bold name
// ----------------------------------------------------------------------------

// TestColour_Model_Bold (T-3) verifies that ModelProbe.Render, after Apply,
// wraps the model name in bold (\x1b[1m … \x1b[0m).
func TestColour_Model_Bold(t *testing.T) {
	p := &probes.ModelProbe{}
	th := colourTheme()
	cfg := probes.Config{ModelEnabled: true}

	d := probes.Data{Stdin: stdin.Payload{Model: stdin.Model{ID: "claude-sonnet-4-6"}}}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	// T-3: output must contain bold-on (\x1b[1m) and reset (\x1b[0m).
	if !strings.Contains(got, "\x1b[1m") {
		t.Errorf("T-3 model bold: want \\x1b[1m in Apply output, got %q", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("T-3 model reset: want \\x1b[0m in Apply output, got %q", got)
	}
}

// ----------------------------------------------------------------------------
// T-4: EffortProbe — magenta / dim / default
// ----------------------------------------------------------------------------

// TestColour_Effort_High_Magenta (T-4) verifies that effort=high renders with
// magenta (\x1b[35m) after Apply.
func TestColour_Effort_High_Magenta(t *testing.T) {
	p := &probes.EffortProbe{}
	th := colourTheme()
	cfg := probes.Config{EffortEnabled: true}

	for _, lvl := range []string{"high", "xhigh", "max"} {
		t.Run(lvl, func(t *testing.T) {
			d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: lvl}}}
			raw := p.Render(d, cfg, th, probes.LevelFull)
			got := renderer.Apply(raw, th)

			if !strings.Contains(got, "\x1b[35m") {
				t.Errorf("T-4 effort=%s: want \\x1b[35m (magenta) in Apply output, got %q", lvl, got)
			}
		})
	}
}

// TestColour_Effort_Low_Dim (T-4) verifies that effort=low renders with
// dim (\x1b[2m) after Apply.
func TestColour_Effort_Low_Dim(t *testing.T) {
	p := &probes.EffortProbe{}
	th := colourTheme()
	cfg := probes.Config{EffortEnabled: true}

	d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: "low"}}}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	if !strings.Contains(got, "\x1b[2m") {
		t.Errorf("T-4 effort=low: want \\x1b[2m (dim) in Apply output, got %q", got)
	}
}

// TestColour_Effort_Medium_NoColour (T-4) verifies that effort=medium renders
// without any colour or style escape sequences (default, no marker).
func TestColour_Effort_Medium_NoColour(t *testing.T) {
	p := &probes.EffortProbe{}
	th := colourTheme()
	cfg := probes.Config{EffortEnabled: true}

	d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: "medium"}}}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	// medium must NOT contain bold/italic/dim/magenta escape sequences.
	if strings.Contains(got, "\x1b[3") {
		t.Errorf("T-4 effort=medium: must NOT contain \\x1b[3x escape, got %q", got)
	}
	if strings.Contains(got, "\x1b[35m") {
		t.Errorf("T-4 effort=medium: must NOT contain magenta \\x1b[35m, got %q", got)
	}
}

// TestColour_Model_EffortGlyph_Magenta (regression RC1) verifies that the
// effort glyph ModelProbe appends — the glyph actually shown on the header
// line — is colour-wrapped. Before the fix ModelProbe emitted a BARE icon, so
// effort=high rendered grey on the real path despite EffortProbe's (unwired)
// magenta logic passing its own unit test.
func TestColour_Model_EffortGlyph_Magenta(t *testing.T) {
	p := &probes.ModelProbe{}
	th := colourTheme()
	cfg := probes.Config{ModelEnabled: true}

	d := probes.Data{Stdin: stdin.Payload{
		Model:  stdin.Model{ID: "claude-opus-4-8"},
		Effort: stdin.Effort{Level: "high"},
	}}
	got := renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)

	if !strings.Contains(got, "\x1b[35m") {
		t.Errorf("RC1 model+effort=high: want magenta \\x1b[35m around the appended effort glyph, got %q", got)
	}
}

// ----------------------------------------------------------------------------
// T-5: GitProbe — branch cyan, ⚠N yellow
// ----------------------------------------------------------------------------

// TestColour_Git_BranchCyan (T-5) verifies that the branch segment is wrapped
// in cyan (\x1b[36m) after Apply.
func TestColour_Git_BranchCyan(t *testing.T) {
	p := &probes.GitProbe{}
	th := colourTheme()
	cfg := probes.Config{GitEnabled: true}

	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        "main",
			ModifiedCount: 0,
		},
	}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	if !strings.Contains(got, "\x1b[36m") {
		t.Errorf("T-5 git branch cyan: want \\x1b[36m in Apply output, got %q", got)
	}
}

// TestColour_Git_WarningYellow (T-5) verifies that the ⚠N segment is wrapped
// in yellow (\x1b[33m) when ModifiedCount > 0.
func TestColour_Git_WarningYellow(t *testing.T) {
	p := &probes.GitProbe{}
	th := colourTheme()
	cfg := probes.Config{GitEnabled: true}

	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        "main",
			ModifiedCount: 3,
		},
	}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	// Must contain ⚠ and yellow colour.
	if !strings.Contains(got, "⚠") {
		t.Errorf("T-5 git warning: output must contain ⚠, got %q", got)
	}
	if !strings.Contains(got, "\x1b[33m") {
		t.Errorf("T-5 git warning yellow: want \\x1b[33m around ⚠N in Apply output, got %q", got)
	}
}

// ----------------------------------------------------------------------------
// T-6: ProgressBarColor thresholds — ctx and quota bars
// ----------------------------------------------------------------------------

// TestColour_ProgressBar_Thresholds (T-6) verifies the four colour thresholds
// for progress bars: 30%→green, 60%→yellow, 80%→orange, 95%→red.
// Exercises both CtxProbe and QuotaProbe (both use ProgressBarColor).
func TestColour_ProgressBar_Thresholds(t *testing.T) {
	th := colourTheme()

	cases := []struct {
		name    string
		pct     float64
		wantESC string
	}{
		{"30pct green", 30, "\x1b[32m"},
		{"60pct yellow", 60, "\x1b[33m"},
		{"80pct orange", 80, "\x1b[38;5;208m"},
		{"95pct red", 95, "\x1b[31m"},
	}

	for _, tc := range cases {
		t.Run("ctx/"+tc.name, func(t *testing.T) {
			p := &probes.CtxProbe{}
			cfg := probes.Config{CtxEnabled: true}
			size := 1000
			used := int(tc.pct / 100.0 * float64(size))
			d := probes.Data{Stdin: stdin.Payload{
				ContextWindow: stdin.ContextWindow{
					Size: size,
					CurrentUsage: map[string]int{
						"cache_read_input_tokens": used,
					},
				},
			}}
			raw := p.Render(d, cfg, th, probes.LevelFull)
			got := renderer.Apply(raw, th)
			if !strings.Contains(got, tc.wantESC) {
				t.Errorf("T-6 ctx pct=%.0f: want %q in Apply output, got %q", tc.pct, tc.wantESC, got)
			}
		})

		t.Run("quota/"+tc.name, func(t *testing.T) {
			p := &probes.QuotaProbe{}
			cfg := probes.Config{QuotaEnabled: true}
			now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
			resetsAt := json.RawMessage(fmt.Sprintf("%d", now.Add(2*time.Hour).Unix()))
			rl := &stdin.RateLimits{
				FiveHour: stdin.RateWindow{
					UsedPercentage: tc.pct,
					ResetsAt:       resetsAt,
				},
				SevenDay: stdin.RateWindow{
					UsedPercentage: tc.pct,
					ResetsAt:       resetsAt,
				},
			}
			d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}
			raw := p.Render(d, cfg, th, probes.LevelFull)
			got := renderer.Apply(raw, th)
			if !strings.Contains(got, tc.wantESC) {
				t.Errorf("T-6 quota pct=%.0f: want %q in Apply output, got %q", tc.pct, tc.wantESC, got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// T-7: QuotaProbe — ↻ reset yellow <30m, no colour ≥30m
// ----------------------------------------------------------------------------

// TestColour_Quota_ResetYellow (T-7) verifies that the ↻ reset countdown is
// wrapped in yellow (\x1b[33m) when time-to-reset < 30 minutes.
func TestColour_Quota_ResetYellow(t *testing.T) {
	p := &probes.QuotaProbe{}
	th := colourTheme()
	cfg := probes.Config{QuotaEnabled: true}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	// Reset in 20 minutes — below the 30m threshold.
	resetsAt := json.RawMessage(fmt.Sprintf("%d", now.Add(20*time.Minute).Unix()))
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 40,
			ResetsAt:       resetsAt,
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 40,
			ResetsAt:       resetsAt,
		},
	}
	d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	// ↻ must be present and surrounded by yellow.
	if !strings.Contains(got, "↻") {
		t.Errorf("T-7 quota reset <30m: output must contain ↻, got %q", got)
	}
	if !strings.Contains(got, "\x1b[33m") {
		t.Errorf("T-7 quota reset <30m: want \\x1b[33m (yellow) around ↻ in Apply output, got %q", got)
	}
}

// TestColour_Quota_ResetNoColour (T-7) verifies that the ↻ reset countdown
// has NO yellow colour escape when time-to-reset ≥ 30 minutes.
func TestColour_Quota_ResetNoColour(t *testing.T) {
	p := &probes.QuotaProbe{}
	th := colourTheme()
	cfg := probes.Config{QuotaEnabled: true}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	// Reset in 45 minutes — above the 30m threshold; ↻ should be default colour.
	resetsAt := json.RawMessage(fmt.Sprintf("%d", now.Add(45*time.Minute).Unix()))
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 40,
			ResetsAt:       resetsAt,
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 40,
			ResetsAt:       resetsAt,
		},
	}
	d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	// ↻ must be present but must NOT be surrounded by yellow.
	if !strings.Contains(got, "↻") {
		t.Errorf("T-7 quota reset >=30m: output must contain ↻, got %q", got)
	}

	// Find position of ↻; the byte immediately before must not be the start of
	// a yellow escape. We check that \x1b[33m does not appear adjacent to ↻.
	// Simple check: if the only \x1b[33m occurrences come from bar colour (not reset),
	// we split on ↻ and verify the prefix of the ↻-containing segment has no \x1b[33m
	// that is unclosed before ↻.
	// Simpler invariant: the rendered string must not contain \x1b[33m↻ as a substring.
	if strings.Contains(got, "\x1b[33m↻") {
		t.Errorf("T-7 quota reset >=30m: \\x1b[33m must NOT precede ↻ directly, got %q", got)
	}
}

// ----------------------------------------------------------------------------
// T-8: CostProbe — no colour around $x
// ----------------------------------------------------------------------------

// TestColour_Cost_Neutral (T-8) verifies that CostProbe renders with no ANSI
// colour escape codes at any CostBudgetUSD value (cost is neutral/informational).
func TestColour_Cost_Neutral(t *testing.T) {
	p := &probes.CostProbe{}
	th := colourTheme()

	cases := []struct {
		name string
		cost float64
	}{
		{"zero", 0.00},
		{"small", 1.23},
		{"large", 999.99},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := probes.Config{CostEnabled: true}
			d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: tc.cost}}}

			for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
				raw := p.Render(d, cfg, th, level)
				got := renderer.Apply(raw, th)

				// T-8: cost must contain NO escape codes whatsoever.
				if strings.Contains(got, "\x1b[") {
					t.Errorf("T-8 cost neutral (cost=%.2f, level=%v): output must NOT contain \\x1b[, got %q",
						tc.cost, level, got)
				}
			}
		})
	}
}

// ----------------------------------------------------------------------------
// T-9: CacheProbe — ⏱ colour by remaining minutes
// ----------------------------------------------------------------------------

// TestColour_Cache_TTL_Colours (T-9) verifies that the ⏱Nm TTL block is
// coloured by remaining time: ≤10m red, ≤30m yellow, >30m no colour.
func TestColour_Cache_TTL_Colours(t *testing.T) {
	p := &probes.CacheProbe{}
	th := colourTheme()

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	// OrchTTL=60m. remaining = 60 - elapsed.
	// For remaining=8m: elapsed = 52m → LastTimestamp = now - 52m.
	// For remaining=25m: elapsed = 35m → LastTimestamp = now - 35m.
	// For remaining=45m: elapsed = 15m → LastTimestamp = now - 15m.

	cases := []struct {
		name      string
		remaining int // minutes remaining in TTL
		wantESC   string
		wantNoESC bool // true = must NOT contain ANY \x1b[
	}{
		{"8m remaining → red", 8, "\x1b[31m", false},
		{"25m remaining → yellow", 25, "\x1b[33m", false},
		{"45m remaining → no colour", 45, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orchTTL := 60
			elapsed := orchTTL - tc.remaining
			lastTS := now.Add(-time.Duration(elapsed) * time.Minute)

			d := probes.Data{
				Session: &parser.SessionStats{
					Totals: parser.TokenCounts{
						CacheRead:   1000,
						CacheCreate: 500,
						Output:      200,
					},
					LastTimestamp: lastTS,
					TurnCount:     3,
				},
				Stdin: stdin.Payload{
					Cost: stdin.Cost{
						TotalCostUSD:       0.10,
						TotalAPIDurationMS: 60000,
					},
				},
				Now: now,
			}
			cfg := probes.Config{
				CacheEnabled:   true,
				CostEnabled:    true,
				OrchTTLMinutes: orchTTL,
			}

			raw := p.Render(d, cfg, th, probes.LevelFull)
			got := renderer.Apply(raw, th)

			// Verify ⏱ is present (TTL block should appear).
			if !strings.Contains(got, "⏱") {
				t.Errorf("T-9 cache TTL (%s): output must contain ⏱, got %q", tc.name, got)
			}

			if tc.wantNoESC {
				// The ⏱ segment must not be coloured.
				// Check that no escape appears immediately before ⏱.
				if strings.Contains(got, "\x1b[31m⏱") || strings.Contains(got, "\x1b[33m⏱") {
					t.Errorf("T-9 cache TTL (%s): ⏱ must have no colour escape, got %q", tc.name, got)
				}
			} else {
				if !strings.Contains(got, tc.wantESC) {
					t.Errorf("T-9 cache TTL (%s): want %q around ⏱ in Apply output, got %q",
						tc.name, tc.wantESC, got)
				}
			}
		})
	}
}

// ----------------------------------------------------------------------------
// T-10: TimeProbe / EmailProbe / ProjectProbe — dim wrapper
// ----------------------------------------------------------------------------

// TestColour_Time_Dim (T-10) verifies that TimeProbe.Render, after Apply,
// contains the dim escape (\x1b[2m) wrapping the MM:SS value.
func TestColour_Time_Dim(t *testing.T) {
	p := &probes.TimeProbe{}
	th := colourTheme()
	cfg := probes.Config{TimeEnabled: true}

	d := makeTimeData(120000) // 2m00s
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	if !strings.Contains(got, "\x1b[2m") {
		t.Errorf("T-10 time dim: want \\x1b[2m in Apply output, got %q", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("T-10 time reset: want \\x1b[0m in Apply output, got %q", got)
	}
}

// TestColour_Email_Dim (T-10) verifies that EmailProbe.Render, after Apply,
// contains the dim escape (\x1b[2m) wrapping the email value.
func TestColour_Email_Dim(t *testing.T) {
	p := &probes.EmailProbe{}
	th := colourTheme()
	cfg := probes.Config{EmailEnabled: true, Email: "user@example.com"}

	d := probes.Data{Stdin: stdin.Payload{}}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	if !strings.Contains(got, "\x1b[2m") {
		t.Errorf("T-10 email dim: want \\x1b[2m in Apply output, got %q", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("T-10 email reset: want \\x1b[0m in Apply output, got %q", got)
	}
}

// TestColour_Project_Dim (T-10) verifies that ProjectProbe.Render, after Apply,
// contains the dim escape (\x1b[2m) wrapping the project name.
func TestColour_Project_Dim(t *testing.T) {
	p := &probes.ProjectProbe{}
	th := colourTheme()
	cfg := probes.Config{ProjectEnabled: true}

	d := probes.Data{Stdin: stdin.Payload{Cwd: "/Users/user/projects/myapp"}}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	got := renderer.Apply(raw, th)

	if !strings.Contains(got, "\x1b[2m") {
		t.Errorf("T-10 project dim: want \\x1b[2m in Apply output, got %q", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("T-10 project reset: want \\x1b[0m in Apply output, got %q", got)
	}
}

// ----------------------------------------------------------------------------
// T-13: Regression — AnsiEnabled=false → no escape codes from any probe
// ----------------------------------------------------------------------------

// TestColour_Regression_NoAnsi (T-13) verifies that under Theme{} (zero value,
// AnsiEnabled=false) none of the probes produce \x1b[ escape codes in their
// rendered+applied output.
func TestColour_Regression_NoAnsi(t *testing.T) {
	th := renderer.Theme{} // AnsiEnabled=false
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	resetsAt := json.RawMessage(fmt.Sprintf("%d", now.Add(2*time.Hour).Unix()))
	lastTS := now.Add(-5 * time.Minute)

	type renderFunc func() string

	cases := []struct {
		name   string
		render renderFunc
	}{
		{
			"model",
			func() string {
				p := &probes.ModelProbe{}
				cfg := probes.Config{ModelEnabled: true}
				d := probes.Data{Stdin: stdin.Payload{Model: stdin.Model{ID: "claude-sonnet-4-6"}}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"effort/high",
			func() string {
				p := &probes.EffortProbe{}
				cfg := probes.Config{EffortEnabled: true}
				d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: "high"}}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"effort/low",
			func() string {
				p := &probes.EffortProbe{}
				cfg := probes.Config{EffortEnabled: true}
				d := probes.Data{Stdin: stdin.Payload{Effort: stdin.Effort{Level: "low"}}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"git/branch",
			func() string {
				p := &probes.GitProbe{}
				cfg := probes.Config{GitEnabled: true}
				d := probes.Data{Git: &parser.GitStatus{Branch: "main", ModifiedCount: 1}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"ctx",
			func() string {
				p := &probes.CtxProbe{}
				cfg := probes.Config{CtxEnabled: true}
				d := probes.Data{Stdin: stdin.Payload{
					ContextWindow: stdin.ContextWindow{
						Size:         200000,
						CurrentUsage: map[string]int{"cache_read_input_tokens": 80000},
					},
				}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"quota",
			func() string {
				p := &probes.QuotaProbe{}
				cfg := probes.Config{QuotaEnabled: true}
				rl := &stdin.RateLimits{
					FiveHour: stdin.RateWindow{UsedPercentage: 40, ResetsAt: resetsAt},
					SevenDay: stdin.RateWindow{UsedPercentage: 60, ResetsAt: resetsAt},
				}
				d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"cost",
			func() string {
				p := &probes.CostProbe{}
				cfg := probes.Config{CostEnabled: true}
				d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: 1.23}}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"cache/ttl",
			func() string {
				p := &probes.CacheProbe{}
				cfg := probes.Config{CacheEnabled: true, CostEnabled: true, OrchTTLMinutes: 60}
				d := probes.Data{
					Session: &parser.SessionStats{
						Totals:        parser.TokenCounts{CacheRead: 1000, CacheCreate: 500, Output: 200},
						LastTimestamp: lastTS,
						TurnCount:     3,
					},
					Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: 0.10, TotalAPIDurationMS: 60000}},
					Now:   now,
				}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"time",
			func() string {
				p := &probes.TimeProbe{}
				cfg := probes.Config{TimeEnabled: true}
				d := makeTimeData(120000)
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"email",
			func() string {
				p := &probes.EmailProbe{}
				cfg := probes.Config{EmailEnabled: true, Email: "user@example.com"}
				d := probes.Data{Stdin: stdin.Payload{}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
		{
			"project",
			func() string {
				p := &probes.ProjectProbe{}
				cfg := probes.Config{ProjectEnabled: true}
				d := probes.Data{Stdin: stdin.Payload{Cwd: "/home/user/myapp"}}
				return renderer.Apply(p.Render(d, cfg, th, probes.LevelFull), th)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.render()
			if strings.Contains(got, "\x1b[") {
				t.Errorf("T-13 regression (probe=%s): AnsiEnabled=false but got escape code in %q", tc.name, got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// T-14: format.VisualLen — markers are zero-width
// ----------------------------------------------------------------------------

// TestColour_VisualLen_MarkersZeroWidth (T-14) verifies that format.VisualLen
// treats {{color:..}}, {{dim}}, {{bold}}, {{reset}} as zero-width markers.
// VisualLen(string with markers) must equal VisualLen(plain text without markers).
func TestColour_VisualLen_MarkersZeroWidth(t *testing.T) {
	cases := []struct {
		name        string
		withMarkers string
		plainText   string
	}{
		{
			"bold model name",
			"{{bold}}sonnet-4-6{{reset}}",
			"sonnet-4-6",
		},
		{
			"dim time value",
			"{{dim}}02:00{{reset}}",
			"02:00",
		},
		{
			"color:cyan branch",
			"{{color:cyan}}main{{reset}}",
			"main",
		},
		{
			"color:yellow warning",
			"{{color:yellow}}⚠3{{reset}}",
			"⚠3",
		},
		{
			"color:magenta effort",
			"{{color:magenta}}◑{{reset}}",
			"◑",
		},
		{
			"color:red TTL",
			"{{color:red}}⏱ 8m{{reset}}",
			"⏱ 8m",
		},
		{
			"color:orange bar segment",
			"{{color:orange}}██░░░{{reset}}",
			"██░░░",
		},
		{
			"mixed markers and text",
			"{{color:cyan}}⎇ main{{reset}} {{color:yellow}}⚠2{{reset}}",
			"⎇ main ⚠2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLen := format.VisualLen(tc.withMarkers)
			plainLen := format.VisualLen(tc.plainText)
			if withLen != plainLen {
				t.Errorf("T-14 VisualLen(%q) = %d, want %d (== VisualLen(%q))",
					tc.withMarkers, withLen, plainLen, tc.plainText)
			}
		})
	}
}
