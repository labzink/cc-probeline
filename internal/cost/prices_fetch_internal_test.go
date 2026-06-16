package cost

import (
	"testing"
	"time"
)

// fakeFetcher records call count and returns a canned body or error.
type fakeFetcher struct {
	body  []byte
	err   error
	calls int
}

func (f *fakeFetcher) fetch(string) ([]byte, error) {
	f.calls++
	return f.body, f.err
}

const samplePricesJSON = `{"schema":1,"latest_version":"0.2.0","prices":{` +
	`"opus":{"in":6,"cache_read":0.6,"cache_write_5m":7.5,"cache_write_1h":12,"out":30}}}`

// isolatePrices points the shared price cache at a temp dir for the test.
func isolatePrices(t *testing.T) {
	t.Helper()
	t.Setenv("CC_PROBELINE_PRICES_DIR", t.TempDir())
}

// TestRefreshPrices_FetchAndCache: a successful fetch is stored and CachedPrices
// reads it back (real invariant: fetched body round-trips through the cache).
func TestRefreshPrices_FetchAndCache(t *testing.T) {
	isolatePrices(t)
	f := &fakeFetcher{body: []byte(samplePricesJSON)}
	refreshPrices(f, time.Now())
	if f.calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", f.calls)
	}
	pf, ok := CachedPrices()
	if !ok {
		t.Fatal("CachedPrices ok=false after successful fetch")
	}
	if pf.LatestVersion != "0.2.0" {
		t.Errorf("LatestVersion = %q, want 0.2.0", pf.LatestVersion)
	}
	if pf.Prices["opus"].In != 6 {
		t.Errorf("opus In = %g, want 6", pf.Prices["opus"].In)
	}
}

// TestRefreshPrices_TTLGate: a fresh cache suppresses the network entirely.
func TestRefreshPrices_TTLGate(t *testing.T) {
	isolatePrices(t)
	now := time.Now()
	// First fetch populates the cache and stamps CheckedAt=now.
	first := &fakeFetcher{body: []byte(samplePricesJSON)}
	refreshPrices(first, now)
	// A second refresh one hour later (< 24h TTL) must not hit the network.
	second := &fakeFetcher{body: []byte(samplePricesJSON)}
	refreshPrices(second, now.Add(time.Hour))
	if second.calls != 0 {
		t.Errorf("fetch calls within TTL = %d, want 0", second.calls)
	}
}

// TestRefreshPrices_TTLExpiredRefetches: past the TTL, the network is hit again.
func TestRefreshPrices_TTLExpiredRefetches(t *testing.T) {
	isolatePrices(t)
	now := time.Now()
	refreshPrices(&fakeFetcher{body: []byte(samplePricesJSON)}, now)
	second := &fakeFetcher{body: []byte(samplePricesJSON)}
	refreshPrices(second, now.Add(pricesCacheTTL+time.Minute))
	if second.calls != 1 {
		t.Errorf("fetch calls past TTL = %d, want 1", second.calls)
	}
}

// TestRefreshPrices_OfflineFailSoft: a fetch error does not panic, does not store
// garbage, leaves CachedPrices empty (no prior success), and stamps the attempt so
// the next tick within TTL does not retry.
func TestRefreshPrices_OfflineFailSoft(t *testing.T) {
	isolatePrices(t)
	now := time.Now()
	fail := &fakeFetcher{err: errFake}
	refreshPrices(fail, now)
	if _, ok := CachedPrices(); ok {
		t.Error("CachedPrices ok=true after a failed first fetch; want false")
	}
	// Throttle: a second attempt within TTL must not hit the network.
	again := &fakeFetcher{err: errFake}
	refreshPrices(again, now.Add(time.Minute))
	if again.calls != 0 {
		t.Errorf("retry within TTL after failure = %d calls, want 0", again.calls)
	}
}

// TestRefreshPrices_FailureKeepsPriorSuccess: once a good body is cached, a later
// failed refresh must not wipe it.
func TestRefreshPrices_FailureKeepsPriorSuccess(t *testing.T) {
	isolatePrices(t)
	now := time.Now()
	refreshPrices(&fakeFetcher{body: []byte(samplePricesJSON)}, now)
	// TTL expires, next fetch fails.
	refreshPrices(&fakeFetcher{err: errFake}, now.Add(pricesCacheTTL+time.Minute))
	pf, ok := CachedPrices()
	if !ok || pf.LatestVersion != "0.2.0" {
		t.Errorf("prior cached prices lost after failed refresh: ok=%v ver=%q", ok, pf.LatestVersion)
	}
}

// TestRefreshPrices_MalformedBodyNotStored: an unparseable body is rejected; the
// cache is not populated with garbage.
func TestRefreshPrices_MalformedBodyNotStored(t *testing.T) {
	isolatePrices(t)
	refreshPrices(&fakeFetcher{body: []byte("<<not json>>")}, time.Now())
	if _, ok := CachedPrices(); ok {
		t.Error("CachedPrices ok=true after malformed body; want false")
	}
}

// errFake is a sentinel network error for the fake fetcher.
var errFake = fakeErr("offline")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }
