package probes

import (
	"fmt"
	"math"
	"time"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// CacheProbe renders the row-2 aggregate: cache read/create, output tokens,
// cost, elapsed time, and TTL countdown. Visible only when Session is non-nil.
//
// Display:
//
//	Full:    "cache <readK>/<createK> | out <outK> | cost: $<cost> | time: MM:SS ⏱Nm"
//	Compact: "<readK>/<createK> | <outK> | $<cost> | MM:SS ⏱Nm"
//	Minimal: "<readK>/<createK> | <outK> | $<cost>"   (no TTL, no time)
//
// TTL block (⏱Nm) is omitted when:
//   - d.Session is nil
//   - d.Session.LastTimestamp is zero
//   - d.Session.TurnCount == 0
//   - remaining minutes <= 0 (cache window expired)
//   - level == LevelMinimal
type CacheProbe struct{}

func (p *CacheProbe) Name() string  { return "cache" }
func (p *CacheProbe) Priority() int { return 2 }
func (p *CacheProbe) MinWidth() int { return len("0K/0K | 0K | $0.00") }

// cacheTTL computes the ⏱Nm suffix for the cache row.
// Returns "" when TTL should be hidden (expired, zero timestamp, zero turns, or zero window).
// remaining = window − floor((now − lastTimestamp).Minutes()), floor applied.
func cacheTTL(now time.Time, lastTimestamp time.Time, turnCount int, orchTTLMinutes int) string {
	if orchTTLMinutes <= 0 {
		return ""
	}
	if lastTimestamp.IsZero() {
		return ""
	}
	if turnCount == 0 {
		return ""
	}
	elapsed := now.Sub(lastTimestamp)
	elapsedMinutes := int(math.Floor(elapsed.Minutes()))
	remaining := orchTTLMinutes - elapsedMinutes
	if remaining <= 0 {
		return ""
	}
	return fmt.Sprintf("⏱ %dm", remaining)
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
// TTL block (⏱Nm) is appended to Full and Compact when conditions are met (see cacheTTL).
func (p *CacheProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	readK := formatK(d.Session.Totals.CacheRead)
	createK := formatK(d.Session.Totals.CacheCreate)
	outK := formatK(d.Session.Totals.Output)
	cost := fmt.Sprintf("$%.2f", d.Stdin.Cost.TotalCostUSD)
	mmss := formatMMSS(d.Stdin.Cost.TotalAPIDurationMS)

	// TTL is computed for Full/Compact; always empty for Minimal.
	ttl := cacheTTL(d.Now, d.Session.LastTimestamp, d.Session.TurnCount, c.OrchTTLMinutes)

	// ttlSuffix returns " ⏱Nm" when ttl is non-empty, "" otherwise.
	ttlSuffix := func() string {
		if ttl == "" {
			return ""
		}
		return " " + ttl
	}

	switch level {
	case LevelFull:
		if !c.CostEnabled {
			return fmt.Sprintf("cache %s/%s | out %s | time: %s%s",
				readK, createK, outK, mmss, ttlSuffix())
		}
		return fmt.Sprintf("cache %s/%s | out %s | cost: %s | time: %s%s",
			readK, createK, outK, cost, mmss, ttlSuffix())
	case LevelCompact:
		if !c.CostEnabled {
			return fmt.Sprintf("%s/%s | %s | %s%s",
				readK, createK, outK, mmss, ttlSuffix())
		}
		return fmt.Sprintf("%s/%s | %s | %s | %s%s",
			readK, createK, outK, cost, mmss, ttlSuffix())
	default: // LevelMinimal — no TTL, no time block
		if !c.CostEnabled {
			return fmt.Sprintf("%s/%s | %s",
				readK, createK, outK)
		}
		return fmt.Sprintf("%s/%s | %s | %s",
			readK, createK, outK, cost)
	}
}
