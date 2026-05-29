// Package probes_test — black-box tests for CacheProbe.
// Covers visibility (nil Session → hidden), rendering across Full/Compact/Minimal
// levels with known token counts, and the TTL-absent guarantee for Phase 4.1.
//
// CacheProbe renders the entire row-2 aggregate:
//
//	Full:    "cache <read>K/<create>K | out <out>K | cost: $<cost> | time: MM:SS"
//	Compact: "<read>K/<create>K | <out>K | $<cost> | MM:SS"   (drop labels)
//	Minimal: "<read>K/<create>K | <out>K | $<cost>"           (drop time block)
//
// In Phase 4.1 the TTL detector (cache_events, Phase 4.4) is absent; the ⏱…
// block must never appear in output.
//
// Token sources (Phase 4.1):
//
//	cache_read   = Session.Totals.CacheRead
//	cache_create = Session.Totals.CacheCreate
//	out          = Session.Totals.Output
//	cost         = Stdin.Cost.TotalCostUSD
//	time         = Stdin.Cost.TotalAPIDurationMS / 1000  (seconds → MM:SS)
package probes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// newCacheData builds a probes.Data with the given token counts and cost.
// TotalAPIDurationMS=228000 → 228 s → 3:48.
func newCacheData(cacheRead, cacheCreate, out int, costUSD float64, durationMS int64) probes.Data {
	return probes.Data{
		Session: &parser.SessionStats{
			Totals: parser.TokenCounts{
				CacheRead:   cacheRead,
				CacheCreate: cacheCreate,
				Output:      out,
			},
		},
		Stdin: stdin.Payload{
			Cost: stdin.Cost{
				TotalCostUSD:       costUSD,
				TotalAPIDurationMS: durationMS,
			},
		},
	}
}

// TestCache_Visible_NoTurns verifies that CacheProbe.Visible returns false when
// Session is nil (no parsed session data available).
func TestCache_Visible_NoTurns(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := probes.Config{}
	d := probes.Data{Session: nil}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(Session=nil): want false, got true")
	}
}

// TestCache_Render_Full verifies CacheProbe rendering at LevelFull with
// representative token numbers:
//
//	cache_read=108000 → 108K, cache_create=24000 → 24K
//	out=13000 → 13K
//	cost=$0.57
//	time=228000 ms → 228 s → "03:48"
//
// Expected: "cache 108K/24K | out 13K | cost: $0.57 | time: 03:48"
func TestCache_Render_Full(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	th := renderer.Theme{}
	d := newCacheData(108000, 24000, 13000, 0.57, 228000)

	got := p.Render(d, cfg, th, probes.LevelFull)
	want := "cache 108K/24K | out 13K | cost: $0.57 | time: 03:48"
	if got != want {
		t.Errorf("Render(Full): want %q, got %q", want, got)
	}
}

// TestCache_Render_Compact verifies CacheProbe rendering at LevelCompact:
// labels ("cache ", "out ", "cost: ", "time: ") are dropped per §A4 P2.
//
// Expected: "108K/24K | 13K | $0.57 | 03:48"
func TestCache_Render_Compact(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	th := renderer.Theme{}
	d := newCacheData(108000, 24000, 13000, 0.57, 228000)

	got := p.Render(d, cfg, th, probes.LevelCompact)
	want := "108K/24K | 13K | $0.57 | 03:48"
	if got != want {
		t.Errorf("Render(Compact): want %q, got %q", want, got)
	}
}

// TestCache_Render_Minimal verifies CacheProbe rendering at LevelMinimal:
// time block is dropped entirely.
//
// Expected: "108K/24K | 13K | $0.57"
func TestCache_Render_Minimal(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	th := renderer.Theme{}
	d := newCacheData(108000, 24000, 13000, 0.57, 228000)

	got := p.Render(d, cfg, th, probes.LevelMinimal)
	want := "108K/24K | 13K | $0.57"
	if got != want {
		t.Errorf("Render(Minimal): want %q, got %q", want, got)
	}
}

// TestCache_NoTTL verifies that the TTL block (⏱…) never appears when the
// session has no TTL context: Config.OrchTTLMinutes=0 and d.Session.LastTimestamp
// is zero and TurnCount is zero (newCacheData does not set these fields).
// TTL is hidden by CacheProbe when OrchTTLMinutes=0 or LastTimestamp/TurnCount
// are zero — this test acts as a regression guard for that suppression logic.
func TestCache_NoTTL(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := newCacheData(108000, 24000, 13000, 0.57, 228000)

	for _, level := range []probes.Level{probes.LevelFull, probes.LevelCompact, probes.LevelMinimal} {
		got := p.Render(d, cfg, th, level)
		if strings.Contains(got, "⏱") {
			t.Errorf("Render(%v): TTL block ⏱ must not appear in Phase 4.1 output, got %q",
				level, got)
		}
	}
}

// newCacheTTLData builds a probes.Data with token counts, cost, and timing
// information needed for TTL tests (Phase 6.5.b2).
// durationMS=60000 → 60 s → "01:00".
func newCacheTTLData(cacheRead, cacheCreate, out int, costUSD float64, durationMS int64, now time.Time, lastTS time.Time, turnCount int) probes.Data {
	return probes.Data{
		Session: &parser.SessionStats{
			Totals: parser.TokenCounts{
				CacheRead:   cacheRead,
				CacheCreate: cacheCreate,
				Output:      out,
			},
			LastTimestamp: lastTS,
			TurnCount:     turnCount,
		},
		Stdin: stdin.Payload{
			Cost: stdin.Cost{
				TotalCostUSD:       costUSD,
				TotalAPIDurationMS: durationMS,
			},
		},
		Now: now,
	}
}

// T-6: TestCacheProbe_TTL_Full verifies that a non-expired TTL renders ⏱Nm
// in Full and Compact output, and is absent in Minimal.
//
// Setup: OrchTTLMinutes=60, d.Now=10:08:00, LastTimestamp=10:00:00 (8 min ago),
// TurnCount=3. remaining = 60 - floor(8) = 52 → ⏱52m.
func TestCacheProbe_TTL_Full(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	cfg.OrchTTLMinutes = 60
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 8, 0, 0, time.UTC)
	lastTS := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	d := newCacheTTLData(1000, 2000, 500, 0.10, 60000, now, lastTS, 3)

	fullOut := p.Render(d, cfg, th, probes.LevelFull)
	if !strings.Contains(fullOut, "⏱52m") {
		t.Errorf("Render(Full): want ⏱52m in output, got %q", fullOut)
	}

	compactOut := p.Render(d, cfg, th, probes.LevelCompact)
	if !strings.Contains(compactOut, "⏱52m") {
		t.Errorf("Render(Compact): want ⏱52m in output, got %q", compactOut)
	}

	minimalOut := p.Render(d, cfg, th, probes.LevelMinimal)
	if strings.Contains(minimalOut, "⏱") {
		t.Errorf("Render(Minimal): ⏱ must not appear in Minimal output, got %q", minimalOut)
	}
}

// T-7: TestCacheProbe_TTL_Expired verifies that an expired TTL (remaining ≤ 0)
// produces no ⏱ block in any output level.
//
// Setup: OrchTTLMinutes=60, LastTimestamp=70 min ago (remaining = 60-70 = -10).
func TestCacheProbe_TTL_Expired(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	cfg.OrchTTLMinutes = 60
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	lastTS := now.Add(-70 * time.Minute)
	d := newCacheTTLData(1000, 2000, 500, 0.10, 60000, now, lastTS, 3)

	fullOut := p.Render(d, cfg, th, probes.LevelFull)
	if strings.Contains(fullOut, "⏱") {
		t.Errorf("Render(Full): ⏱ must not appear when TTL expired, got %q", fullOut)
	}

	compactOut := p.Render(d, cfg, th, probes.LevelCompact)
	if strings.Contains(compactOut, "⏱") {
		t.Errorf("Render(Compact): ⏱ must not appear when TTL expired, got %q", compactOut)
	}
}

// T-8: TestCacheProbe_TTL_EmptySession verifies that ⏱ is omitted when
// LastTimestamp is zero or TurnCount is zero, regardless of OrchTTLMinutes.
func TestCacheProbe_TTL_EmptySession(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	cfg.OrchTTLMinutes = 60
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	// Case A: zero LastTimestamp, zero TurnCount.
	dZeroTS := newCacheTTLData(1000, 2000, 500, 0.10, 60000, now, time.Time{}, 0)
	gotA := p.Render(dZeroTS, cfg, th, probes.LevelFull)
	if strings.Contains(gotA, "⏱") {
		t.Errorf("Render(Full, zeroTS): ⏱ must not appear when LastTimestamp is zero, got %q", gotA)
	}

	// Case B: valid LastTimestamp but TurnCount=0.
	lastTS := now.Add(-5 * time.Minute)
	dZeroTurns := newCacheTTLData(1000, 2000, 500, 0.10, 60000, now, lastTS, 0)
	gotB := p.Render(dZeroTurns, cfg, th, probes.LevelFull)
	if strings.Contains(gotB, "⏱") {
		t.Errorf("Render(Full, zeroTurns): ⏱ must not appear when TurnCount=0, got %q", gotB)
	}
}

// T-9: TestCacheProbe_TTL_Minimal verifies that ⏱ never appears in Minimal
// output, even when the TTL is valid and would show at Full/Compact levels.
//
// Uses the same setup as T-6 (52m remaining).
func TestCacheProbe_TTL_Minimal(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	cfg.OrchTTLMinutes = 60
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 8, 0, 0, time.UTC)
	lastTS := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	d := newCacheTTLData(1000, 2000, 500, 0.10, 60000, now, lastTS, 3)

	got := p.Render(d, cfg, th, probes.LevelMinimal)
	if strings.Contains(got, "⏱") {
		t.Errorf("Render(Minimal): ⏱ must never appear in Minimal output, got %q", got)
	}
}
