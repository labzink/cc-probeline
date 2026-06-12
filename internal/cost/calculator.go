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
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// Reconcile recomputes the per-turn cost map from scratch on every call
// (Phase 7.45 B2 — stateless distribution). Instead of accumulating per-turn
// deltas (which mis-attributed cost to the wrong turn when CC reported a turn's
// price one tick after the turn first appeared), each tick spreads the whole
// SessionTotal across the in-session turn pool, weighted by model family
// weights × token counts:
//
//	PerTurnCost[turn] = SessionTotal × units(turn) / Σ units(pool)
//
// The pool is every turn (orchestrator and subagent/sidechain) strictly newer
// than st.BaselineTurnTime. This guarantees Σ PerTurnCost == SessionTotal
// exactly and that the turn with the largest weighted tokens carries the
// largest cost. No residual branch, no off-by-one, no immutability.
//
// First call (st.Initialized=false): captures BaselineCost=ccTotal,
// BaselineDurMS=durMS, BaselineTurnTime=newest turn timestamp, sets
// Initialized=true. SessionTotal is 0 at this point, so the recomputed map is
// empty (the visible turns predate observation and render "—").
//
// LastSeenTotal is kept as a monotonic high-water mark used only to baseline
// prompt groups for LastRequest; it never drives the per-turn distribution.
func Reconcile(st *state.Session, ccTotal float64, durMS int64, turns []parser.Turn) {
	slog.Debug("cost.Reconcile start", "initialized", st.Initialized, "ccTotal", ccTotal, "durMS", durMS, "turns", len(turns))

	if !st.Initialized {
		st.BaselineCost = ccTotal
		st.BaselineDurMS = durMS
		st.BaselineTurnTime = maxTurnTime(turns)
		st.LastSeenTotal = ccTotal
		st.Initialized = true
		recomputePerTurn(st, ccTotal, turns)
		slog.Info("cost.Reconcile baseline captured", "baseline", ccTotal, "baselineDurMS", durMS, "baselineTurnTime", st.BaselineTurnTime)
		return
	}

	// Record PromptCost[groupID] = cost at the start of each newly seen group,
	// using the pre-rise LastSeenTotal (the running total when the group began).
	// LastRequest = ccTotal − PromptCost[group].
	if st.PromptCost == nil {
		st.PromptCost = make(map[int]float64)
	}
	for _, t := range turns {
		if t.GroupID > 0 {
			if _, seen := st.PromptCost[t.GroupID]; !seen {
				st.PromptCost[t.GroupID] = st.LastSeenTotal
			}
		}
	}

	// LastSeenTotal is a monotonic high-water mark: a transient CC dip must not
	// lower it, else a later rise back would mis-baseline a prompt group.
	if ccTotal > st.LastSeenTotal {
		st.LastSeenTotal = ccTotal
	}

	recomputePerTurn(st, ccTotal, turns)
}

// recomputePerTurn rebuilds st.PerTurnCost for the current tick: SessionTotal
// (ccTotal − BaselineCost, clamped at 0) spread across the pool of turns newer
// than BaselineTurnTime, weighted by model family weights × token counts. When
// the pool has zero total weighted units it falls back to an equal split; an
// empty pool yields an empty map (every visible turn renders "—").
func recomputePerTurn(st *state.Session, ccTotal float64, turns []parser.Turn) {
	sessionTotal := ccTotal - st.BaselineCost
	if sessionTotal < 0 {
		sessionTotal = 0
	}

	type weightedTurn struct {
		uuid  string
		units float64
	}
	var pool []weightedTurn
	var totalUnits float64
	for _, t := range turns {
		if t.UUID == "" || !t.Timestamp.After(st.BaselineTurnTime) {
			continue
		}
		w := ModelWeights(t.Model)
		units := float64(t.Tokens.Input)*w.In +
			float64(t.Tokens.CacheRead)*w.CacheRead +
			float64(t.Tokens.CacheCreate)*w.CacheCreate +
			float64(t.Tokens.Output)*w.Out
		pool = append(pool, weightedTurn{uuid: t.UUID, units: units})
		totalUnits += units
	}

	m := make(map[string]float64, len(pool))
	switch {
	case len(pool) == 0:
		// nothing in session yet
	case totalUnits > 0:
		for _, p := range pool {
			m[p.uuid] = sessionTotal * p.units / totalUnits
		}
	default:
		per := sessionTotal / float64(len(pool))
		for _, p := range pool {
			m[p.uuid] = per
		}
	}
	st.PerTurnCost = m
	slog.Debug("cost.recomputePerTurn", "sessionTotal", sessionTotal, "pool", len(pool), "totalUnits", totalUnits)
}

// maxTurnTime returns the newest Timestamp among the given turns, or the zero
// time when turns is empty. Used to fix BaselineTurnTime at observation start.
func maxTurnTime(turns []parser.Turn) time.Time {
	var max time.Time
	for _, t := range turns {
		if t.Timestamp.After(max) {
			max = t.Timestamp
		}
	}
	return max
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
