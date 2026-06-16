package config_test

// alerts_test.go — tests for ToCacheEvents in internal/config/alerts.go.
// Part of C1 gate: coverage for internal/config/ must reach >=90%.

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/parser"
)

// T-CA1: empty errs slice → nil result (no event produced).
func TestToCacheEvents_EmptyErrs_ReturnsNil(t *testing.T) {
	result := config.ToCacheEvents(nil)
	if result != nil {
		t.Errorf("ToCacheEvents(nil): expected nil, got %v", result)
	}

	result = config.ToCacheEvents([]config.Error{})
	if result != nil {
		t.Errorf("ToCacheEvents([]): expected nil, got %v", result)
	}
}

// T-CA2: warnings only → nil result (warnings don't trigger a config alert).
func TestToCacheEvents_WarningsOnly_ReturnsNil(t *testing.T) {
	errs := []config.Error{
		{
			Severity: config.SeverityWarning,
			Field:    "general.refresh_interval_hint",
			Message:  "refresh interval 10000 is unusual",
		},
		{
			Severity: config.SeverityWarning,
			Field:    "probes.email.address",
			Message:  "email looks malformed",
		},
	}

	result := config.ToCacheEvents(errs)
	if result != nil {
		t.Errorf("ToCacheEvents(warnings only): expected nil, got %v", result)
	}
}

// T-CA3: one SeverityError → single ConfigError event with correct Type and recent Timestamp.
func TestToCacheEvents_OneSeverityError_ReturnsEvent(t *testing.T) {
	before := time.Now()
	errs := []config.Error{
		{
			Severity: config.SeverityError,
			Field:    "thresholds.ctx_warn_ratio",
			Message:  "ratio must be in [0.0, 1.0]",
		},
	}

	result := config.ToCacheEvents(errs)
	after := time.Now()

	if len(result) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(result), result)
	}

	ev := result[0]
	if ev.Type != parser.ConfigError {
		t.Errorf("event.Type = %v, want parser.ConfigError", ev.Type)
	}

	// Timestamp must be within the call window (before ≤ ts ≤ after).
	if ev.Timestamp.Before(before) || ev.Timestamp.After(after) {
		t.Errorf("event.Timestamp %v is outside [%v, %v]", ev.Timestamp, before, after)
	}
}

// T-CA4: mix of warnings and errors → still returns single event (collapse all).
func TestToCacheEvents_MixedSeverity_ReturnsSingleEvent(t *testing.T) {
	errs := []config.Error{
		{Severity: config.SeverityWarning, Field: "w1", Message: "warn"},
		{Severity: config.SeverityError, Field: "e1", Message: "err"},
		{Severity: config.SeverityWarning, Field: "w2", Message: "warn"},
	}

	result := config.ToCacheEvents(errs)
	if len(result) != 1 {
		t.Fatalf("expected 1 collapsed event, got %d: %v", len(result), result)
	}
	if result[0].Type != parser.ConfigError {
		t.Errorf("event.Type = %v, want parser.ConfigError", result[0].Type)
	}
}
