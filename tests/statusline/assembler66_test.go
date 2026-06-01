// Package statusline_test — Phase 6.6.c / C1 updated test for assembler subagent passthrough.
//
// C1 (Phase 6.8 FIXES): perTurnTable now calls RenderUnified (not Builder.AddSubagents).
// Subagent turns appear via SubagentStats.Turns (IsSidechain=true) or via
// sidechain turns in Session.Turns. Aggregate-only SubagentStats (no Turns) do not
// produce rows in the new table; instead they are conveyed via SubagentProbe (line0).
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
)

// makeDataWithSubagents builds a probes.Data with turns and a non-empty Subagents slice.
func makeDataWithSubagents(turnCount int, subs []parser.SubagentStats) probes.Data {
	d := makeData(turnCount)
	d.Subagents = subs
	return d
}

// -------------------------------------------------------------------
// TestAssembler66_PassesSubagents
//
// C1: subagent rows appear in the table when SubagentStats.Turns is non-empty.
// When all Subagents have empty Turns AND Session.Turns has no sidechain turns,
// no "sub" role rows appear.
// -------------------------------------------------------------------
func TestAssembler66_PassesSubagents(t *testing.T) {
	// Minimal probe registries — just need Standard mode to render a table.
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// SubagentStats with a populated Turns slice (sidechain turn).
	subTurn := parser.Turn{
		Role:        "code-reviewer",
		Model:       "sonnet-4",
		UUID:        "sub-turn-1",
		GroupID:     1,
		Timestamp:   base.Add(5 * time.Second),
		Tokens:      parser.TokenCounts{Output: 200},
		ToolUse:     "Read",
		IsSidechain: true,
	}
	subWithTurns := parser.SubagentStats{
		AgentID:   "agent-abc",
		AgentType: "code-reviewer",
		Model:     "sonnet-4",
		Tokens:    parser.TokenCounts{CacheRead: 1000, Output: 200},
		LastTool:  "Read",
		Turns:     []parser.Turn{subTurn},
	}

	t.Run("with_subagent_turns_shows_sub_role", func(t *testing.T) {
		// Given: 3 orch turns + 1 subagent with 1 Turn.
		d := makeDataWithSubagents(3, []parser.SubagentStats{subWithTurns})
		// Timestamp for orch turns (needed to interleave): set base times.
		for i := range d.Session.Turns {
			d.Session.Turns[i].Timestamp = base.Add(time.Duration(i*10) * time.Second)
		}

		a := makeAssembler(mode.Standard)
		out := a.Render(d)

		// Subagent turn has role "code-reviewer" which should appear in the table.
		// The role cell may be truncated (e.g. "code-r…viewe"), so check for a
		// stable prefix that survives middle-truncation.
		if !strings.Contains(out, "code-r") {
			t.Errorf("Assembler with SubagentStats.Turns must include sidechain turn in table;\n"+
				"  want 'code-r' (prefix of 'code-reviewer' role) in output\noutput:\n%s", out)
		}
	})

	t.Run("empty_subagent_turns_no_extra_rows", func(t *testing.T) {
		// Given: 3 orch turns + 1 SubagentStats with no Turns (aggregate only).
		subNoTurns := parser.SubagentStats{
			AgentID:   "agent-xyz",
			AgentType: "code-reviewer",
			Model:     "sonnet-4",
		}
		d := makeDataWithSubagents(3, []parser.SubagentStats{subNoTurns})

		a := makeAssembler(mode.Standard)
		out := a.Render(d)

		// Table must render (has orch turns), but no ↳ legacy indicator.
		if !strings.Contains(out, "┌") {
			t.Errorf("Table border must appear for 3 orch turns; output:\n%s", out)
		}
		// Old ↳ indicator should not appear in RenderUnified output.
		if strings.Contains(out, "↳") {
			t.Errorf("Assembler with SubagentStats without Turns must NOT produce '↳';\noutput:\n%s", out)
		}
	})
}
