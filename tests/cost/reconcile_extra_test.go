// Package cost_test — additional coverage tests for edge paths in Reconcile and LastRequest.
// These are additive: they do not duplicate RED-phase assertions.
package cost_test

import (
	"testing"

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

// TestReconcile_ZeroOutputTokens verifies equal distribution when all new turns
// have 0 output tokens (fallback from proportional to equal-share).
func TestReconcile_ZeroOutputTokens(t *testing.T) {
	turns := []parser.Turn{
		{UUID: "turn-1", Tokens: parser.TokenCounts{Output: 0}},
		{UUID: "turn-2", Tokens: parser.TokenCounts{Output: 0}},
	}
	st := &state.Session{Initialized: false}
	// Prime: first observation only initialises baseline (delta=0, cost fix).
	cost.Reconcile(st, 0.0, int64(0), turns)
	// delta = 2.0 distributed equally between 2 turns → 1.0 each.
	cost.Reconcile(st, 2.0, int64(0), turns)

	got1, ok1 := cost.PerTurn(st, "turn-1")
	got2, ok2 := cost.PerTurn(st, "turn-2")
	if !ok1 || !ok2 {
		t.Fatalf("Reconcile zero output: turns not in PerTurnCost after Reconcile")
	}
	if !approxEqual(got1, 1.0) {
		t.Errorf("PerTurn(turn-1) = %.6f; want 1.0 (equal share)", got1)
	}
	if !approxEqual(got2, 1.0) {
		t.Errorf("PerTurn(turn-2) = %.6f; want 1.0 (equal share)", got2)
	}
}

// TestReconcile_FirstInitNoDistribution locks the cost-inflation fix: the first
// observation of a session captures the baseline and records LastSeenTotal=ccTotal,
// distributing nothing. Distributing the full historical ccTotal here dumped it
// onto the first visible turns (and re-dumped on every re-init), inflating
// Σ PerTurnCost far beyond ccTotal (observed $389 vs $116).
func TestReconcile_FirstInitNoDistribution(t *testing.T) {
	turns := []parser.Turn{
		{UUID: "t1", Model: "claude-opus-4", Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "t2", Model: "claude-opus-4", Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	cost.Reconcile(st, 93.50, int64(1000), turns)

	if _, ok := cost.PerTurn(st, "t1"); ok {
		t.Errorf("first-init must distribute nothing; PerTurn(t1) unexpectedly set")
	}
	// SessionTotal at first observation must be 0 (baseline == ccTotal).
	if got := cost.SessionTotal(st, 93.50); !approxEqual(got, 0.0) {
		t.Errorf("SessionTotal after first init = %.6f; want 0 (baseline==ccTotal)", got)
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
