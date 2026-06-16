package cost

import "testing"

// snapshotPrices saves the mutable price table and registers a cleanup that
// restores it, so a test mutating the global familyWeights cannot leak into the
// next one.
func snapshotPrices(t *testing.T) {
	t.Helper()
	saved := make(map[string]Weights, len(familyWeights))
	for k, v := range familyWeights {
		saved[k] = v
	}
	savedDefault := defaultWeights
	t.Cleanup(func() {
		for k := range familyWeights {
			delete(familyWeights, k)
		}
		for k, v := range saved {
			familyWeights[k] = v
		}
		defaultWeights = savedDefault
	})
}

// TestApplyPrices_Override checks a well-formed file replaces the named family's
// weights (real invariant: a price edit reaches ModelWeights).
func TestApplyPrices_Override(t *testing.T) {
	snapshotPrices(t)
	ApplyPrices(PriceFile{
		Schema:        1,
		LatestVersion: "9.9.9",
		Prices: map[string]ModelPrices{
			"opus": {In: 7, CacheRead: 0.7, CacheWrite5m: 8.75, CacheWrite1h: 14, Out: 35},
		},
	})
	if got := ModelWeights("opus").In; got != 7 {
		t.Errorf("opus In after override = %g, want 7", got)
	}
	if got := ModelWeights("claude-opus-9").CacheCreate1h; got != 14 {
		t.Errorf("opus CacheCreate1h after override = %g, want 14", got)
	}
}

// TestApplyPrices_PartialKeepsBaked confirms a family omitted from the file keeps
// its baked value (override is per-family, not a wholesale replace).
func TestApplyPrices_PartialKeepsBaked(t *testing.T) {
	snapshotPrices(t)
	bakedHaiku := ModelWeights("haiku")
	ApplyPrices(PriceFile{
		Schema: 1,
		Prices: map[string]ModelPrices{
			"sonnet": {In: 4, CacheRead: 0.4, CacheWrite5m: 5, CacheWrite1h: 8, Out: 20},
		},
	})
	if got := ModelWeights("haiku"); got != bakedHaiku {
		t.Errorf("haiku changed by sonnet-only override: %+v != %+v", got, bakedHaiku)
	}
	if got := ModelWeights("sonnet").In; got != 4 {
		t.Errorf("sonnet In = %g, want 4", got)
	}
}

// TestApplyPrices_UnknownSchemaIgnored confirms an unrecognised schema leaves the
// baked table untouched (forward-compat fail-soft).
func TestApplyPrices_UnknownSchemaIgnored(t *testing.T) {
	snapshotPrices(t)
	baked := ModelWeights("opus")
	ApplyPrices(PriceFile{
		Schema: 99,
		Prices: map[string]ModelPrices{
			"opus": {In: 999, CacheRead: 1, CacheWrite5m: 1, CacheWrite1h: 1, Out: 1},
		},
	})
	if got := ModelWeights("opus"); got != baked {
		t.Errorf("opus changed despite unknown schema: %+v != %+v", got, baked)
	}
}

// TestApplyPrices_MalformedRowSkipped confirms a row with a non-positive input
// price is skipped while sane rows in the same file still apply.
func TestApplyPrices_MalformedRowSkipped(t *testing.T) {
	snapshotPrices(t)
	bakedOpus := ModelWeights("opus")
	ApplyPrices(PriceFile{
		Schema: 1,
		Prices: map[string]ModelPrices{
			"opus":   {In: 0, CacheRead: 1, CacheWrite5m: 1, CacheWrite1h: 1, Out: 1}, // malformed
			"sonnet": {In: 4, CacheRead: 0.4, CacheWrite5m: 5, CacheWrite1h: 8, Out: 20},
		},
	})
	if got := ModelWeights("opus"); got != bakedOpus {
		t.Errorf("opus changed despite malformed (In<=0) row: %+v != %+v", got, bakedOpus)
	}
	if got := ModelWeights("sonnet").In; got != 4 {
		t.Errorf("sonnet In = %g, want 4 (sane row in same file should apply)", got)
	}
}
