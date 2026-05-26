// Package hint_test — Phase 6.d ConfigError alert + ToCacheEvents tests
// (T-HA1..T-HA9).
//
// Tests verify:
//   - AlertTexts[parser.ConfigError] is populated and mentions "check-config".
//   - criticalTypes includes parser.ConfigError after CompactHeuristic.
//   - BuildAlert correctly dispatches ConfigError events.
//   - config.ToCacheEvents collapses SeverityError → single CacheEvent.
//   - Warnings and empty input produce nil.
//
// All tests are intentionally RED until Phase 6.d GREEN lands:
//   - AlertTexts does not yet contain ConfigError.
//   - criticalTypes does not yet include ConfigError.
//   - config.ToCacheEvents returns nil unconditionally (stub).
package hint_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/parser"
)

// ─── T-HA1: AlertTexts[ConfigError] is present ───────────────────────────────

// TestAlertTexts_ConfigErrorPresent verifies that the AlertTexts map has a
// non-empty entry for parser.ConfigError (Phase 6.d §2.3 patch).
func TestAlertTexts_ConfigErrorPresent(t *testing.T) {
	text, ok := hint.AlertTexts[parser.ConfigError]
	if !ok {
		t.Errorf("T-HA1: AlertTexts[parser.ConfigError] key is absent; must be present after Phase 6.d GREEN")
		return
	}
	if text == "" {
		t.Errorf("T-HA1: AlertTexts[parser.ConfigError] is empty string; must be non-empty")
	}
}

// ─── T-HA2: AlertTexts[ConfigError] mentions "check-config" ─────────────────

// TestAlertTexts_ConfigErrorMentionsCheckConfig verifies the planned alert text
// "⚠ Config error · run cc-probeline check-config" contains the action hint
// "check-config" so users know what command to run for details.
func TestAlertTexts_ConfigErrorMentionsCheckConfig(t *testing.T) {
	text := hint.AlertTexts[parser.ConfigError]
	if text == "" {
		t.Skip("T-HA2: AlertTexts[ConfigError] absent — covered by T-HA1; skipping duplicate failure")
	}
	wantSubstr := "check-config"
	if len(text) < len(wantSubstr) || !containsStr(text, wantSubstr) {
		t.Errorf("T-HA2: AlertTexts[ConfigError] = %q; must contain %q", text, wantSubstr)
	}
}

// ─── T-HA3: criticalTypes includes ConfigError ────────────────────────────────

// TestCriticalTypes_ConfigErrorIncluded verifies that parser.ConfigError is
// present in the exported criticalTypes slice (§2.3 patch). We verify via
// BuildAlert behaviour: a ConfigError event must produce a non-empty alert.
func TestCriticalTypes_ConfigErrorIncluded(t *testing.T) {
	events := []parser.CacheEvent{
		{Type: parser.ConfigError},
	}
	got := hint.BuildAlert(events)
	if got == "" {
		t.Errorf("T-HA3: BuildAlert([ConfigError]) = %q; ConfigError must be in criticalTypes (currently absent → empty alert)", got)
	}
}

// ─── T-HA4: ConfigError priority after CompactHeuristic ──────────────────────

// TestCriticalTypes_PriorityOrder verifies that when both CompactHeuristic and
// ConfigError events are present, ConfigError does NOT override CompactHeuristic
// (CompactHeuristic appears earlier in criticalTypes → lower index → higher priority).
//
// Per §2.3: ConfigError is appended after CompactHeuristic in criticalTypes,
// so CompactHeuristic wins in a tie between the two.
//
// This test is properly RED when ConfigError is absent from criticalTypes AND
// absent from AlertTexts, because:
//  1. A standalone ConfigError event must produce a non-empty alert (T-HA3 covers this).
//  2. The priority test requires ConfigError to be present in criticalTypes first —
//     we use a combined event set and verify both that ConfigError-only produces an
//     alert AND that CompactHeuristic beats it.
func TestCriticalTypes_PriorityOrder(t *testing.T) {
	// Pre-condition: ConfigError must be in criticalTypes (i.e. produce an alert).
	// If not, this test is not meaningful yet — but we still FAIL to indicate the gap.
	configErrorAlert := hint.AlertTexts[parser.ConfigError]
	if configErrorAlert == "" {
		t.Errorf("T-HA4: precondition: AlertTexts[ConfigError] is absent — ConfigError must be in criticalTypes before priority order can be validated")
		// Do not return: continue to test priority order as well.
	}

	// Verify that a ConfigError-only event produces the ConfigError alert text.
	// This fails if ConfigError is not in criticalTypes (BuildAlert returns "").
	onlyConfigErr := []parser.CacheEvent{{Type: parser.ConfigError}}
	gotConfigOnly := hint.BuildAlert(onlyConfigErr)
	if gotConfigOnly == "" {
		t.Errorf("T-HA4: BuildAlert([ConfigError]) = %q; must be non-empty when ConfigError is in criticalTypes", gotConfigOnly)
	}

	// Now verify CompactHeuristic wins when both are present.
	events := []parser.CacheEvent{
		{Type: parser.CompactHeuristic},
		{Type: parser.ConfigError},
	}
	got := hint.BuildAlert(events)
	wantContains := hint.AlertTexts[parser.CompactHeuristic]
	if wantContains == "" {
		t.Skip("T-HA4: AlertTexts[CompactHeuristic] absent unexpectedly; skipping priority comparison")
	}
	// CompactHeuristic (earlier in criticalTypes) must win over ConfigError.
	if got != wantContains {
		t.Errorf("T-HA4: BuildAlert(CompactHeuristic+ConfigError) = %q; want CompactHeuristic alert %q (CompactHeuristic must be higher priority)", got, wantContains)
	}
}

// ─── T-HA5: BuildAlert dispatches ConfigError → expected text ────────────────

// TestBuildAlert_ConfigError_ReturnsExpectedText verifies the exact round-trip:
// BuildAlert with a single ConfigError event must return AlertTexts[ConfigError].
func TestBuildAlert_ConfigError_ReturnsExpectedText(t *testing.T) {
	wantText := hint.AlertTexts[parser.ConfigError]
	if wantText == "" {
		t.Skip("T-HA5: AlertTexts[ConfigError] absent; covered by T-HA1")
	}
	events := []parser.CacheEvent{
		{Type: parser.ConfigError},
	}
	got := hint.BuildAlert(events)
	if got != wantText {
		t.Errorf("T-HA5: BuildAlert([ConfigError]) = %q; want %q", got, wantText)
	}
}

// ─── T-HA6: ToCacheEvents — one SeverityError → one ConfigError event ────────

// TestConfigToCacheEvents_OneSeverityError_ProducesOneEvent verifies that a
// single SeverityError in the input slice produces exactly one CacheEvent of
// type parser.ConfigError.
//
// Verify-RED arithmetic: input=[{Severity:SeverityError}], expected len=1,
// expected event.Type=parser.ConfigError. Stub returns nil → len assertion fails.
func TestConfigToCacheEvents_OneSeverityError_ProducesOneEvent(t *testing.T) {
	errs := []config.Error{
		{
			Severity: config.SeverityError,
			Field:    "version",
			Message:  "unsupported version: 99",
		},
	}
	events := config.ToCacheEvents(errs)

	if len(events) != 1 {
		t.Errorf("T-HA6: ToCacheEvents([SeverityError]) returned %d events; want 1", len(events))
		return
	}
	if events[0].Type != parser.ConfigError {
		t.Errorf("T-HA6: event[0].Type = %v; want parser.ConfigError (%v)", events[0].Type, parser.ConfigError)
	}
}

// ─── T-HA7: ToCacheEvents — warnings only → nil ──────────────────────────────

// TestConfigToCacheEvents_OnlyWarnings_ReturnsNil verifies §2.4:
// SeverityWarning entries alone must NOT produce a ConfigError event.
// Warnings (e.g. unknown fields) are non-fatal and must not alarm the user.
//
// Verify-RED arithmetic: input=[{Severity:SeverityWarning}], expected nil.
// Stub returns nil → this test would PASS vacuously.
// To make it properly RED we also check the non-nil case is correct (T-HA6).
func TestConfigToCacheEvents_OnlyWarnings_ReturnsNil(t *testing.T) {
	errs := []config.Error{
		{
			Severity: config.SeverityWarning,
			Field:    "ctx_silly_ratio",
			Message:  "unknown field",
		},
	}
	events := config.ToCacheEvents(errs)

	// Warnings must be silently dropped; nil or empty slice both acceptable.
	if len(events) != 0 {
		t.Errorf("T-HA7: ToCacheEvents([SeverityWarning]) = %v (len %d); want nil/empty", events, len(events))
	}
}

// ─── T-HA8: ToCacheEvents — multiple errors → collapsed to one event ──────────

// TestConfigToCacheEvents_MultipleErrors_CollapsedToOne verifies the collapse
// policy (§2.4): all SeverityError entries produce exactly ONE CacheEvent
// (one alert per render). Users run check-config for details.
//
// Verify-RED arithmetic: 5 SeverityError inputs → expected len=1.
// Stub returns nil → len assertion fails with got=0, want=1.
func TestConfigToCacheEvents_MultipleErrors_CollapsedToOne(t *testing.T) {
	errs := []config.Error{
		{Severity: config.SeverityError, Field: "version", Message: "unsupported version: 99"},
		{Severity: config.SeverityError, Field: "theme.colors.cyan", Message: "invalid hex color: notacolor"},
		{Severity: config.SeverityError, Field: "thresholds.ctx_warn_ratio", Message: "below minimum: -0.1"},
		{Severity: config.SeverityError, Field: "probes.email.address", Message: "invalid email: notanemail"},
		{Severity: config.SeverityError, Field: "thresholds.cost_budget_usd", Message: "below minimum: -5.0"},
	}
	events := config.ToCacheEvents(errs)

	// Collapse policy: 5 errors → 1 event.
	if len(events) != 1 {
		t.Errorf("T-HA8: ToCacheEvents(5×SeverityError) returned %d events; want 1 (collapse policy)", len(events))
		return
	}
	if events[0].Type != parser.ConfigError {
		t.Errorf("T-HA8: event[0].Type = %v; want parser.ConfigError", events[0].Type)
	}
}

// ─── T-HA9: ToCacheEvents — nil input → nil output ────────────────────────────

// TestConfigToCacheEvents_EmptyInput_ReturnsNil verifies that ToCacheEvents(nil)
// returns nil (not an empty non-nil slice) to avoid caller confusion.
//
// Verify-RED arithmetic: input=nil, expected nil.
// Stub returns nil → this test PASSES vacuously (stub correct for this case).
// Listed here for completeness and regression protection after GREEN.
func TestConfigToCacheEvents_EmptyInput_ReturnsNil(t *testing.T) {
	events := config.ToCacheEvents(nil)
	if events != nil {
		t.Errorf("T-HA9: ToCacheEvents(nil) = %v; want nil", events)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// containsStr reports whether s contains substr (reimplemented here to avoid
// importing strings in a test package that doesn't otherwise need it).
func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
