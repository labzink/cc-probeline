// Package probes_test — black-box tests for QuotaProbe reset-time resolution
// (Phase 6.9, R2/R3): unknown reset → "↻ ??m" plain; cross-session fallback to
// the persisted snapshot reset when the live stdin window is unknown.
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

// TestQuota_ResetUnknownTime (R2) verifies that when the reset time is unknown
// (live stdin resets_at absent AND no persisted snapshot), the probe renders
// "↻ ??m" — not the misleading "↻ 0m". The window just reset and the next one
// has not started ticking yet.
func TestQuota_ResetUnknownTime(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir()) // empty dir → no snapshot

	p := &probes.QuotaProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{QuotaEnabled: true}
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 0, ResetsAt: nil},
		SevenDay: stdin.RateWindow{UsedPercentage: 0, ResetsAt: nil},
	}
	d := probes.Data{Now: time.Now(), Stdin: stdin.Payload{RateLimits: rl}}

	got := p.Render(d, cfg, th, probes.LevelFull)
	if !strings.Contains(got, "↻ ??m") {
		t.Errorf("R2: want '↻ ??m' for unknown reset time, got %q", got)
	}
	if strings.Contains(got, "↻ 0m") {
		t.Errorf("R2: must not show '↻ 0m' when reset time is unknown, got %q", got)
	}
}

// TestQuota_ResetFallsBackToSnapshot (R3) verifies cross-session reset display:
// when the live stdin window has no reset time but the persisted snapshot does
// (an active session observed the new window), the probe shows the snapshot's
// countdown rather than "↻ ??m".
func TestQuota_ResetFallsBackToSnapshot(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Now()
	snap5h := now.Add(40*time.Minute + 30*time.Second).Unix() // ≈ 40m ahead
	if err := quota.Update(quota.Snapshot{
		TS: now.UnixMilli(), FiveHourPct: 12, SevenDayPct: 5,
		FiveHourReset: snap5h, SevenDayReset: now.Add(72 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	p := &probes.QuotaProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{QuotaEnabled: true}
	// This idle session has not observed the new window: live reset is absent.
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 12, ResetsAt: nil},
		SevenDay: stdin.RateWindow{UsedPercentage: 5, ResetsAt: nil},
	}
	d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

	got := p.Render(d, cfg, th, probes.LevelFull)
	if strings.Contains(got, "↻ ??m") {
		t.Errorf("R3: snapshot reset must be used as fallback (not ??m), got %q", got)
	}
	if !strings.Contains(got, "↻ 0h:40m") {
		t.Errorf("R3: want snapshot-derived countdown '↻ 0h:40m', got %q", got)
	}
}
