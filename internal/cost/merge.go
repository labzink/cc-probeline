// Package cost — turn-pool merger for F4 fix (Phase 6.9 FIXES).
//
// MergeTurns combines orchestrator and subagent turns into a single slice
// that is safe to pass to Reconcile. This ensures subagent UUIDs enter the
// PerTurnCost pool so SubagentTotal returns a non-zero value.
package cost

import "github.com/labzink/cc-probeline/internal/parser"

// MergeTurns appends every subagent's Turns slice (in slice order) after the
// orchestrator session turns, producing a unified list safe to pass to Reconcile.
//
// Reconcile is idempotent for already-reconciled UUIDs, so passing a merged list
// on every call is safe: turns whose UUID already has a PerTurnCost entry are
// skipped; only new UUIDs consume the current delta.
//
// The orchestrator turns always precede subagent turns to preserve stable order
// (deterministic cost distribution across reconcile calls).
func MergeTurns(sessionTurns []parser.Turn, subagents []parser.SubagentStats) []parser.Turn {
	total := len(sessionTurns)
	for i := range subagents {
		total += len(subagents[i].Turns)
	}

	merged := make([]parser.Turn, len(sessionTurns), total)
	copy(merged, sessionTurns)

	for i := range subagents {
		merged = append(merged, subagents[i].Turns...)
	}
	return merged
}
