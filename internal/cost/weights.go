// Package cost — model weight table for weighted delta distribution.
//
// Weights are relative $/Mtok values (scale does not matter; normalization
// Δ·units/Σunits cancels it out). Values from the Phase 6.9 design table.
// Family lookup uses substring prefix matching (opus/sonnet/haiku) so future
// model versions (e.g. "claude-opus-5") inherit the family automatically.
package cost

import "strings"

// Weights holds per-token-type relative weight factors for one model family.
// Fields correspond to the four token classes CC reports.
type Weights struct {
	In          float64 // input tokens weight
	CacheRead   float64 // cache_read tokens weight
	CacheCreate float64 // cache_create tokens weight
	Out         float64 // output tokens weight
}

// familyWeights maps model family names to their relative weight table
// (relative $/Mtok; scale cancels under normalization). Phase 7.45 B3-1
// refreshed the stale opus/haiku rows to the current model generation: the old
// values were Opus 4.1 (15/1.5/18.75/75) and Haiku 3.5 (0.80/0.08/1/4), ~2× the
// current prices. Internal per-token-class proportions are identical across all
// families (out=5·in, read=0.1·in, write=1.25·in), so the stale rows did not
// distort distribution WITHIN one model — but they broke the cross-model scale
// in mixed pools (orchestrator-opus + subagent-sonnet share one delta), where
// opus turns were over-weighted. Corrected ratios fix that.
var familyWeights = map[string]Weights{
	"fable":  {In: 10, CacheRead: 1, CacheCreate: 12.50, Out: 50},  // Fable 5 = 2× Opus 4.8
	"opus":   {In: 5, CacheRead: 0.5, CacheCreate: 6.25, Out: 25},  // Opus 4.8 (was Opus 4.1)
	"sonnet": {In: 3, CacheRead: 0.30, CacheCreate: 3.75, Out: 15}, // Sonnet 4.6 (unchanged)
	"haiku":  {In: 1, CacheRead: 0.10, CacheCreate: 1.25, Out: 5},  // Haiku 4.5 (was Haiku 3.5)
}

// defaultWeights is the fallback used for unknown model strings. Phase 7.45 B3-1
// switched the default from sonnet to opus: a new, unrecognised model is more
// likely to be a high tier than a low one, so erring upward is the safer bet
// for cost estimation.
var defaultWeights = familyWeights["opus"]

// ModelWeights returns the relative Weights for the given model string.
// Resolution uses family-prefix matching (version-fallback):
//   - contains "fable"  → fable family
//   - contains "opus"   → opus family
//   - contains "sonnet" → sonnet family
//   - contains "haiku"  → haiku family
//   - unknown or empty  → opus (default fallback, spec §B3-1)
func ModelWeights(model string) Weights {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "fable"):
		return familyWeights["fable"]
	case strings.Contains(lower, "opus"):
		return familyWeights["opus"]
	case strings.Contains(lower, "haiku"):
		return familyWeights["haiku"]
	case strings.Contains(lower, "sonnet"):
		return familyWeights["sonnet"]
	default:
		return defaultWeights
	}
}
