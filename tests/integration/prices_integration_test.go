//go:build integration

package integration_test

// prices_integration_test.go — the ONE real-network test for the self-healing
// price table (Phase 7.46 Wave B / BL-36). It exercises the production path
// (cost.RefreshPrices → cost.CachedPrices) against the real public prices.json
// at raw.githubusercontent.com, with no fake client.
//
// It is gated behind the `integration` build tag, so the hermetic `go test ./...`
// gate never opens a socket (the unit tests in internal/cost use a fake fetcher).
// When the document is unreachable — offline, or the mirror repo is still private
// pre-release — the test SKIPS rather than fails, so CI stays green until the
// repo is public; once public it validates the live file end-to-end.

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/cost"
)

func TestPrices_RealFetch(t *testing.T) {
	t.Setenv("CC_PROBELINE_PRICES_DIR", t.TempDir())

	cost.RefreshPrices(time.Now())

	pf, ok := cost.CachedPrices()
	if !ok {
		t.Skip("prices.json unreachable (offline or mirror repo not yet public) — skipping real-fetch integration")
	}

	if pf.Schema != 1 {
		t.Errorf("live prices.json schema = %d, want 1", pf.Schema)
	}
	if pf.LatestVersion == "" {
		t.Error("live prices.json has empty latest_version")
	}
	if len(pf.Prices) == 0 {
		t.Fatal("live prices.json has no price rows")
	}
	if _, has := pf.Prices["opus"]; !has {
		t.Error("live prices.json missing 'opus' family")
	}
}
