package hint

import "github.com/labzink/cc-probeline/internal/parser"

// AlertTexts maps each parser.CacheEventType to a printf-style template.
// Templates that consume CacheEvent.Detail include a single %s slot
// (e.g. ModelSwitched). Populated in 4.4.b.
var AlertTexts = map[parser.CacheEventType]string{}

// BuildAlert returns the most critical alert text for events, or "" when
// the slice is empty or contains no known event type. Tie-break order
// follows the parser.CacheEventType iota (OrchTTL > ModelSwitched > …).
//
// Phase 4.4.0 foundation: stub returns "". Real selection lands in 4.4.b.
func BuildAlert(events []parser.CacheEvent) string {
	_ = events
	return ""
}
