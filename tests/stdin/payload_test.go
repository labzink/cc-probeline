// Package stdin_test contains black-box tests for internal/stdin.Decode.
// Tests cover happy-path decoding, unknown field handling (warn vs error),
// and malformed JSON rejection.
package stdin_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/stdin"
	"github.com/labzink/cc-probeline/tests/testutil"
)

// TestDecode_Empty verifies that an empty JSON object decodes to a zero-value
// Payload without error.
func TestDecode_Empty(t *testing.T) {
	r := bytes.NewBufferString(`{}`)
	got, err := stdin.Decode(r, false)
	if err != nil {
		t.Fatalf("Decode empty: unexpected error: %v", err)
	}
	// Zero-value checks for the most critical fields.
	if got.Model.ID != "" {
		t.Errorf("Model.ID: want %q, got %q", "", got.Model.ID)
	}
	if got.Effort.Level != "" {
		t.Errorf("Effort.Level: want %q, got %q", "", got.Effort.Level)
	}
	if len(got.Tasks) != 0 {
		t.Errorf("Tasks: want empty, got %d elements", len(got.Tasks))
	}
}

// TestDecode_Full verifies that a typical CC hookData payload is decoded
// correctly into all Payload fields.
func TestDecode_Full(t *testing.T) {
	const raw = `{
		"model":          {"id": "claude-opus-4-7-20250805", "display_name": "Claude Opus 4.7"},
		"effort":         {"level": "high"},
		"session_id":     "sess-abc123",
		"transcript_path":"/home/user/.claude/projects/-home-user-foo/sess-abc123.jsonl",
		"cwd":            "/home/user/foo",
		"context_window": {"context_window_size": 200000, "current_usage": {"input_tokens": 128000}},
		"cost":           {"total_cost_usd": 13.79, "total_api_duration_ms": 2998000}
	}`

	got, err := stdin.Decode(strings.NewReader(raw), false)
	if err != nil {
		t.Fatalf("Decode full: unexpected error: %v", err)
	}

	if got.Model.ID != "claude-opus-4-7-20250805" {
		t.Errorf("Model.ID: want %q, got %q", "claude-opus-4-7-20250805", got.Model.ID)
	}
	if got.Model.Name != "Claude Opus 4.7" {
		t.Errorf("Model.Name: want %q, got %q", "Claude Opus 4.7", got.Model.Name)
	}
	if got.Effort.Level != "high" {
		t.Errorf("Effort.Level: want %q, got %q", "high", got.Effort.Level)
	}
	if got.SessionID != "sess-abc123" {
		t.Errorf("SessionID: want %q, got %q", "sess-abc123", got.SessionID)
	}
	if got.Cwd != "/home/user/foo" {
		t.Errorf("Cwd: want %q, got %q", "/home/user/foo", got.Cwd)
	}
	if got.ContextWindow.Size != 200000 {
		t.Errorf("ContextWindow.Size: want %d, got %d", 200000, got.ContextWindow.Size)
	}
	if got.Cost.TotalCostUSD != 13.79 {
		t.Errorf("Cost.TotalCostUSD: want %v, got %v", 13.79, got.Cost.TotalCostUSD)
	}
	if got.Cost.TotalAPIDurationMS != 2998000 {
		t.Errorf("Cost.TotalAPIDurationMS: want %d, got %d", int64(2998000), got.Cost.TotalAPIDurationMS)
	}
}

// TestDecode_WithTasks verifies that tasks[] in stdin JSON are decoded into
// the Payload.Tasks slice.
func TestDecode_WithTasks(t *testing.T) {
	const raw = `{
		"tasks": [
			{"id": "agentXYZ", "name": "code-reviewer", "type": "subagent",
			 "status": "running", "description": "reviewing PR"},
			{"id": "agentABC", "name": "feature-dev",   "type": "subagent",
			 "status": "completed"}
		]
	}`

	got, err := stdin.Decode(strings.NewReader(raw), false)
	if err != nil {
		t.Fatalf("Decode with tasks: unexpected error: %v", err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("Tasks: want 2 elements, got %d", len(got.Tasks))
	}

	if got.Tasks[0].ID != "agentXYZ" {
		t.Errorf("Tasks[0].ID: want %q, got %q", "agentXYZ", got.Tasks[0].ID)
	}
	if got.Tasks[0].Name != "code-reviewer" {
		t.Errorf("Tasks[0].Name: want %q, got %q", "code-reviewer", got.Tasks[0].Name)
	}
	if got.Tasks[0].Status != "running" {
		t.Errorf("Tasks[0].Status: want %q, got %q", "running", got.Tasks[0].Status)
	}
	if got.Tasks[1].ID != "agentABC" {
		t.Errorf("Tasks[1].ID: want %q, got %q", "agentABC", got.Tasks[1].ID)
	}
}

// TestDecode_UnknownField verifies that an unknown top-level field in non-strict
// mode results in a slog.Warn (not an error). The Payload must still decode
// successfully.
func TestDecode_UnknownField(t *testing.T) {
	const raw = `{"unknown_top_level": "x", "model": {"id": "claude-opus-4-7"}}`

	h := testutil.NewCaptureHandler()
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	got, err := stdin.Decode(strings.NewReader(raw), false)
	if err != nil {
		t.Fatalf("Decode unknown field (non-strict): unexpected error: %v", err)
	}
	if got.Model.ID != "claude-opus-4-7" {
		t.Errorf("Model.ID: want %q, got %q", "claude-opus-4-7", got.Model.ID)
	}
	if !h.HasWarnContaining("stdin.payload: unknown field") {
		t.Error("expected slog.Warn with message containing \"stdin.payload: unknown field\", got none")
	}
}

// TestDecode_StrictUnknownField verifies that in strict mode an unknown field
// returns a non-nil error immediately.
func TestDecode_StrictUnknownField(t *testing.T) {
	const raw = `{"unknown_top_level": "x"}`

	_, err := stdin.Decode(strings.NewReader(raw), true)
	if err == nil {
		t.Fatal("Decode strict unknown field: want error, got nil")
	}
	// Error message must reference the unknown field.
	if !strings.Contains(err.Error(), "stdin.payload") {
		t.Errorf("error %q: want message to contain \"stdin.payload\"", err.Error())
	}
}

// TestDecode_SecondPassUnmarshalError verifies that a valid top-level JSON object
// with a known key but wrong value type triggers an error on the second-pass
// unmarshal into the typed Payload struct.
//
// Example: {"cost": "notanobject"} passes the first-pass map decode (the value
// is valid JSON), but fails the second pass because cost expects an object.
// The test ensures an error is returned (payload.go:119).
func TestDecode_SecondPassUnmarshalError(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"cost_string", `{"cost": "notanobject"}`},
		{"context_window_string", `{"context_window": "string"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := stdin.Decode(strings.NewReader(tc.in), false)
			if err == nil {
				t.Fatalf("Decode(%q): want error for wrong type, got nil", tc.in)
			}
		})
	}
}

// TestDecode_MalformedJSON verifies that invalid JSON produces an error that
// wraps a json.SyntaxError.
func TestDecode_MalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"truncated", `{"model":`},
		{"bad_value", `{"model": ???}`},
		{"empty_input", ``},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := stdin.Decode(strings.NewReader(tc.in), false)
			if err == nil {
				t.Fatalf("Decode(%q): want error, got nil", tc.in)
			}
			var synErr *json.SyntaxError
			var unmarshalErr *json.UnmarshalTypeError
			if !errors.As(err, &synErr) && !errors.As(err, &unmarshalErr) {
				// Accept either: SyntaxError (most cases) or an opaque wrap
				// that still carries the decode failure prefix.
				if !strings.Contains(err.Error(), "stdin.payload: decode failed") {
					t.Errorf("Decode(%q): error %q: want json.SyntaxError or decode-failed prefix", tc.in, err.Error())
				}
			}
		})
	}
}
