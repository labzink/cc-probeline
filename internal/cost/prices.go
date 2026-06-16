// Package cost — network-loadable price table (Phase 7.46 Wave B / BL-36).
//
// The baked weight table (familyWeights, weights.go) is the source of truth at
// build time. At runtime it can be overridden by prices.json — a small public
// document we host at raw.githubusercontent.com and fetch once per session
// (24h-cached, opt-out, fail-soft offline; see prices_fetch.go). This lets a
// price change ship as a two-line JSON edit that every install picks up within a
// day, without a rebuild. The same document also carries the latest released
// version for the "update available" hint (#7c).
//
// The on-disk file assets/prices.json IS the baked default; a sync test asserts
// the two never drift, so editing one without the other fails CI.
package cost

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// priceSchema is the prices.json layout version this binary understands. A file
// declaring any other schema is ignored wholesale (the baked table stands), so a
// future incompatible format can never corrupt pricing on an old install.
const priceSchema = 1

// PriceFile is the over-the-wire / on-disk schema of prices.json.
type PriceFile struct {
	Schema        int                    `json:"schema"`
	LatestVersion string                 `json:"latest_version"`
	Prices        map[string]ModelPrices `json:"prices"`
}

// ModelPrices is one family's per-token-class price in $/Mtok. Field names mirror
// the JSON keys; toWeights maps them onto the internal Weights struct.
type ModelPrices struct {
	In           float64 `json:"in"`
	CacheRead    float64 `json:"cache_read"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	Out          float64 `json:"out"`
}

// toWeights converts the JSON price row into the internal Weights vector used by
// the estimator. The two cache-write classes map onto the 5-minute and 1-hour
// TTL weights (CacheCreate / CacheCreate1h).
func (m ModelPrices) toWeights() Weights {
	return Weights{
		In:            m.In,
		CacheRead:     m.CacheRead,
		CacheCreate:   m.CacheWrite5m,
		CacheCreate1h: m.CacheWrite1h,
		Out:           m.Out,
	}
}

// ParsePrices decodes prices.json bytes into a PriceFile. It does not validate
// schema or row sanity — that is ApplyPrices' job, so a caller can still read
// LatestVersion from a file whose price rows it would not apply.
func ParsePrices(data []byte) (PriceFile, error) {
	var pf PriceFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return PriceFile{}, fmt.Errorf("cost: parse prices: %w", err)
	}
	return pf, nil
}

// ApplyPrices overrides the in-memory price table with the rows present in pf,
// keeping the baked default for any family pf omits. It is meant to run exactly
// once at startup, after a successful (cached) fetch, before the render pipeline
// begins — offline or with the check disabled, the baked table simply stands.
//
// Guards (all fail-soft): a schema this binary does not understand is ignored
// wholesale, and any row with a non-positive input price is skipped as malformed.
// Not safe for concurrent use; the single-shot statusline process applies prices
// before reconciling cost, so there is no contention.
func ApplyPrices(pf PriceFile) {
	if pf.Schema != priceSchema {
		slog.Warn("cost.ApplyPrices: unknown schema, keeping baked prices", "schema", pf.Schema)
		return
	}
	applied := 0
	for fam, mp := range pf.Prices {
		w := mp.toWeights()
		if w.In <= 0 {
			slog.Warn("cost.ApplyPrices: skipping malformed row", "family", fam, "in", w.In)
			continue
		}
		familyWeights[fam] = w
		applied++
	}
	defaultWeights = familyWeights["opus"]
	slog.Debug("cost.ApplyPrices done", "applied", applied, "latest_version", pf.LatestVersion)
}
