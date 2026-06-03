// Package parser_test — RED tests for Phase 6.9 FIXES: F5 and F8.
//
// F5 (CRITICAL): GroupID does not advance through the real ParseLines→Aggregate pipeline
// because ParseLines drops all non-assistant records before Aggregate ever sees them.
// Root cause: jsonl.go:83 `if raw.Type != "assistant" { continue }` discards
// user-text prompt-boundary records, so isUserTextRecord in session.go is never
// triggered in production. GroupID stays 0 for all orchestrator turns.
//
// F8 (IMPORTANT): activationIdx logic in subagent.go:455-463 has a misleading
// comment ("activationIdx stays 0") that contradicts the actual code
// (`activationIdx = len(s.Turns)`). The behavior is correct; these tests are
// regression guards so the GREEN clarity refactor cannot introduce behavioral drift.
package parser_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// F5 helpers — f5-prefixed to avoid collision with existing helpers.
// ---------------------------------------------------------------------------

// f5AssistantLine builds one minimal assistant JSONL line with non-zero usage.
// contentJSON is the raw JSON for message.content (e.g. `[]`).
func f5AssistantLine(uuid, requestID, ts, contentJSON string) string {
	return fmt.Sprintf(
		`{"type":"assistant","uuid":%q,"requestId":%q,"timestamp":%q,`+
			`"isSidechain":false,"message":{`+
			`"model":"claude-opus-4-7-20250514",`+
			`"usage":{"input_tokens":100,"output_tokens":20,`+
			`"cache_read_input_tokens":0,"cache_creation_input_tokens":0},`+
			`"content":%s}}`,
		uuid, requestID, ts, contentJSON,
	)
}

// f5UserTextLine builds a user JSONL line whose message.content is an array with
// a text block. This matches the format used by Claude Code (see agent-sendmsg.jsonl).
// These records are the prompt boundaries that should advance GroupID via isUserTextRecord.
func f5UserTextLine(uuid, requestID, ts string) string {
	return fmt.Sprintf(
		`{"type":"user","uuid":%q,"requestId":%q,"timestamp":%q,`+
			`"isSidechain":false,"message":{"content":[{"type":"text","text":"Write a test suite"}]}}`,
		uuid, requestID, ts,
	)
}

// f5UserToolResultLine builds a user JSONL line whose message.content is an
// array with a tool_result block. This must NOT count as a prompt boundary.
func f5UserToolResultLine(uuid, requestID, ts string) string {
	return fmt.Sprintf(
		`{"type":"user","uuid":%q,"requestId":%q,"timestamp":%q,`+
			`"isSidechain":false,"message":{"content":[{"type":"tool_result","tool_use_id":"tu-1"}]}}`,
		uuid, requestID, ts,
	)
}

// ---------------------------------------------------------------------------
// F5-A — TestF5_Pipeline_GroupIDAdvancesThroughParseLines
//
// THE KEY TEST (anti seam-bypass, anti not-wired):
// Verifies the FULL ParseLines→Aggregate production pipeline produces correct
// GroupID values. Must FAIL today because ParseLines drops user records.
// ---------------------------------------------------------------------------

// TestF5_Pipeline_GroupIDAdvancesThroughParseLines verifies that orchestrator turns
// after separate user-text prompts receive distinct, incrementing GroupIDs when
// processed through the full production pipeline (ParseLines then Aggregate).
//
// Scenario:
//
//	line 1: user-text prompt #1  → boundary, GroupID should become 1
//	line 2: assistant A1         → GroupID=1
//	line 3: assistant A2         → GroupID=1 (same prompt)
//	line 4: user-text prompt #2  → boundary, GroupID should become 2
//	line 5: assistant B1         → GroupID=2
//	line 6: user tool_result     → NOT a boundary (tool_result content)
//	line 7: assistant B2         → GroupID=2 (tool_result did not advance)
//
// This FAILS today: ParseLines returns only assistant records → Aggregate
// never sees user-text records → all turns get GroupID=0.
func TestF5_Pipeline_GroupIDAdvancesThroughParseLines(t *testing.T) {
	const (
		tsUser1 = "2026-06-03T10:00:00.000000000Z"
		tsA1    = "2026-06-03T10:01:00.000000000Z"
		tsA2    = "2026-06-03T10:02:00.000000000Z"
		tsUser2 = "2026-06-03T10:03:00.000000000Z"
		tsB1    = "2026-06-03T10:04:00.000000000Z"
		tsTool  = "2026-06-03T10:05:00.000000000Z"
		tsB2    = "2026-06-03T10:06:00.000000000Z"
	)

	lines := []string{
		f5UserTextLine("f5a-user-1", "f5a-req-u1", tsUser1),
		f5AssistantLine("f5a-asst-a1", "f5a-req-a1", tsA1, `[]`),
		f5AssistantLine("f5a-asst-a2", "f5a-req-a2", tsA2,
			`[{"type":"tool_use","name":"Bash","input":{}}]`),
		f5UserTextLine("f5a-user-2", "f5a-req-u2", tsUser2),
		f5AssistantLine("f5a-asst-b1", "f5a-req-b1", tsB1, `[]`),
		f5UserToolResultLine("f5a-user-tr", "f5a-req-tr", tsTool),
		f5AssistantLine("f5a-asst-b2", "f5a-req-b2", tsB2,
			`[{"type":"tool_use","name":"Read","input":{}}]`),
	}

	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	// Production pipeline: ParseLines then Aggregate — not a shortcut.
	records, parseErrs, err := parser.ParseLines(&buf)
	if err != nil {
		t.Fatalf("ParseLines: unexpected I/O error: %v", err)
	}
	if len(parseErrs) != 0 {
		t.Errorf("ParseLines: unexpected parseErrors: %v", parseErrs)
	}

	s := parser.Aggregate(records)

	// Exactly 4 assistant turns must exist (user records do not become turns).
	if len(s.Turns) != 4 {
		t.Fatalf("len(s.Turns)=%d, want 4 (only assistant records become turns)", len(s.Turns))
	}

	// Turns ordered by timestamp: a1, a2, b1, b2.
	type wantTurn struct {
		uuid    string
		groupID int
	}
	want := []wantTurn{
		{"f5a-asst-a1", 1}, // after prompt #1 → GroupID=1
		{"f5a-asst-a2", 1}, // same prompt → GroupID=1
		{"f5a-asst-b1", 2}, // after prompt #2 → GroupID=2
		{"f5a-asst-b2", 2}, // tool_result did NOT advance counter → GroupID=2
	}

	for i, w := range want {
		turn := s.Turns[i]
		if turn.UUID != w.uuid {
			t.Errorf("Turns[%d].UUID=%q, want %q", i, turn.UUID, w.uuid)
		}
		if turn.GroupID != w.groupID {
			// Descriptive failure message: explains the root cause.
			t.Errorf("Turns[%d] (uuid=%q) GroupID=%d, want %d — "+
				"EXPECTED RED: ParseLines drops user records so GroupID stays 0 through real pipeline",
				i, turn.UUID, turn.GroupID, w.groupID)
		}
	}
}

// TestF5_Pipeline_ToolResultDoesNotAdvanceGroup verifies that a user record with
// tool_result content does NOT advance GroupID, even when user-text records are
// correctly passed through the pipeline.
//
// Scenario: user-text → assistant → user tool_result → assistant.
// Expected: both assistant turns have GroupID=1.
func TestF5_Pipeline_ToolResultDoesNotAdvanceGroup(t *testing.T) {
	const (
		tsUser1 = "2026-06-03T11:00:00.000000000Z"
		tsA1    = "2026-06-03T11:01:00.000000000Z"
		tsTool  = "2026-06-03T11:02:00.000000000Z"
		tsA2    = "2026-06-03T11:03:00.000000000Z"
	)

	lines := []string{
		f5UserTextLine("f5b-user-1", "f5b-req-u1", tsUser1),
		f5AssistantLine("f5b-asst-1", "f5b-req-a1", tsA1, `[]`),
		f5UserToolResultLine("f5b-tr", "f5b-req-tr", tsTool),
		f5AssistantLine("f5b-asst-2", "f5b-req-a2", tsA2, `[]`),
	}

	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	records, _, err := parser.ParseLines(&buf)
	if err != nil {
		t.Fatalf("ParseLines: unexpected I/O error: %v", err)
	}

	s := parser.Aggregate(records)
	if len(s.Turns) != 2 {
		t.Fatalf("len(s.Turns)=%d, want 2", len(s.Turns))
	}

	// Both assistant turns must share GroupID=1 — tool_result must not advance counter.
	for i, turn := range s.Turns {
		if turn.GroupID != 1 {
			t.Errorf("Turns[%d] (uuid=%q) GroupID=%d, want 1 — "+
				"EXPECTED RED: ParseLines drops user records so GroupID stays 0",
				i, turn.UUID, turn.GroupID)
		}
	}
}

// ---------------------------------------------------------------------------
// F8 — regression guards for activationIdx edge cases.
//
// These tests MUST PASS now (behavior is correct). They lock the semantics of
// the activation boundary computation so the GREEN clarity refactor in
// subagent.go:455-463 cannot introduce behavioral regression.
//
// Edge cases covered:
//   F8-A: single assistant turn strictly after boundary → CurrentTurnNum=1
//   F8-B: turn timestamp equals boundary (not strictly after) → not in new activation
//   F8-C: all turns before boundary → fallback to first turn, CurrentTurnNum=total
// ---------------------------------------------------------------------------

// f8WriteJSONL creates a temp sessionDir with a single agent-f8x.jsonl file
// containing the given JSONL lines. Returns sessionDir.
func f8WriteJSONL(t *testing.T, agentID string, lines []string) string {
	t.Helper()
	sessionDir := t.TempDir()
	subDir := sessionDir + "/subagents"
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatalf("f8WriteJSONL: mkdir %q: %v", subDir, err)
	}

	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	jsonlPath := subDir + "/agent-" + agentID + ".jsonl"
	if err := os.WriteFile(jsonlPath, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("f8WriteJSONL: write %q: %v", jsonlPath, err)
	}
	return sessionDir
}

// f8SidechainAssistantLine builds one assistant JSONL line with isSidechain=true.
// These are the records that aggregateSubagent processes.
func f8SidechainAssistantLine(uuid, requestID, ts string) string {
	return fmt.Sprintf(
		`{"type":"assistant","uuid":%q,"requestId":%q,"timestamp":%q,`+
			`"isSidechain":true,"message":{`+
			`"model":"claude-sonnet-4-6-20250929",`+
			`"usage":{"input_tokens":100,"output_tokens":20,`+
			`"cache_read_input_tokens":0,"cache_creation_input_tokens":0},`+
			`"content":[]}}`,
		uuid, requestID, ts,
	)
}

// f8SidechainUserTextLine builds a user JSONL line with isSidechain=true,
// representing a SendMessage boundary inside a subagent transcript.
// Content uses array format (matching agent-sendmsg.jsonl fixture) so that
// scanLastUserBoundary can successfully parse the timestamp.
func f8SidechainUserTextLine(uuid, ts string) string {
	return fmt.Sprintf(
		`{"type":"user","uuid":%q,"timestamp":%q,`+
			`"isSidechain":true,"message":{"content":[{"type":"text","text":"Continue with next subtask"}]}}`,
		uuid, ts,
	)
}

// TestF8_ActivationIdx_SingleTurnAfterBoundary verifies that when exactly one
// assistant turn exists strictly after the user-text boundary, ActivationStart
// equals that turn's timestamp and CurrentTurnNum=1.
//
// Regression guard: activationIdx must be set to the index of the single
// post-boundary turn, not reset to 0.
func TestF8_ActivationIdx_SingleTurnAfterBoundary(t *testing.T) {
	// Layout:
	//   assistant @ 08:00  — before boundary
	//   user-text @ 08:01  — boundary (scanLastUserBoundary returns 08:01)
	//   assistant @ 08:02  — single turn strictly after boundary
	//
	// activationIdx loop: Turns[0].Timestamp(08:00) not After(08:01) → activationIdx=1
	//                     Turns[1].Timestamp(08:02) After(08:01) → activationIdx=1, break
	// ActivationStart = Turns[1].Timestamp = 08:02, CurrentTurnNum = 2-1 = 1.
	sessionDir := f8WriteJSONL(t, "f8a", []string{
		f8SidechainAssistantLine("f8a-asst-1", "f8a-req-1", "2026-06-03T08:00:00.000000000Z"),
		f8SidechainUserTextLine("f8a-user-1", "2026-06-03T08:01:00.000000000Z"),
		f8SidechainAssistantLine("f8a-asst-2", "f8a-req-2", "2026-06-03T08:02:00.000000000Z"),
	})

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]

	wantActivation := time.Date(2026, 6, 3, 8, 2, 0, 0, time.UTC)
	if !agent.ActivationStart.Equal(wantActivation) {
		t.Errorf("ActivationStart=%v, want %v (single turn after boundary)",
			agent.ActivationStart, wantActivation)
	}
	if agent.CurrentTurnNum != 1 {
		t.Errorf("CurrentTurnNum=%d, want 1 (single turn in new activation)",
			agent.CurrentTurnNum)
	}
}

// TestF8_ActivationIdx_TurnTimestampEqualsBoundary verifies that a turn whose
// timestamp EQUALS the boundary timestamp is NOT counted as "after" the boundary.
// The code uses t.Timestamp.After(boundary) which is strictly greater than.
//
// Layout:
//
//	assistant @ 09:01  — timestamp equals boundary
//	user-text @ 09:01  — boundary
//	assistant @ 09:02  — strictly after boundary → new activation
//
// Result: activationIdx=1, ActivationStart=09:02, CurrentTurnNum=1.
func TestF8_ActivationIdx_TurnTimestampEqualsBoundary(t *testing.T) {
	sessionDir := f8WriteJSONL(t, "f8b", []string{
		f8SidechainAssistantLine("f8b-asst-1", "f8b-req-1", "2026-06-03T09:01:00.000000000Z"),
		f8SidechainUserTextLine("f8b-user-1", "2026-06-03T09:01:00.000000000Z"),
		f8SidechainAssistantLine("f8b-asst-2", "f8b-req-2", "2026-06-03T09:02:00.000000000Z"),
	})

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]

	// Turn at 09:01 == boundary → not strictly after → not in new activation.
	// Turn at 09:02 > boundary → activationIdx=1.
	wantActivation := time.Date(2026, 6, 3, 9, 2, 0, 0, time.UTC)
	if !agent.ActivationStart.Equal(wantActivation) {
		t.Errorf("ActivationStart=%v, want %v (equal-timestamp turn is NOT in new activation)",
			agent.ActivationStart, wantActivation)
	}
	if agent.CurrentTurnNum != 1 {
		t.Errorf("CurrentTurnNum=%d, want 1 (only the strictly-after turn counts)",
			agent.CurrentTurnNum)
	}
}

// ---------------------------------------------------------------------------
// F5-C — TestF5_Pipeline_StringContentBoundaryAdvancesGroup
//
// Coverage gap addressed: the existing F5 tests only exercise array-content user
// records (message.content as a JSON array). Real Claude Code JSONL frequently
// encodes user text prompts with message.content as a bare STRING, e.g.:
//
//   {"type":"user",...,"message":{"content":"Write a test suite"}}
//
// A spike on a real session showed ~4 of 5 user prompts used string content.
// With the current rawMessage struct (Content []rawContentBlock), a string value
// causes the WHOLE-LINE json.Unmarshal to fail → the line is dropped as a
// parseError → boundary never reaches Aggregate → GroupID stays 0 even after
// an array-content-only fix in ParseLines.
//
// The GREEN fix must make content-decoding tolerate BOTH forms (e.g. via
// json.RawMessage + a helper that treats a bare string as one text block) so
// that string-content user records are also recognised as boundaries.
// ---------------------------------------------------------------------------

// f5UserTextLineString builds a user JSONL line whose message.content is a bare
// JSON string (not an array). This is the dominant format in real Claude Code
// session JSONL (~4 of 5 user prompts observed in a real session spike).
//
// With the current code this line fails json.Unmarshal entirely because
// rawMessage.Content is typed as []rawContentBlock — a string value aborts the
// whole-record decode and the line is dropped as a parseError.
func f5UserTextLineString(uuid, requestID, ts string) string {
	return fmt.Sprintf(
		`{"type":"user","uuid":%q,"requestId":%q,"timestamp":%q,`+
			`"isSidechain":false,"message":{"content":"Write a test suite"}}`,
		uuid, requestID, ts,
	)
}

// TestF5_Pipeline_StringContentBoundaryAdvancesGroup verifies that user-text
// records with STRING-encoded message.content are recognised as prompt boundaries
// and advance GroupID through the full ParseLines→Aggregate production pipeline.
//
// This is RED today for TWO compounding reasons:
//  1. ParseLines discards all non-assistant records (the same root cause as
//     TestF5_Pipeline_GroupIDAdvancesThroughParseLines).
//  2. Even before that discard check, the string-content line fails
//     json.Unmarshal (rawMessage.Content is []rawContentBlock) → the line is
//     dropped as a parseError before the type-filter is even reached.
//
// The GREEN fix must address BOTH: (a) decode string content without aborting
// the whole-line unmarshal, and (b) emit the resulting user-text record as a
// boundary. An array-content-only fix leaves ~4/5 real-session prompts broken.
//
// NOTE: we do NOT assert len(parseErrs)==0 here because the current code
// produces a parseError for the string-content lines. The contract asserted is
// only on GroupID (must be 1/2 after the fix). The comment above describes what
// the GREEN fix must change about error handling.
func TestF5_Pipeline_StringContentBoundaryAdvancesGroup(t *testing.T) {
	const (
		tsUser1 = "2026-06-03T12:00:00.000000000Z"
		tsA1    = "2026-06-03T12:01:00.000000000Z"
		tsUser2 = "2026-06-03T12:02:00.000000000Z"
		tsB1    = "2026-06-03T12:03:00.000000000Z"
	)

	lines := []string{
		f5UserTextLineString("f5c-user-1", "f5c-req-u1", tsUser1), // string-content boundary #1
		f5AssistantLine("f5c-asst-a1", "f5c-req-a1", tsA1, `[]`),
		f5UserTextLineString("f5c-user-2", "f5c-req-u2", tsUser2), // string-content boundary #2
		f5AssistantLine("f5c-asst-b1", "f5c-req-b1", tsB1, `[]`),
	}

	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}

	// Full production pipeline — not a shortcut through Aggregate.
	records, parseErrs, err := parser.ParseLines(&buf)
	if err != nil {
		t.Fatalf("ParseLines: unexpected I/O error: %v", err)
	}

	// Document current (broken) behaviour without asserting len==0.
	// After the GREEN fix, parseErrs must be empty for string-content user lines.
	// Today they appear as json.Unmarshal failures before the type-filter is reached.
	if len(parseErrs) != 0 {
		t.Logf("ParseLines produced %d parseError(s) — expected today (string content "+
			"aborts whole-line unmarshal); GREEN fix must eliminate these: %v", len(parseErrs), parseErrs)
	}

	s := parser.Aggregate(records)

	// Two assistant turns must exist regardless of how many records ParseLines emits.
	if len(s.Turns) != 2 {
		t.Fatalf("len(s.Turns)=%d, want 2 (only assistant records become turns)", len(s.Turns))
	}

	// Contract: A1 after prompt #1 → GroupID=1; B1 after prompt #2 → GroupID=2.
	type wantTurn struct {
		uuid    string
		groupID int
	}
	want := []wantTurn{
		{"f5c-asst-a1", 1},
		{"f5c-asst-b1", 2},
	}

	for i, w := range want {
		turn := s.Turns[i]
		if turn.UUID != w.uuid {
			t.Errorf("Turns[%d].UUID=%q, want %q", i, turn.UUID, w.uuid)
		}
		if turn.GroupID != w.groupID {
			t.Errorf("Turns[%d] (uuid=%q) GroupID=%d, want %d — "+
				"EXPECTED RED: string-content user lines fail unmarshal AND ParseLines "+
				"discards non-assistant records; both must be fixed in GREEN",
				i, turn.UUID, turn.GroupID, w.groupID)
		}
	}
}

// TestF8_ActivationIdx_AllTurnsBeforeBoundary verifies the fallback: when all
// assistant turns are at or before the boundary timestamp (no turn strictly after),
// activationIdx is reset to 0, ActivationStart equals the first turn, and
// CurrentTurnNum equals the total number of turns.
//
// This exercises the guard at subagent.go:467 (`if activationIdx >= len(s.Turns)`).
// The misleading comment at :462 says "stays 0" but the code sets it to len(s.Turns);
// the guard then resets it to 0. Net effect: correct fallback, confusing code.
func TestF8_ActivationIdx_AllTurnsBeforeBoundary(t *testing.T) {
	// Layout:
	//   assistant @ 08:00  — before boundary
	//   assistant @ 08:01  — before boundary
	//   user-text @ 09:00  — boundary AFTER all turns
	//
	// Loop iteration 0: Turns[0] @ 08:00, not After(09:00) → activationIdx = len=2
	// Loop iteration 1: Turns[1] @ 08:01, not After(09:00) → activationIdx = len=2
	// Guard: activationIdx(2) >= len(2) → activationIdx = 0
	// ActivationStart = Turns[0].Timestamp = 08:00, CurrentTurnNum = 2-0 = 2.
	sessionDir := f8WriteJSONL(t, "f8c", []string{
		f8SidechainAssistantLine("f8c-asst-1", "f8c-req-1", "2026-06-03T08:00:00.000000000Z"),
		f8SidechainAssistantLine("f8c-asst-2", "f8c-req-2", "2026-06-03T08:01:00.000000000Z"),
		f8SidechainUserTextLine("f8c-user-1", "2026-06-03T09:00:00.000000000Z"),
	})

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]

	// Fallback: all turns before boundary → first turn is the anchor.
	wantActivation := time.Date(2026, 6, 3, 8, 0, 0, 0, time.UTC)
	if !agent.ActivationStart.Equal(wantActivation) {
		t.Errorf("ActivationStart=%v, want %v (fallback: all turns before boundary → first turn)",
			agent.ActivationStart, wantActivation)
	}
	if agent.CurrentTurnNum != 2 {
		t.Errorf("CurrentTurnNum=%d, want 2 (fallback: all turns counted as one activation)",
			agent.CurrentTurnNum)
	}
}
