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

// familyWeights maps model family names to their relative weight table.
// Values from the Phase 6.9 design doc "Model weights" section.
var familyWeights = map[string]Weights{
	"opus":   {In: 15, CacheRead: 1.5, CacheCreate: 18.75, Out: 75},
	"sonnet": {In: 3, CacheRead: 0.30, CacheCreate: 3.75, Out: 15},
	"haiku":  {In: 0.80, CacheRead: 0.08, CacheCreate: 1, Out: 4},
}

// defaultWeights is the fallback used for unknown model strings.
// Sonnet is chosen as the middle-tier default (spec §2.2).
var defaultWeights = familyWeights["sonnet"]

// ModelWeights returns the relative Weights for the given model string.
// Resolution uses family-prefix matching (version-fallback):
//   - contains "opus"   → opus family
//   - contains "sonnet" → sonnet family
//   - contains "haiku"  → haiku family
//   - unknown or empty  → sonnet (default fallback)
func ModelWeights(model string) Weights {
	lower := strings.ToLower(model)
	switch {
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
