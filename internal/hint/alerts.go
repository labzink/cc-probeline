package hint

import (
	"fmt"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// AlertTexts maps each parser.CacheEventType to a printf-style template.
// Templates that include a %s slot interpolate CacheEvent.Detail.
//
// Transient types (shown for 2 min, surfaced by the assembler's recency filter):
//
//	OrchTTL, ModelSwitched, SubagentCacheExpired, CompactHeuristic.
//
// Persistent type (shown until resolved; not subject to the 2-min recency filter
// because it is synthesised by main from a config error, not from a transcript event):
//
//	ConfigError.
var AlertTexts = map[parser.CacheEventType]string{
	parser.OrchTTL:              "⚠ Cache rebuilt · 60-min idle TTL passed",
	parser.ModelSwitched:        "⚠ Cache rebuilt · model switched (%s)",
	parser.SubagentCacheExpired: "⚠ Subagent %s cache expired · 5-min gap",
	parser.CompactHeuristic:     "⟳ Context compacted · cache rebuilt",
	parser.ConfigError:          "⚠ Config error · run cc-probeline check-config",
}

// BuildAlert returns the alert text to display, or "" when there are no events
// to surface.
//
// Algorithm (Phase 6.95.d newest-wins):
//  1. Partition events into transient (all types except ConfigError) and
//     persistent (ConfigError).
//  2. Among transient events, pick the one with the largest Timestamp
//     (newest-wins). If found, return its formatted alert text.
//  3. If no transient events, fall back to ConfigError when present.
//  4. Otherwise return "".
//
// Note: the 2-min recency window that limits which transient events are live
// is applied by the assembler (assembler.go hint() method) BEFORE calling
// BuildAlert. BuildAlert itself has no clock parameter and trusts its input.
func BuildAlert(events []parser.CacheEvent) string {
	if len(events) == 0 {
		return ""
	}

	// Find the newest transient event (max Timestamp among non-ConfigError).
	var best *parser.CacheEvent
	for i := range events {
		e := &events[i]
		if e.Type == parser.ConfigError {
			continue
		}
		if best == nil || e.Timestamp.After(best.Timestamp) {
			best = e
		}
	}
	if best != nil {
		return formatAlert(best)
	}

	// Fallback: ConfigError (persistent, shown whenever no live transient exists).
	for i := range events {
		if events[i].Type == parser.ConfigError {
			return formatAlert(&events[i])
		}
	}

	return ""
}

// formatAlert formats a single CacheEvent into its display string using the
// AlertTexts template. If the template contains a %s verb and Detail is
// non-empty, Detail is interpolated; otherwise the template is returned as-is
// to avoid spurious fmt.Sprintf artefacts ("%!(EXTRA ...)").
func formatAlert(e *parser.CacheEvent) string {
	tpl, ok := AlertTexts[e.Type]
	if !ok {
		return ""
	}
	if e.Detail != "" && containsVerb(tpl) {
		return fmt.Sprintf(tpl, e.Detail)
	}
	return tpl
}

// containsVerb reports whether s contains a fmt %s verb.
func containsVerb(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '%' && s[i+1] == 's' {
			return true
		}
	}
	return false
}

// IsTransient reports whether t is a transient alert type (subject to the
// 2-min recency window). ConfigError is NOT transient — it stays until the
// config is fixed.
func IsTransient(t parser.CacheEventType) bool {
	return t != parser.ConfigError
}

// transientAlertWindow is the recency window for transient alerts.
// Events older than this are not surfaced by the assembler.
const TransientAlertWindow = 2 * time.Minute
