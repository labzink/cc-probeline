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
	"time"

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
	cost.Reconcile(st, ccTotal, int64(0), turns)

	// Then: Session is initialized with BaselineCost set to ccTotal.
	if !st.Initialized {
		t.Errorf("TestReconcile_Baseline: Initialized must be true after first Reconcile, got false")
	}
	if !approxEqual(st.BaselineCost, ccTotal) {
		t.Errorf("TestReconcile_Baseline: BaselineCost = %.6f; want %.6f", st.BaselineCost, ccTotal)
	}

	// When: second Reconcile with a higher ccTotal.
	ccTotal2 := 3.00
	cost.Reconcile(st, ccTotal2, int64(0), turns)

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
		name      string
		baseline  float64
		ccTotal   float64
		wantDelta float64
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

// TestPerTurn_DeltaStable verifies the stateless distribution (Phase 7.45 B2):
//  1. Per-turn costs are proportional to each turn's output tokens (same model).
//  2. When a new turn appears, the whole map is recomputed from SessionTotal —
//     every in-session turn reflects the current pool (no frozen/immutable
//     entries), and Σ PerTurn == SessionTotal.
func TestPerTurn_DeltaStable(t *testing.T) {
	ts := time.Now()
	// Given: two turns with output tokens 3000 and 1000 (3:1 ratio), newer than
	// the baseline (primed with an empty turn slice so BaselineTurnTime=zero).
	turns := []parser.Turn{
		{UUID: "turn-A", Timestamp: ts, Tokens: parser.TokenCounts{Output: 3000}},
		{UUID: "turn-B", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
	}
	st := &state.Session{Initialized: false}

	// Prime: first observation captures baseline=0 with no in-session turns.
	cost.Reconcile(st, 0.0, int64(0), nil)
	// When: SessionTotal becomes 2.00, spread over the two turns.
	cost.Reconcile(st, 2.00, int64(0), turns)

	gotA, okA := cost.PerTurn(st, "turn-A")
	if !okA {
		t.Fatalf("TestPerTurn_DeltaStable: PerTurn(turn-A) not found")
	}
	gotB, okB := cost.PerTurn(st, "turn-B")
	if !okB {
		t.Fatalf("TestPerTurn_DeltaStable: PerTurn(turn-B) not found")
	}
	// SessionTotal=2.00, total output=4000; A: 3000/4000*2.00=1.50; B: 0.50.
	if !approxEqual(gotA, 1.50) {
		t.Errorf("PerTurn(turn-A) = %.6f; want 1.50 (3/4 of SessionTotal 2.00)", gotA)
	}
	if !approxEqual(gotB, 0.50) {
		t.Errorf("PerTurn(turn-B) = %.6f; want 0.50 (1/4 of SessionTotal 2.00)", gotB)
	}

	// When: a new turn-C appears and SessionTotal grows to 3.00. The map is
	// recomputed over the full pool {A,B,C} with outputs 3000/1000/2000 = 6000.
	turns2 := []parser.Turn{
		{UUID: "turn-A", Timestamp: ts, Tokens: parser.TokenCounts{Output: 3000}},
		{UUID: "turn-B", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "turn-C", Timestamp: ts, Tokens: parser.TokenCounts{Output: 2000}},
	}
	cost.Reconcile(st, 3.00, int64(0), turns2)

	gotA2, _ := cost.PerTurn(st, "turn-A")
	gotB2, _ := cost.PerTurn(st, "turn-B")
	gotC, okC := cost.PerTurn(st, "turn-C")
	if !okC {
		t.Fatalf("TestPerTurn_DeltaStable: PerTurn(turn-C) not found")
	}
	// A: 3000/6000*3.00=1.50; B: 1000/6000*3.00=0.50; C: 2000/6000*3.00=1.00.
	if !approxEqual(gotA2, 1.50) {
		t.Errorf("PerTurn(turn-A) = %.6f; want 1.50 (3/6 of SessionTotal 3.00)", gotA2)
	}
	if !approxEqual(gotB2, 0.50) {
		t.Errorf("PerTurn(turn-B) = %.6f; want 0.50 (1/6 of SessionTotal 3.00)", gotB2)
	}
	if !approxEqual(gotC, 1.00) {
		t.Errorf("PerTurn(turn-C) = %.6f; want 1.00 (2/6 of SessionTotal 3.00)", gotC)
	}
	// Invariant: Σ PerTurn == SessionTotal.
	if got, want := gotA2+gotB2+gotC, cost.SessionTotal(st, 3.00); !approxEqual(got, want) {
		t.Errorf("Σ PerTurn = %.6f; want == SessionTotal %.6f", got, want)
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

// =============================================================================
// Phase 6.9.a — RED tests (T-16, T-16b, T-17, T-19, T-20)
//
// NOTE: These tests call cost.Reconcile with the NEW 4-argument signature:
//   Reconcile(st *state.Session, ccTotal float64, durMS int64, turns []parser.Turn)
//
// The existing tests above (T-C1..T-C3) use the old 3-arg form and will need
// updating by the GREEN agent when the signature changes. Both are intentionally
// left here; the RED phase means this file does NOT compile until GREEN lands.
// =============================================================================

// ---------------------------------------------------------------------------
// T-16: TestReconcile_WeightedSumEqualsDelta
// Spec: §2.3 — Σ PerTurnCost of new turns == Δ (exact, within float epsilon)
//
// Weight values from design table (relative, not pricing):
//   opus:  out=75, in=15
//   haiku: out=4,  in=0.80
//
// Scenario: two new turns, delta=2.00
//   Turn-A: opus,  out=1000  → units_A = 75*1000 = 75000
//   Turn-B: haiku, out=1000  → units_B = 4*1000  = 4000
//   Σunits = 79000
//   cost_A = 2.00 * 75000/79000 ≈ 1.898734...
//   cost_B = 2.00 *  4000/79000 ≈ 0.101265...
//   cost_A + cost_B = 2.00 (exact by construction)
// ---------------------------------------------------------------------------

// TestReconcile_WeightedSumEqualsDelta verifies that the sum of per-turn costs
// assigned to the new turns in a single Reconcile call equals the delta exactly
// (within floating-point epsilon). This is the core invariant of the weighted
// distribution: Σ cost = Δ, regardless of individual weight magnitudes.
func TestReconcile_WeightedSumEqualsDelta(t *testing.T) {
	// Given: a fresh session (baseline not yet captured) and two turns with
	// different models; the only tokens are output tokens to keep the scenario
	// focused on the out-weight ratio.
	st := &state.Session{Initialized: false}
	ts := time.Now()
	turns := []parser.Turn{
		{UUID: "turn-opus-A", Model: "claude-opus-4", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
		{UUID: "turn-haiku-B", Model: "claude-haiku-3-5", Timestamp: ts, Tokens: parser.TokenCounts{Output: 1000}},
	}
	delta := 2.00

	// Prime: first observation captures baseline=0 with no in-session turns.
	cost.Reconcile(st, 0.0, int64(1000), nil)
	// When: the next Reconcile spreads SessionTotal (=2.00) over the two turns.
	cost.Reconcile(st, delta, int64(1000), turns)

	// Then: the sum of per-turn costs must equal delta exactly (within 1e-9).
	costA, okA := cost.PerTurn(st, "turn-opus-A")
	if !okA {
		t.Fatalf("TestReconcile_WeightedSumEqualsDelta: PerTurn(turn-opus-A) not found")
	}
	costB, okB := cost.PerTurn(st, "turn-haiku-B")
	if !okB {
		t.Fatalf("TestReconcile_WeightedSumEqualsDelta: PerTurn(turn-haiku-B) not found")
	}
	sum := costA + costB
	if !approxEqual(sum, delta) {
		t.Errorf("TestReconcile_WeightedSumEqualsDelta: Σ cost = %.9f; want %.9f (delta); diff = %.2e",
			sum, delta, math.Abs(sum-delta))
	}
}

// ---------------------------------------------------------------------------
// T-16b: TestReconcile_WeightedShare
// Spec: §2.3 — when Opus and Haiku turns have equal output tokens, Opus
// receives a larger cost share (out_opus=75 >> out_haiku=4).
// ---------------------------------------------------------------------------

// TestReconcile_WeightedShare verifies that, with equal output token counts,
// an opus turn receives a larger PerTurnCost share than a haiku turn.
// This confirms the weight table is applied (not output-proportional fallback).
func TestReconcile_WeightedShare(t *testing.T) {
	// Given: two turns with identical output tokens but different model families.
	// opus out-weight=75, haiku out-weight=4 → opus gets ~94.9% of delta.
	st := &state.Session{Initialized: false}
	ts := time.Now()
	turns := []parser.Turn{
		{UUID: "turn-opus", Model: "claude-opus-4", Timestamp: ts, Tokens: parser.TokenCounts{Output: 100}},
		{UUID: "turn-haiku", Model: "claude-haiku-3-5", Timestamp: ts, Tokens: parser.TokenCounts{Output: 100}},
	}

	// Prime: first observation captures baseline=0 with no in-session turns.
	cost.Reconcile(st, 0.0, int64(500), nil)
	// When: the next Reconcile spreads SessionTotal (=1.00) over the two turns.
	cost.Reconcile(st, 1.00, int64(500), turns)

	// Then: opus share > haiku share (weight ratio 75:4).
	opusCost, okO := cost.PerTurn(st, "turn-opus")
	if !okO {
		t.Fatalf("TestReconcile_WeightedShare: PerTurn(turn-opus) not found")
	}
	haikuCost, okH := cost.PerTurn(st, "turn-haiku")
	if !okH {
		t.Fatalf("TestReconcile_WeightedShare: PerTurn(turn-haiku) not found")
	}
	if opusCost <= haikuCost {
		t.Errorf("TestReconcile_WeightedShare: opus cost (%.6f) must be > haiku cost (%.6f) for equal out tokens",
			opusCost, haikuCost)
	}
	// Sanity: ratio should reflect weight ratio 75:4 ≈ 18.75.
	// Allow loose check (just > 10x) to be robust to minor table changes.
	if haikuCost > 0 {
		ratio := opusCost / haikuCost
		if ratio < 10 {
			t.Errorf("TestReconcile_WeightedShare: opus/haiku cost ratio = %.2f; want ≥ 10 (weights 75:4 ≈ 18.75x)", ratio)
		}
	}
}

// ---------------------------------------------------------------------------
// T-17: TestReconcile_SubagentTurnsInPool
// Spec: §2.3 — subagent turns (IsSidechain=true) are included in the
// distribution pool and receive a PerTurnCost entry.
// ---------------------------------------------------------------------------

// TestReconcile_SubagentTurnsInPool verifies that turns with IsSidechain=true
// are not skipped during delta distribution; they receive PerTurnCost entries
// just like orchestrator turns.
func TestReconcile_SubagentTurnsInPool(t *testing.T) {
	// Given: one orchestrator turn and one sidechain (subagent) turn.
	st := &state.Session{Initialized: false}
	ts := time.Now()
	turns := []parser.Turn{
		{
			UUID:        "orch-turn-1",
			Model:       "claude-sonnet-4-6",
			IsSidechain: false,
			Timestamp:   ts,
			Tokens:      parser.TokenCounts{Output: 500},
		},
		{
			UUID:        "sub-turn-1",
			Model:       "claude-haiku-3-5",
			IsSidechain: true,
			Timestamp:   ts,
			Tokens:      parser.TokenCounts{Output: 500},
		},
	}

	// Prime: first observation captures baseline=0 with no in-session turns.
	cost.Reconcile(st, 0.0, int64(2000), nil)
	// When: the next Reconcile spreads SessionTotal (=3.00) across both turns.
	cost.Reconcile(st, 3.00, int64(2000), turns)

	// Then: both turns must have a PerTurnCost entry (sidechain is in the pool).
	_, okOrch := cost.PerTurn(st, "orch-turn-1")
	if !okOrch {
		t.Errorf("TestReconcile_SubagentTurnsInPool: PerTurn(orch-turn-1) not found — orchestrator turn missing")
	}
	_, okSub := cost.PerTurn(st, "sub-turn-1")
	if !okSub {
		t.Errorf("TestReconcile_SubagentTurnsInPool: PerTurn(sub-turn-1) not found — sidechain turn must be in pool")
	}
}

// ---------------------------------------------------------------------------
// T-19: TestSubagentTotal_SumByUUIDs
// Spec: §2.2 — SubagentTotal(st, uuids) = Σ PerTurnCost for the given UUIDs
// ---------------------------------------------------------------------------

// TestSubagentTotal_SumByUUIDs verifies that SubagentTotal returns the exact
// sum of PerTurnCost values for the given UUID list, ignoring any other turns
// in the session.
func TestSubagentTotal_SumByUUIDs(t *testing.T) {
	tests := []struct {
		name      string
		perTurn   map[string]float64
		uuids     []string
		wantTotal float64
	}{
		{
			name: "single agent UUID",
			perTurn: map[string]float64{
				"sub-uuid-1": 0.50,
				"sub-uuid-2": 0.30,
				"orch-uuid":  1.20,
			},
			uuids:     []string{"sub-uuid-1"},
			wantTotal: 0.50,
		},
		{
			name: "multiple agent UUIDs sum",
			perTurn: map[string]float64{
				"sub-uuid-1": 0.50,
				"sub-uuid-2": 0.30,
				"orch-uuid":  1.20,
			},
			uuids:     []string{"sub-uuid-1", "sub-uuid-2"},
			wantTotal: 0.80,
		},
		{
			name: "unknown UUID returns 0",
			perTurn: map[string]float64{
				"sub-uuid-1": 0.50,
			},
			uuids:     []string{"nonexistent-uuid"},
			wantTotal: 0.00,
		},
		{
			name:      "empty UUID list returns 0",
			perTurn:   map[string]float64{"sub-uuid-1": 0.50},
			uuids:     []string{},
			wantTotal: 0.00,
		},
		{
			name:      "nil PerTurnCost returns 0",
			perTurn:   nil,
			uuids:     []string{"sub-uuid-1"},
			wantTotal: 0.00,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a session with pre-populated PerTurnCost map.
			st := &state.Session{
				Initialized: true,
				PerTurnCost: tc.perTurn,
			}

			// When: computing subagent cumulative cost.
			got := cost.SubagentTotal(st, tc.uuids)

			// Then: result must equal the sum of mapped costs for given UUIDs.
			if !approxEqual(got, tc.wantTotal) {
				t.Errorf("SubagentTotal(%v) = %.6f; want %.6f", tc.uuids, got, tc.wantTotal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-20: TestReconcile_BaselineDurCapturedAndDelta
// Spec: §2.3 — first Reconcile sets BaselineDurMS=durMS;
// SessionDuration = durMS − BaselineDurMS; second call with larger durMS yields
// positive duration.
// ---------------------------------------------------------------------------

// TestReconcile_BaselineDurCapturedAndDelta verifies that:
//  1. The first Reconcile call captures BaselineDurMS from the durMS argument.
//  2. SessionDuration(st, durMS) returns durMS − BaselineDurMS (zero on first call).
//  3. A subsequent call with a larger durMS returns a positive duration delta.
func TestReconcile_BaselineDurCapturedAndDelta(t *testing.T) {
	// Given: a fresh session and an initial durMS.
	st := &state.Session{Initialized: false}
	turns := []parser.Turn{
		{UUID: "turn-dur-1", Model: "claude-sonnet-4-6", Tokens: parser.TokenCounts{Output: 100}},
	}
	const baselineDur = int64(5000) // 5 seconds in ms at session start.

	// When: first Reconcile call.
	cost.Reconcile(st, 1.00, baselineDur, turns)

	// Then: BaselineDurMS is captured.
	if st.BaselineDurMS != baselineDur {
		t.Errorf("TestReconcile_BaselineDurCapturedAndDelta: BaselineDurMS = %d; want %d",
			st.BaselineDurMS, baselineDur)
	}

	// Then: SessionDuration at the moment of baseline = 0 (no elapsed time yet).
	durAtBaseline := cost.SessionDuration(st, baselineDur)
	if durAtBaseline != 0 {
		t.Errorf("TestReconcile_BaselineDurCapturedAndDelta: SessionDuration at baseline = %d ms; want 0",
			durAtBaseline)
	}

	// When: later, TotalAPIDurationMS has grown (more turns processed).
	const laterDur = int64(8500) // 3.5 seconds elapsed since baseline.
	turns2 := []parser.Turn{
		{UUID: "turn-dur-1", Model: "claude-sonnet-4-6", Tokens: parser.TokenCounts{Output: 100}},
		{UUID: "turn-dur-2", Model: "claude-sonnet-4-6", Tokens: parser.TokenCounts{Output: 200}},
	}
	cost.Reconcile(st, 1.50, laterDur, turns2)

	// Then: BaselineDurMS must remain unchanged (immutable after first capture).
	if st.BaselineDurMS != baselineDur {
		t.Errorf("TestReconcile_BaselineDurCapturedAndDelta: BaselineDurMS changed on second call: %d; want %d",
			st.BaselineDurMS, baselineDur)
	}

	// Then: SessionDuration returns the positive elapsed delta.
	wantDelta := laterDur - baselineDur // 3500 ms
	gotDelta := cost.SessionDuration(st, laterDur)
	if gotDelta != wantDelta {
		t.Errorf("TestReconcile_BaselineDurCapturedAndDelta: SessionDuration(laterDur=%d) = %d ms; want %d ms",
			laterDur, gotDelta, wantDelta)
	}
}
