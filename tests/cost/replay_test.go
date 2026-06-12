// Package cost_test — Phase 7.45 B2 replay regression for stateless per-turn cost.
//
// The single-shot harnesses elsewhere prime a baseline and apply one delta. The
// real bug (money landing on the wrong turn) only shows under an incremental
// replay: turns appear over successive ticks while ccTotal trails one tick
// behind. This test replays such a trajectory through the production Reconcile
// path and locks the two contract invariants of the stateless distribution:
//
//	(1) Σ PerTurn over in-session turns == SessionTotal (exactly).
//	(2) the turn with the largest output carries the largest per-turn cost.
package cost_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// TestB2_ReplayStatelessDistribution replays an incremental fable session: a
// pre-observation baseline turn, then three turns (A, B-peak, C) appearing on
// successive ticks with ccTotal trailing. After the final tick the in-session
// pool {A,B,C} must sum to SessionTotal exactly and B (the peak-output turn)
// must hold the largest cost.
func TestB2_ReplayStatelessDistribution(t *testing.T) {
	base := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	const model = "claude-fable-5" // single model → output-proportional

	// Pre-observation turn (predates the baseline; must render "—").
	baseTurn := parser.Turn{UUID: "t0", Model: model, Timestamp: base, Tokens: parser.TokenCounts{Output: 1000}}

	// In-session turns; B has the peak output.
	turnA := parser.Turn{UUID: "tA", Model: model, Timestamp: base.Add(1 * time.Minute), Tokens: parser.TokenCounts{Output: 5000}}
	turnB := parser.Turn{UUID: "tB", Model: model, Timestamp: base.Add(2 * time.Minute), Tokens: parser.TokenCounts{Output: 20000}}
	turnC := parser.Turn{UUID: "tC", Model: model, Timestamp: base.Add(3 * time.Minute), Tokens: parser.TokenCounts{Output: 8000}}

	st := &state.Session{Initialized: false}

	// Tick 0 — first observation: baseline=$10.00, only the pre-obs turn visible.
	cost.Reconcile(st, 10.00, 0, []parser.Turn{baseTurn})
	// Ticks 1..3 — each adds a turn and ccTotal rises (trailing by construction).
	cost.Reconcile(st, 10.50, 0, []parser.Turn{baseTurn, turnA})
	cost.Reconcile(st, 12.00, 0, []parser.Turn{baseTurn, turnA, turnB})
	cost.Reconcile(st, 13.50, 0, []parser.Turn{baseTurn, turnA, turnB, turnC})

	// Pre-observation turn must not be attributed any cost.
	if _, ok := cost.PerTurn(st, "t0"); ok {
		t.Errorf("pre-observation turn t0 must render \"—\" (PerTurn ok=true)")
	}

	cA, okA := cost.PerTurn(st, "tA")
	cB, okB := cost.PerTurn(st, "tB")
	cC, okC := cost.PerTurn(st, "tC")
	if !okA || !okB || !okC {
		t.Fatalf("in-session turns missing: A=%v B=%v C=%v", okA, okB, okC)
	}

	// Invariant (1): Σ PerTurn == SessionTotal (= 13.50 − 10.00 = 3.50).
	sessionTotal := cost.SessionTotal(st, 13.50)
	if got := cA + cB + cC; !approxEqual(got, sessionTotal) {
		t.Errorf("Σ PerTurn = %.6f; want == SessionTotal %.6f", got, sessionTotal)
	}
	if !approxEqual(sessionTotal, 3.50) {
		t.Errorf("SessionTotal = %.6f; want 3.50", sessionTotal)
	}

	// Invariant (2): the peak-output turn (B) carries the largest cost.
	if !(cB > cA && cB > cC) {
		t.Errorf("peak-output turn B must hold the largest cost: A=%.4f B=%.4f C=%.4f", cA, cB, cC)
	}

	// Sanity: equal model → output-proportional. B/A == 20000/5000 == 4.
	if cA > 0 {
		if ratio := cB / cA; !approxEqual(ratio, 4.0) {
			t.Errorf("cB/cA = %.4f; want 4.0 (output ratio 20000:5000)", ratio)
		}
	}
}
