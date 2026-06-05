package parser

import (
	"fmt"
	"time"
)

// CacheEventType identifies a cache invalidation cause.
// Detection heuristics live in DetectCacheEvents / DetectSubagentCacheEvents
// (Phase 4.4.a / Phase 6.95.d).
//
// Transient types (shown for 2 min, newest-wins): OrchTTL, ModelSwitched,
// SubagentCacheExpired, CompactHeuristic.
// Persistent type (shown until resolved): ConfigError.
type CacheEventType int

const (
	OrchTTL CacheEventType = iota
	ModelSwitched
	SubagentCacheExpired // inter-turn gap ≥ 5 min inside a subagent transcript
	CompactHeuristic
	ConfigError // Phase 6: synthesised by main from config.LoadCascade errors
)

// cacheShrinkRatio is the threshold below which a drop in cache_read is
// treated as a cache invalidation (base condition). If curr.CacheRead <
// prev.CacheRead * cacheShrinkRatio, cache is considered invalidated.
const cacheShrinkRatio = 0.5

// orchTTLThreshold is the minimum idle gap between two orchestrator turns that
// triggers an OrchTTL event (Claude cache TTL is 60 minutes).
const orchTTLThreshold = 60 * time.Minute

// subagentGapThreshold is the minimum inter-turn gap inside a subagent
// transcript that triggers a SubagentCacheExpired event.
const subagentGapThreshold = 5 * time.Minute

// CacheEvent describes one detected cache invalidation occurrence.
// Detail is a free-form string used to fill the %s slot in alert templates
// (e.g. "opus-4-7 → sonnet-4-6" for ModelSwitched).
type CacheEvent struct {
	Type      CacheEventType
	Timestamp time.Time
	Detail    string
}

// isCacheInvalidated returns true if curr.CacheRead dropped significantly
// relative to prev.CacheRead (base condition shared by all DetectCacheEvents
// sub-detectors). Covers: curr==0 and large-drop (ratio < cacheShrinkRatio).
func isCacheInvalidated(prev, curr Turn) bool {
	if prev.Tokens.CacheRead == 0 {
		// DRIFT from PLAN T-6: requires prev.CacheRead > 0 as a precondition.
		// T-6 allows "curr.CacheRead == 0" as sufficient, but firing on a session's
		// first turn (no prior cache) would produce a spurious alert. Deliberate
		// improvement over the spec.
		return false
	}
	ratio := float64(curr.Tokens.CacheRead) / float64(prev.Tokens.CacheRead)
	return ratio < cacheShrinkRatio
}

// detectOrchTTL returns an OrchTTL event if both turns are orchestrator turns
// and the timestamp gap exceeds orchTTLThreshold.
// Both prev and curr must be non-sidechain to avoid a false positive when a
// subagent turn (prev.IsSidechain=true) is followed by an orch turn after >60 min.
func detectOrchTTL(prev, curr Turn) *CacheEvent {
	if prev.IsSidechain || curr.IsSidechain {
		return nil
	}
	gap := curr.Timestamp.Sub(prev.Timestamp)
	if gap < orchTTLThreshold {
		return nil
	}
	return &CacheEvent{
		Type:      OrchTTL,
		Timestamp: curr.Timestamp,
		Detail:    fmt.Sprintf("%.0f-min idle (%.0f min)", orchTTLThreshold.Minutes(), gap.Minutes()),
	}
}

// detectModelSwitched returns a ModelSwitched event if the canonical model key
// changed between prev and curr.
func detectModelSwitched(prev, curr Turn) *CacheEvent {
	if prev.Model == curr.Model {
		return nil
	}
	return &CacheEvent{
		Type:      ModelSwitched,
		Timestamp: curr.Timestamp,
		Detail:    fmt.Sprintf("%s → %s", prev.Model, curr.Model),
	}
}

// detectCompactHeuristic returns a CompactHeuristic event when:
//   - prev.CacheRead is large (> 0)
//   - curr.CacheRead == 0 (full drop)
//   - next.CacheCreate < prev.CacheRead (new cache smaller than old)
func detectCompactHeuristic(prev, curr Turn, turns []Turn, currIdx int) *CacheEvent {
	if curr.Tokens.CacheRead != 0 {
		return nil
	}
	if prev.Tokens.CacheRead == 0 {
		return nil
	}
	nextIdx := currIdx + 1
	if nextIdx >= len(turns) {
		return nil
	}
	next := turns[nextIdx]
	if next.Tokens.CacheCreate >= prev.Tokens.CacheRead {
		return nil
	}
	return &CacheEvent{
		Type:      CompactHeuristic,
		Timestamp: curr.Timestamp,
		Detail: fmt.Sprintf("cache shrunk from %d to %d tokens",
			prev.Tokens.CacheRead, next.Tokens.CacheCreate),
	}
}

// DetectCacheEvents scans turns for orchestrator-level cache invalidation events
// (orch TTL gap, model switch, compaction heuristic).
//
// The now parameter is reserved for future detectors that compare the latest
// turn's Timestamp against wall-clock time (e.g. "orchestrator stalled" — no
// detector currently uses it). Production callers should pass time.Now();
// tests pass a fixed time for determinism. Removing this parameter is a
// breaking change deferred to Phase 7+ if no caller materializes.
//
// Subagent-scoped events (SubagentCacheExpired) live in
// DetectSubagentCacheEvents because they require SubagentStats.Turns which
// Aggregate does not have access to.
//
// Returns nil (not empty slice) when input is nil or has fewer than 2 turns.
func DetectCacheEvents(turns []Turn, now time.Time) []CacheEvent {
	if len(turns) < 2 {
		return nil
	}

	var events []CacheEvent

	for i := 1; i < len(turns); i++ {
		prev := turns[i-1]
		curr := turns[i]

		if !isCacheInvalidated(prev, curr) {
			continue
		}

		// OrchTTL: orch-to-orch gap > 60 min.
		if e := detectOrchTTL(prev, curr); e != nil {
			events = append(events, *e)
		}

		// ModelSwitched: canonical model key changed.
		if e := detectModelSwitched(prev, curr); e != nil {
			events = append(events, *e)
		}

		// CompactHeuristic: curr.CacheRead==0 and next.CacheCreate < prev.CacheRead.
		// Compact (explicit /compact) detection is deferred to Phase 7 (requires
		// content parsing). For now only heuristic is implemented.
		if e := detectCompactHeuristic(prev, curr, turns, i); e != nil {
			events = append(events, *e)
		}
	}

	if len(events) == 0 {
		return nil
	}
	return events
}

// DetectSubagentCacheEvents scans subagent Turns for inter-turn gaps that
// indicate a cache expiry event (SubagentCacheExpired).
//
// Algorithm: for each subagent, iterate consecutive turn pairs
// Turns[i-1] → Turns[i]. If Turns[i].Timestamp − Turns[i-1].Timestamp ≥
// subagentGapThreshold (5 min), emit one SubagentCacheExpired event with the
// timestamp of the later turn. Each subagent produces at most one event
// (first qualifying gap wins).
//
// The now parameter is accepted for API symmetry with DetectCacheEvents and
// for future detectors that require a wall-clock reference. No current detector
// consumes it.
//
// Detail carries the AgentID (bare, no prefix). The assembler enriches it with
// "<role>:<name>" before surfacing to BuildAlert.
//
// Returns nil (not empty slice) when input is nil or empty.
func DetectSubagentCacheEvents(subagents []SubagentStats, now time.Time) []CacheEvent {
	if len(subagents) == 0 {
		return nil
	}

	var events []CacheEvent

	for _, sa := range subagents {
		if len(sa.Turns) < 2 {
			continue
		}
		for i := 1; i < len(sa.Turns); i++ {
			gap := sa.Turns[i].Timestamp.Sub(sa.Turns[i-1].Timestamp)
			if gap >= subagentGapThreshold {
				events = append(events, CacheEvent{
					Type:      SubagentCacheExpired,
					Timestamp: sa.Turns[i].Timestamp,
					Detail:    sa.AgentID,
				})
				break // one event per subagent
			}
		}
	}

	if len(events) == 0 {
		return nil
	}
	return events
}
