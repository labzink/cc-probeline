// Package probes_test — RED tests for Phase 6.8.e cache separator dimming (T-E3).
//
// T-E3 (TestCache_DimSeparators): CacheProbe.Render(Full) must use
// "{{dim}} • {{reset}}" between fields, not plain " • ".
//
// Background: cache.go currently joins fields with " • " (plain ASCII bullet).
// Phase 6.8.e dev will replace every " • " in cache format strings with
// "{{dim}} • {{reset}}" so that the separator is visually dimmed, consistent
// with line0/line1 separators in the assembler.
//
// The test calls Render at all three levels and verifies:
//   - "{{dim}} • {{reset}}" is present  (dim separator required)
//   - plain " • " without markers is absent (plain separator forbidden)
package probes_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// newCacheDimData builds a probes.Data with enough fields to trigger all
// separator positions in CacheProbe.Render (cache + out + cost + time).
func newCacheDimData() probes.Data {
	return probes.Data{
		Session: &parser.SessionStats{
			Totals: parser.TokenCounts{
				CacheRead:   10000,
				CacheCreate: 5000,
				Output:      2000,
			},
		},
		Stdin: stdin.Payload{
			Cost: stdin.Cost{
				TotalCostUSD:       0.05,
				TotalAPIDurationMS: 12000, // 12 s → "0:12"
			},
		},
	}
}

// TestCache_DimSeparators verifies that CacheProbe.Render uses
// "{{dim}} • {{reset}}" (dim marker) between fields at all levels,
// not plain " • " without markers.
func TestCache_DimSeparators(t *testing.T) {
	p := &probes.CacheProbe{}
	// AnsiEnabled=false: markers stay as literal tokens (not yet resolved to ANSI).
	// This lets us assert on the raw marker strings before renderer.Apply.
	th := renderer.Theme{AnsiEnabled: false}
	cfg := cfgAllOn() // all widgets enabled so all separator positions are active

	levels := []struct {
		name  string
		level probes.Level
	}{
		{"full", probes.LevelFull},
		{"compact", probes.LevelCompact},
		{"minimal", probes.LevelMinimal},
	}

	d := newCacheDimData()

	for _, lc := range levels {
		t.Run(lc.name, func(t *testing.T) {
			got := p.Render(d, cfg, th, lc.level)

			// T-E3: dim separator must be present.
			if !strings.Contains(got, "{{dim}} • {{reset}}") {
				t.Errorf("Cache_DimSeparators[%s]: expected '{{dim}} • {{reset}}' in output, got %q",
					lc.name, got)
			}

			// Plain undecorated " • " (without markers) must not appear.
			// Check by verifying that every " • " occurrence is adjacent to a marker.
			// Simple approach: strip the dim markers and confirm no bare " • " survives.
			stripped := strings.ReplaceAll(got, "{{dim}} • {{reset}}", "")
			if strings.Contains(stripped, " • ") {
				t.Errorf("Cache_DimSeparators[%s]: found plain ' • ' (without dim markers) in output %q",
					lc.name, got)
			}
		})
	}
}
