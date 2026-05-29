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
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestQuota_Visible_Disabled verifies that QuotaProbe.Visible returns false
// when Config.QuotaEnabled is false, regardless of other fields.
func TestQuota_Visible_Disabled(t *testing.T) {
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
// Updated in Phase 6.5.b4: uses real RateLimits data (stubs removed).
// 5h=23% → ProgressBar floors to 20% → bar "█░░░░"
// 7d=41% → ProgressBar floors to 40% → bar "██░░░"
// reset shown from resets_at Unix timestamps.
func TestQuota_Render_Full(t *testing.T) {
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
	want := "5h: █░░░░ ↻2h13m · 7d: ██░░░ ↻3d12h"
	if got != want {
		t.Errorf("Render(Full): want %q, got %q", want, got)
	}
}

// TestQuota_Render_Compact verifies QuotaProbe.Render at LevelCompact.
// Labels "5h: " and "7d: " are dropped per §A4 P1.
// Updated in Phase 6.5.b4: uses real RateLimits data.
func TestQuota_Render_Compact(t *testing.T) {
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
	want := "█░░░░ ↻2h13m · ██░░░ ↻3d12h"
	if got != want {
		t.Errorf("Render(Compact): want %q, got %q", want, got)
	}
}

// TestQuota_Render_Minimal verifies QuotaProbe.Render at LevelMinimal.
// Progress bars and time-to-reset are dropped; only percent values remain.
// Updated in Phase 6.5.b4: uses real RateLimits data; format is integer %.
func TestQuota_Render_Minimal(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 23.0},
		SevenDay: stdin.RateWindow{UsedPercentage: 41.0},
	}
	d := probes.Data{Stdin: stdin.Payload{RateLimits: rl}}

	got := p.Render(d, cfg, th, probes.LevelMinimal)
	want := "23% · 41%"
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
//	5h: UsedPercentage=40%, resets_at = now+133min → 2h13m  → bar "██░░░"
//	    40% floor→40; 40/20=2 full segs → "██░░░"
//	7d: UsedPercentage=60%, resets_at = now+84h   → 3d12h  → bar "███░░"
//	    60% floor→60; 60/20=3 full segs → "███░░"
//
// RED: fails until QuotaProbe.Render reads real RateLimits from d.Stdin.
func TestQuotaProbe_RealRender(t *testing.T) {
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

	wantFull := "5h: ██░░░ ↻2h13m · 7d: ███░░ ↻3d12h"
	if got := p.Render(d, cfg, th, probes.LevelFull); got != wantFull {
		t.Errorf("Render(Full): want %q, got %q", wantFull, got)
	}

	wantCompact := "██░░░ ↻2h13m · ███░░ ↻3d12h"
	if got := p.Render(d, cfg, th, probes.LevelCompact); got != wantCompact {
		t.Errorf("Render(Compact): want %q, got %q", wantCompact, got)
	}

	wantMinimal := "40% · 60%"
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
			if !containsStr(minimal, tc.wantMinimal) {
				t.Errorf("Render(Minimal) case %s: want %q in %q", tc.name, tc.wantMinimal, minimal)
			}
		})
	}
}

// containsStr is a local helper to avoid importing strings in this package.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
