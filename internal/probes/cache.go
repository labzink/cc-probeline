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

// Visible returns false when CacheEnabled is false or Session is nil (no JSONL data parsed yet).
func (p *CacheProbe) Visible(d Data, c Config) bool {
	if !c.CacheEnabled {
		return false
	}
	return d.Session != nil
}

// Render formats the cache aggregate row at the given level.
// When c.CostEnabled is false, the cost segment is omitted from all levels.
func (p *CacheProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	readK := formatK(d.Session.Totals.CacheRead)
	createK := formatK(d.Session.Totals.CacheCreate)
	outK := formatK(d.Session.Totals.Output)
	cost := fmt.Sprintf("$%.2f", d.Stdin.Cost.TotalCostUSD)
	mmss := formatMMSS(d.Stdin.Cost.TotalAPIDurationMS)

	switch level {
	case LevelFull:
		if !c.CostEnabled {
			return fmt.Sprintf("cache %s/%s | out %s | time: %s",
				readK, createK, outK, mmss)
		}
		return fmt.Sprintf("cache %s/%s | out %s | cost: %s | time: %s",
			readK, createK, outK, cost, mmss)
	case LevelCompact:
		if !c.CostEnabled {
			return fmt.Sprintf("%s/%s | %s | %s",
				readK, createK, outK, mmss)
		}
		return fmt.Sprintf("%s/%s | %s | %s | %s",
			readK, createK, outK, cost, mmss)
	default: // LevelMinimal
		if !c.CostEnabled {
			return fmt.Sprintf("%s/%s | %s",
				readK, createK, outK)
		}
		return fmt.Sprintf("%s/%s | %s | %s",
			readK, createK, outK, cost)
	}
}
