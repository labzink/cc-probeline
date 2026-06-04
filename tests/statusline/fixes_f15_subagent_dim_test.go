// Package statusline_test — RED tests for Phase 6.9 FIXES group G-render, F15.
//
// F15: subagent row dim-by-age.
//
// Contract: a subagent whose ActivationStart is strictly before the freshest
// orchestrator group's start time (min Timestamp among turns of maxOrchGroup)
// belongs to a completed request and its collapsed row must render DIM.
// A subagent whose ActivationStart is within/after the freshest group start
// must NOT be dim.
//
// Current code (assembler.go:347):
//
//	Dim: false,  ← hardcoded, never dims subagent rows
//
// These tests FAIL until the assembler compares ActivationStart against the
// freshest group's start time and sets Dim accordingly.
//
// All assertions go through the production path Assembler.Render (production
// path) and check raw {{dim}} markers in output (pre-Apply level), consistent
// with the existing TestAssemble_DimOlderGroups approach.
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
)

// buildSubagentStats builds a parser.SubagentStats with the given ActivationStart
// and single turn. Uses the provided tool name as a unique marker in the row.
func buildSubagentStats(agentID string, agentType string, activationStart time.Time, lastTimestamp time.Time, tool string) parser.SubagentStats {
	turn := parser.Turn{
		Index:       1,
		Role:        agentType,
		UUID:        agentID + "-t1",
		GroupID:     0,
		Timestamp:   lastTimestamp,
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     tool,
		IsSidechain: true,
	}
	return parser.SubagentStats{
		AgentID:         agentID,
		AgentType:       agentType,
		Model:           "claude-sonnet-4-6",
		CurrentTurnNum:  1,
		ActivationStart: activationStart,
		LastTimestamp:   lastTimestamp,
		TurnCount:       1,
		Turns:           []parser.Turn{turn},
		Tokens:          parser.TokenCounts{Output: 50, CacheCreate: 500},
	}
}

// ---------------------------------------------------------------------------
// TestF15_OldSubagentRowIsDim
//
// F15 contract (old subagent): a subagent whose ActivationStart is strictly
// before the freshest orchestrator group's start time must render DIM.
//
// Fixture:
//   - Group 1 (history): 1 orch turn at base+0s (GroupID=1, maxOrchGroup−1).
//   - Group 2 (fresh): 1 orch turn at base+30s (GroupID=2 = maxOrchGroup).
//     Freshest group start = base+30s (the single turn's Timestamp).
//   - Old subagent: ActivationStart=base+5s (strictly before base+30s).
//     → This subagent was active during the old G1 request → must be DIM.
//
// The test asserts that the old subagent's collapsed row contains "{{dim}}" in
// the raw output (before Apply).
//
// FAILS on current code: assembler.go:347 hardcodes Dim:false for all subagents.
// ---------------------------------------------------------------------------
func TestF15_OldSubagentRowIsDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Orchestrator turns: Group1 (history, base), Group2 (fresh, base+30s).
	orchG1 := orchTurn(1, "f15-g1-orch", 1, base, "ToolG1", "")
	orchG2 := orchTurn(2, "f15-g2-orch", 2, base.Add(30*time.Second), "ToolG2", "")

	// Old subagent: ActivationStart=base+5s, well before freshest group start (base+30s).
	// Its collapsed row must be DIM because it belongs to the G1 (old) request.
	oldSA := buildSubagentStats(
		"f15-old-agent",
		"code-reviewer",
		base.Add(5*time.Second),   // ActivationStart: during G1 (before G2 start at base+30s)
		base.Add(10*time.Second),  // LastTimestamp: also before G2 start
		"F15OldTool",
	)

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 2,
			// Newest-first for session turns; assembler re-sorts.
			Turns: []parser.Turn{orchG2, orchG1},
		},
		Subagents:    []parser.SubagentStats{oldSA},
		Now:          base.Add(2 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	if !strings.Contains(out, "┌") {
		t.Fatalf("F15-old: no table in output\noutput:\n%s", out)
	}

	// Find the old subagent row (contains "F15OldTool").
	oldRowFound := false
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "F15OldTool") {
			continue
		}
		oldRowFound = true

		// F15 contract: old subagent row must be WHOLE-ROW dim.
		// A whole-row dim row starts with "{{dim}}" followed immediately by a box
		// character NOT closed by "{{reset}}" (whole-row wrapper pattern):
		//   {{dim}}│<content>{{reset}} — whole-row dim (all content inside outer dim)
		// vs per-border dim (fresh rows):
		//   {{dim}}│{{reset}} <content> — only the border char is dim, content is not wrapped.
		//
		// Detection: whole-row dim starts "{{dim}}" AND does NOT start with
		// "{{dim}}│{{reset}}" or "{{dim}}├{{reset}}" (per-border patterns).
		isWholeRowDim := strings.HasPrefix(l, "{{dim}}") &&
			!strings.HasPrefix(l, "{{dim}}│{{reset}}") &&
			!strings.HasPrefix(l, "{{dim}}├{{reset}}")
		if !isWholeRowDim {
			t.Errorf("F15-old: old subagent row must be WHOLE-ROW dim;\n"+
				"  ActivationStart=base+5s is before freshest group start (base+30s).\n"+
				"  Current code: Dim=false hardcoded for all subagents (assembler.go:347).\n"+
				"  Fix: compare ActivationStart with min(freshestGroupTurns.Timestamp) and set Dim=true when before.\n"+
				"  row: %s", l)
		}
	}

	if !oldRowFound {
		t.Errorf("F15-old: old subagent row 'F15OldTool' not found in output\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// TestF15_FreshSubagentRowNotDim
//
// F15 contract (fresh subagent): a subagent whose ActivationStart is within/after
// the freshest orchestrator group's start time must NOT be dim.
//
// Fixture:
//   - Group 1 (history): 1 orch turn at base+0s (GroupID=1).
//   - Group 2 (fresh): 1 orch turn at base+30s (GroupID=2 = maxOrchGroup).
//     Freshest group start = base+30s.
//   - Fresh subagent: ActivationStart=base+35s (after base+30s).
//     → This subagent was activated after G2 started → must NOT be DIM.
//
// This test verifies the "not dim" side of the F15 contract. A subagent
// active in the current (freshest) request must render normally (non-dim).
//
// On current code this test PASSES (Dim=false hardcoded). After GREEN
// (F15 fix), the "old" subagent becomes dim and the "fresh" stays non-dim.
// We keep this test here to prevent over-dimming (regression guard).
// ---------------------------------------------------------------------------
func TestF15_FreshSubagentRowNotDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Orchestrator turns: Group1 (history, base), Group2 (fresh, base+30s).
	orchG1 := orchTurn(1, "f15b-g1-orch", 1, base, "ToolG1b", "")
	orchG2 := orchTurn(2, "f15b-g2-orch", 2, base.Add(30*time.Second), "ToolG2b", "")

	// Fresh subagent: ActivationStart=base+35s, AFTER freshest group start (base+30s).
	// Its collapsed row must NOT be DIM.
	freshSA := buildSubagentStats(
		"f15-fresh-agent",
		"general-purpose",
		base.Add(35*time.Second),  // ActivationStart: during G2 (after G2 start at base+30s)
		base.Add(50*time.Second),  // LastTimestamp
		"F15FreshTool",
	)

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 2,
			Turns:     []parser.Turn{orchG2, orchG1},
		},
		Subagents:    []parser.SubagentStats{freshSA},
		Now:          base.Add(2 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	if !strings.Contains(out, "┌") {
		t.Fatalf("F15-fresh: no table in output\noutput:\n%s", out)
	}

	// Find the fresh subagent row (contains "F15FreshTool").
	freshRowFound := false
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "F15FreshTool") {
			continue
		}
		freshRowFound = true

		// F15 contract: fresh subagent row must NOT be whole-row dim.
		// A whole-row dim starts with "{{dim}}" AND has the entire content wrapped.
		// Per-border dim ({{dim}}│{{reset}}) is acceptable; whole-row is not.
		isWholeRowDim := strings.HasPrefix(l, "{{dim}}") &&
			!strings.HasPrefix(l, "{{dim}}│{{reset}}") &&
			!strings.HasPrefix(l, "{{dim}}├{{reset}}")
		if isWholeRowDim {
			t.Errorf("F15-fresh: fresh subagent row must NOT be whole-row dim;\n"+
				"  ActivationStart=base+35s is AFTER freshest group start (base+30s).\n"+
				"  Fresh subagent belongs to current request → non-dim.\n"+
				"  row: %s", l)
		}
	}

	if !freshRowFound {
		t.Errorf("F15-fresh: fresh subagent row 'F15FreshTool' not found in output\noutput:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// TestF15_OldAndFreshSubagentTwoGroupSession
//
// F15 integration: two-group session with both an old and a fresh subagent.
// The old subagent row must be dim; the fresh subagent row must not.
//
// Fixture (same structure as above, combined):
//   - Group 1 (history): orch turn at base+0s.
//   - Group 2 (fresh): orch turn at base+30s.
//   - Old subagent: ActivationStart=base+5s (before G2 start at base+30s) → DIM.
//   - Fresh subagent: ActivationStart=base+35s (after G2 start) → NOT DIM.
//
// FAILS on current code because the old subagent row is not dim.
// ---------------------------------------------------------------------------
func TestF15_OldAndFreshSubagentTwoGroupSession(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchG1 := orchTurn(1, "f15c-g1", 1, base, "OrcToolG1c", "")
	orchG2 := orchTurn(2, "f15c-g2", 2, base.Add(30*time.Second), "OrcToolG2c", "")

	oldSA := buildSubagentStats(
		"f15c-old",
		"code-writer",
		base.Add(5*time.Second),
		base.Add(15*time.Second),
		"F15COldTool",
	)
	freshSA := buildSubagentStats(
		"f15c-fresh",
		"researcher",
		base.Add(35*time.Second),
		base.Add(45*time.Second),
		"F15FreshC",
	)

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount: 2,
			Turns:     []parser.Turn{orchG2, orchG1},
		},
		Subagents:    []parser.SubagentStats{oldSA, freshSA},
		Now:          base.Add(2 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	if !strings.Contains(out, "┌") {
		t.Fatalf("F15-combined: no table in output\noutput:\n%s", out)
	}

	// Old subagent row must be WHOLE-ROW dim (same pattern as orch history rows).
	oldFound := false
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "F15COldTool") {
			continue
		}
		oldFound = true
		isWholeRowDim := strings.HasPrefix(l, "{{dim}}") &&
			!strings.HasPrefix(l, "{{dim}}│{{reset}}") &&
			!strings.HasPrefix(l, "{{dim}}├{{reset}}")
		if !isWholeRowDim {
			t.Errorf("F15-combined: old subagent row ('F15COldTool') must be WHOLE-ROW dim;\n"+
				"  ActivationStart=%v is before freshest group start (base+30s).\n"+
				"  Current code hardcodes Dim=false for all subagents.\n"+
				"  row: %s", base.Add(5*time.Second), l)
		}
	}
	if !oldFound {
		t.Errorf("F15-combined: old subagent row 'F15COldTool' not found\noutput:\n%s", out)
	}

	// Fresh subagent row must NOT be whole-row dim.
	freshFound := false
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "F15FreshC") {
			continue
		}
		freshFound = true
		isWholeRowDim := strings.HasPrefix(l, "{{dim}}") &&
			!strings.HasPrefix(l, "{{dim}}│{{reset}}") &&
			!strings.HasPrefix(l, "{{dim}}├{{reset}}")
		if isWholeRowDim {
			t.Errorf("F15-combined: fresh subagent row ('F15FreshC') must NOT be whole-row dim;\n"+
				"  ActivationStart=%v is AFTER freshest group start (base+30s).\n"+
				"  row: %s", base.Add(35*time.Second), l)
		}
	}
	if !freshFound {
		t.Errorf("F15-combined: fresh subagent row 'F15FreshC' not found\noutput:\n%s", out)
	}
}
