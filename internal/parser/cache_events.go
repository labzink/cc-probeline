package parser

import (
	"fmt"
	"time"
)

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
	ConfigError // Phase 6: synthesised by main from config.LoadCascade errors
)

// cacheShrinkRatio is the threshold below which a drop in cache_read is
// treated as a cache invalidation (base condition). If curr.CacheRead <
// prev.CacheRead * cacheShrinkRatio, cache is considered invalidated.
const cacheShrinkRatio = 0.5

// orchTTLThreshold is the minimum idle gap between two orchestrator turns that
// triggers an OrchTTL event (Claude cache TTL is 60 minutes).
const orchTTLThreshold = 60 * time.Minute

// subagentGapThreshold is the minimum gap used for both SendMessageGap and
// SlowInternal detection.
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
// Subagent-scoped events (SendMessageGap, SlowInternal) live in
// DetectSubagentCacheEvents because they require SubagentStats which
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

// DetectSubagentCacheEvents scans subagent stats for subagent-level cache
// invalidation events (SendMessageGap, SlowInternal).
//
// The now parameter is reserved for future detectors that compare the latest
// subagent span against wall-clock time (e.g. "subagent still running after
// N minutes" — span proxy uses internal timestamps, so now is not consumed
// by any current detector). Production callers should pass time.Now();
// tests pass a fixed time for determinism. Removing this parameter is a
// breaking change deferred to Phase 7+ if no caller materializes.
//
// Detail holds only AgentID (no "subagent#" prefix). The prefix is added by
// hint.AlertTemplate (verify-RED Finding #1, 2026-05-20).
//
// Returns nil (not empty slice) when input is nil or empty.
func DetectSubagentCacheEvents(subagents []SubagentStats, now time.Time) []CacheEvent {
	if len(subagents) == 0 {
		return nil
	}

	var events []CacheEvent

	for _, sa := range subagents {
		span := sa.LastTimestamp.Sub(sa.FirstTimestamp)

		if sa.TurnCount >= 2 && span > subagentGapThreshold {
			// SendMessageGap: repeated subagent invocation with long idle.
			events = append(events, CacheEvent{
				Type:      SendMessageGap,
				Timestamp: sa.LastTimestamp,
				Detail:    sa.AgentID,
			})
		} else if sa.TurnCount == 1 && span > subagentGapThreshold {
			// SlowInternal: single turn that took too long internally.
			// NOTE: dead branch on real-world data (Phase 7 backlog, Finding #2).
			events = append(events, CacheEvent{
				Type:      SlowInternal,
				Timestamp: sa.LastTimestamp,
				Detail:    sa.AgentID,
			})
		}
	}

	if len(events) == 0 {
		return nil
	}
	return events
}
