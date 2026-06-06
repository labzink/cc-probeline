// Package probes_test — black-box tests for QuotaProbe.
// Covers visibility (QuotaEnabled toggle) and rendering across all three
// display levels with Phase-4.1 hardcoded stubs.
//
// In Phase 4.1 there is no real quota API. QuotaProbe returns hardcoded stub
// values: 5-hour usage at 23% and 7-day usage at 41%. Tests assert on these
// exact stub strings; they will be updated in Phase 6 when real API values
// are plumbed through Config.
package probes_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestQuota_Visible_Disabled verifies that QuotaProbe.Visible returns false
// when Config.QuotaEnabled is false, regardless of other fields.
func TestQuota_Visible_Disabled(t *testing.T) {
	// C4: isolate from real quota file so Freshest() always returns false.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	d := probes.Data{Stdin: stdin.Payload{}}
	cfg := probes.Config{QuotaEnabled: false}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(QuotaEnabled=false): want false, got true")
	}
}

// TestQuota_Visible_Enabled verifies that QuotaProbe.Visible returns true
// when Config.QuotaEnabled is true AND RateLimits data is present.
// Updated in Phase 6.5.b4: real data required; nil RateLimits → false (T-17).
func TestQuota_Visible_Enabled(t *testing.T) {
	// C4: isolate from real quota file.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 50},
		SevenDay: stdin.RateWindow{UsedPercentage: 50},
	}
	d := probes.Data{Stdin: stdin.Payload{RateLimits: rl}}
	cfg := probes.Config{QuotaEnabled: true}

	got := p.Visible(d, cfg)
	if got != true {
		t.Errorf("Visible(QuotaEnabled=true, RateLimits!=nil): want true, got false")
	}
}

// TestQuota_Render_Full verifies QuotaProbe.Render at LevelFull.
// Phase 6.6: Full uses ProgressBar10 (10 runes); reset format "↻ Xh:Ym" / "↻ Xd.Yh".
// 5h=23% → ProgressBar10 floors to 20% → bar "██░░░░░░░░"
// 7d=41% → ProgressBar10 floors to 40% → bar "████░░░░░░"
// 133min → 2h13m → "↻ 2h:13m"; 84h → 3d12h → "↻ 3d.12h"
func TestQuota_Render_Full(t *testing.T) {
	// C4: isolate from real quota file so probe uses d.Stdin.RateLimits only.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 23.0,
			ResetsAt:       json.RawMessage(fmt.Sprintf("%d", now.Add(133*time.Minute).Unix())),
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 41.0,
			ResetsAt:       json.RawMessage(fmt.Sprintf("%d", now.Add(84*time.Hour).Unix())),
		},
	}
	d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

	got := p.Render(d, cfg, th, probes.LevelFull)
	want := "5h: ██░░░░░░░░ ↻ 2h:13m · 7d: ████░░░░░░ ↻ 3d.12h"
	if got != want {
		t.Errorf("Render(Full): want %q, got %q", want, got)
	}
}

// TestQuota_Render_Compact verifies QuotaProbe.Render at LevelCompact.
// Labels "5h: " and "7d: " are dropped per §A4 P1.
// Phase 6.6: Compact uses ProgressBar (5 runes); reset format "↻ Xh:Ym" / "↻ Xd.Yh".
// 23% → ProgressBar floors to 20% → bar "█░░░░"
// 41% → ProgressBar floors to 40% → bar "██░░░"
func TestQuota_Render_Compact(t *testing.T) {
	// C4: isolate from real quota file so probe uses d.Stdin.RateLimits only.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 23.0,
			ResetsAt:       json.RawMessage(fmt.Sprintf("%d", now.Add(133*time.Minute).Unix())),
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 41.0,
			ResetsAt:       json.RawMessage(fmt.Sprintf("%d", now.Add(84*time.Hour).Unix())),
		},
	}
	d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

	got := p.Render(d, cfg, th, probes.LevelCompact)
	want := "█░░░░ ↻ 2h:13m · ██░░░ ↻ 3d.12h"
	if got != want {
		t.Errorf("Render(Compact): want %q, got %q", want, got)
	}
}

// TestQuota_Render_Minimal verifies QuotaProbe.Render at LevelMinimal.
// The progress bar is dropped, but the percent value and the reset countdown
// are kept (Phase 6.95.e). With no resets_at and no stored snapshot the
// countdown renders the "↻ ??m" unknown form. Theme is plain (AnsiEnabled
// false) so the percent carries no colour markers.
func TestQuota_Render_Minimal(t *testing.T) {
	// C4: isolate from real quota file so probe uses d.Stdin.RateLimits only.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 23.0},
		SevenDay: stdin.RateWindow{UsedPercentage: 41.0},
	}
	d := probes.Data{Stdin: stdin.Payload{RateLimits: rl}}

	got := p.Render(d, cfg, th, probes.LevelMinimal)
	want := "23% ↻ ??m · 41% ↻ ??m"
	if got != want {
		t.Errorf("Render(Minimal): want %q, got %q", want, got)
	}
}

// TestQuotaProbe_HiddenWhenNil (T-17) verifies that QuotaProbe.Visible returns
// false when RateLimits is nil, even if QuotaEnabled=true.
//
// RED: fails until Visible checks d.Stdin.RateLimits != nil in addition to
// c.QuotaEnabled.
func TestQuotaProbe_HiddenWhenNil(t *testing.T) {
	// C4: isolate from real quota file; with empty dir, Freshest()=false.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	d := probes.Data{Stdin: stdin.Payload{}} // RateLimits not set → nil
	c := probes.Config{QuotaEnabled: true}

	got := p.Visible(d, c)
	if got != false {
		t.Errorf("Visible(QuotaEnabled=true, RateLimits=nil): want false, got true")
	}
}

// TestQuotaProbe_RealRender (T-18) verifies QuotaProbe.Render with real
// RateLimits data decoded from Unix timestamps.
//
// Setup (verified manually):
//
//	now = 2024-01-01T10:00:00Z
//	5h: UsedPercentage=40%, resets_at = now+133min → 2h13m
//	    Phase 6.6: Full bar ProgressBar10: 40% → "████░░░░░░"; Compact ProgressBar: 40% → "██░░░"
//	7d: UsedPercentage=60%, resets_at = now+84h   → 3d12h
//	    Phase 6.6: Full bar ProgressBar10: 60% → "██████░░░░"; Compact ProgressBar: 60% → "███░░"
//	reset format: "↻ 2h:13m" and "↻ 3d.12h"
func TestQuotaProbe_RealRender(t *testing.T) {
	// C4: isolate from real quota file so probe uses d.Stdin.RateLimits only.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	fiveHourResetsAt := now.Add(133 * time.Minute) // 2h13m from now
	sevenDayResetsAt := now.Add(84 * time.Hour)    // 3d12h from now

	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 40.0,
			ResetsAt:       json.RawMessage(fmt.Sprintf("%d", fiveHourResetsAt.Unix())),
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 60.0,
			ResetsAt:       json.RawMessage(fmt.Sprintf("%d", sevenDayResetsAt.Unix())),
		},
	}

	d := probes.Data{
		Now:   now,
		Stdin: stdin.Payload{RateLimits: rl},
	}

	// Phase 6.6: Full uses ProgressBar10 (10 runes) + new reset format.
	wantFull := "5h: ████░░░░░░ ↻ 2h:13m · 7d: ██████░░░░ ↻ 3d.12h"
	if got := p.Render(d, cfg, th, probes.LevelFull); got != wantFull {
		t.Errorf("Render(Full): want %q, got %q", wantFull, got)
	}

	// Phase 6.6: Compact uses ProgressBar (5 runes) + new reset format.
	wantCompact := "██░░░ ↻ 2h:13m · ███░░ ↻ 3d.12h"
	if got := p.Render(d, cfg, th, probes.LevelCompact); got != wantCompact {
		t.Errorf("Render(Compact): want %q, got %q", wantCompact, got)
	}

	// Phase 6.95.e: Minimal keeps the reset countdown (same source as Full/Compact).
	wantMinimal := "40% ↻ 2h:13m · 60% ↻ 3d.12h"
	if got := p.Render(d, cfg, th, probes.LevelMinimal); got != wantMinimal {
		t.Errorf("Render(Minimal): want %q, got %q", wantMinimal, got)
	}
}

// TestQuotaProbe_Boundary (T-19) verifies QuotaProbe.Render at the 0% and 100%
// boundary values.
//
// Case A (0%):  bar "░░░░░", Minimal contains "0%"
// Case B (100%): bar "█████", Minimal contains "100%"
//
// RED: fails until QuotaProbe.Render reads real RateLimits.
func TestQuotaProbe_Boundary(t *testing.T) {
	// C4: isolate from real quota file so probe uses d.Stdin.RateLimits only.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	futureUnix := json.RawMessage(fmt.Sprintf("%d", now.Add(24*time.Hour).Unix()))

	cases := []struct {
		name        string
		pct         float64
		wantBarFull string
		wantMinimal string
	}{
		{"0%", 0.0, "5h: ░░░░░", "0%"},
		{"100%", 100.0, "5h: █████", "100%"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl := &stdin.RateLimits{
				FiveHour: stdin.RateWindow{
					UsedPercentage: tc.pct,
					ResetsAt:       futureUnix,
				},
				SevenDay: stdin.RateWindow{
					UsedPercentage: tc.pct,
					ResetsAt:       futureUnix,
				},
			}
			d := probes.Data{
				Now:   now,
				Stdin: stdin.Payload{RateLimits: rl},
			}

			full := p.Render(d, cfg, th, probes.LevelFull)
			if len(full) < len(tc.wantBarFull) || full[:len(tc.wantBarFull)] != tc.wantBarFull {
				t.Errorf("Render(Full) case %s: want prefix %q, got %q", tc.name, tc.wantBarFull, full)
			}

			minimal := p.Render(d, cfg, th, probes.LevelMinimal)
			if !strings.Contains(minimal, tc.wantMinimal) {
				t.Errorf("Render(Minimal) case %s: want %q in %q", tc.name, tc.wantMinimal, minimal)
			}
		})
	}
}

// --- Phase 6.9.b: T-23, T-24, T-25 ---

// TestQuotaRender_PctSuffixInRed (T-23) verifies that Full/Compact render
// inserts " NN%" between the progress bar and the reset countdown when pct ≥ 90,
// and that no such suffix appears when pct < 90.
//
// Spec §2.3: "Full/Compact insert ` NN%` between bar and reset when pct >= 90,
// coloured with ProgressBarColor(pct)".
//
// Current behaviour: no % suffix is emitted at any pct level → test is RED.
func TestQuotaRender_PctSuffixInRed(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	// Plain theme so colour-marker presence can be tested via string contains without ANSI.
	th := renderer.Theme{}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// Reset is well in the future (2h) so the reset countdown is non-zero.
	resetUnix := json.RawMessage(fmt.Sprintf("%d", now.Add(2*time.Hour).Unix()))

	cases := []struct {
		name       string
		pct        float64
		wantSuffix bool   // whether " NN%" should appear in output
		pctStr     string // the exact percent digits (e.g. "92")
	}{
		{
			name:       "pct_92_triggers_suffix",
			pct:        92.0,
			wantSuffix: true,
			pctStr:     "92",
		},
		{
			name:       "pct_90_triggers_suffix",
			pct:        90.0,
			wantSuffix: true,
			pctStr:     "90",
		},
		{
			name:       "pct_85_no_suffix",
			pct:        85.0,
			wantSuffix: false,
			pctStr:     "85",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl := &stdin.RateLimits{
				FiveHour: stdin.RateWindow{
					UsedPercentage: tc.pct,
					ResetsAt:       resetUnix,
				},
				SevenDay: stdin.RateWindow{
					UsedPercentage: 50.0, // below threshold — no suffix expected for 7d here
					ResetsAt:       resetUnix,
				},
			}
			d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

			for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact} {
				got := p.Render(d, cfg, th, level)
				// The pct suffix should appear as " NN%" (space + digits + percent sign).
				suffix := " " + tc.pctStr + "%"
				hasSuffix := strings.Contains(got, suffix)
				if tc.wantSuffix && !hasSuffix {
					t.Errorf("T-23 %s level=%v: want %q in render output (pct≥90 triggers suffix), got %q",
						tc.name, level, suffix, got)
				}
				if !tc.wantSuffix && hasSuffix {
					// Only fail if the suffix appears in a contextually pct-suffix position;
					// the Minimal-level "NN% · NN%" would contain "85%" but that's not a
					// bar-adjacent suffix. Since we are testing Full/Compact, a false match
					// would be ↻…85%… which would indicate a real bug.
					t.Errorf("T-23 %s level=%v: must NOT contain %q (pct<90 no suffix), got %q",
						tc.name, level, suffix, got)
				}
			}
		})
	}
}

// TestQuotaRender_5hResetColour (T-24) verifies the gradient colour rule for
// the 5-hour reset countdown:
//
//	> 60m           → no colour marker
//	≤ 60m && > 30m  → {{color:green}}
//	≤ 30m && > 10m  → {{color:orange}}
//	≤ 10m           → {{color:red}}
//
// Spec §2.3: "5h reset — >60m no marker; >30m&&<=60m green; >10m&&<=30m orange; <=10m red".
//
// Current behaviour: single threshold < 30m → yellow; no green/orange/red gradient → RED.
func TestQuotaRender_5hResetColour(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	// AnsiEnabled=true so colour markers are emitted in raw form.
	th := renderer.Theme{AnsiEnabled: true, Colors: renderer.DefaultPalette()}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		resetIn     time.Duration // duration from now until reset
		wantMarker  string        // expected colour marker; "" means no marker
		noMarker    bool          // true when no colour marker expected
	}{
		{
			name:       "45m_green",
			resetIn:    45 * time.Minute,
			wantMarker: "{{color:green}}",
		},
		{
			name:       "20m_orange",
			resetIn:    20 * time.Minute,
			wantMarker: "{{color:orange}}",
		},
		{
			name:       "5m_red",
			resetIn:    5 * time.Minute,
			wantMarker: "{{color:red}}",
		},
		{
			name:     "90m_no_marker",
			resetIn:  90 * time.Minute,
			noMarker: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetUnix := json.RawMessage(fmt.Sprintf("%d", now.Add(tc.resetIn).Unix()))
			// 7d reset is far in the future so it doesn't interfere with colour assertions.
			farFuture := json.RawMessage(fmt.Sprintf("%d", now.Add(72*time.Hour).Unix()))

			rl := &stdin.RateLimits{
				FiveHour: stdin.RateWindow{
					UsedPercentage: 50.0,
					ResetsAt:       resetUnix,
				},
				SevenDay: stdin.RateWindow{
					UsedPercentage: 30.0,
					ResetsAt:       farFuture,
				},
			}
			d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

			got := p.Render(d, cfg, th, probes.LevelFull)

			if tc.noMarker {
				// None of the gradient markers should appear for the 5h part.
				// We verify by checking that the 5h reset portion (which is before the "·")
				// does not contain any colour marker.
				fiveHourPart := got
				if idx := strings.Index(got, " · "); idx >= 0 {
					fiveHourPart = got[:idx]
				}
				for _, forbidden := range []string{"{{color:green}}", "{{color:orange}}", "{{color:red}}", "{{color:yellow}}"} {
					if strings.Contains(fiveHourPart, forbidden) {
						t.Errorf("T-24 %s: 5h part must have no colour marker for reset>60m, but found %q in %q",
							tc.name, forbidden, fiveHourPart)
					}
				}
			} else {
				// The 5h part must contain the expected colour marker.
				fiveHourPart := got
				if idx := strings.Index(got, " · "); idx >= 0 {
					fiveHourPart = got[:idx]
				}
				if !strings.Contains(fiveHourPart, tc.wantMarker) {
					t.Errorf("T-24 %s: 5h reset countdown: want marker %q in 5h part %q (full: %q)",
						tc.name, tc.wantMarker, fiveHourPart, got)
				}
				// Must NOT contain the old yellow marker (regression guard).
				if strings.Contains(fiveHourPart, "{{color:yellow}}") {
					t.Errorf("T-24 %s: 5h reset must use gradient (not yellow); got %q",
						tc.name, fiveHourPart)
				}
			}
		})
	}
}

// TestQuotaRender_7dResetColour (T-25) verifies the gradient colour rule for
// the 7-day reset countdown:
//
//	> 2d            → no colour marker
//	≤ 2d && > 1d    → {{color:green}}
//	≤ 24h && > 5h   → {{color:orange}}
//	≤ 5h            → {{color:red}}
//
// Spec §2.3: "7d reset — >2d no marker; >1d&&<=2d green; >5h&&<=24h orange; <=5h red".
//
// Current behaviour: single threshold < 30m → yellow; no gradient for 7d → RED.
func TestQuotaRender_7dResetColour(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{AnsiEnabled: true, Colors: renderer.DefaultPalette()}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Keep 5h reset far in the future so it doesn't produce markers that
	// could interfere with assertions on the 7d part.
	farFuture5h := json.RawMessage(fmt.Sprintf("%d", now.Add(3*time.Hour).Unix()))

	cases := []struct {
		name       string
		resetIn    time.Duration
		wantMarker string
		noMarker   bool
	}{
		{
			name:       "36h_green",
			resetIn:    36 * time.Hour,
			wantMarker: "{{color:green}}",
		},
		{
			name:       "12h_orange",
			resetIn:    12 * time.Hour,
			wantMarker: "{{color:orange}}",
		},
		{
			name:       "3h_red",
			resetIn:    3 * time.Hour,
			wantMarker: "{{color:red}}",
		},
		{
			name:     "50h_no_marker",
			resetIn:  50 * time.Hour,
			noMarker: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetUnix := json.RawMessage(fmt.Sprintf("%d", now.Add(tc.resetIn).Unix()))

			rl := &stdin.RateLimits{
				FiveHour: stdin.RateWindow{
					UsedPercentage: 30.0,
					ResetsAt:       farFuture5h,
				},
				SevenDay: stdin.RateWindow{
					UsedPercentage: 40.0,
					ResetsAt:       resetUnix,
				},
			}
			d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

			got := p.Render(d, cfg, th, probes.LevelFull)

			// Isolate the 7d part (after the "·" separator).
			sevenDayPart := got
			if idx := strings.Index(got, " · "); idx >= 0 {
				sevenDayPart = got[idx+3:] // skip " · "
			}

			if tc.noMarker {
				for _, forbidden := range []string{"{{color:green}}", "{{color:orange}}", "{{color:red}}", "{{color:yellow}}"} {
					if strings.Contains(sevenDayPart, forbidden) {
						t.Errorf("T-25 %s: 7d part must have no colour marker for reset>2d, but found %q in %q",
							tc.name, forbidden, sevenDayPart)
					}
				}
			} else {
				if !strings.Contains(sevenDayPart, tc.wantMarker) {
					t.Errorf("T-25 %s: 7d reset countdown: want marker %q in 7d part %q (full: %q)",
						tc.name, tc.wantMarker, sevenDayPart, got)
				}
				// Must NOT use old yellow marker (regression guard).
				if strings.Contains(sevenDayPart, "{{color:yellow}}") {
					t.Errorf("T-25 %s: 7d reset must use gradient (not yellow); got %q",
						tc.name, sevenDayPart)
				}
			}
		})
	}
}
