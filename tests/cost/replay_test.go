// Package cost_test — Phase 7.46 replay regression for table-driven per-turn cost.
//
// The real bug (a big turn's displayed cost crawling for many ticks) only shows
// under an incremental replay: turns appear over successive ticks while the
// official ccTotal trails behind. This test replays such a trajectory through
// the production Reconcile path and locks the Phase 7.46 contract:
//
//	(1) each turn is priced from its own tokens the moment it appears and that
//	    price never moves as later turns arrive (no dancing numbers);
//	(2) SessionTotal == Σ per-turn estimates;
//	(3) the turn with the largest output carries the largest per-turn cost.
package cost_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// TestReplay_TableStableNoDancing replays an incremental fable session: a turn
// observed at baseline, then three turns (A, B-peak, C) appearing on successive
// ticks while ccTotal trails. tA's cost captured the tick it appears must equal
// its cost after all later turns arrive; SessionTotal must equal the sum of the
// estimates; B (peak output) must be the largest.
func TestReplay_TableStableNoDancing(t *testing.T) {
	base := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	const model = "claude-fable-5" // single model → output-proportional; fable Out=50

	t0 := parser.Turn{UUID: "t0", Model: model, Timestamp: base, Tokens: parser.TokenCounts{Output: 1000}}
	tA := parser.Turn{UUID: "tA", Model: model, Timestamp: base.Add(1 * time.Minute), Tokens: parser.TokenCounts{Output: 5000}}
	tB := parser.Turn{UUID: "tB", Model: model, Timestamp: base.Add(2 * time.Minute), Tokens: parser.TokenCounts{Output: 20000}}
	tC := parser.Turn{UUID: "tC", Model: model, Timestamp: base.Add(3 * time.Minute), Tokens: parser.TokenCounts{Output: 8000}}

	st := &state.Session{Initialized: false}

	cost.Reconcile(st, 10.00, 0, []parser.Turn{t0})
	// Capture tA's cost the tick it first appears (ccTotal trailing).
	cost.Reconcile(st, 10.50, 0, []parser.Turn{t0, tA})
	cA1, _ := cost.PerTurn(st, "tA")
	cost.Reconcile(st, 12.00, 0, []parser.Turn{t0, tA, tB})
	cost.Reconcile(st, 13.50, 0, []parser.Turn{t0, tA, tB, tC})

	// All turns are priced — no "—" for the baseline turn anymore.
	c0, ok0 := cost.PerTurn(st, "t0")
	cA, okA := cost.PerTurn(st, "tA")
	cB, okB := cost.PerTurn(st, "tB")
	cC, okC := cost.PerTurn(st, "tC")
	if !ok0 || !okA || !okB || !okC {
		t.Fatalf("all turns must be priced: t0=%v tA=%v tB=%v tC=%v", ok0, okA, okB, okC)
	}

	// Invariant (1): tA's cost did not dance between its first appearance and now.
	if !approxEqual(cA, cA1) {
		t.Errorf("tA cost danced: %.6f at first appearance → %.6f after later turns (must be stable)", cA1, cA)
	}
	// fable Out weight = 50: estimate = out*50/1e6.
	if want := 20000 * 50.0 / 1e6; !approxEqual(cB, want) {
		t.Errorf("tB estimate = %.6f; want %.6f (20000*50/1e6)", cB, want)
	}

	// Invariant (2): SessionTotal == Σ all estimates.
	if got, want := cost.SessionTotal(st, 13.50), c0+cA+cB+cC; !approxEqual(got, want) {
		t.Errorf("SessionTotal %.6f != Σ PerTurn %.6f", got, want)
	}

	// Invariant (3): the peak-output turn (B) carries the largest cost.
	if !(cB > cA && cB > cC && cB > c0) {
		t.Errorf("peak-output turn B must hold the largest cost: t0=%.4f A=%.4f B=%.4f C=%.4f", c0, cA, cB, cC)
	}
}
