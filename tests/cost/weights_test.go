// Package cost_test — RED-phase tests for ModelWeights (Phase 6.9.a).
//
// T-18: ModelWeights version-fallback — opus-4-9 and opus-5 resolve to opus
// family weights; unknown model falls back to sonnet defaults.
//
// The functions under test (cost.ModelWeights, cost.Weights type) do not exist
// yet. This file is intentionally RED until the GREEN agent creates
// internal/cost/weights.go.
package cost_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/cost"
)

// ---------------------------------------------------------------------------
// T-18: TestModelWeights_VersionFallback
// Spec: §2.2 — ModelWeights version-fallback by family prefix (opus/sonnet/haiku).
// Design: docs/.../2026-06-01-phase-6.9-design.md "Model weights" section
// ---------------------------------------------------------------------------

// TestModelWeights_VersionFallback verifies that ModelWeights resolves model
// strings to the correct family weight table using prefix-based fallback:
//   - "opus-*"   → opus family   (out=75, in=15, cache_read=1.5, cache_create=18.75)
//   - "sonnet-*" → sonnet family (out=15, in=3, cache_read=0.30, cache_create=3.75)
//   - "haiku-*"  → haiku family  (out=4, in=0.80, cache_read=0.08, cache_create=1)
//   - unknown    → sonnet (default fallback)
//
// Weight values are taken verbatim from the design table; they are relative
// (scale does not matter, only ratios), so these assertions encode the
// documented table as the contract.
func TestModelWeights_VersionFallback(t *testing.T) {
	tests := []struct {
		name            string
		model           string
		wantOut         float64
		wantIn          float64
		wantCacheRead   float64
		wantCacheCreate float64
	}{
		// Exact canonical names — baseline correctness.
		{
			name:            "opus canonical",
			model:           "claude-opus-4",
			wantOut:         75,
			wantIn:          15,
			wantCacheRead:   1.5,
			wantCacheCreate: 18.75,
		},
		{
			name:            "sonnet canonical",
			model:           "claude-sonnet-4-6",
			wantOut:         15,
			wantIn:          3,
			wantCacheRead:   0.30,
			wantCacheCreate: 3.75,
		},
		{
			name:            "haiku canonical",
			model:           "claude-haiku-3-5",
			wantOut:         4,
			wantIn:          0.80,
			wantCacheRead:   0.08,
			wantCacheCreate: 1,
		},
		// Version-fallback: future model versions must inherit family weights.
		{
			name:            "opus future version opus-4-9",
			model:           "claude-opus-4-9",
			wantOut:         75,
			wantIn:          15,
			wantCacheRead:   1.5,
			wantCacheCreate: 18.75,
		},
		{
			name:            "opus future major version opus-5",
			model:           "claude-opus-5",
			wantOut:         75,
			wantIn:          15,
			wantCacheRead:   1.5,
			wantCacheCreate: 18.75,
		},
		{
			name:            "sonnet future version sonnet-5",
			model:           "claude-sonnet-5",
			wantOut:         15,
			wantIn:          3,
			wantCacheRead:   0.30,
			wantCacheCreate: 3.75,
		},
		{
			name:            "haiku future version haiku-5",
			model:           "claude-haiku-5",
			wantOut:         4,
			wantIn:          0.80,
			wantCacheRead:   0.08,
			wantCacheCreate: 1,
		},
		// Unknown model must fall back to sonnet (documented default).
		{
			name:            "unknown model falls back to sonnet",
			model:           "claude-mythos-1",
			wantOut:         15,
			wantIn:          3,
			wantCacheRead:   0.30,
			wantCacheCreate: 3.75,
		},
		{
			name:            "empty model string falls back to sonnet",
			model:           "",
			wantOut:         15,
			wantIn:          3,
			wantCacheRead:   0.30,
			wantCacheCreate: 3.75,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When: resolving weights for the given model string.
			w := cost.ModelWeights(tc.model)

			// Then: each weight component must match the documented table value.
			if !approxEqual(w.Out, tc.wantOut) {
				t.Errorf("ModelWeights(%q).Out = %.4f; want %.4f", tc.model, w.Out, tc.wantOut)
			}
			if !approxEqual(w.In, tc.wantIn) {
				t.Errorf("ModelWeights(%q).In = %.4f; want %.4f", tc.model, w.In, tc.wantIn)
			}
			if !approxEqual(w.CacheRead, tc.wantCacheRead) {
				t.Errorf("ModelWeights(%q).CacheRead = %.4f; want %.4f", tc.model, w.CacheRead, tc.wantCacheRead)
			}
			if !approxEqual(w.CacheCreate, tc.wantCacheCreate) {
				t.Errorf("ModelWeights(%q).CacheCreate = %.4f; want %.4f", tc.model, w.CacheCreate, tc.wantCacheCreate)
			}
		})
	}
}
