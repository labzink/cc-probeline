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
// strings to the correct family weight table using prefix-based fallback
// (Phase 7.45 B3-1 refreshed opus/haiku and changed the default to opus):
//   - "opus-*"   → opus family   (out=25, in=5, cache_read=0.5, cache_create=6.25)
//   - "sonnet-*" → sonnet family (out=15, in=3, cache_read=0.30, cache_create=3.75)
//   - "haiku-*"  → haiku family  (out=5, in=1, cache_read=0.10, cache_create=1.25)
//   - unknown    → opus (default fallback — err upward for unrecognised models)
//
// Weight values are relative (scale does not matter, only ratios), so these
// assertions encode the current model-generation table as the contract.
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
			wantOut:         25,
			wantIn:          5,
			wantCacheRead:   0.5,
			wantCacheCreate: 6.25,
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
			wantOut:         5,
			wantIn:          1,
			wantCacheRead:   0.10,
			wantCacheCreate: 1.25,
		},
		// Version-fallback: future model versions must inherit family weights.
		{
			name:            "opus future version opus-4-9",
			model:           "claude-opus-4-9",
			wantOut:         25,
			wantIn:          5,
			wantCacheRead:   0.5,
			wantCacheCreate: 6.25,
		},
		{
			name:            "opus future major version opus-5",
			model:           "claude-opus-5",
			wantOut:         25,
			wantIn:          5,
			wantCacheRead:   0.5,
			wantCacheCreate: 6.25,
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
			wantOut:         5,
			wantIn:          1,
			wantCacheRead:   0.10,
			wantCacheCreate: 1.25,
		},
		// Unknown / empty model must fall back to opus (documented default, B3-1).
		{
			name:            "unknown model falls back to opus",
			model:           "claude-mythos-1",
			wantOut:         25,
			wantIn:          5,
			wantCacheRead:   0.5,
			wantCacheCreate: 6.25,
		},
		{
			name:            "empty model string falls back to opus",
			model:           "",
			wantOut:         25,
			wantIn:          5,
			wantCacheRead:   0.5,
			wantCacheCreate: 6.25,
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
