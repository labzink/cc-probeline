package parser

import "time"

// CacheEventType identifies a cache invalidation cause.
// Detection heuristics live in DetectCacheEvents / DetectSubagentCacheEvents
// (Phase 4.4.a). The enum order is used by hint.BuildAlert as the tie-break
// priority when multiple events fire on the same render.
type CacheEventType int

const (
	OrchTTL CacheEventType = iota
	ModelSwitched
	SendMessageGap
	SlowInternal
	Compact
	CompactHeuristic
)

// CacheEvent describes one detected cache invalidation occurrence.
// Detail is a free-form string used to fill the %s slot in alert templates
// (e.g. "opus-4-7 → sonnet-4-6" for ModelSwitched).
type CacheEvent struct {
	Type      CacheEventType
	Timestamp time.Time
	Detail    string
}

// DetectCacheEvents scans turns for orchestrator-level cache invalidation:
// OrchTTL, ModelSwitched, Compact, CompactHeuristic. Subagent-scoped events
// (SendMessageGap, SlowInternal) live in DetectSubagentCacheEvents because
// they require SubagentStats which Aggregate does not have access to.
//
// Phase 4.4.0 foundation: stub returns nil. Real heuristics land in 4.4.a.
func DetectCacheEvents(turns []Turn, now time.Time) []CacheEvent {
	_ = turns
	_ = now
	return nil
}

// DetectSubagentCacheEvents scans subagent stats for cache invalidation:
// SendMessageGap (>5 min between Sends), SlowInternal (>5 min single turn).
//
// Phase 4.4.0 foundation: stub returns nil. Real heuristics land in 4.4.a.
func DetectSubagentCacheEvents(subagents []SubagentStats, now time.Time) []CacheEvent {
	_ = subagents
	_ = now
	return nil
}
