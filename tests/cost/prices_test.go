// Package cost_test — Phase 7.46 Wave B / BL-36 price-table tests.
//
// Real invariant: the public assets/prices.json (what we serve over the network)
// must match the baked weight table byte-for-value. If a maintainer edits one
// without the other, this test fails — that is the whole anti-drift guarantee
// behind "edit two lines of JSON and every install picks it up".
package cost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/cost"
)

// pricesJSONPath is the repo-relative path to the public price document. TestMain
// (testmain_test.go) chdirs to the module root, so this resolves from there.
const pricesJSONPath = "assets/prices.json"

// TestPrices_BakedSync asserts every family/field in assets/prices.json equals
// the baked ModelWeights the binary ships with. The mapping is:
//
//	in → In, cache_read → CacheRead, cache_write_5m → CacheCreate,
//	cache_write_1h → CacheCreate1h, out → Out.
func TestPrices_BakedSync(t *testing.T) {
	data, err := os.ReadFile(filepath.Clean(pricesJSONPath))
	if err != nil {
		t.Fatalf("read %s: %v", pricesJSONPath, err)
	}
	pf, err := cost.ParsePrices(data)
	if err != nil {
		t.Fatalf("ParsePrices: %v", err)
	}
	if pf.Schema != 1 {
		t.Fatalf("schema = %d, want 1", pf.Schema)
	}
	if len(pf.Prices) == 0 {
		t.Fatal("prices map is empty")
	}

	for fam, mp := range pf.Prices {
		w := cost.ModelWeights(fam)
		checks := []struct {
			field        string
			json, weight float64
		}{
			{"in", mp.In, w.In},
			{"cache_read", mp.CacheRead, w.CacheRead},
			{"cache_write_5m", mp.CacheWrite5m, w.CacheCreate},
			{"cache_write_1h", mp.CacheWrite1h, w.CacheCreate1h},
			{"out", mp.Out, w.Out},
		}
		for _, c := range checks {
			if c.json != c.weight {
				t.Errorf("family %q field %s: prices.json=%g, baked weight=%g (drift)",
					fam, c.field, c.json, c.weight)
			}
		}
	}
}

// TestParsePrices_Malformed confirms invalid JSON surfaces an error rather than a
// zero-value PriceFile that could silently zero out the table.
func TestParsePrices_Malformed(t *testing.T) {
	if _, err := cost.ParsePrices([]byte("{not json")); err == nil {
		t.Fatal("ParsePrices on invalid JSON: want error, got nil")
	}
}
