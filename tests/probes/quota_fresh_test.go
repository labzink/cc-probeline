// Package probes_test — RED tests for Phase 6.8.b quota freshness.
//
// Tests T-Q3 and T-Q4 verify that QuotaProbe reads from quota.Freshest()
// across sessions and applies bold_red colour when usage exceeds 95%.
//
// T-Q3: probe renders the freshest snapshot stored via quota.Update; when the
//        snapshot is stale, renders "as of Xm ago" suffix.
// T-Q4: FiveHourPct > 95 or SevenDayPct > 95 → {{color:bold_red}} in raw render;
//        at or below 95 → no bold_red marker.
//
// Isolation: CC_PROBELINE_QUOTA_DIR is set to t.TempDir() so tests do not
// touch real user state files.
package probes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/quota"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestQuotaProbe_FreshAcrossSessions (T-Q3 / T-12) verifies that QuotaProbe
// renders data from quota.Freshest(), not from d.Stdin.RateLimits alone.
//
// The test simulates the "fresh across sessions" contract:
//  1. A previous session wrote a snapshot with FiveHourPct=67 via quota.Update.
//  2. The current session has d.Stdin.RateLimits with FiveHourPct=30 (stale payload).
//  3. QuotaProbe.Render must output "67" (from Freshest), not "30" (from Stdin).
//
// Sub-case 2 verifies that when the snapshot is older than the staleness
// threshold (> 10 minutes ago), the rendered output contains "as of" and
// a minute-count (e.g. "as of 15m ago").
func TestQuotaProbe_FreshAcrossSessions(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{} // plain-text; colour tested separately in T-Q4

	// Sub-case A: fresh snapshot (just written) — probe renders freshest pct.
	t.Run("renders_freshest_pct", func(t *testing.T) {
		t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

		now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		snap := quota.Snapshot{
			TS:          now.UnixMilli(),
			FiveHourPct: 67.0,
			SevenDayPct: 42.0,
		}
		if err := quota.Update(snap); err != nil {
			t.Fatalf("quota.Update: %v", err)
		}

		// d.Stdin.RateLimits carries stale 30% — probe must prefer Freshest().
		rl := &stdin.RateLimits{
			FiveHour: stdin.RateWindow{UsedPercentage: 30.0},
			SevenDay: stdin.RateWindow{UsedPercentage: 20.0},
		}
		d := probes.Data{
			Now:   now,
			Stdin: stdin.Payload{RateLimits: rl},
		}

		got := p.Render(d, cfg, th, probes.LevelMinimal)
		if !strings.Contains(got, "67") {
			t.Errorf("T-Q3 render_freshest_pct: want '67' (from Freshest) in %q; probe must read from quota.Freshest(), not d.Stdin.RateLimits", got)
		}
		// Must NOT contain the stale payload value '30' as the leading pct.
		if strings.HasPrefix(got, "30") {
			t.Errorf("T-Q3 render_freshest_pct: got stale value '30' as prefix in %q; probe must use Freshest, not Stdin.RateLimits", got)
		}
	})

	// Sub-case B: snapshot older than staleness threshold — output must contain "as of".
	t.Run("stale_snapshot_shows_age", func(t *testing.T) {
		t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

		now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		// TS is 15 minutes in the past — exceeds any reasonable staleness threshold.
		staleTS := now.Add(-15 * time.Minute).UnixMilli()
		snap := quota.Snapshot{
			TS:          staleTS,
			FiveHourPct: 55.0,
			SevenDayPct: 33.0,
		}
		if err := quota.Update(snap); err != nil {
			t.Fatalf("quota.Update: %v", err)
		}

		rl := &stdin.RateLimits{
			FiveHour: stdin.RateWindow{UsedPercentage: 55.0},
			SevenDay: stdin.RateWindow{UsedPercentage: 33.0},
		}
		d := probes.Data{
			Now:   now,
			Stdin: stdin.Payload{RateLimits: rl},
		}

		got := p.Render(d, cfg, th, probes.LevelFull)
		if !strings.Contains(got, "as of") {
			t.Errorf("T-Q3 stale_snapshot_shows_age: want 'as of' in render output for stale snapshot, got %q", got)
		}
		// Must contain a minute count (e.g. "15m").
		if !strings.Contains(got, "m ago") {
			t.Errorf("T-Q3 stale_snapshot_shows_age: want 'Xm ago' in render output, got %q", got)
		}
	})
}

// TestQuotaProbe_BoldRedAbove95 (T-Q4) verifies the bold_red colour rule:
//   - FiveHourPct > 95 → raw Render output contains "{{color:bold_red}}".
//   - SevenDayPct > 95 → raw Render output contains "{{color:bold_red}}".
//   - Both ≤ 95 → raw Render output does NOT contain "{{color:bold_red}}".
//
// The test inspects the raw marker string (before renderer.Apply) because
// the colour contract is expressed at the marker level; Apply converts it
// to an ANSI code in a separate step already tested in colour_test.go.
func TestQuotaProbe_BoldRedAbove95(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	// AnsiEnabled=true and BoldRed populated so that Apply would resolve the marker;
	// but we assert on the raw marker string, not on the post-Apply ANSI code.
	th := renderer.Theme{AnsiEnabled: true, Colors: renderer.DefaultPalette()}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	const boldRedMarker = "{{color:bold_red}}"

	cases := []struct {
		name         string
		fiveHourPct  float64
		sevenDayPct  float64
		wantBoldRed  bool
	}{
		{
			name:        "five_hour_above_95_triggers_bold_red",
			fiveHourPct: 96.0,
			sevenDayPct: 50.0,
			wantBoldRed: true,
		},
		{
			name:        "seven_day_above_95_triggers_bold_red",
			fiveHourPct: 50.0,
			sevenDayPct: 97.5,
			wantBoldRed: true,
		},
		{
			name:        "both_at_95_no_bold_red",
			fiveHourPct: 95.0,
			sevenDayPct: 95.0,
			wantBoldRed: false,
		},
		{
			name:        "both_below_95_no_bold_red",
			fiveHourPct: 60.0,
			sevenDayPct: 40.0,
			wantBoldRed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

			snap := quota.Snapshot{
				TS:          now.UnixMilli(),
				FiveHourPct: tc.fiveHourPct,
				SevenDayPct: tc.sevenDayPct,
			}
			if err := quota.Update(snap); err != nil {
				t.Fatalf("quota.Update: %v", err)
			}

			rl := &stdin.RateLimits{
				FiveHour: stdin.RateWindow{UsedPercentage: tc.fiveHourPct},
				SevenDay: stdin.RateWindow{UsedPercentage: tc.sevenDayPct},
			}
			d := probes.Data{
				Now:   now,
				Stdin: stdin.Payload{RateLimits: rl},
			}

			// Inspect raw render output (before Apply) for the marker.
			raw := p.Render(d, cfg, th, probes.LevelMinimal)

			hasBoldRed := strings.Contains(raw, boldRedMarker)
			if tc.wantBoldRed && !hasBoldRed {
				t.Errorf("T-Q4 %s: FiveHourPct=%.1f SevenDayPct=%.1f; want %q in raw render, got %q",
					tc.name, tc.fiveHourPct, tc.sevenDayPct, boldRedMarker, raw)
			}
			if !tc.wantBoldRed && hasBoldRed {
				t.Errorf("T-Q4 %s: FiveHourPct=%.1f SevenDayPct=%.1f ≤ 95; must NOT contain %q in raw render, got %q",
					tc.name, tc.fiveHourPct, tc.sevenDayPct, boldRedMarker, raw)
			}
		})
	}
}
