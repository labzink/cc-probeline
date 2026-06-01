// Package statusline_test — RED test for Phase 6.8 FIXES bug N1 (role colour).
//
// Root cause (found by hands-on smoke, missed by unit tests):
//   roleColour (internal/renderer/table.go:171) matches role == "orch" literally,
//   but RenderUnified.buildUnifiedRow passes the raw Turn.Role, which the parser
//   sets to "orchestrator" (session.go:133) for orchestrator turns and "agent"
//   (subagent.go:387) for sidechain turns. "orchestrator" != "orch" → falls to
//   yellow. The C1 table test missed this because its fixture used the UNREALISTIC
//   Role="orch", which happened to match the literal check.
//
// Fix vector: colour the role by Turn.IsSidechain (false → cyan, true → yellow),
//   not by string match. Label keeps the real role name.
//
// Production path verified: Assembler.Render(d) → perTurnTable → RenderUnified.
//
// RED: orchestrator row is yellow (no cyan anywhere) until the fix lands.
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// TestTable_OrchestratorRoleCyan verifies, through the production render path,
// that an orchestrator turn (IsSidechain=false, real Role="orchestrator") is
// coloured cyan, and a sidechain turn (Role="agent") is coloured yellow.
//
// Uses REALISTIC Role values as produced by parser.Aggregate / subagent merge
// (the lesson from the missed C1 test: never use convenient fake role strings).
func TestTable_OrchestratorRoleCyan(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	orchTurn := parser.Turn{
		Index:       1,
		Role:        "orchestrator", // parser session.go:133 — NOT "orch"
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-orch",
		GroupID:     1,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 100},
		ToolUse:     "ToolA",
		IsSidechain: false,
	}
	agentTurn := parser.Turn{
		Role:        "agent", // parser subagent.go:387
		Model:       "claude-sonnet-4-6",
		UUID:        "uuid-agent",
		GroupID:     1,
		Timestamp:   base.Add(10 * time.Second),
		Tokens:      parser.TokenCounts{Output: 50},
		ToolUse:     "SubTool",
		IsSidechain: true,
	}

	d := probes.Data{
		Session: &parser.SessionStats{
			Turns:  []parser.Turn{agentTurn, orchTurn}, // newest-first
			Totals: parser.TokenCounts{CacheRead: 1000, CacheCreate: 500, Output: 150},
		},
	}

	a := &statusline.Assembler{
		Mode:   mode.Standard,
		Theme:  renderer.Theme{AnsiEnabled: false}, // markers ({{color:...}}) still emitted
		Cols:   120,
		Config: probes.Config{},
	}

	out := a.Render(d)
	if out == "" {
		t.Fatal("N1: Render returned empty output")
	}

	// Find the orchestrator data row.
	var orchRow, agentRow string
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "orchestrator") {
			orchRow = ln
		}
		if strings.Contains(ln, "agent") && !strings.Contains(ln, "orchestrator") {
			agentRow = ln
		}
	}
	if orchRow == "" {
		t.Fatalf("N1: orchestrator row not found in output:\n%s", out)
	}

	// Orchestrator must be cyan (IsSidechain=false).
	if !strings.Contains(orchRow, "{{color:cyan}}") {
		t.Errorf("N1: orchestrator role must be cyan ({{color:cyan}}); got row:\n%s\n"+
			"FIX: colour role by Turn.IsSidechain in buildUnifiedRow, not by role==\"orch\"", orchRow)
	}
	if strings.Contains(orchRow, "{{color:yellow}}") {
		t.Errorf("N1: orchestrator role must NOT be yellow; got row:\n%s", orchRow)
	}

	// Sidechain agent must stay yellow.
	if agentRow != "" && !strings.Contains(agentRow, "{{color:yellow}}") {
		t.Errorf("N1: sidechain agent role must be yellow ({{color:yellow}}); got row:\n%s", agentRow)
	}
}
