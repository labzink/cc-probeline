//go:build integration

package integration_test

// Integration tests for the parser core on real anonymized CC sessions.
// Run with: go test -tags=integration ./tests/integration/... -v
//
// Golden values are derived from scripts/session_stats.py (independent Python
// reference). Do NOT modify the expected constants without re-running the
// Python reference on the anonymized fixtures.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ─── Fixture path resolution ──────────────────────────────────────────────────

// findProjectRoot walks at most 2 levels up from cwd looking for go.mod.
// Sufficient for the current tests/integration/ package depth; revisit if the
// package is moved deeper.
// Under `go test ./tests/integration/` the cwd is the package directory
// (tests/integration/); under `go test ./...` from the repo root the cwd is
// the repo root itself.
func findProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Running from tests/integration/ — go up two levels.
	candidate := filepath.Join(wd, "..", "..")
	if isProjectRoot(candidate) {
		return filepath.Clean(candidate), nil
	}
	// Running from project root directly (go test ./...).
	if isProjectRoot(wd) {
		return wd, nil
	}
	return "", os.ErrNotExist
}

// isProjectRoot returns true when dir contains go.mod (project root marker).
func isProjectRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}

// fixturePath resolves a path relative to the project root.
// Accepts both *testing.T and *testing.B via the testing.TB interface.
func fixturePath(tb testing.TB, rel string) string {
	tb.Helper()
	root, err := findProjectRoot()
	if err != nil {
		tb.Fatalf("fixturePath: cannot locate project root: %v", err)
	}
	return filepath.Join(root, rel)
}

// benchSink prevents the compiler from eliminating the Aggregate call via
// dead-code elimination. Assigned inside BenchmarkParseMedium.
var benchSink parser.SessionStats

// ─── Fixture relative paths (resolved at runtime via projectRoot) ─────────────

const (
	fixtureShort        = "tests/fixtures/integration/real-session-short.jsonl"
	fixtureMedium       = "tests/fixtures/integration/real-session-medium.jsonl"
	fixtureSubagents    = "tests/fixtures/integration/real-session-subagents.jsonl"
	fixtureSubagentsDir = "tests/fixtures/integration/real-session-subagents"
)

// ─── Golden constants — Fixture 1: real-session-short ────────────────────────

const (
	shortTurnCount    = 21
	shortToolUseCount = 6

	shortFirstTimestamp = "2026-05-16T07:57:23.202Z"
	shortLastTimestamp  = "2026-05-16T08:11:33.825Z"

	shortTotalsInput       = 25
	shortTotalsOutput      = 23541
	shortTotalsCacheRead   = 1874297
	shortTotalsCacheCreate = 112251
)

// ─── Golden constants — Fixture 2: real-session-medium ───────────────────────

const (
	mediumTurnCount    = 25
	mediumToolUseCount = 4

	mediumFirstTimestamp = "2026-05-16T03:21:59.077Z"
	mediumLastTimestamp  = "2026-05-16T03:55:46.595Z"

	mediumTotalsInput       = 44
	mediumTotalsOutput      = 68767
	mediumTotalsCacheRead   = 2532884
	mediumTotalsCacheCreate = 253377
)

// ─── Golden constants — Fixture 3: real-session-subagents (orchestrator) ─────

const (
	subOrchestratorTurnCount    = 30
	subOrchestratorToolUseCount = 1

	subOrchestratorFirstTimestamp = "2026-05-16T04:18:53.470Z"
	subOrchestratorLastTimestamp  = "2026-05-16T05:29:08.491Z"

	subOrchestratorTotalsInput       = 74
	subOrchestratorTotalsOutput      = 62366
	subOrchestratorTotalsCacheRead   = 3007611
	subOrchestratorTotalsCacheCreate = 141416
)

// ─── Golden subagent table ────────────────────────────────────────────────────

// goldenSubagent is the expected stats for one subagent.
type goldenSubagent struct {
	agentID     string
	model       string // canonical key (canonicalModelKey output, "opus-4-7" etc.)
	turns       int
	tools       int
	input       int
	output      int
	cacheRead   int
	cacheCreate int
}

// goldenSubagents lists the 5 expected subagents ordered by AgentID (for
// map-based lookup — order-independent).
var goldenSubagents = []goldenSubagent{
	{"a09c2d1be526067ce", "sonnet-4-6", 16, 4, 18, 1104, 596380, 99006},
	{"a37d356d03020d6c4", "sonnet-4-6", 12, 6, 14, 2104, 338158, 42860},
	{"a8497b77be9dd6bb9", "opus-4-7", 8, 4, 13, 5013, 293390, 143034},
	{"abcb03ad99d90aed5", "sonnet-4-6", 25, 7, 8756, 1440, 1372166, 74273},
	{"adb67a0ff3c1d7283", "sonnet-4-6", 28, 6, 30, 1663, 1314838, 62788},
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// mustParseTime parses an RFC3339Nano timestamp string and fails the test if
// parsing fails. The Z suffix is handled by time.RFC3339Nano.
func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("mustParseTime: cannot parse %q: %v", s, err)
	}
	return ts.UTC()
}

// mustOpenAndParse opens a fixture (path relative to project root), calls
// ParseLines, and fatals on any I/O or scan error. parse-level errors are
// logged but do not fail the test (real sessions may produce parse warnings).
func mustOpenAndParse(t *testing.T, relPath string) []parser.Record {
	t.Helper()
	path := fixturePath(t, relPath)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open(%q): %v", path, err)
	}
	defer f.Close()

	records, parseErrs, scanErr := parser.ParseLines(f)
	if scanErr != nil {
		t.Fatalf("ParseLines scan error on %q: %v", path, scanErr)
	}
	if len(parseErrs) > 0 {
		t.Logf("ParseLines: %d parse warnings on %q (first: line %d: %s)",
			len(parseErrs), path, parseErrs[0].LineNumber, parseErrs[0].Reason)
	}
	return records
}

// assertTotals checks each token field of got against the expected values.
func assertTotals(t *testing.T, label string, got parser.TokenCounts, wantIn, wantOut, wantCR, wantCC int) {
	t.Helper()
	if got.Input != wantIn {
		t.Errorf("%s: Input = %d, want %d", label, got.Input, wantIn)
	}
	if got.Output != wantOut {
		t.Errorf("%s: Output = %d, want %d", label, got.Output, wantOut)
	}
	if got.CacheRead != wantCR {
		t.Errorf("%s: CacheRead = %d, want %d", label, got.CacheRead, wantCR)
	}
	if got.CacheCreate != wantCC {
		t.Errorf("%s: CacheCreate = %d, want %d", label, got.CacheCreate, wantCC)
	}
}

// ─── DRY helper for single-model basic session tests ─────────────────────────

// goldenBasicSession holds expected values for a single-model session test
// (ShortSession, MediumSession). For multi-model or subagent sessions, use
// the bespoke test body (see TestIntegration_SubagentsSession).
type goldenBasicSession struct {
	Fixture        string
	TurnCount      int
	ToolUseCount   int
	FirstTimestamp string // RFC3339Nano (Z suffix)
	LastTimestamp  string
	Totals         parser.TokenCounts
	ModelKey       string // canonicalModelKey output, e.g. "opus-4-7"
}

// runBasicSessionTest validates ParseLines+Aggregate for a basic single-model
// session: turn/tool counts, time window, totals, and PerModel entry.
func runBasicSessionTest(t *testing.T, g goldenBasicSession) {
	t.Helper()
	records := mustOpenAndParse(t, g.Fixture)
	stats := parser.Aggregate(records)

	if stats.TurnCount != g.TurnCount {
		t.Errorf("TurnCount = %d, want %d", stats.TurnCount, g.TurnCount)
	}
	if stats.ToolUseCount != g.ToolUseCount {
		t.Errorf("ToolUseCount = %d, want %d", stats.ToolUseCount, g.ToolUseCount)
	}

	wantFirst := mustParseTime(t, g.FirstTimestamp)
	wantLast := mustParseTime(t, g.LastTimestamp)
	if !stats.FirstTimestamp.Equal(wantFirst) {
		t.Errorf("FirstTimestamp = %v, want %v", stats.FirstTimestamp, wantFirst)
	}
	if !stats.LastTimestamp.Equal(wantLast) {
		t.Errorf("LastTimestamp = %v, want %v", stats.LastTimestamp, wantLast)
	}

	assertTotals(t, "Totals", stats.Totals,
		g.Totals.Input, g.Totals.Output, g.Totals.CacheRead, g.Totals.CacheCreate)

	pm, ok := stats.PerModel[g.ModelKey]
	if !ok {
		t.Fatalf("PerModel[%q] missing; keys present: %v", g.ModelKey, modelKeys(stats.PerModel))
	}
	assertTotals(t, "PerModel["+g.ModelKey+"]", pm,
		g.Totals.Input, g.Totals.Output, g.Totals.CacheRead, g.Totals.CacheCreate)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestIntegration_ShortSession validates ParseLines+Aggregate on the short
// fixture (21 turns, opus-only, no subagents).
func TestIntegration_ShortSession(t *testing.T) {
	runBasicSessionTest(t, goldenBasicSession{
		Fixture:        fixtureShort,
		TurnCount:      shortTurnCount,
		ToolUseCount:   shortToolUseCount,
		FirstTimestamp: shortFirstTimestamp,
		LastTimestamp:  shortLastTimestamp,
		Totals: parser.TokenCounts{
			Input:       shortTotalsInput,
			Output:      shortTotalsOutput,
			CacheRead:   shortTotalsCacheRead,
			CacheCreate: shortTotalsCacheCreate,
		},
		ModelKey: "opus-4-7",
	})
}

// TestIntegration_MediumSession validates ParseLines+Aggregate on the medium
// fixture (25 turns, opus-only, longer session with higher cache traffic).
func TestIntegration_MediumSession(t *testing.T) {
	runBasicSessionTest(t, goldenBasicSession{
		Fixture:        fixtureMedium,
		TurnCount:      mediumTurnCount,
		ToolUseCount:   mediumToolUseCount,
		FirstTimestamp: mediumFirstTimestamp,
		LastTimestamp:  mediumLastTimestamp,
		Totals: parser.TokenCounts{
			Input:       mediumTotalsInput,
			Output:      mediumTotalsOutput,
			CacheRead:   mediumTotalsCacheRead,
			CacheCreate: mediumTotalsCacheCreate,
		},
		ModelKey: "opus-4-7",
	})
}

// TestIntegration_SubagentsSession validates:
// (a) orchestrator aggregate from the main JSONL file,
// (b) CollectSubagents — counts, per-agent model/turns/tools/tokens.
func TestIntegration_SubagentsSession(t *testing.T) {
	// --- Part (a): orchestrator aggregate ---
	records := mustOpenAndParse(t, fixtureSubagents)
	stats := parser.Aggregate(records)

	if stats.TurnCount != subOrchestratorTurnCount {
		t.Errorf("orchestrator TurnCount = %d, want %d", stats.TurnCount, subOrchestratorTurnCount)
	}
	if stats.ToolUseCount != subOrchestratorToolUseCount {
		t.Errorf("orchestrator ToolUseCount = %d, want %d", stats.ToolUseCount, subOrchestratorToolUseCount)
	}

	wantFirst := mustParseTime(t, subOrchestratorFirstTimestamp)
	wantLast := mustParseTime(t, subOrchestratorLastTimestamp)
	if !stats.FirstTimestamp.Equal(wantFirst) {
		t.Errorf("orchestrator FirstTimestamp = %v, want %v", stats.FirstTimestamp, wantFirst)
	}
	if !stats.LastTimestamp.Equal(wantLast) {
		t.Errorf("orchestrator LastTimestamp = %v, want %v", stats.LastTimestamp, wantLast)
	}

	assertTotals(t, "orchestrator Totals",
		stats.Totals,
		subOrchestratorTotalsInput, subOrchestratorTotalsOutput,
		subOrchestratorTotalsCacheRead, subOrchestratorTotalsCacheCreate,
	)

	const opusKey = "opus-4-7"
	pm, ok := stats.PerModel[opusKey]
	if !ok {
		t.Fatalf("orchestrator PerModel[%q] missing; keys present: %v", opusKey, modelKeys(stats.PerModel))
	}
	assertTotals(t, "orchestrator PerModel["+opusKey+"]",
		pm,
		subOrchestratorTotalsInput, subOrchestratorTotalsOutput,
		subOrchestratorTotalsCacheRead, subOrchestratorTotalsCacheCreate,
	)

	// --- Part (b): CollectSubagents ---
	subagents, err := parser.CollectSubagents(context.Background(), fixturePath(t, fixtureSubagentsDir))
	if err != nil {
		t.Fatalf("CollectSubagents: %v", err)
	}

	const wantCount = 5
	if len(subagents) != wantCount {
		t.Fatalf("CollectSubagents: got %d subagents, want %d", len(subagents), wantCount)
	}

	// Build a map by AgentID for order-independent lookup
	// (CollectSubagents sorts by LastTimestamp DESC).
	byID := make(map[string]parser.SubagentStats, len(subagents))
	for _, s := range subagents {
		byID[s.AgentID] = s
	}

	for _, want := range goldenSubagents {
		got, found := byID[want.agentID]
		if !found {
			t.Errorf("subagent %q not found in CollectSubagents result", want.agentID)
			continue
		}

		label := "subagent[" + want.agentID + "]"

		if got.Model != want.model {
			t.Errorf("%s: Model = %q, want %q", label, got.Model, want.model)
		}
		if got.TurnCount != want.turns {
			t.Errorf("%s: TurnCount = %d, want %d", label, got.TurnCount, want.turns)
		}
		if got.ToolUseCount != want.tools {
			t.Errorf("%s: ToolUseCount = %d, want %d", label, got.ToolUseCount, want.tools)
		}

		assertTotals(t, label+" Tokens",
			got.Tokens,
			want.input, want.output, want.cacheRead, want.cacheCreate,
		)
	}
}

// BenchmarkParseMedium measures ParseLines+Aggregate throughput on the ~355 KB
// medium fixture. Budget: < 100 ms / op.
// b.ResetTimer() is called after os.Open so file-open latency is excluded.
// Each iteration seeks to the beginning so parse work is real (not cached).
func BenchmarkParseMedium(b *testing.B) {
	path := fixturePath(b, fixtureMedium)
	f, err := os.Open(path)
	if err != nil {
		b.Fatalf("os.Open(%q): %v", path, err)
	}
	defer f.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := f.Seek(0, 0); err != nil {
			b.Fatalf("Seek: %v", err)
		}
		records, _, err := parser.ParseLines(f)
		if err != nil {
			b.Fatalf("ParseLines: %v", err)
		}
		benchSink = parser.Aggregate(records)
	}
}

// ─── Internal utilities ───────────────────────────────────────────────────────

// modelKeys extracts the keys of a PerModel map for use in diagnostic messages.
func modelKeys(m map[string]parser.TokenCounts) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
