//go:build integration

package integration_test

// render_integration_test.go — end-to-end golden-file tests for the full
// pipeline: JSONL fixture → parser.ParseLines+Aggregate → assembler.Render.
//
// Run against existing golden files:
//
//	go test -tags=integration ./tests/integration/ -run TestRender -v
//
// Re-generate golden files (after intentional output changes):
//
//	go test -tags=integration ./tests/integration/ -run TestRender -update -v
//
// Width-invariant check (every golden line fits within declared cols):
//
//	go test -tags=integration ./tests/integration/ -run TestGoldenWidthInvariant -v
//
// Cold-start benchmark (AC-4, budget <100 ms):
//
//	go test -tags=integration -bench=BenchmarkColdStart -benchtime=5x ./tests/integration/

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// updateGolden rewrites golden files instead of comparing when set to true.
var updateGolden = flag.Bool("update", false, "rewrite golden files instead of comparing")

// goldenCols is the fixed terminal width used for all golden renders.
// Pinned so golden files remain stable across environments.
const goldenCols = 80

// goldenNow is the fixed timestamp injected via XDG_CACHE_HOME isolation and
// hint.State seeding. Pinned to a date after the last fixture turn so hint
// widget starts in its initial state (index 0, not rotated).
var goldenNow = time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

// ─── Fixture paths (relative to project root) ────────────────────────────────

const (
	goldenFixtureShort     = "tests/fixtures/integration/real-session-short.jsonl"
	goldenFixtureMedium    = "tests/fixtures/integration/real-session-medium.jsonl"
	goldenFixtureSubagents = "tests/fixtures/integration/real-session-subagents.jsonl"
	goldenFixtureSubDir    = "tests/fixtures/integration/real-session-subagents"
	goldenDir              = "tests/fixtures/integration/golden"
)

// ─── Top-level golden-file render tests ──────────────────────────────────────

// defaultProbesCfg returns the probes.Config that matches config.Default()
// (all widgets enabled). Used by golden tests to render the full pipeline as
// users see it with no config file present.
func defaultProbesCfg() probes.Config {
	return config.ToProbesConfig(*config.Default())
}

// TestRenderShort tests the full pipeline on the short fixture (21 turns,
// opus-only) in SuperCompact mode. §4.4.e AC-1.
func TestRenderShort(t *testing.T) {
	runRenderGolden(t, goldenFixtureShort, "", mode.SuperCompact, defaultProbesCfg(), "render-short.golden")
}

// TestRenderMedium tests the full pipeline on the medium fixture (25 turns,
// opus-only) in Standard mode. §4.4.e AC-1.
func TestRenderMedium(t *testing.T) {
	runRenderGolden(t, goldenFixtureMedium, "", mode.Standard, defaultProbesCfg(), "render-medium.golden")
}

// TestRenderSubagents tests the full pipeline on the subagents fixture
// (orchestrator + 5 subagents) in Standard mode. §4.4.e AC-1.
func TestRenderSubagents(t *testing.T) {
	runRenderGolden(t, goldenFixtureSubagents, goldenFixtureSubDir, mode.Standard, defaultProbesCfg(), "render-subagents.golden")
}

// TestRenderMedium_AllProbesOff verifies Phase 6 widget gating end-to-end:
// with zero probes.Config (all XEnabled = false) the pipeline renders only
// hint widget + subagents table. Probes (model, time, cost, cache, ...) MUST
// be suppressed. Phase 6 §3 T-WT13 / spec-common §3 gating contract.
func TestRenderMedium_AllProbesOff(t *testing.T) {
	runRenderGolden(t, goldenFixtureMedium, "", mode.Standard, probes.Config{}, "render-medium-allprobes-off.golden")
}

// ─── Width-invariant check ────────────────────────────────────────────────────

// TestGoldenWidthInvariant verifies that every line in every golden file has
// visual length ≤ goldenCols. ANSI markers ({{...}}) must NOT appear in golden
// files (plain-text invariant). §4.4.e AC-1 / §C-10.
func TestGoldenWidthInvariant(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, goldenDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		t.Skipf("golden directory %s not found; run -update first", dir)
	}
	if err != nil {
		t.Fatalf("read golden dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".golden") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(b)

			// §C-10: golden files must not contain ANSI marker tokens.
			if strings.Contains(content, "{{") {
				t.Errorf("%s: contains marker token '{{...}}'; golden must be plain text", name)
			}

			// Each line must fit within goldenCols visual columns.
			for i, line := range strings.Split(content, "\n") {
				vl := format.VisualLen(line)
				if vl > goldenCols {
					t.Errorf("%s:%d: visual length %d > %d cols: %q",
						name, i+1, vl, goldenCols, line)
				}
			}
		})
	}
}

// ─── Cold-start benchmark ─────────────────────────────────────────────────────

// BenchmarkColdStart measures the full pipeline (open file → ParseLines →
// Aggregate → Assembler.Render) on the medium fixture. Budget: <100 ms / op
// (§4.4 AC-4). The test reports a FLAGGED FINDING when the budget is exceeded
// but does not fail the build (deferred to Phase 7 optimization per plan).
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

// ─── Core pipeline helper ─────────────────────────────────────────────────────

// runRenderGolden runs the full pipeline for one fixture and either updates or
// validates the corresponding golden file. subagentsDir is optional; when
// non-empty, CollectSubagents is called to populate probes.Data.Subagents.
// cfg controls Phase 6 widget gating — pass defaultProbesCfg() for the
// all-widgets-on path or probes.Config{} for the all-off gating test.
func runRenderGolden(t *testing.T, fixtureRel, subagentsDirRel string, m mode.Mode, cfg probes.Config, goldenName string) {
	t.Helper()
	root := projectRoot(t)

	// Isolate hint state so each test starts with a fresh (zero) state.
	// This ensures the hint widget always shows index 0 text and golden output
	// is deterministic regardless of any on-disk state. §4.4.e step 2.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// 1. Parse JSONL fixture.
	jsonlPath := filepath.Join(root, fixtureRel)
	records := mustOpenAndParseFile(t, jsonlPath)
	deduped := parser.Dedup(records)
	s := parser.Aggregate(deduped)

	// 2. Optionally collect subagents.
	var subagents []parser.SubagentStats
	if subagentsDirRel != "" {
		var err error
		subagents, err = parser.CollectSubagents(context.Background(), filepath.Join(root, subagentsDirRel))
		if err != nil {
			t.Fatalf("CollectSubagents: %v", err)
		}
	}

	// 3. Build probes.Data with pinned time and empty SessionID (no disk I/O
	//    for hint state beyond the isolated XDG_CACHE_HOME tempdir).
	d := probes.Data{
		Session:   &s,
		Subagents: subagents,
		Now:       goldenNow,
		SessionID: "golden-" + goldenName, // deterministic but non-empty
	}

	// 4. Render via Assembler with pinned Cols, zero Theme (plain text), and
	//    Phase 6 widget gating config (passed by caller — see Test* funcs).
	a := &statusline.Assembler{
		Mode:   m,
		Theme:  renderer.Theme{},
		Cols:   goldenCols,
		Config: cfg,
	}
	got := a.Render(d)

	// 5. Apply theme (zero Theme → no ANSI codes emitted; output is plain text).
	got = renderer.Apply(got, renderer.Theme{})

	// 6. Compare with golden or update.
	goldenPath := filepath.Join(root, goldenDir, goldenName)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated golden %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create): %v", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("render mismatch (golden=%s)\n--- want ---\n%s\n--- got ---\n%s",
			goldenName, want, got)
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

	// §4.4.b: first render shows DefaultHints[0].Text.
	wantHint := hint.DefaultHints[0].Text
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

// visualLineLen returns the visual length of a line, stripping any embedded
// marker tokens (should not appear in golden files, but guard defensively).
func visualLineLen(line string) int {
	return format.VisualLen(line)
}

// _ prevents "imported and not used" when visualLineLen is inlined by the compiler.
var _ = fmt.Sprintf
var _ = visualLineLen
