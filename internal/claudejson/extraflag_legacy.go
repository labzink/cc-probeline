//go:build legacy_overage

// Reader for oauthAccount.hasExtraUsageEnabled, the flag that armed the old
// session-local overage badge (Phase 6.95.h). Since Phase 7.48 the badge reads
// the usage cache instead, which carries the same fact as extra_usage.is_enabled
// plus the figures themselves — so this reader has no caller in the shipping
// build. Kept compilable alongside internal/state/overage_legacy.go, its only
// consumer, under the same build tag.

package claudejson

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

// oauthAccount is a minimal struct that captures only the one field we need.
// All other fields in the real oauthAccount object (tokens, etc.) are ignored
// by the JSON decoder because they are not present in this struct.
type oauthAccount struct {
	HasExtraUsageEnabled bool `json:"hasExtraUsageEnabled"`
}

// claudeJSON is the minimal top-level struct. Only oauthAccount is parsed.
type claudeJSON struct {
	OauthAccount oauthAccount `json:"oauthAccount"`
}

// cacheEntry holds the most recently read value and the mtime at which it was read.
type cacheEntry struct {
	mu    sync.Mutex
	value bool
	mtime time.Time
	valid bool // true once a successful read has populated the cache
}

// pkgCache is the package-level mtime cache.
var pkgCache cacheEntry

// HasExtraUsageEnabled reads oauthAccount.hasExtraUsageEnabled from
// ~/.claude.json and returns its value.
//
// Fail-soft contract:
//   - File missing → false (no log; expected on some setups).
//   - File unreadable / JSON invalid / field absent → false + Warn log (fact only, no data).
//   - HOME not set → false + Warn log.
//
// mtime-cache: the file is re-read only when its mtime has changed since the
// last successful read. A cached value is returned on unchanged mtime.
func HasExtraUsageEnabled() bool {
	pkgCache.mu.Lock()
	defer pkgCache.mu.Unlock()

	p := claudeJSONPath()
	if p == "" {
		slog.Warn("claudejson: HOME not set; cannot locate ~/.claude.json")
		return false
	}

	// Stat the file to check mtime.
	fi, err := os.Stat(p)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("claudejson: stat failed")
		}
		// File missing or inaccessible.
		// Return stale cached value if available (file temporarily gone), else false.
		if pkgCache.valid {
			return pkgCache.value
		}
		return false
	}

	mtime := fi.ModTime()

	// Return cached value if mtime has not changed since last successful read.
	if pkgCache.valid && mtime.Equal(pkgCache.mtime) {
		return pkgCache.value
	}

	// mtime changed (or first call) — re-read the file.
	data, err := os.ReadFile(p)
	if err != nil {
		slog.Warn("claudejson: read failed")
		if pkgCache.valid {
			return pkgCache.value
		}
		return false
	}

	var parsed claudeJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		slog.Warn("claudejson: parse failed")
		if pkgCache.valid {
			return pkgCache.value
		}
		return false
	}

	// Update cache on successful parse.
	pkgCache.value = parsed.OauthAccount.HasExtraUsageEnabled
	pkgCache.mtime = mtime
	pkgCache.valid = true

	return pkgCache.value
}

// ResetCacheForTest clears the package-level mtime cache.
// Must only be called from tests (via t.Cleanup or at the start of each test case).
func ResetCacheForTest() {
	pkgCache.mu.Lock()
	defer pkgCache.mu.Unlock()
	pkgCache.value = false
	pkgCache.mtime = time.Time{}
	pkgCache.valid = false
}
