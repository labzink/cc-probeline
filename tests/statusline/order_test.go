// Package statusline_test — RED tests for Phase 6.8.e probe order (T-E1).
//
// T-E1 (TestProbeOrder_RegistryNotPriority): line0 must render probes in
// registry order (email → project → quota), NOT sorted by Priority().
//
// Background: Line0Registry is [email(P=2), project(P=2), quota(P=1)].
// The current buildProbeEntries sorts entries by Priority ascending, so
// quota (P=1) is moved before email and project (P=2) — violating registry
// order. After the fix, sort.SliceStable is removed from buildProbeEntries
// and probes appear in registry order.
//
// The test also verifies the collapse semantics: when cols is narrowed so
// that not all probes can render at Full level, quota (P=1, lowest collapse
// priority) must collapse FIRST (i.e. it is the most disposable probe for
// the FitLine downgrade passes), while email and project (P=2) stay visible
// longer.
package statusline_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// ---------------------------------------------------------------------------
// T-E1: TestProbeOrder_RegistryNotPriority
// Spec: T-21 — line0 order = registry (email→project→quota); Priority only
// controls FitLine collapse, not left-to-right position.
// ---------------------------------------------------------------------------

// makeOrderAssembler returns an Assembler with the given col width and no ANSI.
func makeOrderAssembler(cols int) *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   mode.SuperCompact,
		Theme:  renderer.Theme{AnsiEnabled: false},
		Cols:   cols,
		Config: probes.Config{},
	}
}

// makeOrderData builds a probes.Data snapshot with live RateLimits so that
// QuotaProbe.Visible returns true. Email and project have no data-driven
// preconditions beyond Config flags (set via swapLine0 in callers).
func makeOrderData() probes.Data {
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 42},
		SevenDay: stdin.RateWindow{UsedPercentage: 20},
	}
	return probes.Data{
		Stdin:        stdin.Payload{RateLimits: rl},
		TerminalCols: 200,
	}
}

// TestProbeOrder_RegistryNotPriority verifies that the assembler renders
// line0 probes in registry order (email → project → quota), not in Priority
// order (quota P=1 would come before email/project P=2 if sorted).
//
// Given: Line0Registry = [email(P=2), project(P=2), quota(P=1)] (real registry
//
//	unchanged — swapLine0 replaces with fake probes at the same priorities).
//
// When:  Assembler.Render is called with wide cols (200) so all three are visible.
// Then:  In line0, email appears to the LEFT of quota.
func TestProbeOrder_RegistryNotPriority(t *testing.T) {
	// Use fake probes with the same Priority values as real probes.
	// email P=2, project P=2, quota P=1 — mirrors real registry priorities.
	fakeEmail := &fakeProbe{name: "email", priority: 2, visible: true, out: "user@example.com"}
	fakeProject := &fakeProbe{name: "project", priority: 2, visible: true, out: "myproject"}
	fakeQuota := &fakeProbe{name: "quota", priority: 1, visible: true, out: "42%·20%"}

	// Register in canonical registry order: email, project, quota.
	swapLine0(t, []probes.Probe{fakeEmail, fakeProject, fakeQuota})
	swapLine1(t, []probes.Probe{})
	swapLine2(t, []probes.Probe{})

	a := makeOrderAssembler(200)
	d := makeOrderData()
	out := a.Render(d)

	// Extract line0 (first line of the output).
	lines := strings.SplitN(out, "\n", 2)
	if len(lines) == 0 {
		t.Fatalf("ProbeOrder: Render() returned empty output")
	}
	line0 := lines[0]

	// Positions of each probe's output within line0.
	posEmail := strings.Index(line0, "user@example.com")
	posProject := strings.Index(line0, "myproject")
	posQuota := strings.Index(line0, "42%·20%")

	if posEmail < 0 {
		t.Fatalf("ProbeOrder: 'user@example.com' not found in line0=%q", line0)
	}
	if posProject < 0 {
		t.Fatalf("ProbeOrder: 'myproject' not found in line0=%q", line0)
	}
	if posQuota < 0 {
		t.Fatalf("ProbeOrder: '42%%·20%%' not found in line0=%q", line0)
	}

	// T-21: registry order → email LEFT of quota, project LEFT of quota.
	// If sorted by priority (current behaviour), quota (P=1) appears before
	// email/project (P=2), which violates registry order → test fails RED.
	if posEmail >= posQuota {
		t.Errorf("ProbeOrder: email must appear before quota in line0 (registry order);\n"+
			"  posEmail=%d posQuota=%d line0=%q", posEmail, posQuota, line0)
	}
	if posProject >= posQuota {
		t.Errorf("ProbeOrder: project must appear before quota in line0 (registry order);\n"+
			"  posProject=%d posQuota=%d line0=%q", posProject, posQuota, line0)
	}
	// email before project (both P=2; registry-stable order is preserved).
	if posEmail >= posProject {
		t.Errorf("ProbeOrder: email must appear before project in line0 (registry order);\n"+
			"  posEmail=%d posProject=%d line0=%q", posEmail, posProject, line0)
	}
}

// TestProbeOrder_NarrowCols_RegistryOrder verifies that even when FitLine
// triggers downgrade passes (narrow cols), the LEFT-TO-RIGHT render position
// still follows registry order (email → project → quota), not priority order.
//
// FitLine downgrade table (truncate.go levelForPass):
//
//	pass=1: P0→Full, P1→Full, P2→Compact, P3+→Compact
//
// At pass=1, quota (P=1) stays Full while email/project (P=2) downgrade to
// Compact. So quota's Full text ("42%·20%") should remain, but must appear
// AFTER email and project in the output (registry order), not BEFORE them.
//
// Currently (priority-sort bug), quota appears at position 0 (leftmost).
// After fix, email appears leftmost, quota appears rightmost — even at
// narrow cols where quota retains Full and email/project show Compact.
func TestProbeOrder_NarrowCols_RegistryOrder(t *testing.T) {
	fakeEmail := &fakeProbe{
		name: "email", priority: 2, visible: true,
		out: "user@example.com", compact: "usr", minimal: "usr",
	}
	fakeProject := &fakeProbe{
		name: "project", priority: 2, visible: true,
		out: "myproject-long", compact: "prj", minimal: "prj",
	}
	// quota P=1 → stays Full longer in FitLine passes.
	fakeQuota := &fakeProbe{
		name: "quota", priority: 1, visible: true,
		out: "42%·20%", compact: "42%", minimal: "",
	}

	swapLine0(t, []probes.Probe{fakeEmail, fakeProject, fakeQuota})
	swapLine1(t, []probes.Probe{})
	swapLine2(t, []probes.Probe{})

	// Narrow enough to exceed Full-all capacity but wide enough that pass=1
	// fits (quota Full + email Compact + project Compact).
	// Full widths: "user@example.com"(16)+"myproject-long"(14)+"42%·20%"(7) = 37 + seps(6) = 43.
	// At pass=1: email→"usr"(3)+project→"prj"(3)+quota→"42%·20%"(7) = 13+seps(6) = 19, fits in 30.
	a := makeOrderAssembler(30)
	d := makeOrderData()
	out := a.Render(d)

	lines := strings.SplitN(out, "\n", 2)
	if len(lines) == 0 {
		t.Fatalf("NarrowCols: Render() returned empty output")
	}
	line0 := lines[0]

	posEmail := strings.Index(line0, "usr")
	posProject := strings.Index(line0, "prj")
	posQuota := strings.Index(line0, "42%")

	if posEmail < 0 {
		t.Fatalf("NarrowCols: email compact 'usr' not found in line0=%q", line0)
	}
	if posProject < 0 {
		t.Fatalf("NarrowCols: project compact 'prj' not found in line0=%q", line0)
	}
	if posQuota < 0 {
		t.Fatalf("NarrowCols: quota '42%%' not found in line0=%q", line0)
	}

	// Registry order must hold even at narrow cols: email BEFORE quota.
	// Current bug: priority sort puts quota (P=1) first → posQuota=0, posEmail>0 → FAIL.
	if posEmail >= posQuota {
		t.Errorf("NarrowCols: email must appear before quota (registry order);\n"+
			"  posEmail=%d posQuota=%d line0=%q", posEmail, posQuota, line0)
	}
	if posProject >= posQuota {
		t.Errorf("NarrowCols: project must appear before quota (registry order);\n"+
			"  posProject=%d posQuota=%d line0=%q", posProject, posQuota, line0)
	}
}
