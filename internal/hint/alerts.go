package hint

import (
	"fmt"

	"github.com/labzink/cc-probeline/internal/parser"
)

// AlertTexts maps each parser.CacheEventType to a printf-style template.
// Templates that include a %s slot interpolate CacheEvent.Detail.
var AlertTexts = map[parser.CacheEventType]string{
	parser.OrchTTL:          "⚠ Cache rebuilt · 60-min idle TTL passed",
	parser.ModelSwitched:    "⚠ Cache rebuilt · model switched (%s)",
	parser.SendMessageGap:   "⚠ Subagent#%s cache lost · 5-min SendMessage gap",
	parser.SlowInternal:     "⚠ Subagent#%s stalled >5 min · cache expired",
	parser.Compact:          "Cache rebuilt by /compact (normal)",
	parser.CompactHeuristic: "Likely /compact triggered (cache shrunk)",
}

// criticalTypes defines priority order for tie-breaking when multiple events
// are present. Earlier in the slice = higher priority.
var criticalTypes = []parser.CacheEventType{
	parser.OrchTTL,
	parser.ModelSwitched,
	parser.SendMessageGap,
	parser.SlowInternal,
	parser.Compact,
	parser.CompactHeuristic,
}

// BuildAlert returns the most critical alert text for events, or "" when
// the slice is empty or contains no known event type.
// Tie-break: first by criticalTypes priority order; within same type, the
// last event in slice order wins (most recent).
func BuildAlert(events []parser.CacheEvent) string {
	if len(events) == 0 {
		return ""
	}
	for _, t := range criticalTypes {
		var last *parser.CacheEvent
		for i := range events {
			if events[i].Type == t {
				last = &events[i]
			}
		}
		if last == nil {
			continue
		}
		tpl := AlertTexts[t]
		// Only interpolate Detail when the template contains a %s verb.
		// Templates without %s (e.g. CompactHeuristic) are returned as-is to
		// avoid fmt.Sprintf producing spurious %!(EXTRA ...) output.
		if last.Detail != "" && containsVerb(tpl) {
			return fmt.Sprintf(tpl, last.Detail)
		}
		return tpl
	}
	return ""
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
