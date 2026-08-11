package claudejson

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// modelNameMaxRunes caps the model label shown in the status line ("fable",
// "opus", …). Claude Code sends a human display name of unbounded length; the
// quota block has no room for a long one.
const modelNameMaxRunes = 8

// centsPerDollar converts the overage figures, which Claude Code stores in
// cents (bundle: monthly_limit = spendLimitCents, used_credits = usedCents).
const centsPerDollar = 100.0

// ModelWindow is a weekly rate-limit window scoped to one model — the third
// limit that appeared in subscriptions in 2026 ("Fable" for the user who first
// hit it). Claude Code never sends it in the status-line payload; it exists
// only inside ~/.claude.json, as an entry of cachedUsageUtilization with
// kind == "weekly_scoped".
type ModelWindow struct {
	// Name is scope.model.display_name, lower-cased and capped. We key off the
	// display name rather than a hard-coded model id exactly as Claude Code's
	// own usage screen does, so a renamed model keeps working.
	Name string
	// Pct is the used percentage, 0–100.
	Pct float64
	// ResetUnix is when the window resets, in unix seconds; 0 when unknown.
	ResetUnix int64
}

// ExtraUsage is the account's paid-overage state: whether extra usage is on,
// how much of it has been spent this month and what the monthly ceiling is.
// Unlike our own estimate this is Anthropic's own figure, account-wide across
// every machine — the same nature as the 5h/7d percentages beside it.
type ExtraUsage struct {
	Enabled  bool
	UsedUSD  float64
	LimitUSD float64
	// Currency is the ISO code the account is billed in ("USD" when absent).
	// Rendering keys off it: a "$" in front of a euro figure is simply wrong.
	Currency string
}

// Usage is one reading of the cachedUsageUtilization branch.
//
// FetchedAt is the moment Claude Code itself last refreshed that branch (its
// fetchedAtMs), NOT the moment we read the file: the file is rewritten
// constantly for unrelated reasons while the usage branch stays frozen. Claude
// Code only refreshes it when the /usage screen runs, and never more often than
// once per 5 minutes.
type Usage struct {
	FetchedAt time.Time
	Model     *ModelWindow // nil when the account has no model-scoped window
	Extra     *ExtraUsage  // nil when the branch carries no extra_usage object
}

// --- wire structs: only the fields we need, so nothing else (the file holds
// OAuth tokens) is ever decoded into memory. ---

type usageScopeModel struct {
	DisplayName string `json:"display_name"`
}

type usageScope struct {
	Model *usageScopeModel `json:"model"`
}

type usageLimit struct {
	Kind     string      `json:"kind"`
	Percent  float64     `json:"percent"`
	ResetsAt string      `json:"resets_at"`
	Scope    *usageScope `json:"scope"`
}

type usageExtra struct {
	IsEnabled    bool     `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	UsedCredits  *float64 `json:"used_credits"`
	Currency     string   `json:"currency"`
}

type usageUtilization struct {
	Limits     []usageLimit `json:"limits"`
	ExtraUsage *usageExtra  `json:"extra_usage"`
}

type cachedUsage struct {
	FetchedAtMs int64            `json:"fetchedAtMs"`
	Utilization usageUtilization `json:"utilization"`
}

type usageFile struct {
	CachedUsage *cachedUsage `json:"cachedUsageUtilization"`
}

// usageCache is a private mtime cache, separate from the one guarding
// HasExtraUsageEnabled so neither call can invalidate the other's reading.
type usageCacheEntry struct {
	mu    sync.Mutex
	value Usage
	ok    bool
	mtime time.Time
	valid bool
}

var usageCache usageCacheEntry

// ReadUsage returns the cachedUsageUtilization reading from ~/.claude.json.
//
// ok=false means "we know nothing" — file missing, branch absent (it only
// appears after the /usage screen has run at least once), or unparseable. The
// caller must then render exactly what it rendered before this feature existed.
//
// mtime-cache: the file is re-read only when its mtime has changed. In
// production this rarely earns its keep — one render is one process, and that
// process calls ReadUsage once — but it keeps repeat calls cheap for callers
// that make them, and holds the last good reading when a read fails against a
// file being rewritten underneath us.
func ReadUsage() (Usage, bool) {
	usageCache.mu.Lock()
	defer usageCache.mu.Unlock()

	p := claudeJSONPath()
	if p == "" {
		slog.Warn("claudejson: HOME not set; cannot locate ~/.claude.json")
		return Usage{}, false
	}

	fi, err := os.Stat(p)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("claudejson.ReadUsage: stat failed")
		}
		if usageCache.valid {
			return usageCache.value, usageCache.ok
		}
		return Usage{}, false
	}

	mtime := fi.ModTime()
	if usageCache.valid && mtime.Equal(usageCache.mtime) {
		return usageCache.value, usageCache.ok
	}

	data, err := os.ReadFile(p)
	if err != nil {
		slog.Warn("claudejson.ReadUsage: read failed")
		if usageCache.valid {
			return usageCache.value, usageCache.ok
		}
		return Usage{}, false
	}

	var parsed usageFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		slog.Warn("claudejson.ReadUsage: parse failed")
		if usageCache.valid {
			return usageCache.value, usageCache.ok
		}
		return Usage{}, false
	}

	u, ok := buildUsage(parsed.CachedUsage)

	usageCache.value = u
	usageCache.ok = ok
	usageCache.mtime = mtime
	usageCache.valid = true

	return u, ok
}

// buildUsage converts the decoded branch into a Usage. Returns ok=false when the
// branch is absent or carries no timestamp — without fetchedAtMs we cannot tell
// how stale the numbers are, and stale numbers must not reach the status line.
func buildUsage(c *cachedUsage) (Usage, bool) {
	if c == nil || c.FetchedAtMs <= 0 {
		return Usage{}, false
	}

	u := Usage{FetchedAt: time.UnixMilli(c.FetchedAtMs)}

	// Model-scoped window: pick the hottest one. Entries without a model scope
	// (session, weekly_all, and Claude Code's internal code-named windows such as
	// tangelo or nimbus_quill) drop out here by construction.
	for _, l := range c.Utilization.Limits {
		if l.Kind != "weekly_scoped" || l.Scope == nil || l.Scope.Model == nil {
			continue
		}
		name := modelLabel(l.Scope.Model.DisplayName)
		if name == "" {
			continue
		}
		if u.Model != nil && l.Percent <= u.Model.Pct {
			continue
		}
		u.Model = &ModelWindow{
			Name:      name,
			Pct:       l.Percent,
			ResetUnix: parseResetUnix(l.ResetsAt),
		}
	}

	if e := c.Utilization.ExtraUsage; e != nil {
		u.Extra = &ExtraUsage{
			Enabled:  e.IsEnabled,
			UsedUSD:  centsToUSD(e.UsedCredits),
			LimitUSD: centsToUSD(e.MonthlyLimit),
			Currency: e.Currency,
		}
	}

	return u, true
}

// modelLabel lower-cases and caps a display name for the status line. Returns ""
// for a name that is empty or whitespace only.
func modelLabel(displayName string) string {
	s := strings.ToLower(strings.TrimSpace(displayName))
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > modelNameMaxRunes {
		return string(r[:modelNameMaxRunes])
	}
	return s
}

// parseResetUnix converts the ISO-8601 reset timestamp used inside
// ~/.claude.json (e.g. "2026-08-16T18:00:00.262577+00:00") to unix seconds.
// Returns 0 when absent or malformed — the caller treats 0 as "unknown".
// Note the asymmetry with the status-line payload, where the same moment
// arrives as a plain unix number.
func parseResetUnix(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		slog.Warn("claudejson.ReadUsage: unparseable resets_at")
		return 0
	}
	return t.Unix()
}

// centsToUSD converts a nullable cents figure to dollars; a null reads as 0.
func centsToUSD(cents *float64) float64 {
	if cents == nil {
		return 0
	}
	return *cents / centsPerDollar
}

// ResetUsageCacheForTest clears the ReadUsage mtime cache.
// Must only be called from tests.
func ResetUsageCacheForTest() {
	usageCache.mu.Lock()
	defer usageCache.mu.Unlock()
	usageCache.value = Usage{}
	usageCache.ok = false
	usageCache.mtime = time.Time{}
	usageCache.valid = false
}
