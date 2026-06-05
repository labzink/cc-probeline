// Package cost_test — residual-delta attribution (Phase 6.9, task 3 / variant A).
package cost_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// TestReconcile_ResidualDeltaToLatestTurn locks the table-vs-cost leak fix: when a
// positive delta arrives but every visible turn is already fixed (no new turns this
// cycle — the cost streamed in after the latest turn was first recorded), the delta
// must attach to the most recent turn rather than being dropped. Otherwise
// LastSeenTotal advances while Σ PerTurnCost does not, leaking the gap (observed
// ~$0.40 in a 90-turn session).
func TestReconcile_ResidualDeltaToLatestTurn(t *testing.T) {
	t0 := time.Now()
	turns := []parser.Turn{
		{UUID: "t1", Model: "claude-opus-4", Timestamp: t0, Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "t2", Model: "claude-opus-4", Timestamp: t0.Add(time.Minute), Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	cost.Reconcile(st, 100.0, 0, turns) // first-init: baseline=100, no distribution
	cost.Reconcile(st, 110.0, 0, turns) // delta=10 across t1,t2 → both now fixed

	c2before, _ := cost.PerTurn(st, "t2")

	// No new turns this cycle; delta=0.5 must attach to the latest turn (t2 by ts).
	cost.Reconcile(st, 110.5, 0, turns)

	c1, _ := cost.PerTurn(st, "t1")
	c2, _ := cost.PerTurn(st, "t2")

	if !approxEqual(c2-c2before, 0.5) {
		t.Errorf("residual: latest turn t2 should gain 0.5, gained %.4f", c2-c2before)
	}
	// Invariant: Σ PerTurnCost == SessionTotal (= LastSeenTotal − BaselineCost).
	if got, want := c1+c2, cost.SessionTotal(st, 110.5); !approxEqual(got, want) {
		t.Errorf("Σ PerTurnCost = %.4f; want == SessionTotal %.4f (no leak)", got, want)
	}
}
