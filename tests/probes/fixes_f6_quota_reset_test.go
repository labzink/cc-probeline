// Package probes_test — RED tests for F6: quota reset-time desync fix.
//
// F6 contract: pct and reset-countdown must come from the SAME source —
// the freshest cross-session snapshot (quota.Freshest()). Currently the code
// uses quota.Freshest() for pct but d.Stdin.RateLimits for reset-time, causing
// desync after a window reset.
//
// After the fix, QuotaProbe.Render must:
//  1. Read reset-time from snap.FiveHourReset / snap.SevenDayReset (unix sec).
//  2. Ignore d.Stdin.RateLimits.FiveHour.ResetsAt / SevenDay.ResetsAt for reset.
//  3. When RateLimits is nil but a valid snapshot exists, still render reset-time
//     from the snapshot (today: reset-countdown is skipped because `if rl != nil`
//     gates the whole reset block → RED).
//
// These tests are RED today because:
//   - reset-time is computed from rl.FiveHour.ResetsAt / rl.SevenDay.ResetsAt.
//   - When rl (RateLimits) is nil, reset-time is always "".
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

// f6snapNow writes a quota.Snapshot with the given pct/reset values to a temp
// quota dir and returns d.Now for use in subsequent Render calls.
// The caller must call t.Setenv("CC_PROBELINE_QUOTA_DIR", dir) before this.
func f6snapNow(t *testing.T, fiveHourPct, sevenDayPct float64, fiveHourResetUnix, sevenDayResetUnix int64, now time.Time) {
	t.Helper()
	snap := quota.Snapshot{
		TS:            now.UnixMilli(),
		FiveHourPct:   fiveHourPct,
		SevenDayPct:   sevenDayPct,
		FiveHourReset: fiveHourResetUnix,
		SevenDayReset: sevenDayResetUnix,
	}
	if err := quota.Update(snap); err != nil {
		t.Fatalf("f6snapNow: quota.Update: %v", err)
	}
}

// TestF6_5hReset_FromSnapshot verifies that when a freshest snapshot carries
// FiveHourReset and d.Stdin.RateLimits carries a DIFFERENT ResetsAt, the
// rendered reset-countdown matches snap.FiveHourReset, NOT RateLimits.ResetsAt.
//
// Setup:
//   - snap.FiveHourReset = now+4h (freshly reset window, ~4h remaining)
//   - rl.FiveHour.ResetsAt = now+30m (stale old window, nearly expired)
//
// Expected: reset-countdown ≈ "↻ 4h:0m" (from snapshot), not "↻ 0h:30m" (from rl).
// Current code reads rl → RED.
func TestF6_5hReset_FromSnapshot(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

	// Snapshot says the window just reset — 4 hours remain.
	snapFiveHourReset := now.Add(4 * time.Hour).Unix()
	snapSevenDayReset := now.Add(72 * time.Hour).Unix()
	f6snapNow(t, 5.0, 10.0, snapFiveHourReset, snapSevenDayReset, now)

	// RateLimits carries stale old-window value: only 30 minutes remain.
	// If the code reads from RateLimits, the countdown will be "↻ 0h:30m".
	staleResetsAt := now.Add(30 * time.Minute).Unix()
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 5.0,
			// Encode as plain unix int, matching ParseResetsAt integer branch.
			ResetsAt: []byte(intToJSON(staleResetsAt)),
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 10.0,
			ResetsAt:       []byte(intToJSON(now.Add(72 * time.Hour).Unix())),
		},
	}
	d := probes.Data{
		Now:   now,
		Stdin: stdin.Payload{RateLimits: rl},
	}

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// The 5h part appears before " · 7d:".
	fivePart := got
	if idx := strings.Index(got, " · "); idx >= 0 {
		fivePart = got[:idx]
	}

	// Snapshot has 4h remaining — must NOT contain short "30m" countdown.
	if strings.Contains(fivePart, "0h:30m") || strings.Contains(fivePart, "30m") {
		t.Errorf("F6/5h: reset-countdown came from RateLimits (stale), got 5h part=%q full=%q; want countdown from snapshot (~4h)", fivePart, got)
	}
	// Must reflect the snapshot's ~4 hour remaining.
	if !strings.Contains(fivePart, "4h") {
		t.Errorf("F6/5h: reset-countdown must reflect snap.FiveHourReset (~4h), got 5h part=%q full=%q", fivePart, got)
	}
}

// TestF6_7dReset_FromSnapshot verifies that when a freshest snapshot carries
// SevenDayReset and d.Stdin.RateLimits carries a DIFFERENT ResetsAt, the
// rendered reset-countdown for 7d matches snap.SevenDayReset, NOT RateLimits.
//
// Setup:
//   - snap.SevenDayReset = now+6d (freshly reset window)
//   - rl.SevenDay.ResetsAt = now+2h (stale, nearly expired)
//
// Expected: 7d part contains "6d" countdown (from snapshot), not "2h" (from rl).
// Current code reads rl → RED.
func TestF6_7dReset_FromSnapshot(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

	// Snapshot: 7d window just reset, 6 days remaining.
	snapFiveHourReset := now.Add(3 * time.Hour).Unix()
	snapSevenDayReset := now.Add(6 * 24 * time.Hour).Unix()
	f6snapNow(t, 3.0, 2.0, snapFiveHourReset, snapSevenDayReset, now)

	// RateLimits: stale 7d, only 2 hours remaining.
	stale7dResetsAt := now.Add(2 * time.Hour).Unix()
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{
			UsedPercentage: 3.0,
			ResetsAt:       []byte(intToJSON(now.Add(3 * time.Hour).Unix())),
		},
		SevenDay: stdin.RateWindow{
			UsedPercentage: 2.0,
			ResetsAt:       []byte(intToJSON(stale7dResetsAt)),
		},
	}
	d := probes.Data{
		Now:   now,
		Stdin: stdin.Payload{RateLimits: rl},
	}

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// The 7d part appears after " · 7d: ".
	sevenPart := got
	if idx := strings.Index(got, " · "); idx >= 0 {
		sevenPart = got[idx+3:]
	}

	// RateLimits has 2h remaining — must NOT contain "2h" countdown for 7d.
	if strings.Contains(sevenPart, "↻ 2h") {
		t.Errorf("F6/7d: reset-countdown came from RateLimits (stale 2h), got 7d part=%q full=%q; want countdown from snapshot (~6d)", sevenPart, got)
	}
	// Must reflect the snapshot's ~6 days remaining.
	if !strings.Contains(sevenPart, "6d") {
		t.Errorf("F6/7d: reset-countdown must reflect snap.SevenDayReset (~6d), got 7d part=%q full=%q", sevenPart, got)
	}
}

// TestF6_NilRateLimits_SnapshotProvidesReset verifies that when RateLimits is nil
// but a fresh snapshot exists with FiveHourReset/SevenDayReset set, the
// reset-countdown is still rendered from the snapshot.
//
// Today: the `if rl != nil` block (quota.go:106) gates reset rendering entirely —
// so when rl is nil, reset is always "". That causes the visible output to show
// the progress bars without any countdown, even though the snapshot has the data.
//
// After fix: `reset5h = formatResetFromUnix(snap.FiveHourReset, now, ...)` — no rl needed.
// Current code → reset-countdown missing → RED.
func TestF6_NilRateLimits_SnapshotProvidesReset(t *testing.T) {
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

	// Snapshot has both pct and reset times.
	fiveHourReset := now.Add(2 * time.Hour).Unix()
	sevenDayReset := now.Add(48 * time.Hour).Unix()
	f6snapNow(t, 40.0, 20.0, fiveHourReset, sevenDayReset, now)

	// RateLimits is nil — no session-local payload available.
	d := probes.Data{
		Now:   now,
		Stdin: stdin.Payload{RateLimits: nil},
	}

	p := &probes.QuotaProbe{}
	cfg := probes.Config{QuotaEnabled: true}
	th := renderer.Theme{}

	// Probe must be visible (snapshot exists).
	if !p.Visible(d, cfg) {
		t.Fatal("F6/nil-rl: QuotaProbe.Visible must return true when snapshot exists even if RateLimits=nil")
	}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// The output must include a reset-countdown derived from the snapshot.
	// With 2h remaining on 5h and 48h remaining on 7d, we expect "↻ 2h" and "↻ 2d".
	if !strings.Contains(got, "↻") {
		t.Errorf("F6/nil-rl: reset-countdown must be rendered from snapshot when RateLimits=nil, got %q (no ↻ symbol)", got)
	}
	// Verify 5h reset is present (~2h).
	if !strings.Contains(got, "2h") {
		t.Errorf("F6/nil-rl: 5h reset-countdown from snapshot (~2h) missing in %q", got)
	}
}

// intToJSON formats a unix timestamp as a plain JSON integer string.
// Matches the integer branch of stdin.ParseResetsAt.
func intToJSON(unix int64) string {
	return formatInt64(unix)
}

// formatInt64 converts int64 to decimal string without importing fmt to avoid
// an import cycle in the test file (fmt is already imported via other files).
// Since probes_test is a multi-file package, fmt is available; use it directly.
func formatInt64(v int64) string {
	// Use strconv to avoid ambiguity with fmt.Sprintf already imported elsewhere.
	return string(appendInt64(nil, v))
}

func appendInt64(buf []byte, v int64) []byte {
	if v < 0 {
		buf = append(buf, '-')
		v = -v
	}
	var tmp [20]byte
	i := len(tmp)
	for v >= 10 {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
	}
	i--
	tmp[i] = byte('0' + v)
	return append(buf, tmp[i:]...)
}
