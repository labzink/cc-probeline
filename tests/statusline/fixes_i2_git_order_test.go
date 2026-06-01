// Package statusline_test — RED tests for Phase 6.8 FIXES: I2 git order in line1.
//
// Root cause (from review-consolidated.md I2):
//   The assembler uses sortByPriority=false for BOTH line0 AND line1.
//   T-21 originally required registry order only for line0.
//   As a result, line1 lost its Priority-based sort. git (P=2) ended up
//   leftmost instead of rightmost because it is registered second in Line1Registry
//   (after model P=?), but without Priority sort it just takes registry position.
//
// Expected line1 order by Priority (ascending = leftmost):
//   ModelProbe (P=1 ... check actual) → CtxProbe → CostProbe → TimeProbe → GitProbe(P=2)
//
// Actual Line1Registry order: [model, git, ctx, cost, time] (registry.go).
// git Priority = 2, ctx/cost/time Priority = 1.
// With sortByPriority=true for line1, git (P=2) should appear AFTER ctx/cost/time (P=1).
// With sortByPriority=false, git stays at registry position 2 (second) = leftmost after model.
//
// Fix vector: restore sortByPriority=true for line1 (revert the over-reach from T-21 fix).
//
// Production path: Assembler.Render → buildProbeEntries(Line1Registry, d, sortByPriority=true).
//
// RED: with sortByPriority=false, git appears before ctx/cost/time → test fails.
package statusline_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// TestLine1_GitRightOfCtxCostTime (I2 / T-21 over-reach) verifies that in line1,
// the git segment appears to the RIGHT of ctx, cost, and time segments.
//
// Context: Line1Registry = [model(P=?), git(P=2), ctx(P=1), cost(P=1), time(P=1)].
// With Priority sort on line1: P=1 probes (ctx, cost, time) appear left of P=2 (git).
// Without Priority sort: git appears at registry position 2 — leftmost after model.
//
// Setup: use real Line1Registry probes with data that makes all of them visible.
// Use fake probes with known priorities to avoid real-probe data dependencies.
//
// Given:
//   - fakeModel (P=1, out="sonnet")
//   - fakeGit   (P=2, out="main+3")    ← must be RIGHTMOST
//   - fakeCtx   (P=1, out="128K/200K")
//   - fakeCost  (P=1, out="$3.50")
//   - fakeTime  (P=1, out="01:30")
//
// Expected: "main+3" appears AFTER "128K/200K", "$3.50", and "01:30" in line1.
//
// RED: without sortByPriority for line1, git(P=2) takes registry slot 2 and
// appears between model and ctx → test fails.
func TestLine1_GitRightOfCtxCostTime(t *testing.T) {
	// We swap Line1Registry with fake probes that mirror real priorities.
	// model: P=1 (ModelProbe real priority)
	// git:   P=2 (GitProbe real priority)
	// ctx:   P=1 (CtxProbe real priority)
	// cost:  P=1 (CostProbe real priority)
	// time:  P=1 (TimeProbe real priority)
	//
	// Registry order (matches real Line1Registry order): model, git, ctx, cost, time.
	// With Priority sort: model+ctx+cost+time appear before git (all P=1 < P=2).
	fakeModel := &fakeProbe{name: "model", priority: 1, visible: true, out: "sonnet"}
	fakeGit := &fakeProbe{name: "git", priority: 2, visible: true, out: "main+3"}
	fakeCtx := &fakeProbe{name: "ctx", priority: 1, visible: true, out: "128K/200K"}
	fakeCost := &fakeProbe{name: "cost", priority: 1, visible: true, out: "$3.50"}
	fakeTime := &fakeProbe{name: "time", priority: 1, visible: true, out: "01:30"}

	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e@x"}})
	// Register in canonical registry order: model, git, ctx, cost, time.
	swapLine1(t, []probes.Probe{fakeModel, fakeGit, fakeCtx, fakeCost, fakeTime})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "cache:0"}})

	a := &statusline.Assembler{
		Mode:   mode.SuperCompact,
		Theme:  renderer.Theme{AnsiEnabled: false},
		Cols:   200, // wide enough to fit all probes at Full level
		Config: probes.Config{},
	}

	d := probes.Data{
		Stdin: stdin.Payload{
			Model: stdin.Model{ID: "claude-sonnet-4-6"},
		},
		Git: &parser.GitStatus{
			Branch:        "main",
			ModifiedCount: 3,
		},
		TerminalCols: 200,
	}

	out := a.Render(d)

	lines := strings.SplitN(out, "\n", 4)
	if len(lines) < 2 {
		t.Fatalf("I2: expected at least 2 lines in output, got %d; output: %q", len(lines), out)
	}
	line1 := lines[1]

	posGit := strings.Index(line1, "main+3")
	posCtx := strings.Index(line1, "128K/200K")
	posCost := strings.Index(line1, "$3.50")
	posTime := strings.Index(line1, "01:30")

	if posGit < 0 {
		t.Fatalf("I2: 'main+3' (git) not found in line1=%q", line1)
	}

	// All three P=1 probes (ctx, cost, time) must appear to the left of git(P=2).
	if posCtx >= 0 && posCtx >= posGit {
		t.Errorf("I2: ctx ('128K/200K') must appear LEFT of git ('main+3') in line1;\n"+
			"  posCtx=%d, posGit=%d, line1=%q\n"+
			"  FIX: restore sortByPriority=true for line1 in assembler.buildProbeEntries",
			posCtx, posGit, line1)
	}
	if posCost >= 0 && posCost >= posGit {
		t.Errorf("I2: cost ('$3.50') must appear LEFT of git ('main+3') in line1;\n"+
			"  posCost=%d, posGit=%d, line1=%q\n"+
			"  FIX: restore sortByPriority=true for line1 in assembler.buildProbeEntries",
			posCost, posGit, line1)
	}
	if posTime >= 0 && posTime >= posGit {
		t.Errorf("I2: time ('01:30') must appear LEFT of git ('main+3') in line1;\n"+
			"  posTime=%d, posGit=%d, line1=%q\n"+
			"  FIX: restore sortByPriority=true for line1 in assembler.buildProbeEntries",
			posTime, posGit, line1)
	}
}
