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
// Distribution is weighted by model family weights × token counts (see weights.go).
// All turns (orchestrator and subagent/sidechain) are included in the pool.
//
// First call (st.Initialized=false): captures BaselineCost=ccTotal and
// BaselineDurMS=durMS, sets Initialized=true. Delta = ccTotal − st.LastSeenTotal
// (zero value = 0) is distributed among all turns present.
//
// Subsequent calls: delta = ccTotal − LastSeenTotal is distributed among turns
// whose UUID is not yet present in st.PerTurnCost. Already-fixed entries are
// immutable (idempotent reconciliation).
//
// LastSeenTotal is updated to ccTotal after every call.
func Reconcile(st *state.Session, ccTotal float64, durMS int64, turns []parser.Turn) {
	slog.Debug("cost.Reconcile start", "initialized", st.Initialized, "ccTotal", ccTotal, "durMS", durMS, "turns", len(turns))

	if !st.Initialized {
		// First observation for this session_id: capture baselines.
		st.BaselineCost = ccTotal
		st.BaselineDurMS = durMS
		st.Initialized = true
		slog.Info("cost.Reconcile baseline captured", "baseline", ccTotal, "baselineDurMS", durMS)
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

	// Collect turns without a PerTurnCost entry (new turns only).
	var newTurns []parser.Turn
	for _, t := range turns {
		if _, fixed := st.PerTurnCost[t.UUID]; !fixed {
			newTurns = append(newTurns, t)
		}
	}

	// Record PromptCost[groupID] = cost at the start of each new group.
	// The start-of-group cost is st.LastSeenTotal (cost before this delta).
	// Only record groups not yet tracked (first time we see turns from that group).
	if len(newTurns) > 0 {
		if st.PromptCost == nil {
			st.PromptCost = make(map[int]float64)
		}
		for _, t := range newTurns {
			if t.GroupID > 0 {
				if _, seen := st.PromptCost[t.GroupID]; !seen {
					st.PromptCost[t.GroupID] = st.LastSeenTotal
				}
			}
		}
	}

	if len(newTurns) > 0 && delta > 0 {
		if st.PerTurnCost == nil {
			st.PerTurnCost = make(map[string]float64, len(newTurns))
		}

		// Compute weighted units for each new turn using model family weights.
		type weightedTurn struct {
			turn  parser.Turn
			units float64
		}
		weighted := make([]weightedTurn, len(newTurns))
		var totalUnits float64
		for i, t := range newTurns {
			w := ModelWeights(t.Model)
			units := float64(t.Tokens.Input)*w.In +
				float64(t.Tokens.CacheRead)*w.CacheRead +
				float64(t.Tokens.CacheCreate)*w.CacheCreate +
				float64(t.Tokens.Output)*w.Out
			weighted[i] = weightedTurn{turn: t, units: units}
			totalUnits += units
		}

		if totalUnits > 0 {
			// Distribute delta proportionally to weighted units: Σ cost = Δ exactly.
			for _, wt := range weighted {
				share := wt.units / totalUnits
				st.PerTurnCost[wt.turn.UUID] = delta * share
			}
		} else {
			// All new turns have zero token counts: distribute equally (spec §2.3 Σunits=0 fallback).
			perTurn := delta / float64(len(newTurns))
			for _, wt := range weighted {
				st.PerTurnCost[wt.turn.UUID] = perTurn
			}
		}
		slog.Debug("cost.Reconcile distributed", "delta", delta, "newTurns", len(newTurns), "totalUnits", totalUnits)
	}

	st.LastSeenTotal = ccTotal
}

// SessionDuration returns the API duration elapsed since the baseline was
// captured on the first Reconcile call:
//
//	SessionDuration = durMS − st.BaselineDurMS
//
// Resets naturally on /clear because a new session_id produces a fresh state
// with a new BaselineDurMS. Returns 0 when st is nil or not yet initialized.
func SessionDuration(st *state.Session, durMS int64) int64 {
	if st == nil || !st.Initialized {
		return 0
	}
	return durMS - st.BaselineDurMS
}

// SubagentTotal returns the cumulative PerTurnCost for all turns whose UUID
// appears in the given list. Turns not present in st.PerTurnCost contribute 0.
// Returns 0 when st is nil or st.PerTurnCost is nil.
func SubagentTotal(st *state.Session, turnUUIDs []string) float64 {
	if st == nil || st.PerTurnCost == nil {
		return 0
	}
	var total float64
	for _, uuid := range turnUUIDs {
		total += st.PerTurnCost[uuid]
	}
	return total
}

// PerTurn returns the finalized cost for the given turn UUID.
// Returns (0, false) when st is nil or the UUID is not in st.PerTurnCost.
// The caller should render "—" when ok=false.
func PerTurn(st *state.Session, turnUUID string) (float64, bool) {
	if st == nil || st.PerTurnCost == nil {
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
// Returns 0 when st is nil (state not yet loaded).
func SessionTotal(st *state.Session, ccTotal float64) float64 {
	if st == nil {
		return 0
	}
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
