package cost

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// pricesURL is the raw GitHub URL of the public price table. A maintainer edits
// this file (two lines for a price change) and every install picks it up within
// the cache TTL. It is OUR document, not Anthropic's API — no token, no telemetry.
const pricesURL = "https://raw.githubusercontent.com/labzink/cc-probeline/main/assets/prices.json"

// Network/cache tuning.
const (
	pricesCacheTTL  = 24 * time.Hour     // one real fetch per machine per day
	pricesCacheName = "price-cache.json" // file in the shared data dir
	fetchTimeout    = 1500 * time.Millisecond
	maxPriceBody    = 64 * 1024 // bytes; the document is tiny, cap defensively
)

// priceFetcher abstracts the network GET so unit tests inject a fake and the
// package stays testable without a real socket.
type priceFetcher interface {
	fetch(url string) ([]byte, error)
}

// realFetcher performs the actual HTTP GET with a hard timeout and a body cap.
type realFetcher struct{}

func (realFetcher) fetch(url string) ([]byte, error) {
	client := &http.Client{Timeout: fetchTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxPriceBody))
}

// priceCache is the on-disk envelope for the shared price cache. CheckedAt is the
// unix-seconds timestamp of the last fetch ATTEMPT (success or failure) so a tick
// cannot hammer the network every 5 seconds; Raw is the body of the last
// SUCCESSFUL fetch (empty until one succeeds).
type priceCache struct {
	CheckedAt int64 `json:"checked_at"`
	// omitempty: an empty Raw (no successful fetch yet) is omitted rather than
	// written as the literal `null`, so it reads back as a zero-length slice and
	// CachedPrices correctly reports "no cached prices".
	Raw json.RawMessage `json:"raw,omitempty"`
}

// pricesDir resolves the directory of the shared price cache, mirroring quota's
// resolution so both global files live together:
// CC_PROBELINE_PRICES_DIR → XDG_DATA_HOME/cc-probeline → ~/.local/share/cc-probeline.
// The dedicated env var lets tests isolate via t.Setenv + t.TempDir.
func pricesDir() string {
	if dir := os.Getenv("CC_PROBELINE_PRICES_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "cc-probeline")
}

// pricesCachePath returns the full path of the price cache file, or "" when the
// directory cannot be determined.
func pricesCachePath() string {
	dir := pricesDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, pricesCacheName)
}

// RefreshPrices makes at most one network request per TTL window: if the shared
// cache is younger than pricesCacheTTL it returns immediately (no socket);
// otherwise it GETs prices.json, validates it parses, and stores it. The attempt
// timestamp is stamped even on failure so an offline machine retries at most once
// per TTL rather than on every 5-second tick. Fully fail-soft — any network,
// parse, or IO error is logged and swallowed; the existing cache and the baked
// table stand. Called once on startup from main (Phase 7.46 Wave B / BL-36).
func RefreshPrices(now time.Time) { refreshPrices(realFetcher{}, now) }

// refreshPrices is the testable core of RefreshPrices (fetcher injected).
func refreshPrices(f priceFetcher, now time.Time) {
	p := pricesCachePath()
	if p == "" {
		slog.Warn("cost.RefreshPrices: data dir unavailable; skipping")
		return
	}

	existing, _ := readPriceCache(p) // zero value when absent/corrupt
	if existing.CheckedAt > 0 && now.Unix()-existing.CheckedAt < int64(pricesCacheTTL.Seconds()) {
		slog.Debug("cost.RefreshPrices: cache fresh, no fetch", "age_s", now.Unix()-existing.CheckedAt)
		return
	}

	next := priceCache{CheckedAt: now.Unix(), Raw: existing.Raw}

	body, err := f.fetch(pricesURL)
	if err != nil {
		slog.Warn("cost.RefreshPrices: fetch failed (keeping cache/baked)", "err", err)
		writePriceCache(p, next) // throttle retries for the TTL window
		return
	}
	if _, perr := ParsePrices(body); perr != nil {
		slog.Warn("cost.RefreshPrices: fetched body unparseable; not storing", "err", perr)
		writePriceCache(p, next)
		return
	}
	next.Raw = body
	writePriceCache(p, next)
	slog.Debug("cost.RefreshPrices: cache updated", "bytes", len(body))
}

// CachedPrices returns the price file from the shared cache, or ok=false when the
// cache is absent, empty (no successful fetch yet), or corrupt — in which case the
// caller keeps the baked table. Pure read, safe at startup before RefreshPrices.
func CachedPrices() (PriceFile, bool) {
	p := pricesCachePath()
	if p == "" {
		return PriceFile{}, false
	}
	c, err := readPriceCache(p)
	if err != nil || len(c.Raw) == 0 {
		return PriceFile{}, false
	}
	pf, err := ParsePrices(c.Raw)
	if err != nil {
		slog.Warn("cost.CachedPrices: cached body unparseable; using baked", "err", err)
		return PriceFile{}, false
	}
	return pf, true
}

// readPriceCache reads and decodes the cache file. Returns os.ErrNotExist when
// absent; a decode error otherwise.
func readPriceCache(p string) (priceCache, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return priceCache{}, err
	}
	var c priceCache
	if err := json.Unmarshal(data, &c); err != nil {
		return priceCache{}, fmt.Errorf("cost: decode price cache %q: %w", p, err)
	}
	return c, nil
}

// writePriceCache persists c atomically under a flock, matching the durability
// pattern of internal/quota. Fail-soft: errors are logged and swallowed (the
// cache is disposable — the baked table is always a valid fallback).
func writePriceCache(p string, c priceCache) {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("cost.writePriceCache: mkdir", "dir", dir, "err", err)
		return
	}
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		slog.Warn("cost.writePriceCache: flock", "err", err)
		return
	}
	defer fl.Unlock() //nolint:errcheck

	data, err := json.Marshal(c)
	if err != nil {
		slog.Warn("cost.writePriceCache: encode", "err", err)
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Warn("cost.writePriceCache: write tmp", "err", err)
		return
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("cost.writePriceCache: rename", "err", err)
	}
}
