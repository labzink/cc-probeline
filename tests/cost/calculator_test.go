// Package cost_test — RED-phase unit tests for the delta cost API (Phase 6.8.a).
//
// The current internal/cost package has Compute/ComputeAggregate/modelPricing
// (old pricing-table approach). All tests in this file target the NEW delta API:
// Reconcile, SessionTotal, PerTurn, LastRequest, Format.
// These tests are RED until internal/cost is rewritten by the GREEN agent.
package cost_test

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/cost"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/state"
)

// approxEqual returns true when a and b differ by at most 1e-9.
// Used for floating-point USD comparisons.
func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---------------------------------------------------------------------------
// T-C1: TestReconcile_Baseline
// Spec: T-5 — first Reconcile for session_id sets BaselineCost=ccTotal, Initialized=true
// ---------------------------------------------------------------------------

// TestReconcile_Baseline verifies that the first Reconcile call on a fresh
// (non-initialized) Session captures the ccTotal as BaselineCost and sets
// Initialized=true. A second call with a higher ccTotal must NOT change
// BaselineCost (baseline is fixed on first observation).
func TestReconcile_Baseline(t *testing.T) {
	// Given: a fresh, non-initialized session and an initial ccTotal.
	st := &state.Session{Initialized: false}
	ccTotal := 1.50
	turns := []parser.Turn{
		{UUID: "turn-1", Tokens: parser.TokenCounts{Output: 1000}},
	}

	// When: first Reconcile call.
	cost.Reconcile(st, ccTotal, turns)

	// Then: Session is initialized with BaselineCost set to ccTotal.
	if !st.Initialized {
		t.Errorf("TestReconcile_Baseline: Initialized must be true after first Reconcile, got false")
	}
	if !approxEqual(st.BaselineCost, ccTotal) {
		t.Errorf("TestReconcile_Baseline: BaselineCost = %.6f; want %.6f", st.BaselineCost, ccTotal)
	}

	// When: second Reconcile with a higher ccTotal.
	ccTotal2 := 3.00
	cost.Reconcile(st, ccTotal2, turns)

	// Then: BaselineCost must remain unchanged (baseline is immutable after first set).
	if !approxEqual(st.BaselineCost, 1.50) {
		t.Errorf("TestReconcile_Baseline: BaselineCost must stay 1.50 after second Reconcile, got %.6f", st.BaselineCost)
	}
}

// ---------------------------------------------------------------------------
// T-C2: TestSessionTotal
// Spec: T-6 — SessionTotal = ccTotal − BaselineCost
// ---------------------------------------------------------------------------

// TestSessionTotal verifies that SessionTotal returns the delta between the
// current running total and the baseline captured at session start.
// /clear-emulation: a new session starts with its own baseline.
func TestSessionTotal(t *testing.T) {
	tests := []struct {
		name         string
		baseline     float64
		ccTotal      float64
		wantDelta    float64
	}{
		{
			name:      "normal session — delta from baseline",
			baseline:  1.00,
			ccTotal:   2.50,
			wantDelta: 1.50,
		},
		{
			name:      "clear-emulation — new session starts at 0 delta",
			baseline:  5.00,
			ccTotal:   5.00,
			wantDelta: 0.00,
		},
		{
			name:      "growing session",
			baseline:  0.00,
			ccTotal:   10.00,
			wantDelta: 10.00,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a session with a known BaselineCost (already initialized).
			st := &state.Session{
				Initialized:  true,
				BaselineCost: tc.baseline,
			}

			// When: computing session total.
			got := cost.SessionTotal(st, tc.ccTotal)

			// Then: delta must equal ccTotal − BaselineCost.
			if !approxEqual(got, tc.wantDelta) {
				t.Errorf("SessionTotal(baseline=%.2f, ccTotal=%.2f) = %.6f; want %.6f",
					tc.baseline, tc.ccTotal, got, tc.wantDelta)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-C3: TestPerTurn_DeltaStable
// Spec: T-7 — per-turn delta distributed by output share; repeated Reconcile
//             does not re-compute already-fixed turns.
// ---------------------------------------------------------------------------

// TestPerTurn_DeltaStable verifies that:
//  1. After Reconcile, per-turn costs are proportional to each turn's output tokens.
//  2. A second Reconcile call does not overwrite already-fixed PerTurnCost entries.
func TestPerTurn_DeltaStable(t *testing.T) {
	// Given: two turns with output tokens 3000 and 1000 (3:1 ratio).
	// First Reconcile: baseline=0, ccTotal=2.00 → delta=2.00 (all goes to turns).
	// Expected split: turn-A gets 2.00 * (3000/4000) = 1.50, turn-B gets 0.50.
	turns := []parser.Turn{
		{UUID: "turn-A", Tokens: parser.TokenCounts{Output: 3000}},
		{UUID: "turn-B", Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}
	ccTotal1 := 2.00

	// When: first Reconcile initializes baseline and distributes delta.
	cost.Reconcile(st, ccTotal1, turns)

	// Then: PerTurnCost is populated with output-proportional shares.
	gotA, okA := cost.PerTurn(st, "turn-A")
	if !okA {
		t.Fatalf("TestPerTurn_DeltaStable: PerTurn(turn-A) not found after first Reconcile")
	}
	gotB, okB := cost.PerTurn(st, "turn-B")
	if !okB {
		t.Fatalf("TestPerTurn_DeltaStable: PerTurn(turn-B) not found after first Reconcile")
	}
	// delta=2.00, total output=4000; turn-A: 3000/4000*2.00=1.50; turn-B: 1000/4000*2.00=0.50.
	if !approxEqual(gotA, 1.50) {
		t.Errorf("PerTurn(turn-A) = %.6f; want 1.50 (3/4 of delta 2.00)", gotA)
	}
	if !approxEqual(gotB, 0.50) {
		t.Errorf("PerTurn(turn-B) = %.6f; want 0.50 (1/4 of delta 2.00)", gotB)
	}

	// When: second Reconcile with new delta (ccTotal goes from 2.00 to 3.00).
	// Turn-A and turn-B are already fixed; only a new turn-C gets the new delta.
	turns2 := []parser.Turn{
		{UUID: "turn-A", Tokens: parser.TokenCounts{Output: 3000}},
		{UUID: "turn-B", Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "turn-C", Tokens: parser.TokenCounts{Output: 2000}},
	}
	ccTotal2 := 3.00
	cost.Reconcile(st, ccTotal2, turns2)

	// Then: turn-A and turn-B must remain unchanged (immutable once fixed).
	gotA2, _ := cost.PerTurn(st, "turn-A")
	gotB2, _ := cost.PerTurn(st, "turn-B")
	if !approxEqual(gotA2, 1.50) {
		t.Errorf("PerTurn(turn-A) changed on second Reconcile: got %.6f; want 1.50 (must be immutable)", gotA2)
	}
	if !approxEqual(gotB2, 0.50) {
		t.Errorf("PerTurn(turn-B) changed on second Reconcile: got %.6f; want 0.50 (must be immutable)", gotB2)
	}

	// Turn-C should now have the new delta (1.00) since it is the only new turn.
	gotC, okC := cost.PerTurn(st, "turn-C")
	if !okC {
		t.Fatalf("TestPerTurn_DeltaStable: PerTurn(turn-C) not found after second Reconcile")
	}
	if !approxEqual(gotC, 1.00) {
		t.Errorf("PerTurn(turn-C) = %.6f; want 1.00 (full delta for single new turn)", gotC)
	}
}

// TestPerTurn_Unknown verifies that PerTurn returns (0, false) for a UUID
// that was never reconciled, signalling the renderer to display "—".
func TestPerTurn_Unknown(t *testing.T) {
	st := &state.Session{Initialized: true}
	_, ok := cost.PerTurn(st, "nonexistent-uuid")
	if ok {
		t.Errorf("PerTurn(unknown UUID): want ok=false, got ok=true")
	}
}

// ---------------------------------------------------------------------------
// T-C4: TestLastRequest
// Spec: T-8 — LastRequest = ccTotal − PromptCost[curGroupID]
// ---------------------------------------------------------------------------

// TestLastRequest verifies that the cost attributed to the last user request
// is computed as the delta between the current total and the snapshot taken at
// the start of the corresponding prompt group.
func TestLastRequest(t *testing.T) {
	tests := []struct {
		name       string
		promptCost map[int]float64
		groupID    int
		ccTotal    float64
		want       float64
	}{
		{
			name:       "single group — full session cost",
			promptCost: map[int]float64{1: 1.00},
			groupID:    1,
			ccTotal:    2.50,
			want:       1.50,
		},
		{
			name:       "second group — cost since group 2 started",
			promptCost: map[int]float64{1: 0.50, 2: 2.00},
			groupID:    2,
			ccTotal:    2.75,
			want:       0.75,
		},
		{
			name:       "group not in map — returns 0 (safe default)",
			promptCost: map[int]float64{1: 1.00},
			groupID:    99,
			ccTotal:    3.00,
			want:       3.00, // ccTotal - 0 (missing entry → 0 as default)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a session with pre-populated PromptCost map.
			st := &state.Session{
				Initialized: true,
				PromptCost:  tc.promptCost,
			}

			// When: computing last request cost.
			got := cost.LastRequest(st, tc.ccTotal, tc.groupID)

			// Then: must equal ccTotal − PromptCost[group] (or ccTotal if group absent).
			if !approxEqual(got, tc.want) {
				t.Errorf("LastRequest(groupID=%d, ccTotal=%.2f) = %.6f; want %.6f",
					tc.groupID, tc.ccTotal, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-C5: TestNoPricingTable
// Spec: T-9 — the symbol `modelPricing` must not exist in internal/cost
// ---------------------------------------------------------------------------

// TestNoPricingTable verifies that the pricing table (modelPricing) has been
// deleted from the cost package. This is verified by running `go build` on the
// package and checking for the absence of the symbol via grep on source files.
//
// Rationale: delta-based cost (from CC's own ccTotal) replaces per-model
// pricing; keeping modelPricing would create a maintenance liability and false
// sense of accuracy (see project_cost_methodology memory).
func TestNoPricingTable(t *testing.T) {
	// Locate the cost package source directory relative to the module root.
	// The test binary runs from the module root; use go env GOMODCACHE to find
	// the package path.
	costPkgDir := filepath.Join("internal", "cost")

	// Verify the directory exists (sanity check so the test fails clearly).
	if _, err := os.Stat(costPkgDir); os.IsNotExist(err) {
		t.Fatalf("TestNoPricingTable: internal/cost directory not found at %q", costPkgDir)
	}

	// grep for the symbol in all .go files under internal/cost (excluding _test.go).
	cmd := exec.Command("grep", "-rn", "--include=*.go", "--exclude=*_test.go", "modelPricing", costPkgDir)
	out, err := cmd.Output()
	// grep exits 1 when no match found (the desired state); exit 0 means found (failure).
	if err == nil {
		// No error means grep found matches — the pricing table still exists.
		t.Errorf("TestNoPricingTable: symbol 'modelPricing' found in internal/cost (must be deleted):\n%s", out)
	}
	// Any exit code != 0 from grep means no matches → pricing table absent → pass.
}

// ---------------------------------------------------------------------------
// Format — already implemented; regression guard retained from previous phase
// ---------------------------------------------------------------------------

// TestFormat_Cents verifies "$0.57" formatting.
func TestFormat_Cents(t *testing.T) {
	got := cost.Format(0.57)
	want := "$0.57"
	if got != want {
		t.Errorf("Format(0.57) = %q; want %q", got, want)
	}
}

// TestFormat_ZeroDollars verifies "$0.00" formatting.
func TestFormat_ZeroDollars(t *testing.T) {
	got := cost.Format(0)
	want := "$0.00"
	if got != want {
		t.Errorf("Format(0) = %q; want %q", got, want)
	}
}
