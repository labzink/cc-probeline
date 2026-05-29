// Package statusline_test — Phase 6.6.c RED test for assembler subagent passthrough.
//
// T-19: perTurnTable must forward d.Subagents to Builder.AddSubagents so that
// subagent rows appear in Standard-mode output.
package statusline_test

import (
	"strings"
	"testing"

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
// T-19: TestAssembler66_PassesSubagents
//
// assembler.perTurnTable with d.Subagents non-empty must produce output
// containing "↳" (the subagent row indicator). With d.Subagents empty
// the output must NOT contain "↳".
// -------------------------------------------------------------------
func TestAssembler66_PassesSubagents(t *testing.T) {
	// Minimal probe registries — just need Standard mode to render a table.
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "email@x"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "sonnet"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	sub := parser.SubagentStats{
		AgentID:   "agent-abc",
		AgentType: "code-reviewer",
		Model:     "sonnet-4",
		Tokens: parser.TokenCounts{
			CacheRead: 1000,
			Output:    200,
		},
		LastTool: "Read",
	}

	t.Run("with_subagents_has_arrow", func(t *testing.T) {
		// Given: 3 turns + 1 subagent.
		d := makeDataWithSubagents(3, []parser.SubagentStats{sub})

		a := makeAssembler(mode.Standard)
		out := a.Render(d)

		// Subagent row must contain "↳" indicator.
		if !strings.Contains(out, "↳") {
			t.Errorf("Assembler with non-empty Subagents must produce '↳' in table output;\noutput:\n%s", out)
		}
	})

	t.Run("empty_subagents_no_arrow", func(t *testing.T) {
		// Given: 3 turns + empty Subagents slice.
		d := makeDataWithSubagents(3, nil)

		a := makeAssembler(mode.Standard)
		out := a.Render(d)

		// No subagent rows when Subagents is nil/empty.
		if strings.Contains(out, "↳") {
			t.Errorf("Assembler with empty Subagents must NOT produce '↳' in table output;\noutput:\n%s", out)
		}
	})
}
