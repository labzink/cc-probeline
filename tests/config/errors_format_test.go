package config_test

// errors_format_test.go — tests for Error.Error() string formatting.
// Part of C1 gate: coverage for internal/config/ must reach >=90%.

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// T-EF1: full location — file:line:col: field: message.
func TestError_Error_FullLocation(t *testing.T) {
	e := config.Error{
		Severity: config.SeverityError,
		File:     "/home/user/.config/cc-probeline/config.toml",
		Line:     5,
		Column:   3,
		Field:    "thresholds.quota_5h_critical_ratio",
		Message:  "ratio must be in [0.0, 1.0]",
	}

	got := e.Error()
	want := "/home/user/.config/cc-probeline/config.toml:5:3: thresholds.quota_5h_critical_ratio: ratio must be in [0.0, 1.0]"
	if got != want {
		t.Errorf("Error.Error() = %q, want %q", got, want)
	}
}

// T-EF2: file + line but no column — file:line: field: message.
func TestError_Error_NoColumn(t *testing.T) {
	e := config.Error{
		Severity: config.SeverityError,
		File:     "/etc/cc-probeline/config.toml",
		Line:     12,
		Column:   0,
		Field:    "version",
		Message:  "unsupported version: 2",
	}

	got := e.Error()
	if !strings.HasPrefix(got, "/etc/cc-probeline/config.toml:12: ") {
		t.Errorf("Error.Error() %q: expected file:line prefix", got)
	}
	if strings.Contains(got, ":12:") && strings.Contains(got, ":12:0") {
		t.Errorf("Error.Error() %q: column 0 must be omitted", got)
	}
	if !strings.Contains(got, "unsupported version: 2") {
		t.Errorf("Error.Error() %q: missing message", got)
	}
}

// T-EF3: no file — field: message (omit location prefix entirely).
func TestError_Error_NoFile(t *testing.T) {
	e := config.Error{
		Severity: config.SeverityError,
		File:     "",
		Line:     0,
		Column:   0,
		Field:    "thresholds.ctx_warn_ratio",
		Message:  "ratio must be in [0.0, 1.0]",
	}

	got := e.Error()
	want := "thresholds.ctx_warn_ratio: ratio must be in [0.0, 1.0]"
	if got != want {
		t.Errorf("Error.Error() = %q, want %q", got, want)
	}
}

// T-EF4: no file and no field — only message.
func TestError_Error_NoFileNoField(t *testing.T) {
	e := config.Error{
		Severity: config.SeverityError,
		File:     "",
		Field:    "",
		Message:  "bare error message",
	}

	got := e.Error()
	if got != "bare error message" {
		t.Errorf("Error.Error() = %q, want %q", got, "bare error message")
	}
}

// T-EF5: file with line=0 — column should also be omitted; no colon suffix before field.
func TestError_Error_FileNoLine(t *testing.T) {
	e := config.Error{
		Severity: config.SeverityError,
		File:     "/tmp/cfg.toml",
		Line:     0,
		Column:   5, // column present but line is 0 → column ignored
		Field:    "version",
		Message:  "test",
	}

	got := e.Error()
	// With line=0, location prefix is just "file: " (no colon-numbers).
	if !strings.HasPrefix(got, "/tmp/cfg.toml: ") {
		t.Errorf("Error.Error() %q: with Line=0 expected prefix '/tmp/cfg.toml: '", got)
	}
	// Column must not appear.
	if strings.Contains(got, ":5") {
		t.Errorf("Error.Error() %q: column must be omitted when Line=0", got)
	}
}

// T-EF6: Severity.String() returns human-readable names.
func TestSeverity_String(t *testing.T) {
	cases := []struct {
		sev  config.Severity
		want string
	}{
		{config.SeverityError, "error"},
		{config.SeverityWarning, "warning"},
	}
	for _, tc := range cases {
		got := tc.sev.String()
		if got != tc.want {
			t.Errorf("Severity(%d).String() = %q, want %q", int(tc.sev), got, tc.want)
		}
	}
}
