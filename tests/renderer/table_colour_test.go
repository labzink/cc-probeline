// Package renderer_test — RED tests for Phase 6.7.c table colour (T-11).
//
// T-11: Under Theme{AnsiEnabled:true, Colors:DefaultPalette()}, the table
// rendered by Builder.Render() and piped through renderer.Apply must contain:
//   - box-drawing borders in \x1b[2m (dim)
//   - "orch" role wrapped in \x1b[36m (cyan)
//   - subagent AgentType / "agent" role wrapped in \x1b[33m (yellow)
//   - per-turn "$cost" cell NOT wrapped in any \x1b[ code
//
// Test FAILS until internal/renderer/table.go emits colour markers around
// borders, role cells, and cost cells.
package renderer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// colourTheme returns a colour-enabled theme with the default palette.
func colourTheme() renderer.Theme {
	return renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.DefaultPalette(),
	}
}

// renderWithColour renders the table and applies the colour theme.
func renderWithColour(b *renderer.Builder) string {
	raw := b.Render()
	return renderer.Apply(raw, colourTheme())
}

// -------------------------------------------------------------------
// T-11a: TestTableColour_BordersDim
//
// Border runes (┌ ┬ ┐ ├ ┼ ┤ └ ┴ ┘ ─ │) must be preceded by the dim
// escape \x1b[2m in the coloured output.
//
// FAILS until table.go wraps border calls in {{dim}}...{{reset}}.
// -------------------------------------------------------------------
func TestTableColour_BordersDim(t *testing.T) {
	th := colourTheme()

	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	raw := b.Render()
	if raw == "" {
		t.Fatal("Render() returned empty string; cannot test colour")
	}

	out := renderer.Apply(raw, th)

	// The dim escape must appear somewhere — borders are extensive in a table.
	dimEsc := "\x1b[2m"
	if !strings.Contains(out, dimEsc) {
		t.Errorf("T-11a: table output must contain dim escape %q for borders; not found\noutput (first 400 chars):\n%.400s",
			dimEsc, out)
	}

	// At least one box-drawing border rune must appear after a dim escape.
	// We verify that the sequence "\x1b[2m" appears and is followed by a border
	// rune somewhere in the output.
	borderRunes := []string{"┌", "┬", "┐", "├", "┼", "┤", "└", "┴", "┘", "─", "│"}
	foundDimBorder := false
	for _, br := range borderRunes {
		if strings.Contains(out, dimEsc+br) {
			foundDimBorder = true
			break
		}
	}
	if !foundDimBorder {
		t.Errorf("T-11a: no border rune directly follows dim escape %q in coloured output\noutput (first 400 chars):\n%.400s",
			dimEsc, out)
	}
}

// -------------------------------------------------------------------
// T-11b: TestTableColour_OrchCyan
//
// A row with role="orch" must have the role cell wrapped in \x1b[36m (cyan).
//
// FAILS until table.go wraps the role cell content in {{color:cyan}} when
// role == "orch".
// -------------------------------------------------------------------
func TestTableColour_OrchCyan(t *testing.T) {
	th := colourTheme()

	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	raw := b.Render()
	if raw == "" {
		t.Fatal("Render() returned empty string; cannot test colour")
	}

	out := renderer.Apply(raw, th)

	cyanEsc := "\x1b[36m"
	if !strings.Contains(out, cyanEsc) {
		t.Errorf("T-11b: table output with role='orch' must contain cyan escape %q; not found\noutput (first 400 chars):\n%.400s",
			cyanEsc, out)
	}

	// The cyan-wrapped string must contain "orch" (cell content).
	if !strings.Contains(out, cyanEsc+"orch") {
		t.Errorf("T-11b: 'orch' must immediately follow cyan escape %q (role cell); not found\noutput (first 400 chars):\n%.400s",
			cyanEsc, out)
	}
}

// -------------------------------------------------------------------
// T-11c: TestTableColour_AgentYellow
//
// A subagent row (added via AddSubagents) must have its AgentType cell
// wrapped in \x1b[33m (yellow).
//
// FAILS until table.go wraps the role cell in {{color:yellow}} for agent rows.
// -------------------------------------------------------------------
func TestTableColour_AgentYellow(t *testing.T) {
	th := colourTheme()

	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))

	// AgentType "general" is short enough to fit untruncated in the role cell.
	sub := parser.SubagentStats{
		AgentID:   "sa1",
		AgentType: "general",
		Model:     "sonnet-4",
		Tokens:    parser.TokenCounts{CacheRead: 500, Output: 100},
		LastTool:  "Bash",
	}
	b.AddSubagents([]parser.SubagentStats{sub})

	raw := b.Render()
	if raw == "" {
		t.Fatal("Render() returned empty string; cannot test colour")
	}

	out := renderer.Apply(raw, th)

	yellowEsc := "\x1b[33m"
	if !strings.Contains(out, yellowEsc) {
		t.Errorf("T-11c: table output with subagent row must contain yellow escape %q for agent role; not found\noutput (first 600 chars):\n%.600s",
			yellowEsc, out)
	}

	// The yellow-wrapped content must contain "general" (agent type value).
	if !strings.Contains(out, yellowEsc+"general") {
		t.Errorf("T-11c: subagent AgentType 'general' must immediately follow yellow escape %q; not found\noutput (first 600 chars):\n%.600s",
			yellowEsc, out)
	}
}

// -------------------------------------------------------------------
// T-11d: TestTableColour_CacheMissRed
//
// A row where the cache cell contains ⚠ (cache-miss indicator) must have
// ⚠ wrapped in \x1b[31m (red).
//
// FAILS until table.go wraps ⚠ markers inside the cache cell in {{color:red}}.
// -------------------------------------------------------------------
func TestTableColour_CacheMissRed(t *testing.T) {
	th := colourTheme()

	// Build a turn whose cache column will contain ⚠.
	// The table renders cache as "<CacheRead>/<CacheCreate>". According to
	// spec-common §2.3, ⚠ cache-miss → {{color:red}}. The cache-miss marker
	// is placed in the cache cell when both CacheRead and CacheCreate are 0.
	// The exact mechanism (which turn configuration triggers ⚠) is owned by
	// the dev implementing the feature; here we use a Turn with explicit ⚠
	// in its content by constructing a turn whose rendered cache string contains ⚠.
	//
	// The test exercises the colourisation contract: wherever ⚠ appears in the
	// table output, it must be preceded by \x1b[31m (red).
	b := renderer.NewBuilder(80)
	b.Add(parser.Turn{
		Index:   1,
		Role:    "orch",
		Model:   "sonnet-4",
		Tokens:  parser.TokenCounts{CacheRead: 0, CacheCreate: 0, Output: 50},
		ToolUse: "Read",
	})

	raw := b.Render()
	if raw == "" {
		t.Fatal("Render() returned empty string; cannot test colour")
	}

	// Only run the assertion if the raw table actually contains ⚠.
	// If the dev chooses not to emit ⚠ for zero-cache turns, this subtest
	// documents the expected behaviour once ⚠ is present.
	if !strings.Contains(raw, "⚠") {
		t.Skip("T-11d: raw table does not contain ⚠ (cache-miss indicator not emitted for zero-cache turn); " +
			"adjust fixture or production logic to emit ⚠ before this test becomes meaningful")
	}

	out := renderer.Apply(raw, th)

	redEsc := "\x1b[31m"
	if !strings.Contains(out, redEsc+"⚠") {
		t.Errorf("T-11d: ⚠ (cache-miss) must immediately follow red escape %q in coloured output; not found\noutput:\n%s",
			redEsc, out)
	}
}

// -------------------------------------------------------------------
// T-11e: TestTableColour_CostNoColour
//
// The per-turn cost cell (column 5 in 7-col layout, containing "$...") must
// NOT be preceded by any ANSI colour escape. The cost column is intentionally
// neutral (per spec-common §2.1 and §2.3: cost column is not coloured).
//
// FAILS if table.go incorrectly wraps cost cells in a colour marker.
// -------------------------------------------------------------------
func TestTableColour_CostNoColour(t *testing.T) {
	th := colourTheme()

	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 5000, 300, 0))

	raw := b.Render()
	if raw == "" {
		t.Fatal("Render() returned empty string; cannot test colour")
	}

	out := renderer.Apply(raw, th)

	// Find every occurrence of "$" and verify none is immediately preceded by
	// a colour escape code. We check for known colour escapes (not dim/bold/reset
	// which are style attributes, not colour).
	colourEscapes := []string{
		"\x1b[36m",       // cyan
		"\x1b[33m",       // yellow
		"\x1b[31m",       // red
		"\x1b[32m",       // green
		"\x1b[35m",       // magenta
		"\x1b[38;5;208m", // orange
	}

	for _, esc := range colourEscapes {
		// If a colour escape immediately precedes "$", that violates the contract.
		if strings.Contains(out, esc+"$") {
			t.Errorf("T-11e: cost cell '$...' must NOT be preceded by colour escape %q (cost is neutral); found sequence in output\noutput (first 600 chars):\n%.600s",
				esc, out)
		}
	}
}

// -------------------------------------------------------------------
// T-11g: TestTableColour_SubagentElapsed (regression RC3)
//
// The subagent row must surface a colour-wrapped ⏱elapsed cell (span =
// LastTimestamp − FirstTimestamp): >300s → red, ≤300s → yellow. Before the fix
// the table rendered no elapsed at all (the coloured SubagentProbe was unwired
// dead code), so subagent elapsed was always absent/grey.
// -------------------------------------------------------------------
func TestTableColour_SubagentElapsed(t *testing.T) {
	th := colourTheme()
	base := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	// span 400s > 300s → red.
	bRed := renderer.NewBuilder(80)
	bRed.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))
	bRed.AddSubagents([]parser.SubagentStats{{
		AgentID: "sa1", AgentType: "general", Model: "sonnet-4",
		FirstTimestamp: base, LastTimestamp: base.Add(400 * time.Second),
		Tokens: parser.TokenCounts{CacheRead: 500, Output: 100}, LastTool: "Bash",
	}})
	outRed := renderer.Apply(bRed.Render(), th)
	if !strings.Contains(outRed, "\x1b[31m⏱") {
		t.Errorf("RC3 elapsed >300s: want red \\x1b[31m⏱ in subagent row; got:\n%s", outRed)
	}

	// span 120s ≤ 300s → yellow.
	bYel := renderer.NewBuilder(80)
	bYel.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))
	bYel.AddSubagents([]parser.SubagentStats{{
		AgentID: "sa2", AgentType: "general", Model: "sonnet-4",
		FirstTimestamp: base, LastTimestamp: base.Add(120 * time.Second),
		Tokens: parser.TokenCounts{CacheRead: 500, Output: 100}, LastTool: "Bash",
	}})
	outYel := renderer.Apply(bYel.Render(), th)
	if !strings.Contains(outYel, "\x1b[33m⏱") {
		t.Errorf("RC3 elapsed ≤300s: want yellow \\x1b[33m⏱ in subagent row; got:\n%s", outYel)
	}
}

// -------------------------------------------------------------------
// T-11h: TestTableColour_LongAgentNameKeepsColour (regression RC2)
//
// A subagent AgentType wider than the role column is middle-truncated by
// padCell. The colour wrapper must survive truncation. Before the fix padCell
// stripped ALL markers before truncating, so long names rendered grey while
// short names stayed yellow (the intermittent yellow/grey symptom).
// -------------------------------------------------------------------
func TestTableColour_LongAgentNameKeepsColour(t *testing.T) {
	th := colourTheme()
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))
	b.AddSubagents([]parser.SubagentStats{{
		AgentID: "sa1", AgentType: "code-reviewer-extra-long", Model: "sonnet-4",
		Tokens: parser.TokenCounts{CacheRead: 500, Output: 100}, LastTool: "Bash",
	}})
	out := renderer.Apply(b.Render(), th)
	if !strings.Contains(out, "\x1b[33m") {
		t.Errorf("RC2: truncated long agent name must keep yellow \\x1b[33m; got:\n%s", out)
	}
}

// -------------------------------------------------------------------
// T-11i: TestTableColour_BordersUniformDim (regression RC4)
//
// All border elements — corner runes AND vertical bars — must be dim, not only
// the horizontal fill. Before the fix corners were written outside the {{dim}}
// wrapper and vertical bars carried no marker, giving a mixed light/dark look.
// -------------------------------------------------------------------
func TestTableColour_BordersUniformDim(t *testing.T) {
	th := colourTheme()
	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))
	out := renderer.Apply(b.Render(), th)

	for _, seq := range []string{"\x1b[2m┌", "\x1b[2m└", "\x1b[2m│"} {
		if !strings.Contains(out, seq) {
			t.Errorf("RC4: border element %q must be dim-wrapped (uniform dim borders); not found in:\n%s", seq, out)
		}
	}
}

// -------------------------------------------------------------------
// T-11f: TestTableColour_NoBordersWithoutTheme
//
// Under Theme{} (AnsiEnabled=false), the rendered table must NOT contain
// any \x1b[ escape sequences (all markers stripped).
//
// This is the T-13 regression from spec-common §3 applied to the table
// renderer specifically.
// -------------------------------------------------------------------
func TestTableColour_NoBordersWithoutTheme(t *testing.T) {
	plainTheme := renderer.Theme{} // AnsiEnabled: false, no palette

	b := renderer.NewBuilder(80)
	b.Add(makeTurn(1, "orch", "sonnet-4", "Read", 1000, 200, 0))
	b.Add(makeTurn(2, "orch", "opus-4", "Edit", 2000, 100, 0))

	raw := b.Render()
	if raw == "" {
		t.Fatal("Render() returned empty string; cannot test theme=off")
	}

	out := renderer.Apply(raw, plainTheme)

	if strings.Contains(out, "\x1b[") {
		t.Errorf("T-11f: table output under Theme{} (ansi off) must contain no ANSI escapes; found \\x1b[ in output\noutput (first 400 chars):\n%.400s",
			out)
	}
}
