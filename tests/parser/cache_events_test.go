// Package parser_test — RED tests for DetectCacheEvents / DetectSubagentCacheEvents (Phase 4.4.a).
// Contract: plans/tasks/phase-4-step4-plan.md (Phase 4.4.a)
// API:
//
//	func DetectCacheEvents(turns []Turn, now time.Time) []CacheEvent
//	func DetectSubagentCacheEvents(subagents []SubagentStats, now time.Time) []CacheEvent
package parser_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// base is the reference time used across all test cases.
var base = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// mkTurn builds a Turn for test use.
//   - idx:       1-based display position
//   - model:     raw model string (e.g. "claude-opus-4-7-20250805")
//   - cacheRead: Tokens.CacheRead value
//   - ts:        Timestamp
//   - sidechain: IsSidechain flag
func mkTurn(idx int, model string, cacheRead int, ts time.Time, sidechain bool) parser.Turn {
	return parser.Turn{
		Index:       idx,
		Model:       parser.CanonicalModelKey(model),
		Tokens:      parser.TokenCounts{CacheRead: cacheRead},
		Timestamp:   ts,
		IsSidechain: sidechain,
	}
}

// mkTurnFull is like mkTurn but also sets Tokens.CacheCreate and Duration.
func mkTurnFull(idx int, model string, cacheRead, cacheCreate int, ts time.Time, dur time.Duration, sidechain bool) parser.Turn {
	return parser.Turn{
		Index:       idx,
		Model:       parser.CanonicalModelKey(model),
		Tokens:      parser.TokenCounts{CacheRead: cacheRead, CacheCreate: cacheCreate},
		Timestamp:   ts,
		Duration:    dur,
		IsSidechain: sidechain,
	}
}

// ---------------------------------------------------------------------------
// DetectCacheEvents — stable session (no events)
// ---------------------------------------------------------------------------

// TestDetectCacheEvents_NoEvents_StableSession verifies that a session with
// a single model and growing cache_read emits no cache events.
func TestDetectCacheEvents_NoEvents_StableSession(t *testing.T) {
	turns := []parser.Turn{
		mkTurn(1, "claude-opus-4-7-20250805", 10000, base, false),
		mkTurn(2, "claude-opus-4-7-20250805", 20000, base.Add(5*time.Minute), false),
		mkTurn(3, "claude-opus-4-7-20250805", 30000, base.Add(10*time.Minute), false),
		mkTurn(4, "claude-opus-4-7-20250805", 40000, base.Add(15*time.Minute), false),
		mkTurn(5, "claude-opus-4-7-20250805", 50000, base.Add(20*time.Minute), false),
	}

	events := parser.DetectCacheEvents(turns, base.Add(25*time.Minute))

	if len(events) != 0 {
		t.Errorf("stable session: expected 0 events, got %d: %+v", len(events), events)
	}
}

// ---------------------------------------------------------------------------
// DetectCacheEvents — OrchTTL
// ---------------------------------------------------------------------------

// TestDetectCacheEvents_OrchTTL verifies that a 60+ minute idle gap with
// cache_read dropping to 0 produces an OrchTTL event.
func TestDetectCacheEvents_OrchTTL(t *testing.T) {
	// prev: orch turn, cache_read=50K
	// curr: orch turn, cache_read=0, gap=65 min → base condition met (0==0) + >60min idle
	prev := mkTurn(1, "claude-opus-4-7-20250805", 50000, base, false)
	curr := mkTurn(2, "claude-opus-4-7-20250805", 0, base.Add(65*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(70*time.Minute))

	if len(events) != 1 {
		t.Fatalf("OrchTTL: expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].Type != parser.OrchTTL {
		t.Errorf("OrchTTL: expected type OrchTTL (%d), got %d", parser.OrchTTL, events[0].Type)
	}
	if !strings.Contains(events[0].Detail, "60-min idle") {
		t.Errorf("OrchTTL: Detail %q should contain '60-min idle'", events[0].Detail)
	}
	if events[0].Timestamp.IsZero() {
		t.Error("OrchTTL: Timestamp should not be zero")
	}
}

// TestDetectCacheEvents_OrchTTL_NotTriggered_UnderHour verifies that a 59-minute
// gap does NOT produce an OrchTTL event even when cache_read drops to 0.
func TestDetectCacheEvents_OrchTTL_NotTriggered_UnderHour(t *testing.T) {
	// gap = 59 min (< 60 min threshold) — OrchTTL must not fire
	// Base condition (cache_read==0) is met, but time threshold is not
	prev := mkTurn(1, "claude-opus-4-7-20250805", 50000, base, false)
	curr := mkTurn(2, "claude-opus-4-7-20250805", 0, base.Add(59*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(60*time.Minute))

	for _, e := range events {
		if e.Type == parser.OrchTTL {
			t.Errorf("OrchTTL_NotTriggered_UnderHour: OrchTTL must not fire at 59-min gap")
		}
	}
}

// ---------------------------------------------------------------------------
// DetectCacheEvents — ModelSwitched
// ---------------------------------------------------------------------------

// TestDetectCacheEvents_ModelSwitched verifies that a model change with
// cache_read dropping to 0 produces a ModelSwitched event.
func TestDetectCacheEvents_ModelSwitched(t *testing.T) {
	// prev: opus, curr: sonnet, cache_read drops 40K→0 → base condition met
	prev := mkTurn(1, "claude-opus-4-7-20250805", 40000, base, false)
	curr := mkTurn(2, "claude-sonnet-4-6-20251015", 0, base.Add(2*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(5*time.Minute))

	// Filter to ModelSwitched events only
	var ms []parser.CacheEvent
	for _, e := range events {
		if e.Type == parser.ModelSwitched {
			ms = append(ms, e)
		}
	}

	if len(ms) != 1 {
		t.Fatalf("ModelSwitched: expected 1 ModelSwitched event, got %d: %+v", len(ms), ms)
	}
	if !strings.Contains(ms[0].Detail, "opus") {
		t.Errorf("ModelSwitched: Detail %q must contain 'opus'", ms[0].Detail)
	}
	if !strings.Contains(ms[0].Detail, "sonnet") {
		t.Errorf("ModelSwitched: Detail %q must contain 'sonnet'", ms[0].Detail)
	}
}

// TestDetectCacheEvents_ModelSwitched_BaseConditionRequired verifies that when
// the model changes but cache_read remains stable (above 50% of prev),
// ModelSwitched is NOT emitted.
func TestDetectCacheEvents_ModelSwitched_BaseConditionRequired(t *testing.T) {
	// prev.CacheRead=50K, curr.CacheRead=48K → ratio=48/50=0.96 > 0.5 → base condition NOT met
	// Even though the model changed, no event should fire
	prev := mkTurn(1, "claude-opus-4-7-20250805", 50000, base, false)
	curr := mkTurn(2, "claude-sonnet-4-6-20251015", 48000, base.Add(2*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(5*time.Minute))

	for _, e := range events {
		if e.Type == parser.ModelSwitched {
			t.Errorf("ModelSwitched_BaseConditionRequired: must not fire when ratio=0.96 > 0.5")
		}
	}
}

// ---------------------------------------------------------------------------
// DetectCacheEvents — CompactHeuristic
// ---------------------------------------------------------------------------

// TestDetectCacheEvents_CompactHeuristic verifies that when prev.CacheRead is
// large, curr.CacheRead drops to 0, and the next turn's CacheCreate is smaller
// than prev.CacheRead, a CompactHeuristic event fires.
func TestDetectCacheEvents_CompactHeuristic(t *testing.T) {
	// prev.CacheRead=80K, curr.CacheRead=0 → base condition met
	// next.CacheCreate=30K < 80K → heuristic: cache shrunk → CompactHeuristic
	prev := mkTurnFull(1, "claude-opus-4-7-20250805", 80000, 0, base, 0, false)
	curr := mkTurnFull(2, "claude-opus-4-7-20250805", 0, 0, base.Add(1*time.Minute), 0, false)
	next := mkTurnFull(3, "claude-opus-4-7-20250805", 30000, 30000, base.Add(2*time.Minute), 0, false)

	turns := []parser.Turn{prev, curr, next}
	events := parser.DetectCacheEvents(turns, base.Add(5*time.Minute))

	var ch []parser.CacheEvent
	for _, e := range events {
		if e.Type == parser.CompactHeuristic {
			ch = append(ch, e)
		}
	}

	if len(ch) != 1 {
		t.Fatalf("CompactHeuristic: expected 1 CompactHeuristic event, got %d: %+v", len(ch), ch)
	}
	if !strings.Contains(ch[0].Detail, "cache shrunk from") {
		t.Errorf("CompactHeuristic: Detail %q must contain 'cache shrunk from'", ch[0].Detail)
	}
}

// TestDetectCacheEvents_CompactHeuristic_NotTriggered_NewCacheLarger verifies
// that when the new CacheCreate exceeds prev CacheRead, CompactHeuristic does
// NOT fire (this is a plain /clear, not /compact).
func TestDetectCacheEvents_CompactHeuristic_NotTriggered_NewCacheLarger(t *testing.T) {
	// prev.CacheRead=80K, curr.CacheRead=0 → base condition met
	// next.CacheCreate=100K > 80K → NOT a compact shrink → no CompactHeuristic
	prev := mkTurnFull(1, "claude-opus-4-7-20250805", 80000, 0, base, 0, false)
	curr := mkTurnFull(2, "claude-opus-4-7-20250805", 0, 0, base.Add(1*time.Minute), 0, false)
	next := mkTurnFull(3, "claude-opus-4-7-20250805", 100000, 100000, base.Add(2*time.Minute), 0, false)

	turns := []parser.Turn{prev, curr, next}
	events := parser.DetectCacheEvents(turns, base.Add(5*time.Minute))

	for _, e := range events {
		if e.Type == parser.CompactHeuristic {
			t.Errorf("CompactHeuristic_NotTriggered_NewCacheLarger: must not fire when new cache (%d) > prev (%d)", 100000, 80000)
		}
	}
}

// ---------------------------------------------------------------------------
// DetectCacheEvents — sidechain exclusion from OrchTTL
// ---------------------------------------------------------------------------

// TestDetectCacheEvents_NoSidechainInOrchTTL verifies that OrchTTL is NOT
// triggered when the current turn is a sidechain (subagent) turn.
func TestDetectCacheEvents_NoSidechainInOrchTTL(t *testing.T) {
	// gap=65 min and cache_read=0, but curr.IsSidechain=true → OrchTTL must not fire
	prev := mkTurn(1, "claude-opus-4-7-20250805", 50000, base, false)
	curr := mkTurn(2, "claude-opus-4-7-20250805", 0, base.Add(65*time.Minute), true) // sidechain

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(70*time.Minute))

	for _, e := range events {
		if e.Type == parser.OrchTTL {
			t.Errorf("NoSidechainInOrchTTL: OrchTTL must not fire for sidechain curr turn")
		}
	}
}

// TestDetectCacheEvents_OrchTTL_NotFiredWhenPrevIsSidechain verifies that
// OrchTTL does NOT fire when prev.IsSidechain=true, even if the timestamp
// gap exceeds the threshold and curr is an orch turn. A subagent→orch
// transition after >60 min must not produce a false OrchTTL event.
func TestDetectCacheEvents_OrchTTL_NotFiredWhenPrevIsSidechain(t *testing.T) {
	// prev: sidechain (subagent) turn, CacheRead=50K
	// curr: orch turn, CacheRead=0, gap=70 min — base condition met, but
	// prev.IsSidechain=true means this is NOT an orch-to-orch transition.
	prev := mkTurn(1, "claude-opus-4-7-20250805", 50000, base, true) // sidechain!
	curr := mkTurn(2, "claude-opus-4-7-20250805", 0, base.Add(70*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(75*time.Minute))

	for _, e := range events {
		if e.Type == parser.OrchTTL {
			t.Errorf("OrchTTL_NotFiredWhenPrevIsSidechain: OrchTTL must not fire when prev is a sidechain turn; got events: %+v", events)
		}
	}
}

// ---------------------------------------------------------------------------
// DetectCacheEvents — edge cases
// ---------------------------------------------------------------------------

// TestDetectCacheEvents_EmptyInput verifies nil input returns nil (not panic,
// not empty slice).
func TestDetectCacheEvents_EmptyInput(t *testing.T) {
	events := parser.DetectCacheEvents(nil, base)
	if events != nil {
		t.Errorf("EmptyInput: expected nil, got %+v", events)
	}
}

// TestDetectCacheEvents_SingleTurn verifies that a single turn returns nil
// (nothing to compare against).
func TestDetectCacheEvents_SingleTurn(t *testing.T) {
	turns := []parser.Turn{
		mkTurn(1, "claude-opus-4-7-20250805", 50000, base, false),
	}

	events := parser.DetectCacheEvents(turns, base.Add(time.Minute))
	if events != nil {
		t.Errorf("SingleTurn: expected nil, got %+v", events)
	}
}

// ---------------------------------------------------------------------------
// isCacheInvalidated — tested through DetectCacheEvents behaviour
// ---------------------------------------------------------------------------

// TestIsCacheInvalidated_ZeroCurrentCache verifies that cache_read dropping to
// zero satisfies the base condition (event fires).
// prev.CacheRead=50K, curr.CacheRead=0 → isCacheInvalidated=true
// We verify by checking that at least one event fires (any type) for the pair.
func TestIsCacheInvalidated_ZeroCurrentCache(t *testing.T) {
	// Use a gap >60 min so OrchTTL fires — which requires isCacheInvalidated=true
	// prev.CacheRead=50K, curr.CacheRead=0 → base condition met → OrchTTL fires
	prev := mkTurn(1, "claude-opus-4-7-20250805", 50000, base, false)
	curr := mkTurn(2, "claude-opus-4-7-20250805", 0, base.Add(65*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(70*time.Minute))

	// isCacheInvalidated=true is proven if at least one event fires
	if len(events) == 0 {
		t.Error("IsCacheInvalidated_ZeroCurrentCache: expected at least 1 event when curr.CacheRead==0, got 0")
	}
}

// TestIsCacheInvalidated_ShrinkRatio verifies that a >50% drop in cache_read
// satisfies the base condition.
// prev.CacheRead=100K, curr.CacheRead=40K → ratio=0.4 < 0.5 → base=true
// We prove via OrchTTL firing at 65-min gap.
func TestIsCacheInvalidated_ShrinkRatio(t *testing.T) {
	// prev=100K, curr=40K: ratio = 40K/100K = 0.40 < 0.50 → base condition met
	prev := mkTurn(1, "claude-opus-4-7-20250805", 100000, base, false)
	curr := mkTurn(2, "claude-opus-4-7-20250805", 40000, base.Add(65*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(70*time.Minute))

	if len(events) == 0 {
		t.Error("IsCacheInvalidated_ShrinkRatio: expected at least 1 event (ratio=0.40 < 0.50), got 0")
	}
	// Confirm the event that fires is OrchTTL (the orch gap is 65 min)
	found := false
	for _, e := range events {
		if e.Type == parser.OrchTTL {
			found = true
		}
	}
	if !found {
		t.Errorf("IsCacheInvalidated_ShrinkRatio: expected OrchTTL event among %+v", events)
	}
}

// TestIsCacheInvalidated_StableCache verifies that a small drop in cache_read
// does NOT satisfy the base condition.
// prev.CacheRead=100K, curr.CacheRead=80K → ratio=0.80 > 0.50 → base=false → no events
func TestIsCacheInvalidated_StableCache(t *testing.T) {
	// prev=100K, curr=80K: ratio = 80K/100K = 0.80 > 0.50 → base NOT met → no events
	// Use 65-min gap to ensure OrchTTL would fire IF base were met
	prev := mkTurn(1, "claude-opus-4-7-20250805", 100000, base, false)
	curr := mkTurn(2, "claude-opus-4-7-20250805", 80000, base.Add(65*time.Minute), false)

	turns := []parser.Turn{prev, curr}
	events := parser.DetectCacheEvents(turns, base.Add(70*time.Minute))

	if len(events) != 0 {
		t.Errorf("IsCacheInvalidated_StableCache: expected 0 events (ratio=0.80 > 0.50), got %d: %+v", len(events), events)
	}
}

// ---------------------------------------------------------------------------
// DetectSubagentCacheEvents — SubagentCacheExpired (inter-turn gap ≥ 5 min)
// ---------------------------------------------------------------------------

// mkSubagentTurn builds a parser.Turn for subagent test use.
func mkSubagentTurn(ts time.Time) parser.Turn {
	return parser.Turn{Timestamp: ts}
}

// TestDetectSubagentCacheEvents_CacheEvent_InterTurnGap verifies that a subagent
// with an inter-turn gap ≥ 5 min produces a SubagentCacheExpired event.
//
// Two turns: T0=base, T1=base+7min → gap=7min ≥ 5min → SubagentCacheExpired.
func TestDetectSubagentCacheEvents_CacheEvent_InterTurnGap(t *testing.T) {
	t0 := base
	t1 := base.Add(7 * time.Minute)
	sa := parser.SubagentStats{
		AgentID:   "abc123",
		TurnCount: 2,
		Turns: []parser.Turn{
			mkSubagentTurn(t0),
			mkSubagentTurn(t1),
		},
	}

	events := parser.DetectSubagentCacheEvents([]parser.SubagentStats{sa}, base.Add(10*time.Minute))

	var expired []parser.CacheEvent
	for _, e := range events {
		if e.Type == parser.SubagentCacheExpired {
			expired = append(expired, e)
		}
	}

	if len(expired) != 1 {
		t.Fatalf("SubagentCacheExpired: expected 1 event, got %d: %+v", len(expired), events)
	}
	// Timestamp must be the later turn (T1 = base+7min).
	if !expired[0].Timestamp.Equal(t1) {
		t.Errorf("SubagentCacheExpired: Timestamp=%v, want %v (later turn)", expired[0].Timestamp, t1)
	}
	// Detail must hold AgentID.
	if !strings.Contains(expired[0].Detail, "abc123") {
		t.Errorf("SubagentCacheExpired: Detail %q must contain AgentID 'abc123'", expired[0].Detail)
	}
}

// TestDetectSubagentCacheEvents_CacheEvent_GapUnderThreshold verifies that a
// gap < 5 min does NOT produce a SubagentCacheExpired event.
//
// Two turns: T0=base, T1=base+4min → gap=4min < 5min → no event.
func TestDetectSubagentCacheEvents_CacheEvent_GapUnderThreshold(t *testing.T) {
	sa := parser.SubagentStats{
		AgentID:   "def456",
		TurnCount: 2,
		Turns: []parser.Turn{
			mkSubagentTurn(base),
			mkSubagentTurn(base.Add(4 * time.Minute)),
		},
	}

	events := parser.DetectSubagentCacheEvents([]parser.SubagentStats{sa}, base.Add(5*time.Minute))

	for _, e := range events {
		if e.Type == parser.SubagentCacheExpired {
			t.Errorf("SubagentCacheExpired must not fire for 4-min gap (< 5 min): %+v", e)
		}
	}
}

// TestDetectSubagentCacheEvents_CacheEvent_SingleTurnNoEvent verifies that a
// subagent with fewer than 2 turns never produces a SubagentCacheExpired event.
func TestDetectSubagentCacheEvents_CacheEvent_SingleTurnNoEvent(t *testing.T) {
	sa := parser.SubagentStats{
		AgentID:   "ghi789",
		TurnCount: 1,
		Turns:     []parser.Turn{mkSubagentTurn(base)},
	}

	events := parser.DetectSubagentCacheEvents([]parser.SubagentStats{sa}, base.Add(10*time.Minute))

	if len(events) != 0 {
		t.Errorf("SingleTurnNoEvent: expected 0 events, got %d: %+v", len(events), events)
	}
}

// TestDetectSubagentCacheEvents_CacheEvent_FirstGapWins verifies that when
// multiple inter-turn gaps exist, only the first qualifying gap fires (one event
// per subagent).
//
// Three turns: T0→T1 gap=2min (< 5min, no fire), T1→T2 gap=6min (≥ 5min, fires).
// The event timestamp must be T2 (the later turn of the qualifying pair).
func TestDetectSubagentCacheEvents_CacheEvent_FirstGapWins(t *testing.T) {
	t0 := base
	t1 := base.Add(2 * time.Minute)
	t2 := t1.Add(6 * time.Minute)
	sa := parser.SubagentStats{
		AgentID:   "multi123",
		TurnCount: 3,
		Turns: []parser.Turn{
			mkSubagentTurn(t0),
			mkSubagentTurn(t1),
			mkSubagentTurn(t2),
		},
	}

	events := parser.DetectSubagentCacheEvents([]parser.SubagentStats{sa}, t2.Add(time.Minute))

	var expired []parser.CacheEvent
	for _, e := range events {
		if e.Type == parser.SubagentCacheExpired {
			expired = append(expired, e)
		}
	}

	// Exactly one event (one per subagent, first qualifying gap).
	if len(expired) != 1 {
		t.Fatalf("FirstGapWins: expected 1 event, got %d: %+v", len(expired), events)
	}
	// The event timestamp is T2 (the turn after the qualifying T1→T2 gap).
	if !expired[0].Timestamp.Equal(t2) {
		t.Errorf("FirstGapWins: Timestamp=%v, want %v (T2, the later turn)", expired[0].Timestamp, t2)
	}
}

// ---------------------------------------------------------------------------
// DetectSubagentCacheEvents — no events (fast subagent)
// ---------------------------------------------------------------------------

// TestDetectSubagentCacheEvents_NoEvents_FastSubagent verifies that a subagent
// with turns within the 5-min threshold emits no events.
func TestDetectSubagentCacheEvents_NoEvents_FastSubagent(t *testing.T) {
	// Two turns 2 min apart → gap < 5 min → no event.
	sa := parser.SubagentStats{
		AgentID:   "ghi789",
		TurnCount: 2,
		Turns: []parser.Turn{
			mkSubagentTurn(base),
			mkSubagentTurn(base.Add(2 * time.Minute)),
		},
	}

	events := parser.DetectSubagentCacheEvents([]parser.SubagentStats{sa}, base.Add(5*time.Minute))

	if len(events) != 0 {
		t.Errorf("NoEvents_FastSubagent: expected 0 events, got %d: %+v", len(events), events)
	}
}

// ---------------------------------------------------------------------------
// DetectSubagentCacheEvents — edge cases
// ---------------------------------------------------------------------------

// TestDetectSubagentCacheEvents_EmptyInput verifies nil input returns nil.
func TestDetectSubagentCacheEvents_EmptyInput(t *testing.T) {
	events := parser.DetectSubagentCacheEvents(nil, base)
	if events != nil {
		t.Errorf("EmptyInput: expected nil, got %+v", events)
	}
}
