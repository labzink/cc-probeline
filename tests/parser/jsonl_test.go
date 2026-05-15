// Package parser_test contains RED tests for internal/parser.
// Contract: plans/concepts/phase-3-step1-concept.md §2, §3, §4, §7.
// API: internal/parser.ParseLines(r io.Reader) ([]Record, []ParseError, error)
package parser_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fixtureReader returns an os.File for a fixture in tests/fixtures/jsonl/.
// Calls t.Fatal if the file is not found.
func fixtureReader(t *testing.T, name string) *os.File {
	t.Helper()
	// Path is relative to module root (tests/fixtures/jsonl/<name>).
	// go test runs from the package directory (tests/parser/),
	// so we go two levels up.
	path := filepath.Join("..", "fixtures", "jsonl", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("fixtureReader: cannot open %q: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// mustParseTime parses an RFC3339 string or panics.
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

// ---------------------------------------------------------------------------
// 1. Fixture-based tests (table-driven)
// ---------------------------------------------------------------------------

// TestParseLines_Small — happy path: 5 valid assistant records,
// no duplicates, all fields correctly parsed.
// see concept §7.1 fixture «small.jsonl».
func TestParseLines_Small(t *testing.T) {
	f := fixtureReader(t, "small.jsonl")

	records, parseErrs, err := parser.ParseLines(f)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(records))
	}

	// Records must be sorted by Timestamp ASC.
	// see concept §3 «Deterministic ordering».
	for i := 1; i < len(records); i++ {
		if records[i].Timestamp.Before(records[i-1].Timestamp) {
			t.Errorf("records not sorted ASC: record[%d].Timestamp %v < record[%d].Timestamp %v",
				i, records[i].Timestamp, i-1, records[i-1].Timestamp)
		}
	}

	// Check the first record in detail.
	r := records[0]
	if r.UUID != "uuid-001" {
		t.Errorf("UUID: want %q, got %q", "uuid-001", r.UUID)
	}
	if r.RequestID != "req-001" {
		t.Errorf("RequestID: want %q, got %q", "req-001", r.RequestID)
	}
	if r.ParentUUID != "parent-uuid-001" {
		t.Errorf("ParentUUID: want %q, got %q", "parent-uuid-001", r.ParentUUID)
	}
	wantTS := mustParseTime("2026-05-14T10:00:00Z")
	if !r.Timestamp.Equal(wantTS) {
		t.Errorf("Timestamp: want %v, got %v", wantTS, r.Timestamp)
	}
	if r.SessionID != "session-abc" {
		t.Errorf("SessionID: want %q, got %q", "session-abc", r.SessionID)
	}
	if r.CWD != "/home/user/project" {
		t.Errorf("CWD: want %q, got %q", "/home/user/project", r.CWD)
	}
	if r.GitBranch != "main" {
		t.Errorf("GitBranch: want %q, got %q", "main", r.GitBranch)
	}
	if r.Version != "1.2.3" {
		t.Errorf("Version: want %q, got %q", "1.2.3", r.Version)
	}
	if r.IsSidechain != false {
		t.Errorf("IsSidechain: want false, got %v", r.IsSidechain)
	}
	if r.UserType != "external" {
		t.Errorf("UserType: want %q, got %q", "external", r.UserType)
	}
	if r.Model != "claude-opus-4-7" {
		t.Errorf("Model: want %q, got %q", "claude-opus-4-7", r.Model)
	}
	if r.StopReason != "end_turn" {
		t.Errorf("StopReason: want %q, got %q", "end_turn", r.StopReason)
	}
	if r.ServiceTier != "standard" {
		t.Errorf("ServiceTier: want %q, got %q", "standard", r.ServiceTier)
	}

	// Usage fields. see concept §2.2 TokenCounts mapping.
	u := r.Usage
	if u.Input != 1000 {
		t.Errorf("Usage.Input: want 1000, got %d", u.Input)
	}
	if u.Output != 200 {
		t.Errorf("Usage.Output: want 200, got %d", u.Output)
	}
	if u.CacheRead != 500 {
		t.Errorf("Usage.CacheRead: want 500, got %d", u.CacheRead)
	}
	if u.CacheCreate != 300 {
		t.Errorf("Usage.CacheCreate: want 300, got %d", u.CacheCreate)
	}
	// Ephemeral splits. see concept §2.2 CacheCreate5m / CacheCreate1h.
	if u.CacheCreate5m != 200 {
		t.Errorf("Usage.CacheCreate5m: want 200, got %d", u.CacheCreate5m)
	}
	if u.CacheCreate1h != 100 {
		t.Errorf("Usage.CacheCreate1h: want 100, got %d", u.CacheCreate1h)
	}

	// ContentBlocks. see concept §2.2 ContentBlock.
	if len(r.Content) != 2 {
		t.Fatalf("Content len: want 2, got %d", len(r.Content))
	}
	if r.Content[0].Type != "text" {
		t.Errorf("Content[0].Type: want %q, got %q", "text", r.Content[0].Type)
	}
	if r.Content[1].Type != "tool_use" {
		t.Errorf("Content[1].Type: want %q, got %q", "tool_use", r.Content[1].Type)
	}
	if r.Content[1].ToolName != "Edit" {
		t.Errorf("Content[1].ToolName: want %q, got %q", "Edit", r.Content[1].ToolName)
	}
	// ToolInput must not be empty.
	if len(r.Content[1].ToolInput) == 0 {
		t.Error("Content[1].ToolInput: expected non-empty json.RawMessage")
	}
}

// TestParseLines_Small_SubagentRecord — the third record in small.jsonl is a subagent entry.
// see concept §7.2 «Subagent record (isSidechain: true)».
func TestParseLines_Small_SubagentRecord(t *testing.T) {
	f := fixtureReader(t, "small.jsonl")
	records, _, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) < 3 {
		t.Fatalf("need at least 3 records, got %d", len(records))
	}
	// Third record by timestamp (uuid-003, isSidechain=true)
	r := records[2]
	if !r.IsSidechain {
		t.Errorf("record[2].IsSidechain: want true, got false (uuid=%s)", r.UUID)
	}
	if r.UserType != "agent" {
		t.Errorf("record[2].UserType: want %q, got %q", "agent", r.UserType)
	}
	if r.Model != "claude-sonnet-4-6" {
		t.Errorf("record[2].Model: want %q, got %q", "claude-sonnet-4-6", r.Model)
	}
}

// TestParseLines_Small_ToolInputPreserved — ToolInput is preserved as a RawMessage
// and can be deserialized into map[string]any.
// see concept §7.2 «Tool input lazily preserved».
func TestParseLines_Small_ToolInputPreserved(t *testing.T) {
	f := fixtureReader(t, "small.jsonl")
	records, _, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// records[0] content[1] = tool_use Edit with input={file_path, new_string}
	if len(records) == 0 {
		t.Fatal("no records")
	}
	r := records[0]
	var editBlock *parser.ContentBlock
	for i := range r.Content {
		if r.Content[i].ToolName == "Edit" {
			editBlock = &r.Content[i]
			break
		}
	}
	if editBlock == nil {
		t.Fatal("Edit tool_use block not found in records[0]")
	}
	var input map[string]any
	if err := json.Unmarshal(editBlock.ToolInput, &input); err != nil {
		t.Errorf("ToolInput cannot be parsed as map: %v", err)
	}
	if _, ok := input["file_path"]; !ok {
		t.Error("ToolInput missing field 'file_path'")
	}
}

// TestParseLines_Empty — empty io.Reader → 0 records, 0 parseErrors, nil err.
// see concept §7.1 fixture «empty.jsonl».
func TestParseLines_Empty(t *testing.T) {
	f := fixtureReader(t, "empty.jsonl")
	records, parseErrs, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d", len(parseErrs))
	}
}

// TestParseLines_Malformed — 8 lines: 5 valid assistant records, 1 broken JSON
// (parseError on line 6), 1 without usage (silent skip on line 7), 1 system (skip on line 8).
// Result: 5 records, 1 parseError on line 6.
// see concept §7.1 fixture «malformed.jsonl».
func TestParseLines_Malformed(t *testing.T) {
	f := fixtureReader(t, "malformed.jsonl")
	records, parseErrs, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5 valid assistant records with usage, 1 no-usage (skip silently), 1 system (skip), 1 broken JSON
	if len(records) != 5 {
		t.Errorf("expected 5 records, got %d", len(records))
	}
	// Only 1 parseError — for the broken JSON on line 6.
	// Empty lines and no-usage are not reported (concept §4, table).
	if len(parseErrs) != 1 {
		t.Errorf("expected 1 parseError, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(parseErrs) > 0 {
		if parseErrs[0].LineNumber != 6 {
			t.Errorf("parseError.LineNumber: want 6, got %d", parseErrs[0].LineNumber)
		}
		if parseErrs[0].Reason == "" {
			t.Error("parseError.Reason: must not be empty")
		}
	}
}

// ---------------------------------------------------------------------------
// 2. strings.NewReader-based unit tests
// ---------------------------------------------------------------------------

// TestParseLines_EmptyReader — empty strings.NewReader.
func TestParseLines_EmptyReader(t *testing.T) {
	records, parseErrs, err := parser.ParseLines(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d", len(parseErrs))
	}
}

// TestParseLines_SystemTypeSkipped — a type=system record is skipped.
// see concept §2 «What is NOT included in Record».
func TestParseLines_SystemTypeSkipped(t *testing.T) {
	input := `{"type":"system","uuid":"uuid-sys","requestId":"req-sys","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}
{"type":"assistant","uuid":"uuid-ok","requestId":"req-ok","timestamp":"2026-05-14T10:01:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":200,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d", len(parseErrs))
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (system skipped), got %d", len(records))
	}
	if records[0].UUID != "uuid-ok" {
		t.Errorf("UUID: want %q, got %q", "uuid-ok", records[0].UUID)
	}
}

// TestParseLines_BrokenJSON — a single broken JSON line → parseError, no panic.
// see concept §4 malformed handling.
func TestParseLines_BrokenJSON(t *testing.T) {
	input := `{ broken json`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
	if len(parseErrs) != 1 {
		t.Fatalf("expected 1 parseError, got %d", len(parseErrs))
	}
	if parseErrs[0].LineNumber != 1 {
		t.Errorf("parseError.LineNumber: want 1, got %d", parseErrs[0].LineNumber)
	}
	if parseErrs[0].Reason == "" {
		t.Error("parseError.Reason: must not be empty")
	}
}

// TestParseLines_NoDeduKey — a record without requestId / uuid / message.id
// is included in records but does not participate in dedup, and produces a
// ParseError with reason="no dedup key".
// see concept §3 rule 4.
func TestParseLines_NoDeduKey(t *testing.T) {
	input := `{"type":"assistant","uuid":"","requestId":"","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (no dedup key still valid), got %d", len(records))
	}
	if len(parseErrs) != 1 {
		t.Errorf("expected 1 parseError (no dedup key), got %d: %v", len(parseErrs), parseErrs)
	}
	if len(parseErrs) > 0 && parseErrs[0].Reason != "no dedup key" {
		t.Errorf("parseError.Reason: want %q, got %q", "no dedup key", parseErrs[0].Reason)
	}
}

// TestParseLines_NoUsageSkipped — a record without usage is silently skipped (no parseError).
// see concept §2 «What is NOT included in Record».
func TestParseLines_NoUsageSkipped(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-no-usage","requestId":"req-no-usage","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records (no-usage skipped), got %d", len(records))
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors (no-usage is silent skip), got %d: %v", len(parseErrs), parseErrs)
	}
}

// TestParseLines_TimestampParsed — timestamp is parsed from ISO 8601 into time.Time UTC.
// see concept §7.1 «Timestamp parsed from ISO 8601».
func TestParseLines_TimestampParsed(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-ts","requestId":"req-ts","timestamp":"2026-05-14T15:30:42Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, _, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	want := mustParseTime("2026-05-14T15:30:42Z")
	if !records[0].Timestamp.Equal(want) {
		t.Errorf("Timestamp: want %v, got %v", want, records[0].Timestamp)
	}
	if records[0].Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp must be UTC, got %v", records[0].Timestamp.Location())
	}
}

// TestParseLines_MissingTimestamp — a record without timestamp is accepted (zero Time)
// and reported in parseErrors with reason="missing timestamp".
// see concept §4 malformed handling table.
func TestParseLines_MissingTimestamp(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-nots","requestId":"req-nots","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Record is accepted with zero Time.
	if len(records) != 1 {
		t.Errorf("expected 1 record (missing ts accepted), got %d", len(records))
	}
	// And is reported in parseErrors.
	if len(parseErrs) != 1 {
		t.Errorf("expected 1 parseError for missing timestamp, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(parseErrs) > 0 && parseErrs[0].Reason != "missing timestamp" {
		t.Errorf("parseError.Reason: want %q, got %q", "missing timestamp", parseErrs[0].Reason)
	}
}

// TestParseLines_AllFields — mapping of all Record fields, including
// sessionId, cwd, gitBranch, version, isSidechain, userType,
// CacheCreate5m, CacheCreate1h, serviceTier, stopReason.
// see concept §2.2 «Types» and plan §«Test-writer brief».
func TestParseLines_AllFields(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-full","requestId":"req-full","parentUuid":"parent-full","timestamp":"2026-05-14T12:00:00Z","sessionId":"sess-full","cwd":"/full/path","gitBranch":"feature/all-fields","version":"2.0.0","isSidechain":true,"userType":"agent","message":{"model":"claude-sonnet-4-6","stop_reason":"max_tokens","service_tier":"priority","usage":{"input_tokens":111,"output_tokens":222,"cache_read_input_tokens":333,"cache_creation_input_tokens":444,"cache_creation":{"ephemeral_5m_input_tokens":300,"ephemeral_1h_input_tokens":144}},"content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}},{"type":"tool_result","tool_use_id":"tid-1"},{"type":"thinking"}]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("unexpected parseErrors: %v", parseErrs)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"UUID", r.UUID, "uuid-full"},
		{"RequestID", r.RequestID, "req-full"},
		{"ParentUUID", r.ParentUUID, "parent-full"},
		{"SessionID", r.SessionID, "sess-full"},
		{"CWD", r.CWD, "/full/path"},
		{"GitBranch", r.GitBranch, "feature/all-fields"},
		{"Version", r.Version, "2.0.0"},
		{"IsSidechain", r.IsSidechain, true},
		{"UserType", r.UserType, "agent"},
		{"Model", r.Model, "claude-sonnet-4-6"},
		{"StopReason", r.StopReason, "max_tokens"},
		{"ServiceTier", r.ServiceTier, "priority"},
		{"Usage.Input", r.Usage.Input, 111},
		{"Usage.Output", r.Usage.Output, 222},
		{"Usage.CacheRead", r.Usage.CacheRead, 333},
		{"Usage.CacheCreate", r.Usage.CacheCreate, 444},
		{"Usage.CacheCreate5m", r.Usage.CacheCreate5m, 300},
		{"Usage.CacheCreate1h", r.Usage.CacheCreate1h, 144},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, c.got)
		}
	}

	// Content blocks.
	if len(r.Content) != 3 {
		t.Fatalf("Content len: want 3, got %d", len(r.Content))
	}
	if r.Content[0].Type != "tool_use" {
		t.Errorf("Content[0].Type: want %q, got %q", "tool_use", r.Content[0].Type)
	}
	if r.Content[0].ToolName != "Bash" {
		t.Errorf("Content[0].ToolName: want %q, got %q", "Bash", r.Content[0].ToolName)
	}
	if len(r.Content[0].ToolInput) == 0 {
		t.Error("Content[0].ToolInput: expected non-empty RawMessage")
	}
	if r.Content[1].Type != "tool_result" {
		t.Errorf("Content[1].Type: want %q, got %q", "tool_result", r.Content[1].Type)
	}
	if r.Content[2].Type != "thinking" {
		t.Errorf("Content[2].Type: want %q, got %q", "thinking", r.Content[2].Type)
	}
}

// TestParseLines_CacheCreateMismatch — mismatch between cache_creation_input_tokens and
// the sum of ephemeral_5m + ephemeral_1h → record accepted with CacheCreate=aggregate,
// parseErrors contains reason="cache_create_mismatch".
// see concept §4 malformed handling table.
func TestParseLines_CacheCreateMismatch(t *testing.T) {
	// CacheCreate=1000, 5m=600, 1h=300 → sum 900 != 1000 → mismatch
	input := `{"type":"assistant","uuid":"uuid-mismatch","requestId":"req-mismatch","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":600,"ephemeral_1h_input_tokens":300}},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (mismatch → accepted), got %d", len(records))
	}
	// Use the aggregate value.
	if records[0].Usage.CacheCreate != 1000 {
		t.Errorf("Usage.CacheCreate: want 1000 (aggregate), got %d", records[0].Usage.CacheCreate)
	}
	// A parseError must be present.
	if len(parseErrs) != 1 {
		t.Errorf("expected 1 parseError for mismatch, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(parseErrs) > 0 && parseErrs[0].Reason != "cache_create_mismatch" {
		t.Errorf("parseError.Reason: want %q, got %q", "cache_create_mismatch", parseErrs[0].Reason)
	}
}

// TestParseLines_ReverseTimestampOrder — records in the file are in reverse timestamp order.
// After parsing, results must be sorted ASC by Timestamp.
// see concept §3 «Deterministic ordering».
func TestParseLines_ReverseTimestampOrder(t *testing.T) {
	// Record with ts=10:05 appears first in the file, 10:00 appears second.
	input := `{"type":"assistant","uuid":"uuid-late","requestId":"req-late","timestamp":"2026-05-14T10:05:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}
{"type":"assistant","uuid":"uuid-early","requestId":"req-early","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, _, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].UUID != "uuid-early" {
		t.Errorf("records[0] should be uuid-early (earlier timestamp), got %q", records[0].UUID)
	}
	if records[1].UUID != "uuid-late" {
		t.Errorf("records[1] should be uuid-late, got %q", records[1].UUID)
	}
}

// TestParseLines_FallbackUUID — a record with empty requestId uses uuid as the dedup key.
// Duplicate by uuid → the earliest by timestamp wins.
// see concept §3 dedup policy «Fallback 1 — UUID».
func TestParseLines_FallbackUUID(t *testing.T) {
	// Two records sharing the same uuid, requestId is empty. The one at ts=10:00 must win.
	input := `{"type":"assistant","uuid":"uuid-dup","requestId":"","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}
{"type":"assistant","uuid":"uuid-dup","requestId":"","timestamp":"2026-05-14T10:01:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":999,"output_tokens":99,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Duplicates are not reported in parseErrors (concept §3: «Dedup is not reported»).
	_ = parseErrs
	if len(records) != 1 {
		t.Fatalf("expected 1 record (dedup by UUID), got %d", len(records))
	}
	// Winner is the earliest by timestamp (input=100, not 999).
	if records[0].Usage.Input != 100 {
		t.Errorf("expected winner to have Usage.Input=100, got %d", records[0].Usage.Input)
	}
}

// TestParseLines_MultipleErrors — multiple broken JSON lines produce multiple parseErrors.
func TestParseLines_MultipleErrors(t *testing.T) {
	input := `{ bad line 1
{ bad line 2
{"type":"assistant","uuid":"uuid-ok2","requestId":"req-ok2","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}
{ bad line 4`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 valid record, got %d", len(records))
	}
	if len(parseErrs) != 3 {
		t.Errorf("expected 3 parseErrors, got %d: %v", len(parseErrs), parseErrs)
	}
	// Verify lineNumbers: 1, 2, 4.
	if len(parseErrs) == 3 {
		wantLines := []int{1, 2, 4}
		for i, pe := range parseErrs {
			if pe.LineNumber != wantLines[i] {
				t.Errorf("parseErrs[%d].LineNumber: want %d, got %d", i, wantLines[i], pe.LineNumber)
			}
		}
	}
}

// TestParseLines_EmptyLinesSkipped — empty lines in the middle of the file are ignored
// and do not generate parseErrors.
// see concept §4 «Empty line → Skip silently».
func TestParseLines_EmptyLinesSkipped(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-e1","requestId":"req-e1","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}

{"type":"assistant","uuid":"uuid-e2","requestId":"req-e2","timestamp":"2026-05-14T10:01:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors (empty lines silent), got %d: %v", len(parseErrs), parseErrs)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

// TestParseLines_CacheCreateNoSplit — a record with cache_creation_input_tokens > 0
// but no cache_creation nested object must NOT produce a cache_create_mismatch parseError.
// Regression for C1 fix: guard fires only when the breakdown is present.
// see concept §2.2 TokenCounts.
func TestParseLines_CacheCreateNoSplit(t *testing.T) {
	// cache_creation_input_tokens=300, no cache_creation object (both ephemeral fields absent)
	input := `{"type":"assistant","uuid":"uuid-nosplit","requestId":"req-nosplit","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":0,"cache_creation_input_tokens":300},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	for _, pe := range parseErrs {
		if pe.Reason == "cache_create_mismatch" {
			t.Errorf("unexpected cache_create_mismatch parseError when split is absent: line %d", pe.LineNumber)
		}
	}
	if records[0].Usage.CacheCreate != 300 {
		t.Errorf("Usage.CacheCreate: want 300, got %d", records[0].Usage.CacheCreate)
	}
}

// TestParseLines_RequestIDSnakeCase — a record using snake_case request_id instead of
// camelCase requestId must populate Record.RequestID correctly.
// Regression for C2 fix.
// see specs.md §28.
func TestParseLines_RequestIDSnakeCase(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-snake","request_id":"req-snake-123","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].RequestID != "req-snake-123" {
		t.Errorf("RequestID: want %q, got %q", "req-snake-123", records[0].RequestID)
	}
}

// TestParseLines_UserTypeFiltered — a record with type="user" carrying a usage block
// must be filtered out (no Record produced, no parseError).
// Regression for C3 fix: allow-list "assistant", not deny-list "system".
// see concept §2 «What is NOT included in Record».
func TestParseLines_UserTypeFiltered(t *testing.T) {
	input := `{"type":"user","uuid":"uuid-user","requestId":"req-user","timestamp":"2026-05-14T10:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":50,"output_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}
{"type":"assistant","uuid":"uuid-asst","requestId":"req-asst","timestamp":"2026-05-14T10:01:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (user type filtered), got %d", len(records))
	}
	if records[0].UUID != "uuid-asst" {
		t.Errorf("UUID: want %q, got %q", "uuid-asst", records[0].UUID)
	}
}

// TestParseLines_MessageID — a record with message.id is parsed into Record.MessageID.
// see concept §3 rule 3 (message.id fallback field; dedup use deferred to Phase 4).
func TestParseLines_MessageID(t *testing.T) {
	input := `{"type":"assistant","uuid":"uuid-mid","requestId":"req-mid","timestamp":"2026-05-14T10:00:00Z","message":{"id":"msg_123","model":"claude-opus-4-7","usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},"content":[]}}`

	records, parseErrs, err := parser.ParseLines(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d: %v", len(parseErrs), parseErrs)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].MessageID != "msg_123" {
		t.Errorf("MessageID: want %q, got %q", "msg_123", records[0].MessageID)
	}
}

// TestParseLines_WithRetryFixture — fixture with-retry.jsonl.
// 5 records, 2 of which are duplicates (req-B appears twice).
// Result: 4 unique. Winner for req-B is the earliest by timestamp (uuid-B-first).
// see concept §7.1 fixture «with-retry.jsonl».
func TestParseLines_WithRetryFixture(t *testing.T) {
	f := fixtureReader(t, "with-retry.jsonl")
	records, parseErrs, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Duplicates are not reported.
	if len(parseErrs) != 0 {
		t.Errorf("expected 0 parseErrors, got %d: %v", len(parseErrs), parseErrs)
	}
	// 5 records - 1 duplicate = 4 unique.
	if len(records) != 4 {
		t.Errorf("expected 4 records (1 deduped), got %d", len(records))
	}

	// Winner for req-B is uuid-B-first (ts=10:01), not uuid-B-retry (ts=10:01:30).
	// Winner's Usage.Input must be 200, not 999.
	var found bool
	for _, r := range records {
		if r.RequestID == "req-B" {
			found = true
			if r.UUID != "uuid-B-first" {
				t.Errorf("dedup winner: want uuid-B-first, got %q", r.UUID)
			}
			if r.Usage.Input != 200 {
				t.Errorf("dedup winner Usage.Input: want 200, got %d", r.Usage.Input)
			}
		}
	}
	if !found {
		t.Error("req-B not found in results at all")
	}
}
