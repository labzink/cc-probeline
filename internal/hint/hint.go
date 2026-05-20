// Package hint provides the rotating hint widget appended at the bottom of
// the status output. Each session shows up to 8 default hints (brainstorm
// Batch 3) on 120-second rotation; critical cache invalidation events
// override the rotation. State is persisted per-session under XDG_CACHE_HOME.
//
// Phase 4.4.0 foundation: type signatures only. Real Pick / rotation lands
// in 4.4.b; alert text mapping lands in 4.4.b via alerts.go.
package hint

import (
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// Hint is one rotation slot. Index is used as the persistent key in State.
type Hint struct {
	Index int
	Text  string
}

// DefaultHints is populated in 4.4.b with the 8 brainstorm Batch 3 entries.
var DefaultHints []Hint

// Widget composes a per-session rotating hint with cache-event alert override.
// Hints is optional; when nil, DefaultHints is used.
type Widget struct {
	Hints  []Hint
	State  State
	Events []parser.CacheEvent
}

// Pick returns the text to render now: critical alert > rotation > "" (hide).
//
// Phase 4.4.0 foundation: stub returns "". Real algorithm lands in 4.4.b.
func (w *Widget) Pick(now time.Time) string {
	_ = now
	return ""
}
