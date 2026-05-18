package probes

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CacheProbe renders the row-2 aggregate: cache read/create, output tokens,
// cost, and elapsed time. Visible only when Session is non-nil.
//
// In Phase 4.1 the TTL block (⏱…) is never included; the cache-events
// detector is not yet implemented (arrives in Phase 4.4).
//
// Display:
//
//	Full:    "cache <readK>/<createK> | out <outK> | cost: $<cost> | time: MM:SS"
//	Compact: "<readK>/<createK> | <outK> | $<cost> | MM:SS"
//	Minimal: "<readK>/<createK> | <outK> | $<cost>"
type CacheProbe struct{}

func (p *CacheProbe) Name() string  { return "cache" }
func (p *CacheProbe) Priority() int { return 2 }
func (p *CacheProbe) MinWidth() int { return len("0K/0K | 0K | $0.00") }

// Visible returns false when Session is nil (no JSONL data parsed yet).
func (p *CacheProbe) Visible(d Data, c Config) bool {
	return d.Session != nil
}

// Render formats the cache aggregate row at the given level.
func (p *CacheProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	if d.Session == nil {
		return ""
	}

	readK := formatK(d.Session.Totals.CacheRead)
	createK := formatK(d.Session.Totals.CacheCreate)
	outK := formatK(d.Session.Totals.Output)
	cost := fmt.Sprintf("$%.2f", d.Stdin.Cost.TotalCostUSD)

	totalSec := d.Stdin.Cost.TotalAPIDurationMS / 1000
	mins := totalSec / 60
	secs := totalSec % 60
	mmss := fmt.Sprintf("%02d:%02d", mins, secs)

	switch level {
	case LevelFull:
		return fmt.Sprintf("cache %s/%s | out %s | cost: %s | time: %s",
			readK, createK, outK, cost, mmss)
	case LevelCompact:
		return fmt.Sprintf("%s/%s | %s | %s | %s",
			readK, createK, outK, cost, mmss)
	default: // LevelMinimal
		return fmt.Sprintf("%s/%s | %s | %s",
			readK, createK, outK, cost)
	}
}
