// Package statusline_test — RED tests for Phase 6.7.c subagent elapsed colour (T-12).
//
// T-12: SubagentProbe.Render under Theme{AnsiEnabled:true, Colors:DefaultPalette()}
// must produce a ⏱elapsed string wrapped in:
//   - \x1b[31m (red)    when elapsed > 300 seconds
//   - \x1b[33m (yellow) when elapsed ≤ 300 seconds
//
// After piping the output through renderer.Apply, the ANSI codes must be present
// in the correct positions.
//
// Tests FAIL until internal/probes/subagent.go emits {{color:red}} / {{color:yellow}}
// markers around the ⏱ elapsed field.
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// colourThemeForStatusline returns a colour-enabled theme with the default palette.
func colourThemeForStatusline() renderer.Theme {
	return renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.DefaultPalette(),
	}
}

// makeSubagentData constructs a probes.Data with a single subagent task whose
// elapsed time equals elapsedSec seconds from d.Now.
func makeSubagentData(elapsedSec int) probes.Data {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	startTime := now.Add(-time.Duration(elapsedSec) * time.Second)

	task := stdin.Task{
		ID:        "agent-abc",
		Name:      "my-agent",
		StartTime: startTime,
	}

	sub := parser.SubagentStats{
		AgentID:   "agent-abc",
		AgentType: "general-purpose",
		Model:     "sonnet-4",
		Tokens: parser.TokenCounts{
			CacheRead: 1000,
			Output:    200,
		},
		LastTool: "Read",
	}

	return probes.Data{
		Stdin: stdin.Payload{
			Tasks: []stdin.Task{task},
		},
		Subagents: []parser.SubagentStats{sub},
		Now:       now,
		Session: &parser.SessionStats{
			TurnCount: 1,
			Turns:     makeTurns(1),
		},
	}
}

// renderSubagentProbe renders the SubagentProbe with the given data and
// applies the colour theme, returning the final ANSI-escaped string.
func renderSubagentProbe(d probes.Data, th renderer.Theme) string {
	p := &probes.SubagentProbe{}
	cfg := probes.Config{SubagentEnabled: true}
	raw := p.Render(d, cfg, th, probes.LevelFull)
	return renderer.Apply(raw, th)
}

// -------------------------------------------------------------------
// T-12a: TestSubagentColour_ElapsedOver300s_Red
//
// When elapsed > 300s (e.g. 301s), the ⏱elapsed token must be wrapped
// in \x1b[31m (red) in the final ANSI output.
//
// FAILS until probes/subagent.go emits {{color:red}} around ⏱elapsed
// when duration > 300s.
// -------------------------------------------------------------------
func TestSubagentColour_ElapsedOver300s_Red(t *testing.T) {
	th := colourThemeForStatusline()

	// Given: elapsed = 301 seconds (> 300s threshold).
	d := makeSubagentData(301)

	// When: probe renders and colour is applied.
	out := renderSubagentProbe(d, th)

	// Then: output must contain the red escape code.
	redEsc := "\x1b[31m"
	if !strings.Contains(out, redEsc) {
		t.Errorf("T-12a: elapsed=301s (>300s) must produce red escape %q in output; not found\nraw probe output:\n%s",
			redEsc, out)
	}

	// The ⏱ character must appear after the red escape.
	if !strings.Contains(out, redEsc+"⏱") {
		t.Errorf("T-12a: ⏱ elapsed glyph must immediately follow red escape %q for elapsed>300s; not found\noutput:\n%s",
			redEsc, out)
	}
}

// -------------------------------------------------------------------
// T-12b: TestSubagentColour_ElapsedAtThreshold_Red
//
// elapsed = 301s is the boundary case strictly greater than 300s.
// This subtest validates the exclusive threshold (>300s → red).
// -------------------------------------------------------------------
func TestSubagentColour_ElapsedAtThreshold_Red(t *testing.T) {
	th := colourThemeForStatusline()

	// Given: elapsed = 301s (exactly one second over the threshold).
	d := makeSubagentData(301)
	out := renderSubagentProbe(d, th)

	redEsc := "\x1b[31m"
	if !strings.Contains(out, redEsc) {
		t.Errorf("T-12b: elapsed=301s must produce red escape %q (threshold is >300s, exclusive); not found\noutput:\n%s",
			redEsc, out)
	}
}

// -------------------------------------------------------------------
// T-12c: TestSubagentColour_ElapsedAt300s_Yellow
//
// When elapsed == 300s (exactly at threshold), the ⏱elapsed token must
// be wrapped in \x1b[33m (yellow), NOT red.
//
// Spec: elapsed ≤ 300s → yellow; only >300s → red.
//
// FAILS until probes/subagent.go emits {{color:yellow}} for elapsed ≤ 300s.
// -------------------------------------------------------------------
func TestSubagentColour_ElapsedAt300s_Yellow(t *testing.T) {
	th := colourThemeForStatusline()

	// Given: elapsed = 300s (exactly at threshold — must be yellow, not red).
	d := makeSubagentData(300)
	out := renderSubagentProbe(d, th)

	yellowEsc := "\x1b[33m"
	redEsc := "\x1b[31m"

	// Must contain yellow.
	if !strings.Contains(out, yellowEsc) {
		t.Errorf("T-12c: elapsed=300s (≤300s) must produce yellow escape %q; not found\noutput:\n%s",
			yellowEsc, out)
	}

	// Must NOT contain red for the ⏱ token.
	// We check that ⏱ is not immediately preceded by red escape.
	if strings.Contains(out, redEsc+"⏱") {
		t.Errorf("T-12c: elapsed=300s must NOT produce red escape before ⏱; found %q+⏱\noutput:\n%s",
			redEsc, out)
	}
}

// -------------------------------------------------------------------
// T-12d: TestSubagentColour_ElapsedUnder300s_Yellow
//
// When elapsed < 300s (e.g. 120s = 2 minutes), the ⏱elapsed token must
// be wrapped in \x1b[33m (yellow).
//
// FAILS until probes/subagent.go emits {{color:yellow}} for elapsed ≤ 300s.
// -------------------------------------------------------------------
func TestSubagentColour_ElapsedUnder300s_Yellow(t *testing.T) {
	th := colourThemeForStatusline()

	// Given: elapsed = 120s (2 minutes, well under 300s threshold).
	d := makeSubagentData(120)
	out := renderSubagentProbe(d, th)

	yellowEsc := "\x1b[33m"
	if !strings.Contains(out, yellowEsc) {
		t.Errorf("T-12d: elapsed=120s (<300s) must produce yellow escape %q; not found\noutput:\n%s",
			yellowEsc, out)
	}

	// ⏱ must appear after the yellow escape.
	if !strings.Contains(out, yellowEsc+"⏱") {
		t.Errorf("T-12d: ⏱ elapsed glyph must immediately follow yellow escape %q for elapsed≤300s; not found\noutput:\n%s",
			yellowEsc, out)
	}
}

// -------------------------------------------------------------------
// T-12e: TestSubagentColour_NoAnsiWhenDisabled
//
// Under Theme{} (AnsiEnabled=false), the subagent probe output must
// contain no ANSI escapes (T-13 regression applied to subagent line).
// -------------------------------------------------------------------
func TestSubagentColour_NoAnsiWhenDisabled(t *testing.T) {
	plainTheme := renderer.Theme{} // AnsiEnabled: false

	d := makeSubagentData(400) // elapsed > 300s: would be red if ANSI enabled
	out := renderSubagentProbe(d, plainTheme)

	if strings.Contains(out, "\x1b[") {
		t.Errorf("T-12e: subagent probe under Theme{} (ansi off) must contain no ANSI escapes; found \\x1b[ in output\noutput:\n%s", out)
	}
}
