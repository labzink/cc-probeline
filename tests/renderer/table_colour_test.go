// Package renderer_test — colour tests for Phase 6.7.c unified table (T-11).
//
// Tests migrated from the legacy Builder.Render() path (Phase 7, BL-28) to
// the production RenderUnified path. Properties verified:
//   - box-drawing borders wrapped in \x1b[2m (dim)
//   - "orch" role cell wrapped in \x1b[36m (cyan)
//   - sidechain agent role cell wrapped in \x1b[33m (yellow), including when
//     the AgentType is middle-truncated (regression RC2)
//   - all borders uniformly dim: corners and vertical bars (regression RC4)
//   - no ANSI escapes under Theme{} (AnsiEnabled=false) (regression T-11f/T-13)
package renderer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
)

// colourTheme returns a colour-enabled theme with the default palette.
func colourTheme() renderer.Theme {
	return renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.DefaultPalette(),
	}
}

// renderUnifiedWithColour renders turns through RenderUnified and applies colour.
func renderUnifiedWithColour(turns []parser.Turn, st *state.Session, cols int) string {
	b := renderer.NewBuilder(cols)
	raw := b.RenderUnified(turns, st)
	return renderer.Apply(raw, colourTheme())
}

// -------------------------------------------------------------------
// T-11a: TestTableColour_BordersDim
//
// Border runes (┌ ┬ ┐ ├ ┼ ┤ └ ┴ ┘ ─ │) must be preceded by the dim
// escape \x1b[2m in the coloured output.
// -------------------------------------------------------------------
func TestTableColour_BordersDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	turns := []parser.Turn{
		{Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "u1", GroupID: 1,
			Timestamp: base, Tokens: parser.TokenCounts{Output: 200}, ToolUse: "Read"},
	}

	out := renderUnifiedWithColour(turns, nil, 80)
	if out == "" {
		t.Fatal("RenderUnified returned empty string; cannot test colour")
	}

	dimEsc := "\x1b[2m"
	if !strings.Contains(out, dimEsc) {
		t.Errorf("T-11a: table output must contain dim escape %q for borders; not found\noutput (first 400 chars):\n%.400s",
			dimEsc, out)
	}

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
// -------------------------------------------------------------------
func TestTableColour_OrchCyan(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	turns := []parser.Turn{
		{Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "u1", GroupID: 1,
			Timestamp: base, Tokens: parser.TokenCounts{Output: 200}, ToolUse: "Read"},
	}

	out := renderUnifiedWithColour(turns, nil, 80)
	if out == "" {
		t.Fatal("RenderUnified returned empty string; cannot test colour")
	}

	cyanEsc := "\x1b[36m"
	if !strings.Contains(out, cyanEsc) {
		t.Errorf("T-11b: table output with role='orch' must contain cyan escape %q; not found\noutput (first 400 chars):\n%.400s",
			cyanEsc, out)
	}
	if !strings.Contains(out, cyanEsc+"orch") {
		t.Errorf("T-11b: 'orch' must immediately follow cyan escape %q (role cell); not found\noutput (first 400 chars):\n%.400s",
			cyanEsc, out)
	}
}

// -------------------------------------------------------------------
// T-11c: TestTableColour_AgentYellow
//
// A sidechain turn (IsSidechain=true) must have its role cell wrapped in
// \x1b[33m (yellow).
// -------------------------------------------------------------------
func TestTableColour_AgentYellow(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	turns := []parser.Turn{
		{Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "u1", GroupID: 1,
			Timestamp: base, Tokens: parser.TokenCounts{Output: 200}, ToolUse: "Read",
			IsSidechain: false},
		{Role: "agent", Model: "claude-haiku-4-5", UUID: "u2", GroupID: 1,
			Timestamp: base.Add(time.Second), Tokens: parser.TokenCounts{Output: 100}, ToolUse: "Bash",
			IsSidechain: true},
	}

	out := renderUnifiedWithColour(turns, nil, 80)
	if out == "" {
		t.Fatal("RenderUnified returned empty string; cannot test colour")
	}

	yellowEsc := "\x1b[33m"
	if !strings.Contains(out, yellowEsc) {
		t.Errorf("T-11c: table output with sidechain turn must contain yellow escape %q for agent role; not found\noutput (first 600 chars):\n%.600s",
			yellowEsc, out)
	}
	if !strings.Contains(out, yellowEsc+"agent") {
		t.Errorf("T-11c: 'agent' role must immediately follow yellow escape %q; not found\noutput (first 600 chars):\n%.600s",
			yellowEsc, out)
	}
}

// -------------------------------------------------------------------
// T-11h: TestTableColour_LongAgentNameKeepsColour (regression RC2)
//
// A sidechain turn with a role wider than the column must be middle-truncated
// by padCell. The colour wrapper must survive truncation.
// -------------------------------------------------------------------
func TestTableColour_LongAgentNameKeepsColour(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	turns := []parser.Turn{
		{Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "u1", GroupID: 1,
			Timestamp: base, Tokens: parser.TokenCounts{Output: 100}, ToolUse: "Read",
			IsSidechain: false},
		// Role "code-reviewer-extra-long" is 24 runes — exceeds the 13-rune inner width
		// of the 14-wide role column → padCell must truncate with colour preserved.
		{Role: "code-reviewer-extra-long", Model: "claude-haiku-4-5", UUID: "u2", GroupID: 1,
			Timestamp: base.Add(time.Second), Tokens: parser.TokenCounts{Output: 50}, ToolUse: "Bash",
			IsSidechain: true},
	}

	out := renderUnifiedWithColour(turns, nil, 80)
	if !strings.Contains(out, "\x1b[33m") {
		t.Errorf("RC2: truncated long agent role must keep yellow \\x1b[33m; got:\n%s", out)
	}
}

// -------------------------------------------------------------------
// T-11i: TestTableColour_BordersUniformDim (regression RC4)
//
// All border elements — corner runes AND vertical bars — must be dim, not only
// the horizontal fill.
// -------------------------------------------------------------------
func TestTableColour_BordersUniformDim(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	turns := []parser.Turn{
		{Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "u1", GroupID: 1,
			Timestamp: base, Tokens: parser.TokenCounts{Output: 200}, ToolUse: "Read"},
	}

	out := renderUnifiedWithColour(turns, nil, 80)
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
// -------------------------------------------------------------------
func TestTableColour_NoBordersWithoutTheme(t *testing.T) {
	plainTheme := renderer.Theme{} // AnsiEnabled: false, no palette
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	turns := []parser.Turn{
		{Index: 1, Role: "orch", Model: "claude-sonnet-4-6", UUID: "u1", GroupID: 1,
			Timestamp: base, Tokens: parser.TokenCounts{Output: 200}, ToolUse: "Read"},
		{Index: 2, Role: "orch", Model: "claude-opus-4-7", UUID: "u2", GroupID: 2,
			Timestamp: base.Add(time.Minute), Tokens: parser.TokenCounts{Output: 100}, ToolUse: "Edit"},
	}

	b := renderer.NewBuilder(80)
	raw := b.RenderUnified(turns, nil)
	if raw == "" {
		t.Fatal("RenderUnified returned empty string; cannot test theme=off")
	}

	out := renderer.Apply(raw, plainTheme)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("T-11f: table output under Theme{} (ansi off) must contain no ANSI escapes; found \\x1b[ in output\noutput (first 400 chars):\n%.400s",
			out)
	}
}
