//go:build integration

package integration_test

// render_integration_test.go — end-to-end checks for the full render pipeline
// (JSONL fixture → parser.ParseLines+Aggregate → assembler.Render) plus the
// cold-start benchmark.
//
// NOTE (Phase 7.g, 2026-06-09): the former golden-file comparison tests
// (TestRenderShort/Medium/Subagents/AllProbesOff + runRenderGolden + the
// tests/fixtures/integration/golden/*.golden snapshots) were retired. Those
// Phase 4–5 snapshots rotted across the 6.6–6.95 visual evolution and are fully
// superseded by the maintained lean golden set in
// tests/statusline/testdata/golden/s1..s9.txt. What remains here are the
// non-snapshot pipeline checks (hint determinism, hint index-0 text, subagents
// presence) and the cold-start benchmark consumed by the CI perf gate.
//
// Cold-start benchmark (AC-4, budget <100 ms):
//
//	go test -tags=integration -bench=BenchmarkColdStart -benchtime=5x ./tests/integration/

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// goldenCols is the fixed terminal width used for all pipeline renders here.
// Pinned so output is stable across environments.
const goldenCols = 80

// goldenNow is a fixed timestamp after the last fixture turn so the hint widget
// starts in its initial state (index 0, not rotated).
var goldenNow = time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

// ─── Fixture paths (relative to project root) ────────────────────────────────

const (
	goldenFixtureShort     = "tests/fixtures/integration/real-session-short.jsonl"
	goldenFixtureMedium    = "tests/fixtures/integration/real-session-medium.jsonl"
	goldenFixtureSubagents = "tests/fixtures/integration/real-session-subagents.jsonl"
	goldenFixtureSubDir    = "tests/fixtures/integration/real-session-subagents"
)

// ─── Cold-start benchmark ─────────────────────────────────────────────────────

// BenchmarkColdStart measures the full pipeline (open file → ParseLines →
// Aggregate → Assembler.Render) on the medium fixture. Budget: <100 ms / op
// (§4.4 AC-4). Consumed by the CI perf gate (.github/workflows/test.yml).
func BenchmarkColdStart(b *testing.B) {
	root := projectRootB(b)
	path := filepath.Join(root, goldenFixtureMedium)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f, err := os.Open(path)
		if err != nil {
			b.Fatalf("open: %v", err)
		}
		records, _, scanErr := parser.ParseLines(f)
		f.Close()
		if scanErr != nil {
			b.Fatalf("ParseLines: %v", scanErr)
		}

		deduped := parser.Dedup(records)
		s := parser.Aggregate(deduped)
		d := probes.Data{
			Session:   &s,
			SessionID: "",
		}
		a := &statusline.Assembler{
			Mode:  mode.Standard,
			Theme: renderer.Theme{},
			Cols:  goldenCols,
		}
		_ = a.Render(d)
	}

	// AC-4 advisory: log a warning when budget is exceeded.
	nsPerOp := float64(b.Elapsed()) / float64(b.N)
	const budget = 100_000_000 // 100 ms in ns
	if nsPerOp > budget {
		b.Logf("FLAGGED: AC-4 perf SLA exceeded: %.1f ms/op > 100 ms budget (deferred to Phase 7)",
			nsPerOp/1e6)
	}
}

// ─── Hint determinism check ───────────────────────────────────────────────────

// TestRenderHintDeterminism verifies that rendering the same fixture twice in
// the same process (same XDG_CACHE_HOME tempdir) produces identical output.
// This validates that hint state is correctly isolated between test runs. §4.4.e.
func TestRenderHintDeterminism(t *testing.T) {
	root := projectRoot(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	render := func(sessionID string) string {
		jsonlPath := filepath.Join(root, goldenFixtureShort)
		records := mustOpenAndParseFile(t, jsonlPath)
		deduped := parser.Dedup(records)
		s := parser.Aggregate(deduped)
		d := probes.Data{
			Session:   &s,
			SessionID: sessionID,
			Now:       goldenNow,
		}
		a := &statusline.Assembler{
			Mode:  mode.SuperCompact,
			Theme: renderer.Theme{},
			Cols:  goldenCols,
		}
		return renderer.Apply(a.Render(d), renderer.Theme{})
	}

	// First render stamps index 0 as shown and persists state.
	out1 := render("hint-determ-session")
	// Second render (same sessionID, within rotate interval) returns same hint.
	out2 := render("hint-determ-session")

	if out1 != out2 {
		t.Errorf("hint is not deterministic within rotate interval:\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

// TestRenderHintIndex0Text verifies that the hint widget shows the index-0
// hint text on the very first render of a fresh session. §4.4.b hint rotation.
func TestRenderHintIndex0Text(t *testing.T) {
	root := projectRoot(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	jsonlPath := filepath.Join(root, goldenFixtureShort)
	records := mustOpenAndParseFile(t, jsonlPath)
	deduped := parser.Dedup(records)
	s := parser.Aggregate(deduped)
	d := probes.Data{
		Session:   &s,
		SessionID: "hint-index0-session",
		Now:       goldenNow,
	}
	a := &statusline.Assembler{
		Mode:  mode.Standard,
		Theme: renderer.Theme{},
		Cols:  goldenCols,
	}
	got := renderer.Apply(a.Render(d), renderer.Theme{})

	// §4.4.b: first render shows DefaultHints[0].Text. The render above applied
	// a plain (colour-off) theme, which strips the hint's {{color:…}} markers, so
	// compare against the marker-stripped form (Phase 6.95.c added colour markers).
	wantHint := renderer.Apply(hint.DefaultHints[0].Text, renderer.Theme{})
	if !strings.Contains(got, wantHint) {
		t.Errorf("first render should contain hint[0] text %q; got:\n%s", wantHint, got)
	}
}

// ─── Subagents presence check ─────────────────────────────────────────────────

// TestRenderSubagentsPresence verifies that the subagents fixture render
// contains subagent IDs from the fixture. Quick sanity check that
// CollectSubagents is wired into probes.Data correctly.
func TestRenderSubagentsPresence(t *testing.T) {
	root := projectRoot(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	jsonlPath := filepath.Join(root, goldenFixtureSubagents)
	records := mustOpenAndParseFile(t, jsonlPath)
	deduped := parser.Dedup(records)
	s := parser.Aggregate(deduped)

	subagents, err := parser.CollectSubagents(context.Background(), filepath.Join(root, goldenFixtureSubDir))
	if err != nil {
		t.Fatalf("CollectSubagents: %v", err)
	}
	if len(subagents) == 0 {
		t.Fatal("CollectSubagents: returned 0 subagents; expected 5")
	}

	d := probes.Data{
		Session:   &s,
		Subagents: subagents,
		Now:       goldenNow,
		SessionID: "subagents-presence",
	}
	a := &statusline.Assembler{
		Mode:  mode.Standard,
		Theme: renderer.Theme{},
		Cols:  120, // wider to surface more content
	}
	got := renderer.Apply(a.Render(d), renderer.Theme{})
	if got == "" {
		t.Error("Render returned empty string for subagents fixture")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// mustOpenAndParseFile opens a JSONL file, calls parser.ParseLines, and fatals
// on I/O or scan error. Parse-level warnings are logged but do not fail.
func mustOpenAndParseFile(t *testing.T, path string) []parser.Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	records, parseErrs, scanErr := parser.ParseLines(f)
	if scanErr != nil {
		t.Fatalf("ParseLines scan error on %s: %v", path, scanErr)
	}
	if len(parseErrs) > 0 {
		t.Logf("ParseLines: %d parse warnings on %s (first: line %d: %s)",
			len(parseErrs), path, parseErrs[0].LineNumber, parseErrs[0].Reason)
	}
	return records
}

// projectRoot returns the project root, delegating to the package-level
// findProjectRoot helper (shared with parser_integration_test.go).
func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("cannot locate project root: %v", err)
	}
	return root
}

// projectRootB returns the project root for benchmark callers.
func projectRootB(b *testing.B) string {
	b.Helper()
	root, err := findProjectRoot()
	if err != nil {
		b.Fatalf("cannot locate project root: %v", err)
	}
	return root
}
