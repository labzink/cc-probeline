// Package probes_test — tests for QuotaProbe snapshot-expiry handling.
//
// Bug (Phase 6.95): quota.Freshest() returns a persisted snapshot whose
// used-percentage is shown verbatim, with no check that the snapshot's own
// rate-limit window has already rolled over. After a laptop sleeps overnight
// (or right after /clear, before CC sends fresh rate_limits) the stored 90%
// keeps showing even though the 5-hour window reset hours ago and is now ~0%.
//
// Fix contract:
//   - A window whose reset time is known and in the past (reset>0 && now>=reset)
//     reads 0% — it has rolled over.
//   - A window whose reset time is unknown (reset==0) but whose snapshot is older
//     than the window length (>5h for 5h, >7d for 7d) reads 0% — it must have
//     rolled over at least once.
//   - A window whose reset is still in the future keeps its stored percentage.
//
// Each window is evaluated independently. Isolation via CC_PROBELINE_QUOTA_DIR.
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

// TestQuotaProbe_ExpiredWindowReadsZero verifies that a snapshot whose window
// has already rolled over no longer shows its stale high percentage.
func TestQuotaProbe_ExpiredWindowReadsZero(t *testing.T) {
	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{} // plain text — assert on bare "NN%"

	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		snap       quota.Snapshot
		want5hZero bool
		want7dZero bool
	}{
		{
			// Laptop-sleep / post-clear: both reset times are in the past.
			name: "both_resets_in_past",
			snap: quota.Snapshot{
				TS:            now.Add(-6 * time.Hour).UnixMilli(),
				FiveHourPct:   90,
				SevenDayPct:   80,
				FiveHourReset: now.Add(-1 * time.Hour).Unix(),
				SevenDayReset: now.Add(-1 * time.Hour).Unix(),
			},
			want5hZero: true,
			want7dZero: true,
		},
		{
			// Reset unknown (CC sent null) but snapshot older than both windows.
			name: "unknown_reset_older_than_both_windows",
			snap: quota.Snapshot{
				TS:          now.Add(-8 * 24 * time.Hour).UnixMilli(),
				FiveHourPct: 90,
				SevenDayPct: 80,
			},
			want5hZero: true,
			want7dZero: true,
		},
		{
			// Reset unknown, age between the two window lengths: only 5h rolled over.
			name: "unknown_reset_older_than_5h_only",
			snap: quota.Snapshot{
				TS:          now.Add(-10 * time.Hour).UnixMilli(),
				FiveHourPct: 90,
				SevenDayPct: 80,
			},
			want5hZero: true,
			want7dZero: false,
		},
		{
			// Both resets in the future: stored percentages are still valid.
			name: "both_resets_in_future",
			snap: quota.Snapshot{
				TS:            now.Add(-2 * time.Minute).UnixMilli(),
				FiveHourPct:   90,
				SevenDayPct:   80,
				FiveHourReset: now.Add(1 * time.Hour).Unix(),
				SevenDayReset: now.Add(48 * time.Hour).Unix(),
			},
			want5hZero: false,
			want7dZero: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())
			if err := quota.Update(tc.snap); err != nil {
				t.Fatalf("quota.Update: %v", err)
			}

			// rl nil simulates the first refresh after wake/clear: CC has not yet
			// sent fresh rate_limits, so the probe runs purely off the snapshot.
			d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: nil}}

			// LevelMinimal renders "<pct5h>% <reset5h> · <pct7d>% <reset7d>".
			got := p.Render(d, cfg, th, probes.LevelMinimal)
			parts := strings.SplitN(got, " · ", 2)
			if len(parts) != 2 {
				t.Fatalf("unexpected render shape %q", got)
			}
			left, right := parts[0], parts[1]

			check := func(side, segment string, wantZero bool, storedPct string) {
				if wantZero {
					if !strings.HasPrefix(segment, "0%") {
						t.Errorf("%s: window rolled over, want '0%%' prefix, got %q (full: %q)", side, segment, got)
					}
					if strings.Contains(segment, storedPct) {
						t.Errorf("%s: must not show stale %q, got %q", side, storedPct, got)
					}
				} else {
					if !strings.HasPrefix(segment, storedPct) {
						t.Errorf("%s: window still valid, want %q prefix, got %q (full: %q)", side, storedPct, segment, got)
					}
				}
			}
			check("5h", left, tc.want5hZero, "90%")
			check("7d", right, tc.want7dZero, "80%")
		})
	}
}
