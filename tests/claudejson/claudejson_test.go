// Package claudejson_test tests internal/claudejson.HasExtraUsageEnabled.
// All tests are hermetic: the CC_PROBELINE_CLAUDE_JSON env var points to a
// temporary file, so the real ~/.claude.json is never touched.
package claudejson_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/claudejson"
)

// writeFixture writes content to a file in dir and returns the full path.
func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFixture: %v", err)
	}
	return p
}

// setPath configures CC_PROBELINE_CLAUDE_JSON and resets the cache.
// The env var is restored automatically via t.Cleanup.
func setPath(t *testing.T, p string) {
	t.Helper()
	t.Setenv("CC_PROBELINE_CLAUDE_JSON", p)
	claudejson.ResetCacheForTest()
}

// ---------------------------------------------------------------------------
// Property: field true → HasExtraUsageEnabled returns true.
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_True(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{"oauthAccount":{"hasExtraUsageEnabled":true}}`)
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); !got {
		t.Errorf("expected true, got false")
	}
}

// ---------------------------------------------------------------------------
// Property: field false → HasExtraUsageEnabled returns false.
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_False(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{"oauthAccount":{"hasExtraUsageEnabled":false}}`)
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); got {
		t.Errorf("expected false, got true")
	}
}

// ---------------------------------------------------------------------------
// Property: field absent → false (oauthAccount exists but no hasExtraUsageEnabled).
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_FieldAbsent(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{"oauthAccount":{}}`)
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); got {
		t.Errorf("expected false when field absent, got true")
	}
}

// ---------------------------------------------------------------------------
// Property: oauthAccount key missing entirely → false.
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_OauthAccountMissing(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{"someOtherKey":"value"}`)
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); got {
		t.Errorf("expected false when oauthAccount missing, got true")
	}
}

// ---------------------------------------------------------------------------
// Property: broken JSON → false (no panic).
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_BrokenJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{not valid json`)
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); got {
		t.Errorf("expected false for broken JSON, got true")
	}
}

// ---------------------------------------------------------------------------
// Property: file missing → false (no panic).
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_FileMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nonexistent.json")
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); got {
		t.Errorf("expected false for missing file, got true")
	}
}

// ---------------------------------------------------------------------------
// Property: mtime unchanged → second call does NOT re-read file.
// Verified by: first call reads file (true), then we delete the file.
// Second call must return the cached true (not false due to missing file).
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_MtimeCache_NoReread(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{"oauthAccount":{"hasExtraUsageEnabled":true}}`)
	setPath(t, p)

	// First call — populate cache.
	got1 := claudejson.HasExtraUsageEnabled()
	if !got1 {
		t.Fatalf("first call: expected true, got false")
	}

	// Delete the file without changing any mtime we stored.
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Second call — file is gone, but mtime in cache matches what was stored.
	// The implementation falls back to cached value when stat fails (file gone).
	got2 := claudejson.HasExtraUsageEnabled()
	if !got2 {
		t.Errorf("second call (file deleted, mtime unchanged): expected cached true, got false")
	}
}

// ---------------------------------------------------------------------------
// Property: mtime changed → second call re-reads the file.
// Verified by: first call caches true, then we rewrite file with false and
// bump mtime by touching the file with a future time.
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_MtimeCache_Reread(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "claude.json", `{"oauthAccount":{"hasExtraUsageEnabled":true}}`)
	setPath(t, p)

	// First call — caches true.
	got1 := claudejson.HasExtraUsageEnabled()
	if !got1 {
		t.Fatalf("first call: expected true, got false")
	}

	// Rewrite file content with false + advance mtime by 2 seconds so the
	// cache detects the change.
	if err := os.WriteFile(p, []byte(`{"oauthAccount":{"hasExtraUsageEnabled":false}}`), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	// Advance mtime explicitly to ensure it differs from the cached one.
	newMtime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, newMtime, newMtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got2 := claudejson.HasExtraUsageEnabled()
	if got2 {
		t.Errorf("second call (mtime changed, content=false): expected false, got true")
	}
}

// ---------------------------------------------------------------------------
// Property: extra unknown fields in oauthAccount are ignored (no decode error).
// ---------------------------------------------------------------------------

func TestHasExtraUsageEnabled_ExtraFieldsIgnored(t *testing.T) {
	dir := t.TempDir()
	// Simulate a realistic payload with tokens and other fields present.
	// We only check that HasExtraUsageEnabled is read correctly; tokens are ignored.
	p := writeFixture(t, dir, "claude.json", `{
		"oauthAccount": {
			"accountUUID": "some-uuid",
			"emailAddress": "user@example.com",
			"hasExtraUsageEnabled": true,
			"accessToken": "REDACTED_DO_NOT_LOG"
		},
		"someOtherTopKey": 42
	}`)
	setPath(t, p)

	if got := claudejson.HasExtraUsageEnabled(); !got {
		t.Errorf("expected true when hasExtraUsageEnabled=true with extra fields, got false")
	}
}
