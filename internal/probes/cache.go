package probes

import (
	"fmt"
	"time"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CacheProbe renders the row-2 aggregate: cache read/create, output tokens,
// cost, elapsed time, and TTL countdown. Visible only when Session is non-nil.
//
// Display:
//
//	Full:    "cache <readK>/<createK> ⏱ Nm{{dim}} • {{reset}}out <outK>{{dim}} • {{reset}}cost: $<cost>{{dim}} • {{reset}}time: MM:SS"
//	Compact: "<readK>/<createK> ⏱ Nm{{dim}} • {{reset}}<outK>{{dim}} • {{reset}}$<cost>{{dim}} • {{reset}}MM:SS"
//	Minimal: "<readK>/<createK> ⏱ Nm{{dim}} • {{reset}}<outK>{{dim}} • {{reset}}$<cost>"   (TTL preserved, no time)
//
// TTL block (⏱Nm) is omitted when:
//   - d.Session is nil
//   - d.Session.LastTimestamp is zero
//   - d.Session.TurnCount == 0
//   - remaining minutes <= 0 (cache window expired)
type CacheProbe struct{}

func (p *CacheProbe) Name() string  { return "cache" }
func (p *CacheProbe) Priority() int { return 2 }
func (p *CacheProbe) MinWidth() int { return len("0K/0K | 0K | $0.00") }

// cacheTTL computes the ⏱Nm suffix for the cache row, delegating the formula to
// renderer.CacheTTL (single source of truth, reused by the per-turn table).
// Returns "" when TTL should be hidden (zero timestamp, zero turns, zero window,
// or subagent context: subagentGapMinutes > 0).
//
// T-23: TTL is suppressed entirely when subagentGapMinutes > 0 (subagent context).
func cacheTTL(now time.Time, lastTimestamp time.Time, turnCount int, orchTTLMinutes int, subagentGapMinutes int, ansiEnabled bool) string {
	// T-23: TTL only for orchestrator.
	if subagentGapMinutes > 0 {
		return ""
	}
	return renderer.CacheTTL(now, lastTimestamp, turnCount, orchTTLMinutes, ansiEnabled)
}

// Visible returns false when CacheEnabled is false or Session is nil (no JSONL data parsed yet).
func (p *CacheProbe) Visible(d Data, c Config) bool {
	if !c.CacheEnabled {
		return false
	}
	return d.Session != nil
}

// Render formats the cache aggregate row at the given level.
// When c.CostEnabled is false, the cost segment is omitted from all levels.
// TTL block (⏱Nm) is appended to all levels (Full, Compact, Minimal) when conditions are met (see cacheTTL).
func (p *CacheProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	readK := formatK(d.Session.Totals.CacheRead)
	createK := formatK(d.Session.Totals.CacheCreate)
	outK := formatK(d.Session.Totals.Output)
	// C2: use LastRequestCost (delta for the current prompt group), not raw cumulative total.
	cost := fmt.Sprintf("$%.2f", d.LastRequestCost)
	mmss := formatMMSS(d.Stdin.Cost.TotalAPIDurationMS)

	// C3: use the explicit IsSubagentContext flag to suppress TTL, not the
	// SubagentGapMinutes threshold. SubagentGapMinutes is a config threshold (minutes),
	// not a "is subagent" runtime flag. Pass 1 (non-zero) to cacheTTL only when
	// the probe is explicitly in a subagent render context.
	subagentArg := 0
	if c.IsSubagentContext {
		subagentArg = 1
	}
	ttl := cacheTTL(d.Now, d.Session.LastTimestamp, d.Session.TurnCount, c.OrchTTLMinutes, subagentArg, t.AnsiEnabled)

	// ttlInfix returns " ⏱ Nm" when ttl is non-empty, "" otherwise.
	// Placed right after cache numbers, before the first separator.
	ttlInfix := func() string {
		if ttl == "" {
			return ""
		}
		return " " + ttl
	}

	switch level {
	case LevelFull:
		if !c.CostEnabled {
			return fmt.Sprintf("cache %s/%s%s{{dim}} • {{reset}}out %s{{dim}} • {{reset}}time: %s",
				readK, createK, ttlInfix(), outK, mmss)
		}
		return fmt.Sprintf("cache %s/%s%s{{dim}} • {{reset}}out %s{{dim}} • {{reset}}cost: %s{{dim}} • {{reset}}time: %s",
			readK, createK, ttlInfix(), outK, cost, mmss)
	case LevelCompact:
		if !c.CostEnabled {
			return fmt.Sprintf("%s/%s%s{{dim}} • {{reset}}%s{{dim}} • {{reset}}%s",
				readK, createK, ttlInfix(), outK, mmss)
		}
		return fmt.Sprintf("%s/%s%s{{dim}} • {{reset}}%s{{dim}} • {{reset}}%s{{dim}} • {{reset}}%s",
			readK, createK, ttlInfix(), outK, cost, mmss)
	default: // LevelMinimal — TTL preserved, no time block
		if !c.CostEnabled {
			return fmt.Sprintf("%s/%s%s{{dim}} • {{reset}}%s",
				readK, createK, ttlInfix(), outK)
		}
		return fmt.Sprintf("%s/%s%s{{dim}} • {{reset}}%s{{dim}} • {{reset}}%s",
			readK, createK, ttlInfix(), outK, cost)
	}
}
