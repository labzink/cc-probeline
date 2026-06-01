// Package parser_test contains tests for Phase 3.4 — subagent collector.
// Contract: plans/concepts/phase-3-step4-concept.md §4–§10.
// API:
//
//	parser.CollectSubagents(ctx context.Context, sessionDir string) ([]SubagentStats, error)
//	parser.SubagentStats{...}
package parser_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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
	if aa1.TranscriptPath == "" {
		t.Error("aa1.TranscriptPath: want non-empty")
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
	if bb2.TranscriptPath == "" {
		t.Error("bb2.TranscriptPath: want non-empty")
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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
	if dd4.TranscriptPath == "" {
		t.Error("TranscriptPath: want non-empty (file path recorded even for empty JSONL)")
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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
	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
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

// ---------------------------------------------------------------------------
// T13 — TieBreakByMtime
// Two empty JSONL files in one sessionDir; newer mtime → index 0.
// Concept §10 AC6: JSONL mtime DESC fallback for empty transcripts.
// ---------------------------------------------------------------------------

// TestCollectSubagents_TieBreakByMtime verifies that when two subagents have
// zero LastTimestamp (empty JSONL), the one with a newer file mtime appears first.
func TestCollectSubagents_TieBreakByMtime(t *testing.T) {
	sessionDir := t.TempDir()
	subDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create two empty JSONL files.
	older := filepath.Join(subDir, "agent-aa1.jsonl")
	newer := filepath.Join(subDir, "agent-zz9.jsonl")
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write %q: %v", p, err)
		}
	}

	// Set mtimes explicitly: older = T-10s, newer = T+0s.
	base := time.Now()
	olderMtime := base.Add(-10 * time.Second)
	newerMtime := base

	if err := os.Chtimes(older, olderMtime, olderMtime); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, newerMtime, newerMtime); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	// zz9 has newer mtime → must be first (index 0).
	if got[0].AgentID != "zz9" {
		t.Errorf("result[0].AgentID=%q, want %q (newer mtime first)", got[0].AgentID, "zz9")
	}
	if got[1].AgentID != "aa1" {
		t.Errorf("result[1].AgentID=%q, want %q (older mtime second)", got[1].AgentID, "aa1")
	}
}

// ---------------------------------------------------------------------------
// T14 — UnreadableJSONLSkipped
// One JSONL with chmod 0o000 → skipped; the other agent returns normally.
// Covers CollectSubagents open-failure branch (lines ~open-error in collectOne).
// ---------------------------------------------------------------------------

// TestCollectSubagents_UnreadableJSONLSkipped verifies that an unreadable
// agent-*.jsonl is skipped and does not prevent other agents from being returned.
func TestCollectSubagents_UnreadableJSONLSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 not supported on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 has no effect")
	}

	sessionDir := setupSessionDir(t, []string{
		"agent-aa1.jsonl", "agent-aa1.meta.json",
		"agent-bb2.jsonl", "agent-bb2.meta.json",
	})
	subDir := filepath.Join(sessionDir, "subagents")
	bb2Path := filepath.Join(subDir, "agent-bb2.jsonl")

	if err := os.Chmod(bb2Path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(bb2Path, 0o600) })

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1 (bb2 unreadable → skipped)", len(got))
	}
	if got[0].AgentID != "aa1" {
		t.Errorf("AgentID=%q, want %q", got[0].AgentID, "aa1")
	}
	for _, s := range got {
		if s.AgentID == "bb2" {
			t.Error("bb2 present in result; want skipped (unreadable JSONL)")
		}
	}
}

// ---------------------------------------------------------------------------
// T15 — DirEntryInSubagents
// A directory named agent-zzz.jsonl in subagents/ → skipped (not-regular-file).
// Covers listSubagentFiles IsDir branch.
// ---------------------------------------------------------------------------

// TestCollectSubagents_DirEntryInSubagents verifies that a directory entry whose
// name matches agent-*.jsonl is skipped and does not appear in results.
func TestCollectSubagents_DirEntryInSubagents(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{"agent-aa1.jsonl", "agent-aa1.meta.json"})
	subDir := filepath.Join(sessionDir, "subagents")

	// Create a directory with a name that matches the JSONL pattern.
	dirEntry := filepath.Join(subDir, "agent-zzz.jsonl")
	if err := os.MkdirAll(dirEntry, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1 (directory entry skipped)", len(got))
	}
	if got[0].AgentID != "aa1" {
		t.Errorf("AgentID=%q, want %q", got[0].AgentID, "aa1")
	}
}

// ---------------------------------------------------------------------------
// T16 — UnreadableSubagentsDir
// chmod 0o000 on subagents/ itself → CollectSubagents returns non-nil error.
// Covers listSubagentFiles non-ENOENT ReadDir error branch.
// ---------------------------------------------------------------------------

// TestCollectSubagents_UnreadableSubagentsDir verifies that when the subagents/
// directory exists but is unreadable, CollectSubagents returns a non-nil error.
func TestCollectSubagents_UnreadableSubagentsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 not supported on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 has no effect")
	}

	sessionDir := t.TempDir()
	subDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(subDir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(subDir, 0o700) })

	_, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err == nil {
		t.Error("CollectSubagents: want non-nil error for unreadable subagents/ dir, got nil")
	}
}

// ---------------------------------------------------------------------------
// T17 — UnreadableMetaContinues
// chmod 0o000 on agent-aa1.meta.json → SubagentStats returned with empty
// AgentType/Description (meta read fails gracefully).
// Covers parseMeta non-ENOENT open-error branch.
// ---------------------------------------------------------------------------

// TestCollectSubagents_UnreadableMetaContinues verifies that an unreadable
// meta.json does not prevent the SubagentStats from being returned; AgentType
// and Description will be empty strings.
func TestCollectSubagents_UnreadableMetaContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 not supported on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 000 has no effect")
	}

	sessionDir := setupSessionDir(t, []string{"agent-aa1.jsonl", "agent-aa1.meta.json"})
	subDir := filepath.Join(sessionDir, "subagents")
	metaPath := filepath.Join(subDir, "agent-aa1.meta.json")

	if err := os.Chmod(metaPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(metaPath, 0o600) })

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	aa1 := got[0]
	if aa1.AgentID != "aa1" {
		t.Errorf("AgentID=%q, want %q", aa1.AgentID, "aa1")
	}
	// Meta unreadable → AgentType and Description must be empty.
	if aa1.AgentType != "" {
		t.Errorf("AgentType=%q, want empty (unreadable meta)", aa1.AgentType)
	}
	if aa1.Description != "" {
		t.Errorf("Description=%q, want empty (unreadable meta)", aa1.Description)
	}
	// JSONL is still readable → SubagentStats should have actual data.
	if aa1.TurnCount == 0 {
		t.Error("TurnCount=0, want >0 (JSONL is readable)")
	}
}

// ---------------------------------------------------------------------------
// T18 — OrphanMetaDoesNotBreakResults
// agent-zzz.meta.json without companion agent-zzz.jsonl → orphan is ignored,
// other agents returned normally. Reinforces T11 with explicit orphan Warn path.
// ---------------------------------------------------------------------------

// TestCollectSubagents_OrphanMetaDoesNotBreakResults verifies that an orphan
// meta file (no companion JSONL) is silently ignored: only paired agents appear.
func TestCollectSubagents_OrphanMetaDoesNotBreakResults(t *testing.T) {
	sessionDir := setupSessionDir(t, []string{"agent-aa1.jsonl", "agent-aa1.meta.json"})
	subDir := filepath.Join(sessionDir, "subagents")

	// Write orphan meta with no companion JSONL.
	orphan := filepath.Join(subDir, "agent-zzz.meta.json")
	if err := os.WriteFile(orphan, []byte(`{"agentType":"orphan"}`), 0o600); err != nil {
		t.Fatalf("write orphan: %v", err)
	}

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1 (orphan meta ignored)", len(got))
	}
	if got[0].AgentID != "aa1" {
		t.Errorf("AgentID=%q, want %q", got[0].AgentID, "aa1")
	}
}

// ---------------------------------------------------------------------------
// Benchmark — BenchmarkCollectSubagents
// Baseline measurement for Phase 5 cache improvement (CONCEPT §7: <50 ms for
// 6 subagents with ~500 JSONL lines each).
// ---------------------------------------------------------------------------

// fixtureSubagentDirPath returns the path to tests/fixtures/jsonl/subagents/
// by resolving from the current working directory. Used by benchmarks that
// cannot call fixtureSubagentDir (which requires *testing.T).
func fixtureSubagentDirPath(b *testing.B) string {
	b.Helper()
	wd, err := os.Getwd()
	if err != nil {
		b.Fatalf("fixtureSubagentDirPath: getwd: %v", err)
	}
	candidate := filepath.Join(wd, "..", "..", "tests", "fixtures", "jsonl", "subagents")
	if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
		return candidate
	}
	candidate = filepath.Join(wd, "tests", "fixtures", "jsonl", "subagents")
	if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
		return candidate
	}
	b.Fatalf("fixtureSubagentDirPath: cannot locate tests/fixtures/jsonl/subagents from %s", wd)
	return ""
}

// BenchmarkCollectSubagents measures CollectSubagents performance against the
// existing fixture set (5 agents: aa1 with 5 records, bb2 with 2, cc3 with 1,
// dd4 empty, ee5 partial). Baseline for Phase 5 cache improvement.
// CONCEPT §7 budget: <50 ms for 6 subagents with ~500 JSONL lines each.
func BenchmarkCollectSubagents(b *testing.B) {
	srcDir := fixtureSubagentDirPath(b)

	fixtures := []string{
		"agent-aa1.jsonl", "agent-aa1.meta.json",
		"agent-bb2.jsonl", "agent-bb2.meta.json",
		"agent-cc3.jsonl",
		"agent-dd4.jsonl", "agent-dd4.meta.json",
		"agent-ee5.jsonl", "agent-ee5.meta.json",
	}

	sessionDir := b.TempDir()
	subDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	for _, name := range fixtures {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(subDir, name)
		in, err := os.Open(src)
		if err != nil {
			b.Fatalf("open %q: %v", src, err)
		}
		out, err := os.Create(dst)
		if err != nil {
			in.Close()
			b.Fatalf("create %q: %v", dst, err)
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		out.Close()
		if copyErr != nil {
			b.Fatalf("copy: %v", copyErr)
		}
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.CollectSubagents(ctx, sessionDir)
	}
}

// ---------------------------------------------------------------------------
// Phase 6.9.d helpers
// ---------------------------------------------------------------------------

// fixtureJSONLDir returns the path to tests/fixtures/jsonl/ (root level, not subagents/).
// Used by Phase 6.9.d tests that need the agent-sendmsg.jsonl fixture.
func fixtureJSONLDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("fixtureJSONLDir: getwd: %v", err)
	}
	// Running from tests/parser/ → two levels up is repo root.
	candidate := filepath.Join(wd, "..", "..", "tests", "fixtures", "jsonl")
	if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
		return candidate
	}
	// Fallback: repo root is cwd itself (go test ./... from root).
	candidate = filepath.Join(wd, "tests", "fixtures", "jsonl")
	if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
		return candidate
	}
	t.Fatalf("fixtureJSONLDir: cannot locate tests/fixtures/jsonl from %s", wd)
	return ""
}

// setupSessionDirFromRoot creates a temp sessionDir with a subagents/ sub-directory,
// copies named fixture files from tests/fixtures/jsonl/ (root level) into it,
// and returns the sessionDir path.
func setupSessionDirFromRoot(t *testing.T, fixtureNames []string) string {
	t.Helper()
	srcDir := fixtureJSONLDir(t)
	sessionDir := t.TempDir()
	subDir := filepath.Join(sessionDir, "subagents")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatalf("setupSessionDirFromRoot: mkdir %q: %v", subDir, err)
	}
	for _, name := range fixtureNames {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(subDir, name)
		copyFile(t, src, dst)
	}
	return sessionDir
}

// writeMetaJSON writes a meta.json file into the subagents/ dir of sessionDir.
// content is raw JSON bytes, e.g. []byte(`{"agentType":"test-writer"}`).
func writeMetaJSON(t *testing.T, sessionDir, agentID string, content []byte) {
	t.Helper()
	metaPath := filepath.Join(sessionDir, "subagents", "agent-"+agentID+".meta.json")
	if err := os.WriteFile(metaPath, content, 0o600); err != nil {
		t.Fatalf("writeMetaJSON: write %q: %v", metaPath, err)
	}
}

// ---------------------------------------------------------------------------
// T-1a — RoleIsAgentType
// meta.agentType="test-writer" → each Turns[i].Role == "test-writer".
// Spec-common §2.3: collectOne prosets Turns[i].Role = AgentType after read.
// ---------------------------------------------------------------------------

// TestSubagent_RoleIsAgentType verifies that when meta.json contains agentType,
// every Turn in Turns has Role equal to that AgentType value.
func TestSubagent_RoleIsAgentType(t *testing.T) {
	// Given: agent-sendmsg.jsonl (4 assistant turns) with meta agentType="test-writer".
	sessionDir := setupSessionDirFromRoot(t, []string{"agent-sendmsg.jsonl"})
	writeMetaJSON(t, sessionDir, "sendmsg", []byte(`{"agentType":"test-writer","description":"Phase 6.9.d fixture"}`))

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]
	if agent.AgentType != "test-writer" {
		t.Fatalf("AgentType=%q, want %q (pre-condition)", agent.AgentType, "test-writer")
	}
	// All Turns must carry the AgentType as their Role.
	if len(agent.Turns) == 0 {
		t.Fatal("Turns is empty; want at least one Turn (4 assistant records in fixture)")
	}
	for i, turn := range agent.Turns {
		if turn.Role != "test-writer" {
			t.Errorf("Turns[%d].Role=%q, want %q (AgentType)", i, turn.Role, "test-writer")
		}
	}
}

// ---------------------------------------------------------------------------
// T-1b — RoleFallbackEmpty
// No meta.json → AgentType="" → Turns[i].Role == "agent" (fallback).
// Spec-common §2.3: fallback "agent" when AgentType is empty.
// ---------------------------------------------------------------------------

// TestSubagent_RoleFallbackEmpty verifies that when meta.json is absent (AgentType=""),
// every Turn in Turns has Role equal to the fallback value "agent".
func TestSubagent_RoleFallbackEmpty(t *testing.T) {
	// Given: agent-sendmsg.jsonl with NO meta.json (meta absent → AgentType="").
	sessionDir := setupSessionDirFromRoot(t, []string{"agent-sendmsg.jsonl"})
	// No writeMetaJSON call — meta.json is intentionally absent.

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]
	if agent.AgentType != "" {
		t.Fatalf("AgentType=%q, want empty (pre-condition: no meta.json)", agent.AgentType)
	}
	if len(agent.Turns) == 0 {
		t.Fatal("Turns is empty; want at least one Turn")
	}
	for i, turn := range agent.Turns {
		if turn.Role != "agent" {
			t.Errorf("Turns[%d].Role=%q, want %q (fallback when AgentType empty)", i, turn.Role, "agent")
		}
	}
}

// ---------------------------------------------------------------------------
// T-2a — ActivationStartFirstTurn
// Fixture without user-text boundary → ActivationStart = first turn timestamp;
// CurrentTurnNum = total assistant turns.
// Spec-common §2.3; Insurance #1 fallback path.
// ---------------------------------------------------------------------------

// TestSubagent_ActivationStartFirstTurn verifies that when a transcript contains
// no user-text boundary, ActivationStart equals the timestamp of the very first
// assistant turn, and CurrentTurnNum equals the total number of turns.
func TestSubagent_ActivationStartFirstTurn(t *testing.T) {
	// Given: agent-aa1.jsonl (5 assistant turns, no user records).
	// agent-aa1 is used here because it has multiple turns and no user boundary.
	sessionDir := setupSessionDir(t, []string{"agent-aa1.jsonl", "agent-aa1.meta.json"})

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]

	// agent-aa1 has 5 assistant turns. First turn timestamp: 2026-05-15T10:00:00Z.
	wantActivationStart := parseTimeUTC(t, "2026-05-15T10:00:00.000000000Z")

	// T-2a: ActivationStart == timestamp of first assistant turn (no user boundary).
	if !agent.ActivationStart.Equal(wantActivationStart) {
		t.Errorf("ActivationStart=%v, want %v (first turn, no user boundary)",
			agent.ActivationStart, wantActivationStart)
	}

	// T-2a: CurrentTurnNum == total assistant turns (all in single activation).
	if agent.CurrentTurnNum != 5 {
		t.Errorf("CurrentTurnNum=%d, want 5 (all turns in one activation)", agent.CurrentTurnNum)
	}
}

// ---------------------------------------------------------------------------
// T-2b — ActivationStartAfterSendMsg
// Fixture agent-sendmsg.jsonl: 2 assistant turns, then user-text boundary, then 2
// assistant turns.
// ActivationStart = timestamp of third assistant record (first after boundary).
// CurrentTurnNum = 2 (only turns after the boundary).
// Spec-common §2.3; Insurance #1 verify path.
// ---------------------------------------------------------------------------

// TestSubagent_ActivationStartAfterSendMsg verifies that when a transcript contains
// a user-text boundary (SendMessage appears as a user record), ActivationStart
// equals the timestamp of the first assistant turn AFTER the boundary, and
// CurrentTurnNum counts only the turns in that activation.
func TestSubagent_ActivationStartAfterSendMsg(t *testing.T) {
	// Given: agent-sendmsg.jsonl layout:
	//   [0] assistant @ 09:00  — activation 1
	//   [1] assistant @ 09:01  — activation 1
	//   [2] user-text @ 09:02  — boundary (SendMessage)
	//   [3] assistant @ 09:03  — activation 2  ← ActivationStart
	//   [4] assistant @ 09:04  — activation 2
	sessionDir := setupSessionDirFromRoot(t, []string{"agent-sendmsg.jsonl"})
	writeMetaJSON(t, sessionDir, "sendmsg", []byte(`{"agentType":"test-writer"}`))

	got, err := parser.CollectSubagents(context.Background(), sessionDir)
	if err != nil {
		t.Fatalf("CollectSubagents: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	agent := got[0]

	// ActivationStart must be the timestamp of the first assistant turn after the
	// user-text boundary (09:03, not 09:00).
	wantActivationStart := parseTimeUTC(t, "2026-06-01T09:03:00.000000000Z")
	if !agent.ActivationStart.Equal(wantActivationStart) {
		t.Errorf("ActivationStart=%v, want %v (first turn after user-text boundary)",
			agent.ActivationStart, wantActivationStart)
	}

	// CurrentTurnNum must be 2: only the two assistant turns in the current activation.
	if agent.CurrentTurnNum != 2 {
		t.Errorf("CurrentTurnNum=%d, want 2 (only turns after user-text boundary)", agent.CurrentTurnNum)
	}
}
