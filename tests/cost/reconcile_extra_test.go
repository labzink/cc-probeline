// Package cost_test — additional coverage tests for edge paths in Reconcile and LastRequest.
// These are additive: they do not duplicate RED-phase assertions.
package cost_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// TestReconcile_NegativeDelta verifies that a negative delta (ccTotal < LastSeenTotal)
// is clamped to 0 and does not corrupt existing PerTurnCost entries.
func TestReconcile_NegativeDelta(t *testing.T) {
	turns := []parser.Turn{
		{UUID: "turn-X", Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}

	// First call: baseline=5.0, PerTurnCost[turn-X] = 5.0.
	cost.Reconcile(st, 5.0, int64(0), turns)
	gotFirst, _ := cost.PerTurn(st, "turn-X")

	// Second call: ccTotal goes down (e.g. after session restore bug).
	// Delta should clamp to 0; existing entries must be unchanged.
	cost.Reconcile(st, 4.0, int64(0), turns)
	gotSecond, _ := cost.PerTurn(st, "turn-X")

	if !approxEqual(gotFirst, gotSecond) {
		t.Errorf("Reconcile negative delta: PerTurn changed from %.6f to %.6f (must be immutable)", gotFirst, gotSecond)
	}
}

// TestReconcile_ZeroTokensCostZero verifies that a turn with no billable tokens
// estimates to $0 (Phase 7.46: cost is purely token-derived — there is no
// equal-split fallback anymore).
func TestReconcile_ZeroTokensCostZero(t *testing.T) {
	ts := time.Now()
	turns := []parser.Turn{
		{UUID: "turn-1", Model: "claude-opus-4-8", Timestamp: ts, Tokens: parser.TokenCounts{}},
		{UUID: "turn-2", Model: "claude-opus-4-8", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	cost.Reconcile(st, 2.0, int64(0), turns)

	got1, ok1 := cost.PerTurn(st, "turn-1")
	got2, ok2 := cost.PerTurn(st, "turn-2")
	if !ok1 || !ok2 {
		t.Fatalf("Reconcile: turns not in PerTurnCost after Reconcile")
	}
	if !approxEqual(got1, 0.0) {
		t.Errorf("PerTurn(turn-1, zero tokens) = %.6f; want 0.0", got1)
	}
	if !approxEqual(got2, 0.025) {
		t.Errorf("PerTurn(turn-2) = %.6f; want 0.025 (1000*25/1e6)", got2)
	}
}

// TestReconcile_FirstInitPricesImmediately verifies the Phase 7.46 contract: the
// first observation prices every turn from its tokens right away (no waiting for
// a second tick), and calling Reconcile again with the same tokens does not
// inflate the totals (idempotent — guards against the old re-dump inflation).
func TestReconcile_FirstInitPricesImmediately(t *testing.T) {
	turns := []parser.Turn{
		{UUID: "t1", Model: "claude-opus-4-8", Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "t2", Model: "claude-opus-4-8", Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	cost.Reconcile(st, 93.50, int64(1000), turns)

	c1, ok := cost.PerTurn(st, "t1")
	if !ok || !approxEqual(c1, 0.025) {
		t.Errorf("first-init must price t1 immediately; got %.6f ok=%v (want 0.025)", c1, ok)
	}
	first := cost.SessionTotal(st, 93.50)
	if !approxEqual(first, 0.050) {
		t.Errorf("SessionTotal after first init = %.6f; want 0.050 (2×1000×25/1e6)", first)
	}
	// Idempotent: same tokens again → same total (no inflation).
	cost.Reconcile(st, 200.0, int64(2000), turns)
	if again := cost.SessionTotal(st, 200.0); !approxEqual(again, first) {
		t.Errorf("re-Reconcile inflated SessionTotal %.6f → %.6f", first, again)
	}
}

// TestReconcile_DipRiseNoDoubleCount locks the high-water-mark fix: when ccTotal
// dips below the last-seen total (transient CC value / wrongly reused session_id)
// and later rises back, the rise must NOT be re-distributed as a fresh delta.
// Without it, a dip→rise double-counts already-attributed cost (observed Σ $229
// vs ccTotal $129, with single turns ballooning to $107).
func TestReconcile_DipRiseNoDoubleCount(t *testing.T) {
	turns := []parser.Turn{
		{UUID: "t1", Model: "claude-opus-4", Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "t2", Model: "claude-opus-4", Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	cost.Reconcile(st, 100.0, int64(0), turns) // first-init: baseline=100, no distribution
	cost.Reconcile(st, 110.0, int64(0), turns) // delta=10 distributed across t1,t2

	c1, _ := cost.PerTurn(st, "t1")
	c2, _ := cost.PerTurn(st, "t2")
	sumAfterRise := c1 + c2

	cost.Reconcile(st, 20.0, int64(0), turns)  // ccTotal dips → must skip, LastSeen stays 110
	cost.Reconcile(st, 110.0, int64(0), turns) // back to 110 → delta=0 → must NOT re-distribute

	d1, _ := cost.PerTurn(st, "t1")
	d2, _ := cost.PerTurn(st, "t2")
	if !approxEqual(c1+c2, d1+d2) {
		t.Errorf("dip→rise must not re-distribute: Σ %.4f → %.4f", sumAfterRise, d1+d2)
	}
	if !approxEqual(st.LastSeenTotal, 110.0) {
		t.Errorf("LastSeenTotal must stay at high-water 110 after a dip, got %.4f", st.LastSeenTotal)
	}
}

// TestLastRequest_NilPromptCost verifies that LastRequest returns ccTotal
// when PromptCost is nil (safe default per spec).
func TestLastRequest_NilPromptCost(t *testing.T) {
	st := &state.Session{Initialized: true, PromptCost: nil}
	got := cost.LastRequest(st, 3.50, 1)
	if !approxEqual(got, 3.50) {
		t.Errorf("LastRequest(nilMap, ccTotal=3.50, group=1) = %.6f; want 3.50", got)
	}
}

// TestReconcile_CostHistoryRecordsSteps verifies the Phase 7.46 diagnostic trail:
// one CostHistory sample per distinct official ccTotal (identical ticks are not
// duplicated), each pairing the official sum with our running estimate, turn
// count, and newest turn UUID.
func TestReconcile_CostHistoryRecordsSteps(t *testing.T) {
	ts := time.Now()
	turns := []parser.Turn{
		{UUID: "t1", Model: "claude-opus-4-8", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	cost.Reconcile(st, 1.0, 100, turns) // first sample
	cost.Reconcile(st, 1.0, 105, turns) // ccTotal unchanged → no new sample
	turns2 := []parser.Turn{
		{UUID: "t1", Model: "claude-opus-4-8", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "t2", Model: "claude-opus-4-8", Timestamp: ts.Add(time.Minute), Tokens: parser.TokenCounts{Output: 2000}},
	}
	cost.Reconcile(st, 2.5, 200, turns2) // ccTotal advanced → new sample

	if len(st.CostHistory) != 2 {
		t.Fatalf("CostHistory len = %d; want 2 (one per distinct ccTotal)", len(st.CostHistory))
	}
	if st.CostHistory[0].CCTotal != 1.0 || st.CostHistory[1].CCTotal != 2.5 {
		t.Errorf("CostHistory ccTotal = [%v %v]; want [1.0 2.5]", st.CostHistory[0].CCTotal, st.CostHistory[1].CCTotal)
	}
	// After t2: Σ estimate = (1000+2000)*25/1e6 = 0.075.
	if !approxEqual(st.CostHistory[1].Estimate, 0.075) {
		t.Errorf("sample estimate = %.6f; want 0.075", st.CostHistory[1].Estimate)
	}
	if st.CostHistory[1].Turns != 2 {
		t.Errorf("sample turns = %d; want 2", st.CostHistory[1].Turns)
	}
	if st.CostHistory[1].NewestTurn != "t2" {
		t.Errorf("sample newest turn = %q; want t2", st.CostHistory[1].NewestTurn)
	}
}
