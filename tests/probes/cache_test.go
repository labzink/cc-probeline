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

// TestCache_NoTTL verifies that the TTL block (⏱…) never appears in any Level
// output during Phase 4.1, because the cache-events detector is not yet
// implemented (it arrives in Phase 4.4).
//
// This test acts as a regression guard: if ⏱ appears, something is wrong.
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
