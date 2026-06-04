package renderer

import (
	"fmt"
	"log/slog"
	"math"
	"time"
)

// CacheTTL computes the "⏱ Nm" cache-window countdown suffix with optional
// colour markers. It is the single source of truth for the TTL formula, shared
// by the per-turn table (TTL right of row, freeze) and the cache probe.
//
// remaining = orchTTLMinutes − floor((now − lastTimestamp).Minutes()).
//
// Returns "" when TTL should be hidden (zero timestamp, zero turns, zero
// window). When remaining ≤ 0 it returns the frozen "⏱ 0m" (red), NOT "".
//
// Colour rules (applied only when ansiEnabled=true):
//
//	>30m remaining → {{color:green}}⏱ Nm{{reset}}
//	≤30m && >10m   → {{color:yellow}}⏱ Nm{{reset}}
//	≤10m && >0     → {{color:red}}⏱ Nm{{reset}}
//	≤0m            → {{color:red}}⏱ 0m{{reset}}
func CacheTTL(now, lastTimestamp time.Time, turnCount, orchTTLMinutes int, ansiEnabled bool) string {
	if orchTTLMinutes <= 0 || lastTimestamp.IsZero() || turnCount == 0 {
		return ""
	}
	elapsedMinutes := int(math.Floor(now.Sub(lastTimestamp).Minutes()))
	remaining := orchTTLMinutes - elapsedMinutes

	if remaining <= 0 {
		if !ansiEnabled {
			return "⏱ 0m"
		}
		return "{{color:red}}⏱ 0m{{reset}}"
	}

	ttlText := fmt.Sprintf("⏱ %dm", remaining)
	if !ansiEnabled {
		return ttlText
	}
	switch {
	case remaining <= 10:
		return "{{color:red}}" + ttlText + "{{reset}}"
	case remaining <= 30:
		return "{{color:yellow}}" + ttlText + "{{reset}}"
	default:
		return "{{color:green}}" + ttlText + "{{reset}}"
	}
}

// CacheExpiredAt reports whether the cache window of an orchestrator turn at
// prevTS has expired by the time curTS arrives, i.e. the gap is ≥ the TTL
// window. Reuses the CacheTTL formula (remaining ≤ 0) with now=curTS so the
// freeze / red-cache-write logic never duplicates the threshold maths.
func CacheExpiredAt(prevTS, curTS time.Time, orchTTLMinutes int) bool {
	if orchTTLMinutes <= 0 || prevTS.IsZero() || curTS.IsZero() {
		return false
	}
	elapsedMinutes := int(math.Floor(curTS.Sub(prevTS).Minutes()))
	expired := orchTTLMinutes-elapsedMinutes <= 0
	slog.Debug("renderer.cacheExpired", "gapMin", elapsedMinutes, "ttl", orchTTLMinutes, "expired", expired)
	return expired
}
