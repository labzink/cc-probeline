// Package claudejson_test — tests for internal/claudejson.ReadUsage.
//
// The fixtures below are REAL cachedUsageUtilization branches, not invented
// shapes: `friendSnapshot` was captured on a Max 20x account that actually has
// the model-scoped ("Fable") weekly limit, and `ownSnapshot` on an account that
// has none. Account uuids are scrubbed. All tests are hermetic — the
// CC_PROBELINE_CLAUDE_JSON env var points at a temporary file.
package claudejson_test

import (
	"os"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/claudejson"
)

// friendSnapshot: account WITH a model-scoped weekly window. The reset instant
// of weekly_all and of the model window is the same moment, and it equals unix
// 1786903200 — the value the status-line payload carries for seven_day on the
// same machine. That cross-source agreement is what TestReadUsage_Fable asserts.
const friendSnapshot = `{
  "oauthAccount": {"hasExtraUsageEnabled": false},
  "cachedUsageUtilization": {
    "accountUuid": "scrubbed",
    "fetchedAtMs": 1786447385992,
    "utilization": {
      "five_hour": {"utilization": 2, "resets_at": "2026-08-11T14:00:00.262342+00:00"},
      "seven_day": {"utilization": 15, "resets_at": "2026-08-16T18:00:00.262362+00:00"},
      "seven_day_opus": null,
      "seven_day_sonnet": null,
      "tangelo": null,
      "nimbus_quill": {"utilization": 0, "resets_at": null},
      "limits": [
        {"kind": "session", "group": "session", "percent": 2, "severity": "normal",
         "resets_at": "2026-08-11T14:00:00.262342+00:00", "scope": null, "is_active": false},
        {"kind": "weekly_all", "group": "weekly", "percent": 15, "severity": "normal",
         "resets_at": "2026-08-16T18:00:00.262362+00:00", "scope": null, "is_active": false},
        {"kind": "weekly_scoped", "group": "weekly", "percent": 25, "severity": "normal",
         "resets_at": "2026-08-16T18:00:00.262577+00:00",
         "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null},
         "is_active": true}
      ]
    }
  }
}`

// ownSnapshot: account WITHOUT any model-scoped window. extra_usage is present
// but switched off, with the monthly ceiling in cents (10746 == $107.46).
const ownSnapshot = `{
  "cachedUsageUtilization": {
    "fetchedAtMs": 1786434463031,
    "utilization": {
      "five_hour": {"utilization": 67, "resets_at": "2026-08-11T07:50:00.304538+00:00"},
      "seven_day": {"utilization": 83, "resets_at": "2026-08-11T08:00:00.304562+00:00"},
      "extra_usage": {"is_enabled": false, "monthly_limit": 10746, "used_credits": 0,
                      "utilization": 0, "disabled_reason": "out_of_credits"},
      "limits": [
        {"kind": "session", "group": "session", "percent": 67, "severity": "normal",
         "resets_at": "2026-08-11T07:50:00.304538+00:00", "scope": null, "is_active": false},
        {"kind": "weekly_all", "group": "weekly", "percent": 83, "severity": "warning",
         "resets_at": "2026-08-11T08:00:00.304562+00:00", "scope": null, "is_active": true}
      ]
    }
  }
}`

// setUsagePath points the package at p and clears the ReadUsage mtime cache.
func setUsagePath(t *testing.T, p string) {
	t.Helper()
	t.Setenv("CC_PROBELINE_CLAUDE_JSON", p)
	claudejson.ResetUsageCacheForTest()
}

// ---------------------------------------------------------------------------
// Property: a real weekly_scoped entry yields the model window, and its ISO-8601
// reset resolves to the same unix instant the status-line payload reports.
// ---------------------------------------------------------------------------

func TestReadUsage_Fable(t *testing.T) {
	setUsagePath(t, writeFixture(t, t.TempDir(), "claude.json", friendSnapshot))

	u, ok := claudejson.ReadUsage()
	if !ok {
		t.Fatal("expected ok=true for a snapshot carrying the usage branch")
	}
	if u.Model == nil {
		t.Fatal("expected a model-scoped window, got nil")
	}
	if u.Model.Name != "fable" {
		t.Errorf("Name = %q, want %q (display_name lower-cased)", u.Model.Name, "fable")
	}
	if u.Model.Pct != 25 {
		t.Errorf("Pct = %v, want 25", u.Model.Pct)
	}
	// 1786903200 is what the status-line payload sends for seven_day on this
	// machine: the model window resets at the very same moment.
	if u.Model.ResetUnix != 1786903200 {
		t.Errorf("ResetUnix = %d, want 1786903200 (same instant as seven_day in stdin)", u.Model.ResetUnix)
	}
	if got, want := u.FetchedAt.UnixMilli(), int64(1786447385992); got != want {
		t.Errorf("FetchedAt = %d ms, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// Property: an account with no model-scoped window reports Model=nil (the status
// line must then look exactly as it did before this feature) while extra usage
// still parses, with cents converted to dollars.
// ---------------------------------------------------------------------------

func TestReadUsage_NoModelWindow_ExtraInCents(t *testing.T) {
	setUsagePath(t, writeFixture(t, t.TempDir(), "claude.json", ownSnapshot))

	u, ok := claudejson.ReadUsage()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if u.Model != nil {
		t.Errorf("expected no model window, got %+v", u.Model)
	}
	if u.Extra == nil {
		t.Fatal("expected extra usage block, got nil")
	}
	if u.Extra.Enabled {
		t.Error("Enabled = true, want false (is_enabled is false in the snapshot)")
	}
	if u.Extra.LimitUSD != 107.46 {
		t.Errorf("LimitUSD = %v, want 107.46 (10746 cents)", u.Extra.LimitUSD)
	}
	if u.Extra.UsedUSD != 0 {
		t.Errorf("UsedUSD = %v, want 0", u.Extra.UsedUSD)
	}
}

// ---------------------------------------------------------------------------
// Property: no usage branch (the state of every machine that has never opened
// /usage) → ok=false. Same for a missing file.
// ---------------------------------------------------------------------------

func TestReadUsage_BranchAbsent(t *testing.T) {
	setUsagePath(t, writeFixture(t, t.TempDir(), "claude.json",
		`{"oauthAccount":{"hasExtraUsageEnabled":true}}`))

	if _, ok := claudejson.ReadUsage(); ok {
		t.Error("expected ok=false when cachedUsageUtilization is absent")
	}
}

func TestReadUsage_FileMissing(t *testing.T) {
	setUsagePath(t, t.TempDir()+"/nope.json")

	if _, ok := claudejson.ReadUsage(); ok {
		t.Error("expected ok=false when the file does not exist")
	}
}

// ---------------------------------------------------------------------------
// Property: a snapshot with no fetchedAtMs is unusable — without it we cannot
// tell how stale the numbers are, and stale numbers must not reach the line.
// ---------------------------------------------------------------------------

func TestReadUsage_NoTimestamp(t *testing.T) {
	setUsagePath(t, writeFixture(t, t.TempDir(), "claude.json",
		`{"cachedUsageUtilization":{"utilization":{"limits":[]}}}`))

	if _, ok := claudejson.ReadUsage(); ok {
		t.Error("expected ok=false when fetchedAtMs is missing")
	}
}

// ---------------------------------------------------------------------------
// Property: with several model-scoped windows the hottest one wins (it is the
// one worth the space), and a long display name is lower-cased and capped.
// ---------------------------------------------------------------------------

func TestReadUsage_HottestModelWindowWins(t *testing.T) {
	const twoScoped = `{
      "cachedUsageUtilization": {
        "fetchedAtMs": 1786447385992,
        "utilization": {"limits": [
          {"kind": "weekly_scoped", "percent": 12, "resets_at": "2026-08-16T18:00:00+00:00",
           "scope": {"model": {"display_name": "Sonnet"}}},
          {"kind": "weekly_scoped", "percent": 71, "resets_at": "2026-08-16T18:00:00+00:00",
           "scope": {"model": {"display_name": "SuperlongModelName"}}}
        ]}
      }
    }`
	setUsagePath(t, writeFixture(t, t.TempDir(), "claude.json", twoScoped))

	u, ok := claudejson.ReadUsage()
	if !ok || u.Model == nil {
		t.Fatal("expected a model window")
	}
	if u.Model.Pct != 71 {
		t.Errorf("Pct = %v, want 71 (hottest window wins)", u.Model.Pct)
	}
	if u.Model.Name != "superlon" {
		t.Errorf("Name = %q, want %q (lower-cased, capped at 8 runes)", u.Model.Name, "superlon")
	}
}

// ---------------------------------------------------------------------------
// Property: entries without a model scope never become a model window. This is
// what keeps Claude Code's internal code-named windows (tangelo, nimbus_quill,
// amber_ladder …) and the plain session/weekly_all limits out of the line.
// ---------------------------------------------------------------------------

func TestReadUsage_UnscopedEntriesIgnored(t *testing.T) {
	const unscopedOnly = `{
      "cachedUsageUtilization": {
        "fetchedAtMs": 1786447385992,
        "utilization": {"limits": [
          {"kind": "session", "percent": 40, "scope": null},
          {"kind": "weekly_all", "percent": 90, "scope": null},
          {"kind": "weekly_scoped", "percent": 99, "scope": {"model": null}},
          {"kind": "weekly_scoped", "percent": 98, "scope": {"model": {"display_name": "  "}}}
        ]}
      }
    }`
	setUsagePath(t, writeFixture(t, t.TempDir(), "claude.json", unscopedOnly))

	u, ok := claudejson.ReadUsage()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if u.Model != nil {
		t.Errorf("expected no model window, got %+v", u.Model)
	}
}

// ---------------------------------------------------------------------------
// Property: the mtime cache serves repeats but does not hide a real change.
// ---------------------------------------------------------------------------

func TestReadUsage_MtimeCacheRefreshes(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", ownSnapshot)
	setUsagePath(t, p)

	if u, ok := claudejson.ReadUsage(); !ok || u.Model != nil {
		t.Fatal("setup: expected a usable snapshot with no model window")
	}

	// Rewrite with the Fable snapshot and move mtime forward.
	if err := os.WriteFile(p, []byte(friendSnapshot), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	u, ok := claudejson.ReadUsage()
	if !ok || u.Model == nil {
		t.Fatal("expected the rewritten snapshot to be picked up")
	}
	if u.Model.Name != "fable" {
		t.Errorf("Name = %q, want %q after refresh", u.Model.Name, "fable")
	}
}
