// Package parser_test contains RED tests for Phase 3.4 — subagent collector.
// Contract: plans/concepts/phase-3-step4-concept.md §4–§10.
// API:
//
//	parser.CollectSubagents(sessionDir string) ([]SubagentStats, error)
//	parser.SubagentStats{...}
package parser_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// Local helper
// ---------------------------------------------------------------------------

// fixtureSubagentDir returns the path to tests/fixtures/jsonl/subagents/.
func fixtureSubagentDir(t *testing.T) string {
	t.Helper()
	// Walk up from the test binary's working directory to find the fixture dir.
	// Under `go test ./tests/parser/` the cwd is the package directory.
	// The repo root is two levels up: tests/parser -> tests -> <root>.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("fixtureSubagentDir: getwd: %v", err)
	}
	// Try <wd>/../../tests/fixtures/jsonl/subagents (running from tests/parser/).
	candidate := filepath.Join(wd, "..", "..", "tests", "fixtures", "jsonl", "subagents")
	if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
		return candidate
	}
	// Fallback: repo root is the cwd itself (e.g. go test ./... from root).
	candidate = filepath.Join(wd, "tests", "fixtures", "jsonl", "subagents")
	if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
		return candidate
	}
	t.Fatalf("fixtureSubagentDir: cannot locate tests/fixtures/jsonl/subagents from %s", wd)
	return ""
}

// setupSessionDir creates a temporary sessionDir with a subagents/ sub-directory,
// copies the named fixture files into it, and returns the sessionDir path.
// fixtureNames is a list of base names to copy from tests/fixtures/jsonl/subagents/.
func setupSessionDir(t *testing.T, fixtureNames []string) string {
	t.Helper()
	srcDir := fixtureSubagentDir(t)
	sessionDir := t.TempDir()
	subDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatalf("setupSessionDir: mkdir %q: %v", subDir, err)
	}
	for _, name := range fixtureNames {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(subDir, name)
		copyFile(t, src, dst)
	}
	return sessionDir
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("copyFile: open %q: %v", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("copyFile: create %q: %v", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copyFile: copy %q -> %q: %v", src, dst, err)
	}
}

// parseTimeUTC parses an RFC3339Nano timestamp string and returns UTC time.
// Fatals the test on parse error.
func parseTimeUTC(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parseTimeUTC(%q): %v", s, err)
	}
	return ts.UTC()
}

// findByAgentID returns the SubagentStats with the given AgentID, or fails the test.
func findByAgentID(t *testing.T, stats []parser.SubagentStats, id string) parser.SubagentStats {
	t.Helper()
	for _, s := range stats {
		if s.AgentID == id {
			return s
		}
	}
	t.Fatalf("findByAgentID: no SubagentStats with AgentID=%q in slice of len %d", id, len(stats))
	return parser.SubagentStats{}
}

// ---------------------------------------------------------------------------
// T1 — Happy_TwoAgents
// sessionDir with aa1+bb2 fixtures; all fields verified for each agent.
// Concept §10 acceptance criterion #1, #3, #4.
// ---------------------------------------------------------------------------

// TestCollectSubagents_Happy_TwoAgents verifies the nominal case: two subagent
// fixtures with valid JSONL and meta.json produce correct SubagentStats.
func TestCollectSubagents_Happy_TwoAgents(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{
		"agent-aa1.jsonl", "agent-aa1.meta.json",
		"agent-bb2.jsonl", "agent-bb2.meta.json",
	})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}

	// --- Verify aa1 ---
	aa1 := findByAgentID(t, got, "aa1")

	if aa1.AgentType != "general-purpose" {
		t.Errorf("aa1.AgentType=%q, want %q", aa1.AgentType, "general-purpose")
	}
	if aa1.Description != "Test fixture A" {
		t.Errorf("aa1.Description=%q, want %q", aa1.Description, "Test fixture A")
	}
	if aa1.Model != "sonnet-4-6" {
		t.Errorf("aa1.Model=%q, want %q", aa1.Model, "sonnet-4-6")
	}
	if aa1.Tokens.Input != 6000 {
		t.Errorf("aa1.Tokens.Input=%d, want 6000", aa1.Tokens.Input)
	}
	if aa1.Tokens.Output != 1200 {
		t.Errorf("aa1.Tokens.Output=%d, want 1200", aa1.Tokens.Output)
	}
	if aa1.TurnCount != 5 {
		t.Errorf("aa1.TurnCount=%d, want 5", aa1.TurnCount)
	}
	if aa1.ToolUseCount != 4 {
		t.Errorf("aa1.ToolUseCount=%d, want 4", aa1.ToolUseCount)
	}
	if aa1.LastTool != "Write" {
		t.Errorf("aa1.LastTool=%q, want %q", aa1.LastTool, "Write")
	}
	wantAA1First := parseTimeUTC(t, "2026-05-15T10:00:00.000000000Z")
	if !aa1.FirstTimestamp.Equal(wantAA1First) {
		t.Errorf("aa1.FirstTimestamp=%v, want %v", aa1.FirstTimestamp, wantAA1First)
	}
	wantAA1Last := parseTimeUTC(t, "2026-05-15T10:04:00.000000000Z")
	if !aa1.LastTimestamp.Equal(wantAA1Last) {
		t.Errorf("aa1.LastTimestamp=%v, want %v", aa1.LastTimestamp, wantAA1Last)
	}
	if aa1.JSONLPath == "" {
		t.Error("aa1.JSONLPath: want non-empty")
	}

	// --- Verify bb2 ---
	bb2 := findByAgentID(t, got, "bb2")

	if bb2.AgentType != "code-reviewer" {
		t.Errorf("bb2.AgentType=%q, want %q", bb2.AgentType, "code-reviewer")
	}
	if bb2.Description != "Test fixture B" {
		t.Errorf("bb2.Description=%q, want %q", bb2.Description, "Test fixture B")
	}
	if bb2.Model != "opus-4-7" {
		t.Errorf("bb2.Model=%q, want %q", bb2.Model, "opus-4-7")
	}
	if bb2.Tokens.Input != 4100 {
		t.Errorf("bb2.Tokens.Input=%d, want 4100", bb2.Tokens.Input)
	}
	if bb2.Tokens.Output != 820 {
		t.Errorf("bb2.Tokens.Output=%d, want 820", bb2.Tokens.Output)
	}
	if bb2.TurnCount != 2 {
		t.Errorf("bb2.TurnCount=%d, want 2", bb2.TurnCount)
	}
	if bb2.ToolUseCount != 1 {
		t.Errorf("bb2.ToolUseCount=%d, want 1", bb2.ToolUseCount)
	}
	if bb2.LastTool != "Glob" {
		t.Errorf("bb2.LastTool=%q, want %q", bb2.LastTool, "Glob")
	}
	if bb2.JSONLPath == "" {
		t.Error("bb2.JSONLPath: want non-empty")
	}
}

// ---------------------------------------------------------------------------
// T2 — MissingSubagentsDir
// sessionDir without subagents/ sub-directory → ([], nil).
// Concept §9 edge cases, §10 acceptance criterion #2.
// ---------------------------------------------------------------------------

// TestCollectSubagents_MissingSubagentsDir verifies fail-soft: when the
// subagents/ directory is absent, CollectSubagents returns an empty slice
// and nil error (no panic, no error propagation).
func TestCollectSubagents_MissingSubagentsDir(t *testing.T) {
	sessionDir := t.TempDir() // no subagents/ sub-dir created

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: want nil error for missing subagents/ dir, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// T3 — EmptySubagentsDir
// subagents/ exists but contains no agent-*.jsonl files → ([], nil).
// Concept §9 edge cases.
// ---------------------------------------------------------------------------

// TestCollectSubagents_EmptySubagentsDir verifies that an existing but empty
// subagents/ directory produces an empty slice without error.
func TestCollectSubagents_EmptySubagentsDir(t *testing.T) {
	sessionDir := t.TempDir()
	subDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: want nil error for empty subagents/ dir, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// T4 — MissingMeta
// cc3: JSONL without meta.json → AgentType=="" and Description=="" but no error.
// Concept §9 edge cases, §10 acceptance criterion #4.
// ---------------------------------------------------------------------------

// TestCollectSubagents_MissingMeta verifies that when agent-*.meta.json is
// absent, AgentType and Description are empty strings and no error is returned.
func TestCollectSubagents_MissingMeta(t *testing.T) {
	// cc3 has no meta.json in the fixture set.
	sessionDir := setupSessionDir(t, []string{"agent-cc3.jsonl"})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	cc3 := got[0]
	if cc3.AgentID != "cc3" {
		t.Errorf("AgentID=%q, want %q", cc3.AgentID, "cc3")
	}
	if cc3.AgentType != "" {
		t.Errorf("AgentType=%q, want empty (no meta.json)", cc3.AgentType)
	}
	if cc3.Description != "" {
		t.Errorf("Description=%q, want empty (no meta.json)", cc3.Description)
	}
	if cc3.TurnCount != 1 {
		t.Errorf("TurnCount=%d, want 1", cc3.TurnCount)
	}
	if cc3.LastTool != "" {
		t.Errorf("LastTool=%q, want empty (no tool_use in cc3)", cc3.LastTool)
	}
}

// ---------------------------------------------------------------------------
// T5 — MalformedMeta
// ee5: meta.json contains invalid JSON → AgentType==""/ Description=="", SubagentStats returned.
// Concept §9 edge cases, §10 acceptance criterion #4.
// ---------------------------------------------------------------------------

// TestCollectSubagents_MalformedMeta verifies that a malformed meta.json does
// not prevent the SubagentStats from being returned; AgentType/Description are
// empty strings and the call returns nil error.
func TestCollectSubagents_MalformedMeta(t *testing.T) {
	// ee5 has valid JSONL (partially) + malformed meta.json.
	sessionDir := setupSessionDir(t, []string{"agent-ee5.jsonl", "agent-ee5.meta.json"})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	ee5 := got[0]
	if ee5.AgentID != "ee5" {
		t.Errorf("AgentID=%q, want %q", ee5.AgentID, "ee5")
	}
	if ee5.AgentType != "" {
		t.Errorf("AgentType=%q, want empty (malformed meta)", ee5.AgentType)
	}
	if ee5.Description != "" {
		t.Errorf("Description=%q, want empty (malformed meta)", ee5.Description)
	}
}

// ---------------------------------------------------------------------------
// T6 — EmptyJSONL
// dd4: empty file (0 bytes) → SubagentStats with AgentID + JSONLPath, zero aggregates.
// Concept §9 edge cases, §10 acceptance criterion #3.
// ---------------------------------------------------------------------------

// TestCollectSubagents_EmptyJSONL verifies that a zero-byte agent-*.jsonl yields a
// SubagentStats with AgentID and JSONLPath populated, all aggregates at zero values.
func TestCollectSubagents_EmptyJSONL(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{"agent-dd4.jsonl", "agent-dd4.meta.json"})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	dd4 := got[0]
	if dd4.AgentID != "dd4" {
		t.Errorf("AgentID=%q, want %q", dd4.AgentID, "dd4")
	}
	if dd4.JSONLPath == "" {
		t.Error("JSONLPath: want non-empty (file path recorded even for empty JSONL)")
	}
	if dd4.AgentType != "general-purpose" {
		t.Errorf("AgentType=%q, want %q (from meta.json)", dd4.AgentType, "general-purpose")
	}
	if dd4.TurnCount != 0 {
		t.Errorf("TurnCount=%d, want 0 (empty file)", dd4.TurnCount)
	}
	if dd4.Tokens.Input != 0 {
		t.Errorf("Tokens.Input=%d, want 0 (empty file)", dd4.Tokens.Input)
	}
	if !dd4.FirstTimestamp.IsZero() {
		t.Errorf("FirstTimestamp=%v, want zero (empty file)", dd4.FirstTimestamp)
	}
	if !dd4.LastTimestamp.IsZero() {
		t.Errorf("LastTimestamp=%v, want zero (empty file)", dd4.LastTimestamp)
	}
}

// ---------------------------------------------------------------------------
// T7 — MalformedJSONLPartial
// ee5: 3 valid lines + 2 broken JSON lines + 1 line without usage.
// ParseLines skips bad lines, aggregates the 3 parseable records.
// Concept §9 edge cases, §10 acceptance criterion #5.
// ---------------------------------------------------------------------------

// TestCollectSubagents_MalformedJSONLPartial verifies that malformed lines in
// an agent-*.jsonl are skipped gracefully and the valid records are aggregated.
func TestCollectSubagents_MalformedJSONLPartial(t *testing.T) {
	// ee5: 3 valid assistant+usage lines, 2 broken JSON, 1 no-usage line.
	sessionDir := setupSessionDir(t, []string{"agent-ee5.jsonl", "agent-ee5.meta.json"})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	ee5 := got[0]
	// 3 valid assistant records with usage → TurnCount == 3.
	if ee5.TurnCount != 3 {
		t.Errorf("TurnCount=%d, want 3 (only valid+usage lines counted)", ee5.TurnCount)
	}
	// Token aggregate from the 3 valid lines.
	if ee5.Tokens.Input != 2430 {
		t.Errorf("Tokens.Input=%d, want 2430", ee5.Tokens.Input)
	}
	if ee5.Tokens.Output != 486 {
		t.Errorf("Tokens.Output=%d, want 486", ee5.Tokens.Output)
	}
}

// ---------------------------------------------------------------------------
// T8 — SortByLastTimestampDesc
// aa1.LastTimestamp (10:04) > bb2.LastTimestamp (09:01) → aa1 is result[0].
// Concept §9 Q3, §10 acceptance criterion #6.
// ---------------------------------------------------------------------------

// TestCollectSubagents_SortByLastTimestampDesc verifies that the returned slice
// is ordered by LastTimestamp descending: the most recently active agent first.
func TestCollectSubagents_SortByLastTimestampDesc(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{
		"agent-aa1.jsonl", "agent-aa1.meta.json",
		"agent-bb2.jsonl", "agent-bb2.meta.json",
	})

	// aa1.LastTimestamp = 2026-05-15T10:04:00Z
	// bb2.LastTimestamp = 2026-05-15T09:01:00Z
	// Expected order: aa1 first (most recent), bb2 second.
	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if got[0].AgentID != "aa1" {
		t.Errorf("result[0].AgentID=%q, want %q (should be most recent)", got[0].AgentID, "aa1")
	}
	if got[1].AgentID != "bb2" {
		t.Errorf("result[1].AgentID=%q, want %q", got[1].AgentID, "bb2")
	}
}

// ---------------------------------------------------------------------------
// T9 — LastToolFromLastToolUseBlock
// aa1: content blocks in records are Read→Bash→Edit→Write. LastTool must be "Write".
// Concept §5.3 aggregateSubagent algorithm, §10 acceptance criterion #3.
// ---------------------------------------------------------------------------

// TestCollectSubagents_LastToolFromLastToolUseBlock verifies that LastTool
// reflects the name of the tool_use block from the chronologically last record
// that contained a tool_use content block.
func TestCollectSubagents_LastToolFromLastToolUseBlock(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{"agent-aa1.jsonl", "agent-aa1.meta.json"})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	// aa1 records in order: Read, Bash, Edit, Write (in record 4), then text-only (record 5).
	// LastTool = Write (last tool_use block across all records, scanning end-to-start).
	if got[0].LastTool != "Write" {
		t.Errorf("LastTool=%q, want %q", got[0].LastTool, "Write")
	}
}

// ---------------------------------------------------------------------------
// T10 — ModelFromLastRecord
// bb2: both records use opus-4-7 → canonical Model=="opus-4-7".
// aa1: all records use sonnet-4-6 → canonical Model=="sonnet-4-6".
// Concept §5.3, Q4.
// ---------------------------------------------------------------------------

// TestCollectSubagents_ModelFromLastRecord verifies that Model in SubagentStats
// reflects the canonical model key of the last assistant record in the JSONL.
func TestCollectSubagents_ModelFromLastRecord(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{
		"agent-aa1.jsonl", "agent-aa1.meta.json",
		"agent-bb2.jsonl", "agent-bb2.meta.json",
	})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}

	aa1 := findByAgentID(t, got, "aa1")
	if aa1.Model != "sonnet-4-6" {
		t.Errorf("aa1.Model=%q, want %q", aa1.Model, "sonnet-4-6")
	}

	bb2 := findByAgentID(t, got, "bb2")
	if bb2.Model != "opus-4-7" {
		t.Errorf("bb2.Model=%q, want %q", bb2.Model, "opus-4-7")
	}
}

// ---------------------------------------------------------------------------
// T11 — OrphanMetaWithoutJsonl
// An extra agent-zzz.meta.json without agent-zzz.jsonl must NOT appear in results.
// Concept §9 edge cases (orphan meta → Warn log, not returned).
// ---------------------------------------------------------------------------

// TestCollectSubagents_OrphanMetaWithoutJsonl verifies that a lone meta.json
// without a matching JSONL file is ignored: it does not produce a SubagentStats
// and does not cause an error.
func TestCollectSubagents_OrphanMetaWithoutJsonl(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{
		"agent-aa1.jsonl", "agent-aa1.meta.json",
		"agent-bb2.jsonl", "agent-bb2.meta.json",
	})

	// Write an orphan meta file with no sibling JSONL.
	orphanMeta := filepath.Join(sessionDir, "subagents", "agent-zzz.meta.json")
	if err := os.WriteFile(orphanMeta, []byte(`{"agentType":"orphan","description":"no jsonl"}`), 0o600); err != nil {
		t.Fatalf("write orphan meta: %v", err)
	}

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	// Only the two paired agents should appear.
	if len(got) != 2 {
		t.Errorf("len(got)=%d, want 2 (orphan meta should be ignored)", len(got))
	}
	for _, s := range got {
		if s.AgentID == "zzz" {
			t.Errorf("result contains AgentID=%q from orphan meta (should be ignored)", s.AgentID)
		}
	}
}

// ---------------------------------------------------------------------------
// T12 — CacheTokensAggregated
// aa1: sum of cache_read and cache_creation across all 5 records.
// Verifies that Tokens.CacheRead and Tokens.CacheCreate are properly summed.
// ---------------------------------------------------------------------------

// TestCollectSubagents_CacheTokensAggregated verifies that cache-related token
// fields are accumulated correctly across all records of a subagent.
func TestCollectSubagents_CacheTokensAggregated(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{"agent-aa1.jsonl", "agent-aa1.meta.json"})

	got, err := parser.CollectSubagents(sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	aa1 := got[0]
	// aa1 CacheRead sum: 500+550+600+650+700 = 3000
	if aa1.Tokens.CacheRead != 3000 {
		t.Errorf("Tokens.CacheRead=%d, want 3000", aa1.Tokens.CacheRead)
	}
	// aa1 CacheCreate sum: 300+330+360+390+420 = 1800
	if aa1.Tokens.CacheCreate != 1800 {
		t.Errorf("Tokens.CacheCreate=%d, want 1800", aa1.Tokens.CacheCreate)
	}
}
