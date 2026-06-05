// Package claudejson reads a single boolean field from ~/.claude.json.
//
// Security note: ~/.claude.json contains OAuth tokens and other sensitive data.
// This package reads ONLY the oauthAccount.hasExtraUsageEnabled field.
// File contents are never logged; on error only the fact is logged.
//
// Path resolution (in priority order):
//  1. CC_PROBELINE_CLAUDE_JSON env var — full path to the file (used by tests).
//  2. $HOME/.claude.json (production default).
//
// The result is cached by the file's mtime: a second call that finds the same
// mtime returns the cached value without re-reading the file.
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

// claudeJSONPath returns the path to ~/.claude.json, honouring the test
// override env var CC_PROBELINE_CLAUDE_JSON.
func claudeJSONPath() string {
	if p := os.Getenv("CC_PROBELINE_CLAUDE_JSON"); p != "" {
		return p
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return home + "/.claude.json"
}

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
