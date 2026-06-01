// Package cost implements the delta-based cost API for cc-probeline.
//
// Rationale: instead of maintaining a per-model pricing table (error-prone
// as Anthropic changes prices), we take the ccTotal USD value reported by CC
// itself and compute deltas relative to a per-session baseline captured on
// the first Reconcile call.
//
// See project_cost_methodology memory and spec-common.md §2.2/§2.3.
package cost

import (
	"fmt"
	"log/slog"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// Reconcile distributes the new cost delta between the previous LastSeenTotal
// and the current ccTotal across turns that do not yet have a PerTurnCost entry.
// Distribution is proportional to each turn's output-token count.
//
// First call (st.Initialized=false): captures BaselineCost=ccTotal, sets
// Initialized=true. Delta = ccTotal − st.LastSeenTotal (zero value = 0) is
// distributed among all turns present. For a fresh session this is ccTotal
// itself (st.LastSeenTotal starts at 0).
//
// Subsequent calls: delta = ccTotal − LastSeenTotal is distributed among turns
// whose UUID is not yet present in st.PerTurnCost. Already-fixed entries are
// immutable (idempotent reconciliation).
//
// LastSeenTotal is updated to ccTotal after every call.
func Reconcile(st *state.Session, ccTotal float64, turns []parser.Turn) {
	slog.Debug("cost.Reconcile start", "initialized", st.Initialized, "ccTotal", ccTotal, "turns", len(turns))

	if !st.Initialized {
		// First observation for this session_id: capture baseline.
		st.BaselineCost = ccTotal
		st.Initialized = true
		slog.Info("cost.Reconcile baseline captured", "baseline", ccTotal)
		// Fall through to distribute delta = ccTotal - 0 (LastSeenTotal zero value).
	}

	// Compute the cost delta since last reconcile.
	delta := ccTotal - st.LastSeenTotal
	if delta < 0 {
		// Negative delta can occur on /clear (new session_id should have been used
		// but wasn't); clamp to 0 to avoid corrupting existing entries.
		slog.Warn("cost.Reconcile negative delta clamped", "delta", delta, "ccTotal", ccTotal, "lastSeen", st.LastSeenTotal)
		delta = 0
	}

	// Collect turns without a PerTurnCost entry.
	var newTurns []parser.Turn
	var totalOutput int
	for _, t := range turns {
		if _, fixed := st.PerTurnCost[t.UUID]; !fixed {
			newTurns = append(newTurns, t)
			totalOutput += t.Tokens.Output
		}
	}

	if len(newTurns) > 0 && delta > 0 {
		if st.PerTurnCost == nil {
			st.PerTurnCost = make(map[string]float64, len(newTurns))
		}
		if totalOutput > 0 {
			// Distribute by output-token share.
			for _, t := range newTurns {
				share := float64(t.Tokens.Output) / float64(totalOutput)
				st.PerTurnCost[t.UUID] = delta * share
			}
		} else {
			// All new turns have 0 output tokens: distribute equally.
			perTurn := delta / float64(len(newTurns))
			for _, t := range newTurns {
				st.PerTurnCost[t.UUID] = perTurn
			}
		}
		slog.Debug("cost.Reconcile distributed", "delta", delta, "newTurns", len(newTurns), "totalOutput", totalOutput)
	}

	st.LastSeenTotal = ccTotal
}

// PerTurn returns the finalized cost for the given turn UUID.
// Returns (0, false) when the UUID is not in st.PerTurnCost (unknown turn).
// The caller should render "—" when ok=false.
func PerTurn(st *state.Session, turnUUID string) (float64, bool) {
	if st.PerTurnCost == nil {
		return 0, false
	}
	v, ok := st.PerTurnCost[turnUUID]
	return v, ok
}

// SessionTotal returns the cost incurred during this session:
//
//	SessionTotal = ccTotal − st.BaselineCost
//
// Resets naturally on /clear because a new session_id produces a new state
// file with a fresh BaselineCost equal to the ccTotal at that moment.
func SessionTotal(st *state.Session, ccTotal float64) float64 {
	return ccTotal - st.BaselineCost
}

// LastRequest returns the cost attributable to the current prompt group:
//
//	LastRequest = ccTotal − st.PromptCost[curGroupID]
//
// When curGroupID is absent from PromptCost, the missing entry defaults to 0,
// so LastRequest = ccTotal (safe default: full session cost).
func LastRequest(st *state.Session, ccTotal float64, curGroupID int) float64 {
	if st.PromptCost == nil {
		return ccTotal
	}
	baseline := st.PromptCost[curGroupID] // zero value when absent
	return ccTotal - baseline
}

// Format renders a USD amount as "$X.XX". Used by the renderer footer.
func Format(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}
