// Package cost_test — RED tests for F4 fix: MergeTurns helper (Phase 6.9 FIXES G-wire).
//
// Root cause: cmd/cc-probeline/main.go calls cost.Reconcile with only session.Turns
// (orchestrator turns). Subagent turns (SubagentStats[].Turns) are never added to
// the cost pool, so cost.SubagentTotal always returns 0 → every subagent panel shows "Σ $0.00".
//
// Contract tested here: GREEN will add
//
//	func MergeTurns(sessionTurns []parser.Turn, subagents []parser.SubagentStats) []parser.Turn
//
// in package internal/cost. The function appends every subagent's Turns slice (in order)
// after the orchestrator session turns, producing a unified list safe to pass to Reconcile.
package cost_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// f4ContainsUUID is a helper that checks whether a UUID appears at least once
// in the given turn slice. Prefix "f4" keeps it scoped to F4 tests.
func f4ContainsUUID(turns []parser.Turn, uuid string) bool {
	for _, t := range turns {
		if t.UUID == uuid {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// TestF4_MergeTurns_IncludesSubagentUUIDs
//
// Given: 2 orchestrator turns (o1, o2) and 2 subagents:
//   - subagent-1 with turns s1a, s1b
//   - subagent-2 with turn s2a
//
// MergeTurns must return a slice that:
//   - contains all five UUIDs (o1, o2, s1a, s1b, s2a)
//   - places orchestrator turns (o1, o2) before any subagent turn
// ---------------------------------------------------------------------------

// TestF4_MergeTurns_IncludesSubagentUUIDs verifies that MergeTurns returns all
// orchestrator and subagent turns, with orchestrator turns appearing first.
func TestF4_MergeTurns_IncludesSubagentUUIDs(t *testing.T) {
	// Given: orchestrator session turns.
	orchTurns := []parser.Turn{
		{UUID: "o1", Model: "claude-sonnet-4-6", IsSidechain: false,
			Tokens: parser.TokenCounts{Output: 500}},
		{UUID: "o2", Model: "claude-sonnet-4-6", IsSidechain: false,
			Tokens: parser.TokenCounts{Output: 300}},
	}

	// Given: two subagents, each with their own turn slices.
	subagents := []parser.SubagentStats{
		{
			AgentID: "subagent-1",
			Turns: []parser.Turn{
				{UUID: "s1a", Model: "claude-haiku-3-5", IsSidechain: true,
					Tokens: parser.TokenCounts{Output: 200}},
				{UUID: "s1b", Model: "claude-haiku-3-5", IsSidechain: true,
					Tokens: parser.TokenCounts{Output: 150}},
			},
		},
		{
			AgentID: "subagent-2",
			Turns: []parser.Turn{
				{UUID: "s2a", Model: "claude-sonnet-4-6", IsSidechain: true,
					Tokens: parser.TokenCounts{Output: 400}},
			},
		},
	}

	// When: merging orchestrator and subagent turns.
	merged := cost.MergeTurns(orchTurns, subagents)

	// Then: all five UUIDs must be present.
	allExpected := []string{"o1", "o2", "s1a", "s1b", "s2a"}
	for _, uuid := range allExpected {
		if !f4ContainsUUID(merged, uuid) {
			t.Errorf("TestF4_MergeTurns_IncludesSubagentUUIDs: UUID %q missing from merged slice (len=%d)",
				uuid, len(merged))
		}
	}

	// Then: total length must equal 5 (2 orch + 2 s1 + 1 s2).
	if len(merged) != 5 {
		t.Errorf("TestF4_MergeTurns_IncludesSubagentUUIDs: len(merged) = %d; want 5", len(merged))
	}

	// Then: orchestrator turns appear before any subagent turn.
	// Find the index of the last orchestrator turn and the first subagent turn.
	lastOrchIdx := -1
	firstSubIdx := len(merged)
	for i, turn := range merged {
		if turn.UUID == "o1" || turn.UUID == "o2" {
			if i > lastOrchIdx {
				lastOrchIdx = i
			}
		}
		if turn.UUID == "s1a" || turn.UUID == "s1b" || turn.UUID == "s2a" {
			if i < firstSubIdx {
				firstSubIdx = i
			}
		}
	}
	if lastOrchIdx >= firstSubIdx {
		t.Errorf("TestF4_MergeTurns_IncludesSubagentUUIDs: orchestrator turns must precede subagent turns; "+
			"last orch idx=%d, first sub idx=%d", lastOrchIdx, firstSubIdx)
	}
}

// ---------------------------------------------------------------------------
// TestF4_SubagentCostNonZero_AfterReconcileMerged
//
// Contrast test proving the F4 bug and its fix:
//
//  WITHOUT merge: Reconcile receives only session.Turns (orchestrator only) →
//    SubagentTotal(st, subagentUUIDs) == 0.
//
//  WITH MergeTurns: Reconcile receives all turns → SubagentTotal > 0.
//
// Uses realistic token counts on subagent turns so they receive meaningful
// weighted cost (haiku out-weight=4, non-trivial output token counts).
// ---------------------------------------------------------------------------

// TestF4_SubagentCostNonZero_AfterReconcileMerged proves the F4 bug (Σ $0.00)
// and verifies that calling Reconcile with MergeTurns output fixes it.
func TestF4_SubagentCostNonZero_AfterReconcileMerged(t *testing.T) {
	// Shared fixtures: orchestrator turns and one subagent with two turns.
	orchTurns := []parser.Turn{
		{UUID: "orch-t1", Model: "claude-sonnet-4-6", IsSidechain: false,
			Tokens: parser.TokenCounts{Input: 3000, Output: 1000}},
	}
	subagentTurns := []parser.Turn{
		{UUID: "sub-t1", Model: "claude-haiku-3-5", IsSidechain: true,
			Tokens: parser.TokenCounts{Input: 1000, Output: 800}},
		{UUID: "sub-t2", Model: "claude-haiku-3-5", IsSidechain: true,
			Tokens: parser.TokenCounts{Input: 500, Output: 600}},
	}
	subagents := []parser.SubagentStats{
		{AgentID: "sub-1", Turns: subagentTurns},
	}
	subUUIDs := []string{"sub-t1", "sub-t2"}

	// ccTotal is non-zero so Reconcile has a real delta to distribute.
	const ccTotal = 5.00
	const durMS = int64(10000)

	t.Run("without merge: subagent cost is zero (bug reproduction)", func(t *testing.T) {
		// Given: a fresh session reconciled with orchestrator-only turns (the old, buggy path).
		st := &state.Session{Initialized: false}

		// When: Reconcile receives only orchestrator session turns — subagent UUIDs absent.
		cost.Reconcile(st, ccTotal, durMS, orchTurns)

		// Then: SubagentTotal must be 0 because sub-t1/sub-t2 are not in PerTurnCost.
		got := cost.SubagentTotal(st, subUUIDs)
		if got != 0 {
			t.Errorf("without merge: SubagentTotal = %.6f; want 0 (subagent turns not in pool)", got)
		}
	})

	t.Run("with MergeTurns: subagent cost is non-zero (fix verification)", func(t *testing.T) {
		// Given: a fresh session reconciled with the merged turn list (the fixed path).
		st := &state.Session{Initialized: false}

		// When: MergeTurns combines orchestrator and subagent turns before Reconcile.
		allTurns := cost.MergeTurns(orchTurns, subagents)
		cost.Reconcile(st, ccTotal, durMS, allTurns)

		// Then: SubagentTotal must be > 0 because sub-t1/sub-t2 now have PerTurnCost entries.
		got := cost.SubagentTotal(st, subUUIDs)
		if got <= 0 {
			t.Errorf("with MergeTurns: SubagentTotal = %.6f; want > 0 (subagent turns in cost pool)", got)
		}
	})
}
