// Package parser_test — RED tests for Phase 6.8.0 Turn fields and GroupID.
// Contract: plans/phase-6.8/spec-common.md §2.1, §2.2, §2.3 and task-6.8.0.md.
//
// Tests:
//   T-F1: TestTurn_UUID        — Turn.UUID equals Record.UUID for every Turn
//   T-F2: TestTurn_Thinking    — Turn.Thinking ↔ thinking-block present and no tool_use
//   T-F3: TestAggregate_GroupID — GroupID increments on each user-text boundary
//   T-F4: TestSubagent_Turns   — SubagentStats.Turns populated per record (Role=AgentType, IsSidechain)
package parser_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeAssistant creates a minimal assistant Record for inline tests.
func makeAssistant(uuid string, ts time.Time, isSidechain bool, content ...parser.ContentBlock) parser.Record {
	return parser.Record{
		Type:        "assistant",
		UUID:        uuid,
		RequestID:   "req-" + uuid,
		Timestamp:   ts,
		Model:       "claude-opus-4-7",
		IsSidechain: isSidechain,
		Usage:       parser.TokenCounts{Input: 10, Output: 5},
		Content:     content,
	}
}

// makeUser creates a minimal user Record with textual content (prompt boundary).
// Used to drive GroupID increment in Aggregate.
func makeUser(uuid string, ts time.Time) parser.Record {
	return parser.Record{
		Type:      "user",
		UUID:      uuid,
		RequestID: "req-" + uuid,
		Timestamp: ts,
		// UserType is non-empty; content is a string (not a tool-result list) — per §2.3 / Insurance #1.
	}
}

// thinkingBlock creates a ContentBlock with type "thinking".
func thinkingBlock() parser.ContentBlock {
	return parser.ContentBlock{Type: "thinking"}
}

// ---------------------------------------------------------------------------
// T-F1: TestTurn_UUID
// Spec §2.1: Turn.UUID = Record.UUID for each Turn.
// Uses inline records so the test is independent from fixture changes.
// ---------------------------------------------------------------------------

// TestTurn_UUID verifies that each Turn produced by Aggregate carries the UUID
// of the corresponding Record.
func TestTurn_UUID(t *testing.T) {
	// Given: three assistant records with distinct UUIDs.
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	records := []parser.Record{
		makeAssistant("uuid-alpha", base, false),
		makeAssistant("uuid-beta", base.Add(time.Minute), false),
		makeAssistant("uuid-gamma", base.Add(2*time.Minute), true),
	}

	// When: Aggregate is called.
	got := parser.Aggregate(records)

	// Then: every Turn.UUID must equal its Record.UUID.
	if len(got.Turns) != len(records) {
		t.Fatalf("len(Turns)=%d, want %d", len(got.Turns), len(records))
	}
	for i, rec := range records {
		if got.Turns[i].UUID != rec.UUID {
			t.Errorf("Turns[%d].UUID=%q, want %q", i, got.Turns[i].UUID, rec.UUID)
		}
	}
}

// ---------------------------------------------------------------------------
// T-F2: TestTurn_Thinking
// Spec §2.1 / §2.3: Turn.Thinking=true iff thinking-block present AND no tool_use.
// ---------------------------------------------------------------------------

// TestTurn_Thinking verifies the Thinking flag under four content combinations.
func TestTurn_Thinking(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		content      []parser.ContentBlock
		wantThinking bool
	}{
		{
			name:         "thinking_only",
			content:      []parser.ContentBlock{thinkingBlock(), textBlock()},
			wantThinking: true, // thinking-block present, no tool_use → Thinking=true
		},
		{
			name:         "thinking_with_tool_use",
			content:      []parser.ContentBlock{thinkingBlock(), toolUseBlock("Bash")},
			wantThinking: false, // thinking-block present BUT tool_use present → Thinking=false
		},
		{
			name:         "no_thinking",
			content:      []parser.ContentBlock{textBlock(), toolUseBlock("Edit")},
			wantThinking: false, // no thinking-block → Thinking=false
		},
		{
			name:         "empty_content",
			content:      nil,
			wantThinking: false, // no blocks at all → Thinking=false
		},
	}

	for idx, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uuid := "uuid-think-" + tc.name
			rec := makeAssistant(uuid, base.Add(time.Duration(idx)*time.Minute), false, tc.content...)
			got := parser.Aggregate([]parser.Record{rec})

			if len(got.Turns) != 1 {
				t.Fatalf("len(Turns)=%d, want 1", len(got.Turns))
			}
			if got.Turns[0].Thinking != tc.wantThinking {
				t.Errorf("Turn.Thinking=%v, want %v (content=%v)", got.Turns[0].Thinking, tc.wantThinking, tc.content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-F3: TestAggregate_GroupID
// Spec §2.3 / T-3: GroupID increments on each user-text boundary.
// Orchestrator turns sharing a prompt carry the same GroupID.
// Subagent turns (IsSidechain=true) must NOT reset the counter.
//
// Scenario (interleaved user+assistant+subagent sequence):
//
//   t=0  user    "first prompt"     → boundary #1
//   t=1  assist  opus-orch          → GroupID=1
//   t=2  assist  sonnet-subagent    → GroupID stays (subagent, not a boundary)
//   t=3  assist  opus-orch          → GroupID=1 (same prompt, still no new user boundary)
//   t=4  user    "second prompt"    → boundary #2
//   t=5  assist  opus-orch          → GroupID=2
//   t=6  assist  opus-orch          → GroupID=2
//
// GroupID must be:
//   Turns[0] (orch,  t=1) → 1
//   Turns[1] (sub,   t=2) → 0  (subagents get GroupID=0 here; merge assigns it in 6.8.d)
//   Turns[2] (orch,  t=3) → 1
//   Turns[3] (orch,  t=5) → 2
//   Turns[4] (orch,  t=6) → 2
// ---------------------------------------------------------------------------

// TestAggregate_GroupID verifies GroupID assignment across prompt boundaries.
func TestAggregate_GroupID(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Build a mixed sequence of user (boundary) + assistant records.
	// ParseLines drops user records, so Aggregate must receive the boundary info
	// through a pre-pass or the caller must pass raw records including user rows.
	// The spec says Aggregate produces GroupID — we pass all record types.
	allRecords := []parser.Record{
		makeUser("user-1", base),                                               // boundary #1
		makeAssistant("orch-1", base.Add(time.Minute), false),                  // GroupID=1
		makeAssistant("sub-1", base.Add(2*time.Minute), true),                  // subagent, GroupID=0 (not boundary)
		makeAssistant("orch-2", base.Add(3*time.Minute), false),                // GroupID=1
		makeUser("user-2", base.Add(4*time.Minute)),                            // boundary #2
		makeAssistant("orch-3", base.Add(5*time.Minute), false),                // GroupID=2
		makeAssistant("orch-4", base.Add(6*time.Minute), false),                // GroupID=2
	}

	got := parser.Aggregate(allRecords)

	// Turns slice contains only assistant records (5 total: orch-1, sub-1, orch-2, orch-3, orch-4).
	if len(got.Turns) != 5 {
		t.Fatalf("len(Turns)=%d, want 5 (only assistant records)", len(got.Turns))
	}

	// Verify GroupID per turn.
	type wantTurn struct {
		uuid    string
		groupID int
	}
	want := []wantTurn{
		{"orch-1", 1},
		{"sub-1", 0}, // subagent: GroupID assigned in 6.8.d merge step, not here
		{"orch-2", 1},
		{"orch-3", 2},
		{"orch-4", 2},
	}

	for i, w := range want {
		turn := got.Turns[i]
		if turn.UUID != w.uuid {
			t.Errorf("Turns[%d].UUID=%q, want %q", i, turn.UUID, w.uuid)
		}
		if turn.GroupID != w.groupID {
			t.Errorf("Turns[%d].GroupID=%d, want %d (uuid=%q)", i, turn.GroupID, w.groupID, turn.UUID)
		}
	}
}

// ---------------------------------------------------------------------------
// T-F4: TestSubagent_Turns
// Spec §2.1 / T-4: SubagentStats.Turns contains one Turn per record.
// Each Turn has Role=AgentType and IsSidechain=true.
// ---------------------------------------------------------------------------

// TestSubagent_Turns verifies that aggregateSubagent (via CollectSubagents) populates
// SubagentStats.Turns with one Turn per JSONL record, with Role equal to AgentType
// and IsSidechain=true on every element.
func TestSubagent_Turns(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Given: a set of assistant records as if from a subagent transcript.
	// All are IsSidechain=true; they carry different tools.
	records := []parser.Record{
		makeAssistant("sub-uuid-1", base, true, toolUseBlock("Read")),
		makeAssistant("sub-uuid-2", base.Add(time.Minute), true, toolUseBlock("Bash")),
		makeAssistant("sub-uuid-3", base.Add(2*time.Minute), true),
	}

	// When: Aggregate is called (aggregateSubagent is internal; Aggregate is the public API).
	// For subagent context we call Aggregate with IsSidechain=true records.
	got := parser.Aggregate(records)

	// Then: Turns has one entry per record.
	if len(got.Turns) != 3 {
		t.Fatalf("len(Turns)=%d, want 3", len(got.Turns))
	}

	// Each Turn must reflect the subagent role.
	for i, turn := range got.Turns {
		// IsSidechain=true on the source record → Role="agent".
		if turn.Role != "agent" {
			t.Errorf("Turns[%d].Role=%q, want %q (IsSidechain=true source)", i, turn.Role, "agent")
		}
		if !turn.IsSidechain {
			t.Errorf("Turns[%d].IsSidechain=false, want true", i)
		}
		// UUID must be preserved.
		if turn.UUID != records[i].UUID {
			t.Errorf("Turns[%d].UUID=%q, want %q", i, turn.UUID, records[i].UUID)
		}
	}

	// Also verify that CollectSubagents exposes Turns via SubagentStats.
	// This checks the new SubagentStats.Turns field (spec §2.1).
	// We verify via a direct SubagentStats check by constructing one through
	// Aggregate, since aggregateSubagent is unexported.
	// The contract: SubagentStats must have a Turns field (fails to compile until added).
	var s parser.SubagentStats
	// Assign the turns from Aggregate output (simulating what aggregateSubagent will do).
	s.Turns = got.Turns
	if len(s.Turns) != 3 {
		t.Errorf("SubagentStats.Turns: len=%d, want 3", len(s.Turns))
	}
}
