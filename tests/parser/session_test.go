// Package parser_test — RED tests for SessionStats aggregator (Phase 3.2).
// Contract: plans/concepts/phase-3-step2-concept.md §2, §3.2, §4, §8.
// API: func Aggregate(records []Record) SessionStats  (internal/parser/session.go)
package parser_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers (local to session_test.go)
// ---------------------------------------------------------------------------

// sessionFixtureReader opens a fixture file from tests/fixtures/jsonl/ and
// returns the parsed records via ParseLines. Fatal on any IO or parse error.
func sessionFixtureReader(t *testing.T, name string) []parser.Record {
	t.Helper()
	path := filepath.Join("..", "fixtures", "jsonl", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("sessionFixtureReader: cannot open %q: %v", path, err)
	}
	defer f.Close()

	records, _, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("sessionFixtureReader: ParseLines failed for %q: %v", name, err)
	}
	return records
}

// makeAssistantRecord builds a minimal assistant Record for in-memory tests.
func makeAssistantRecord(model string, ts time.Time, usage parser.TokenCounts, content []parser.ContentBlock) parser.Record {
	return parser.Record{
		Type:      "assistant",
		UUID:      "uuid-inline-" + model,
		RequestID: "req-inline-" + model,
		Timestamp: ts,
		Model:     model,
		Usage:     usage,
		Content:   content,
	}
}

// toolUseBlock creates a ContentBlock with type "tool_use".
func toolUseBlock(name string) parser.ContentBlock {
	return parser.ContentBlock{Type: "tool_use", ToolName: name}
}

// textBlock creates a ContentBlock with type "text".
func textBlock() parser.ContentBlock {
	return parser.ContentBlock{Type: "text"}
}

// toolResultBlock creates a ContentBlock with type "tool_result".
func toolResultBlock() parser.ContentBlock {
	return parser.ContentBlock{Type: "tool_result"}
}

// addTokenCounts returns the field-wise sum of two TokenCounts values.
func addTokenCounts(a, b parser.TokenCounts) parser.TokenCounts {
	return parser.TokenCounts{
		Input:         a.Input + b.Input,
		Output:        a.Output + b.Output,
		CacheRead:     a.CacheRead + b.CacheRead,
		CacheCreate:   a.CacheCreate + b.CacheCreate,
		CacheCreate5m: a.CacheCreate5m + b.CacheCreate5m,
		CacheCreate1h: a.CacheCreate1h + b.CacheCreate1h,
	}
}

// eqTokenCounts compares two TokenCounts for equality.
func eqTokenCounts(a, b parser.TokenCounts) bool {
	return a.Input == b.Input &&
		a.Output == b.Output &&
		a.CacheRead == b.CacheRead &&
		a.CacheCreate == b.CacheCreate &&
		a.CacheCreate5m == b.CacheCreate5m &&
		a.CacheCreate1h == b.CacheCreate1h
}

// ---------------------------------------------------------------------------
// Test 1 — Happy path single model
// Concept §8.1 scenario 1.
// Fixture small.jsonl: 5 valid assistant records, all claude-opus-4-7.
// Expected totals are computed from the known fixture values:
//
//	uuid-001: input=1000 output=200 cacheRead=500 cacheCreate=300 5m=200 1h=100 (1 tool_use)
//	uuid-002: input=2000 output=400 cacheRead=1000 cacheCreate=600 5m=400 1h=200 (1 tool_use)
//	uuid-003: input=500  output=100 cacheRead=250  cacheCreate=150 5m=100 1h=50  (1 tool_use, isSidechain)
//	uuid-004: input=3000 output=600 cacheRead=1500 cacheCreate=900 5m=600 1h=300 (0 tool_use)
//	uuid-005: input=4000 output=800 cacheRead=2000 cacheCreate=1200 5m=800 1h=400 (1 tool_use)
//
// ---------------------------------------------------------------------------
func TestAggregate_HappyPath(t *testing.T) {
	records := sessionFixtureReader(t, "small.jsonl")

	got := parser.Aggregate(records)

	// small.jsonl has 5 assistant records after dedup.
	const wantTurnCount = 5
	if got.TurnCount != wantTurnCount {
		t.Errorf("TurnCount: want %d, got %d", wantTurnCount, got.TurnCount)
	}

	// Exact token totals summed from fixture values above.
	wantTotals := parser.TokenCounts{
		Input:         10500, // 1000+2000+500+3000+4000
		Output:        2100,  // 200+400+100+600+800
		CacheRead:     5250,  // 500+1000+250+1500+2000
		CacheCreate:   3150,  // 300+600+150+900+1200
		CacheCreate5m: 2100,  // 200+400+100+600+800
		CacheCreate1h: 1050,  // 100+200+50+300+400
	}
	if !eqTokenCounts(got.Totals, wantTotals) {
		t.Errorf("Totals: want %+v, got %+v", wantTotals, got.Totals)
	}

	// small.jsonl has two model families: opus-4-7 (uuid-001,002,004,005)
	// and sonnet-4-6 (uuid-003, isSidechain=true).
	if len(got.PerModel) != 2 {
		t.Errorf("len(PerModel): want 2, got %d (keys: %v)", len(got.PerModel), keysOf(got.PerModel))
	}
	wantOpus := parser.TokenCounts{
		Input:         10000, // 1000+2000+3000+4000
		Output:        2000,  // 200+400+600+800
		CacheRead:     5000,  // 500+1000+1500+2000
		CacheCreate:   3000,  // 300+600+900+1200
		CacheCreate5m: 2000,  // 200+400+600+800
		CacheCreate1h: 1000,  // 100+200+300+400
	}
	opusCounts, ok := got.PerModel["opus-4-7"]
	if !ok {
		t.Fatalf("PerModel[\"opus-4-7\"] not found; keys: %v", keysOf(got.PerModel))
	}
	if !eqTokenCounts(opusCounts, wantOpus) {
		t.Errorf("PerModel[\"opus-4-7\"]: want %+v, got %+v", wantOpus, opusCounts)
	}
	wantSonnet := parser.TokenCounts{
		Input:         500, // uuid-003
		Output:        100,
		CacheRead:     250,
		CacheCreate:   150,
		CacheCreate5m: 100,
		CacheCreate1h: 50,
	}
	sonnetCounts, ok := got.PerModel["sonnet-4-6"]
	if !ok {
		t.Fatalf("PerModel[\"sonnet-4-6\"] not found; keys: %v", keysOf(got.PerModel))
	}
	if !eqTokenCounts(sonnetCounts, wantSonnet) {
		t.Errorf("PerModel[\"sonnet-4-6\"]: want %+v, got %+v", wantSonnet, sonnetCounts)
	}

	// Timestamps come from the sorted records (small.jsonl records are uuid-001..005 in order).
	if !got.FirstTimestamp.Equal(records[0].Timestamp) {
		t.Errorf("FirstTimestamp: want %v, got %v", records[0].Timestamp, got.FirstTimestamp)
	}
	if !got.LastTimestamp.Equal(records[4].Timestamp) {
		t.Errorf("LastTimestamp: want %v, got %v", records[4].Timestamp, got.LastTimestamp)
	}

	// tool_use blocks: uuid-001 (Edit=1), uuid-002 (Bash=1), uuid-003 (Read=1), uuid-004 (0), uuid-005 (Task=1) = 4.
	const wantToolUse = 4
	if got.ToolUseCount != wantToolUse {
		t.Errorf("ToolUseCount: want %d, got %d", wantToolUse, got.ToolUseCount)
	}
}

// keysOf returns the string keys of a map for error messages.
func keysOf(m map[string]parser.TokenCounts) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// Test 2 — Multi-model (orchestrator + subagent turns)
// Concept §8.1 scenario 2, fixture §8.4.
// Fixture multi-model.jsonl: 10 records.
//   6 x claude-opus-4-7 (isSidechain=false):  input=100 output=20 cacheRead=50 cacheCreate=30
//   4 x claude-sonnet-4-6 (isSidechain=true):  input=50  output=10 cacheRead=25 cacheCreate=15
// Totals:
//   opus-4-7:   input=600 output=120 cacheRead=300 cacheCreate=180
//   sonnet-4-6: input=200 output=40  cacheRead=100 cacheCreate=60
//   combined:   input=800 output=160 cacheRead=400 cacheCreate=240
// ---------------------------------------------------------------------------
func TestAggregate_MultiModel(t *testing.T) {
	records := sessionFixtureReader(t, "multi-model.jsonl")

	got := parser.Aggregate(records)

	if got.TurnCount != 10 {
		t.Errorf("TurnCount: want 10, got %d", got.TurnCount)
	}

	if len(got.PerModel) != 2 {
		t.Errorf("len(PerModel): want 2, got %d (keys: %v)", len(got.PerModel), keysOf(got.PerModel))
	}

	wantOpus := parser.TokenCounts{Input: 600, Output: 120, CacheRead: 300, CacheCreate: 180}
	opusCounts, ok := got.PerModel["opus-4-7"]
	if !ok {
		t.Fatalf("PerModel[\"opus-4-7\"] not found; keys: %v", keysOf(got.PerModel))
	}
	if !eqTokenCounts(opusCounts, wantOpus) {
		t.Errorf("PerModel[\"opus-4-7\"]: want %+v, got %+v", wantOpus, opusCounts)
	}

	wantSonnet := parser.TokenCounts{Input: 200, Output: 40, CacheRead: 100, CacheCreate: 60}
	sonnetCounts, ok := got.PerModel["sonnet-4-6"]
	if !ok {
		t.Fatalf("PerModel[\"sonnet-4-6\"] not found; keys: %v", keysOf(got.PerModel))
	}
	if !eqTokenCounts(sonnetCounts, wantSonnet) {
		t.Errorf("PerModel[\"sonnet-4-6\"]: want %+v, got %+v", wantSonnet, sonnetCounts)
	}

	// Totals == opus + sonnet field-wise.
	wantTotals := addTokenCounts(wantOpus, wantSonnet)
	if !eqTokenCounts(got.Totals, wantTotals) {
		t.Errorf("Totals: want %+v, got %+v", wantTotals, got.Totals)
	}
}

// ---------------------------------------------------------------------------
// Test 3 — Empty input (nil and empty slice both return zero SessionStats)
// Concept §8.1 scenario 3.
// ---------------------------------------------------------------------------
func TestAggregate_EmptyInput(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got := parser.Aggregate(nil)
		assertZeroStats(t, got)
	})

	t.Run("empty_slice", func(t *testing.T) {
		got := parser.Aggregate([]parser.Record{})
		assertZeroStats(t, got)
	})
}

// assertZeroStats checks that stats is the zero value of SessionStats.
func assertZeroStats(t *testing.T, got parser.SessionStats) {
	t.Helper()
	if got.TurnCount != 0 {
		t.Errorf("TurnCount: want 0, got %d", got.TurnCount)
	}
	if got.ToolUseCount != 0 {
		t.Errorf("ToolUseCount: want 0, got %d", got.ToolUseCount)
	}
	if got.PerModel != nil {
		t.Errorf("PerModel: want nil, got %v", got.PerModel)
	}
	if !got.FirstTimestamp.IsZero() {
		t.Errorf("FirstTimestamp: want zero, got %v", got.FirstTimestamp)
	}
	if !got.LastTimestamp.IsZero() {
		t.Errorf("LastTimestamp: want zero, got %v", got.LastTimestamp)
	}
	if !eqTokenCounts(got.Totals, parser.TokenCounts{}) {
		t.Errorf("Totals: want zero, got %+v", got.Totals)
	}
}

// ---------------------------------------------------------------------------
// Test 4 — Single turn (boundary condition)
// Concept §8.1 scenario 4.
// ---------------------------------------------------------------------------
func TestAggregate_SingleTurn(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	records := []parser.Record{
		makeAssistantRecord(
			"claude-opus-4-7",
			ts,
			parser.TokenCounts{Input: 500, Output: 100, CacheRead: 250, CacheCreate: 150},
			[]parser.ContentBlock{toolUseBlock("Bash"), textBlock()},
		),
	}

	got := parser.Aggregate(records)

	if got.TurnCount != 1 {
		t.Errorf("TurnCount: want 1, got %d", got.TurnCount)
	}
	if !got.FirstTimestamp.Equal(ts) {
		t.Errorf("FirstTimestamp: want %v, got %v", ts, got.FirstTimestamp)
	}
	if !got.LastTimestamp.Equal(ts) {
		t.Errorf("LastTimestamp: want %v, got %v", ts, got.LastTimestamp)
	}
	if len(got.PerModel) != 1 {
		t.Errorf("len(PerModel): want 1, got %d", len(got.PerModel))
	}
	wantUsage := parser.TokenCounts{Input: 500, Output: 100, CacheRead: 250, CacheCreate: 150}
	if counts, ok := got.PerModel["opus-4-7"]; !ok {
		t.Error("PerModel[\"opus-4-7\"] not found")
	} else if !eqTokenCounts(counts, wantUsage) {
		t.Errorf("PerModel[\"opus-4-7\"]: want %+v, got %+v", wantUsage, counts)
	}
	if got.ToolUseCount != 1 {
		t.Errorf("ToolUseCount: want 1, got %d", got.ToolUseCount)
	}
}

// ---------------------------------------------------------------------------
// Test 5 — Integration with ParseLines (end-to-end pipeline)
// Concept §8.1 scenario 5.
// Fixture with-retry.jsonl: 5 records, req-B appears twice (duplicate).
// After ParseLines dedup: 4 unique records.
// Unique input tokens: uuid-A=100, uuid-B-first=200, uuid-C=300, uuid-D=400.
// TurnCount == 4, Totals.Input == 1000.
// ---------------------------------------------------------------------------
func TestAggregate_IntegrationWithParseLines(t *testing.T) {
	path := filepath.Join("..", "fixtures", "jsonl", "with-retry.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("cannot open with-retry.jsonl: %v", err)
	}
	defer f.Close()

	records, _, err := parser.ParseLines(f)
	if err != nil {
		t.Fatalf("ParseLines error: %v", err)
	}

	got := parser.Aggregate(records)

	// 5 lines - 1 duplicate = 4 unique turns.
	if got.TurnCount != 4 {
		t.Errorf("TurnCount: want 4, got %d", got.TurnCount)
	}

	// Duplicates are NOT double-counted in totals.
	// uuid-A=100, uuid-B-first=200 (winner), uuid-C=300, uuid-D=400.
	const wantInput = 1000
	if got.Totals.Input != wantInput {
		t.Errorf("Totals.Input: want %d (deduped), got %d", wantInput, got.Totals.Input)
	}
}

// ---------------------------------------------------------------------------
// Test 6 — Canonical model key (table-driven)
// Concept §8.2, §4.1 — all 9 rows of the canonicalization table.
// Tested indirectly via Aggregate: each Record gets a single model value,
// we verify the resulting PerModel key.
// ---------------------------------------------------------------------------
func TestAggregate_CanonicalModelKey(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	usage := parser.TokenCounts{Input: 1, Output: 1}

	cases := []struct {
		rawModel    string
		wantKey     string
	}{
		{"", "unknown"},
		{"claude-opus-4-7", "opus-4-7"},
		{"claude-opus-4-7-20250805", "opus-4-7"},
		{"claude-sonnet-4-6-20251101", "sonnet-4-6"},
		{"claude-haiku-4-5", "haiku-4-5"},
		{"claude-haiku-4-5-20251001", "haiku-4-5"},
		{"claude-opus-4-1", "opus-4-1"},
		{"some-custom-model", "some-custom-model"},
		{"claude-foo", "foo"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.rawModel, func(t *testing.T) {
			// Build a unique RequestID per case to avoid dedup collisions.
			rec := parser.Record{
				Type:      "assistant",
				UUID:      "uuid-key-" + tc.rawModel,
				RequestID: "req-key-" + tc.rawModel,
				Timestamp: ts,
				Model:     tc.rawModel,
				Usage:     usage,
			}
			got := parser.Aggregate([]parser.Record{rec})
			if _, ok := got.PerModel[tc.wantKey]; !ok {
				t.Errorf("rawModel=%q: want PerModel key %q, got keys %v",
					tc.rawModel, tc.wantKey, keysOf(got.PerModel))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test 7 — Zero timestamp record does not panic
// Concept §8.2, §3.2.
// ---------------------------------------------------------------------------
func TestAggregate_ZeroTimestamp(t *testing.T) {
	records := []parser.Record{
		{
			Type:      "assistant",
			UUID:      "uuid-zero-ts",
			RequestID: "req-zero-ts",
			Timestamp: time.Time{}, // zero timestamp
			Model:     "claude-opus-4-7",
			Usage:     parser.TokenCounts{Input: 100},
		},
	}

	got := parser.Aggregate(records)

	if got.TurnCount != 1 {
		t.Errorf("TurnCount: want 1, got %d", got.TurnCount)
	}
	if !got.FirstTimestamp.IsZero() {
		t.Errorf("FirstTimestamp: want zero, got %v", got.FirstTimestamp)
	}
	if !got.LastTimestamp.IsZero() {
		t.Errorf("LastTimestamp: want zero, got %v", got.LastTimestamp)
	}
	if got.Totals.Input != 100 {
		t.Errorf("Totals.Input: want 100, got %d", got.Totals.Input)
	}
	if counts, ok := got.PerModel["opus-4-7"]; !ok {
		t.Error("PerModel[\"opus-4-7\"] not found")
	} else if counts.Input != 100 {
		t.Errorf("PerModel[\"opus-4-7\"].Input: want 100, got %d", counts.Input)
	}
}

// ---------------------------------------------------------------------------
// Test 8 — Empty model string falls back to "unknown" key
// Concept §8.2, §3.2.
// ---------------------------------------------------------------------------
func TestAggregate_EmptyModelFallback(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	records := []parser.Record{
		{
			Type:      "assistant",
			UUID:      "uuid-empty-model",
			RequestID: "req-empty-model",
			Timestamp: ts,
			Model:     "", // empty model
			Usage:     parser.TokenCounts{Output: 50},
		},
	}

	got := parser.Aggregate(records)

	if len(got.PerModel) != 1 {
		t.Errorf("len(PerModel): want 1, got %d (keys: %v)", len(got.PerModel), keysOf(got.PerModel))
	}
	counts, ok := got.PerModel["unknown"]
	if !ok {
		t.Fatalf("PerModel[\"unknown\"] not found; keys: %v", keysOf(got.PerModel))
	}
	if counts.Output != 50 {
		t.Errorf("PerModel[\"unknown\"].Output: want 50, got %d", counts.Output)
	}
}

// ---------------------------------------------------------------------------
// Test 9 — ToolUseCount counts all tool_use blocks across all turns
// Concept §8.2, §6 row 6.
// Turn 1: 2 tool_use + 1 text = 2
// Turn 2: 1 text = 0
// Turn 3: 3 tool_use + 1 tool_result = 3
// Total: 5
// ---------------------------------------------------------------------------
func TestAggregate_ToolUseCount(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	model := "claude-opus-4-7"
	usage := parser.TokenCounts{Input: 10, Output: 5}

	records := []parser.Record{
		{
			Type:      "assistant",
			UUID:      "uuid-tool-1",
			RequestID: "req-tool-1",
			Timestamp: ts.Add(0),
			Model:     model,
			Usage:     usage,
			Content: []parser.ContentBlock{
				toolUseBlock("Bash"),
				toolUseBlock("Edit"),
				textBlock(),
			},
		},
		{
			Type:      "assistant",
			UUID:      "uuid-tool-2",
			RequestID: "req-tool-2",
			Timestamp: ts.Add(time.Minute),
			Model:     model,
			Usage:     usage,
			Content: []parser.ContentBlock{
				textBlock(),
			},
		},
		{
			Type:      "assistant",
			UUID:      "uuid-tool-3",
			RequestID: "req-tool-3",
			Timestamp: ts.Add(2 * time.Minute),
			Model:     model,
			Usage:     usage,
			Content: []parser.ContentBlock{
				toolUseBlock("Read"),
				toolUseBlock("Write"),
				toolUseBlock("Glob"),
				toolResultBlock(),
			},
		},
	}

	got := parser.Aggregate(records)

	if got.ToolUseCount != 5 {
		t.Errorf("ToolUseCount: want 5 (2+0+3), got %d", got.ToolUseCount)
	}
	if got.TurnCount != 3 {
		t.Errorf("TurnCount: want 3, got %d", got.TurnCount)
	}
}

// ---------------------------------------------------------------------------
// Test 10 — Cache breakdown fields are summed correctly across turns
// Concept §8.2.
// Turn 1: CacheCreate=1000 CacheCreate5m=800 CacheCreate1h=200
// Turn 2: CacheCreate=500  CacheCreate5m=300 CacheCreate1h=200
// Expected totals: CacheCreate=1500 CacheCreate5m=1100 CacheCreate1h=400
// ---------------------------------------------------------------------------
func TestAggregate_CacheBreakdown(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	model := "claude-opus-4-7"

	records := []parser.Record{
		{
			Type:      "assistant",
			UUID:      "uuid-cache-1",
			RequestID: "req-cache-1",
			Timestamp: ts,
			Model:     model,
			Usage: parser.TokenCounts{
				Input:         100,
				Output:        20,
				CacheCreate:   1000,
				CacheCreate5m: 800,
				CacheCreate1h: 200,
			},
		},
		{
			Type:      "assistant",
			UUID:      "uuid-cache-2",
			RequestID: "req-cache-2",
			Timestamp: ts.Add(time.Minute),
			Model:     model,
			Usage: parser.TokenCounts{
				Input:         50,
				Output:        10,
				CacheCreate:   500,
				CacheCreate5m: 300,
				CacheCreate1h: 200,
			},
		},
	}

	got := parser.Aggregate(records)

	if got.Totals.CacheCreate != 1500 {
		t.Errorf("Totals.CacheCreate: want 1500, got %d", got.Totals.CacheCreate)
	}
	if got.Totals.CacheCreate5m != 1100 {
		t.Errorf("Totals.CacheCreate5m: want 1100, got %d", got.Totals.CacheCreate5m)
	}
	if got.Totals.CacheCreate1h != 400 {
		t.Errorf("Totals.CacheCreate1h: want 400, got %d", got.Totals.CacheCreate1h)
	}
}

// ---------------------------------------------------------------------------
// Test 11 — Sidechain (subagent) turn is included in shared totals
// Concept §8.2, §3.2 — IsSidechain: true does NOT exclude from aggregation.
// ---------------------------------------------------------------------------
func TestAggregate_SidechainInTotals(t *testing.T) {
	ts := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	model := "claude-sonnet-4-6"

	records := []parser.Record{
		{
			Type:        "assistant",
			UUID:        "uuid-orch",
			RequestID:   "req-orch",
			Timestamp:   ts,
			Model:       model,
			IsSidechain: false,
			Usage:       parser.TokenCounts{Input: 200, Output: 40},
		},
		{
			Type:        "assistant",
			UUID:        "uuid-agent",
			RequestID:   "req-agent",
			Timestamp:   ts.Add(time.Minute),
			Model:       model,
			IsSidechain: true, // subagent turn — must still be counted
			Usage:       parser.TokenCounts{Input: 100, Output: 20},
		},
	}

	got := parser.Aggregate(records)

	// Both turns are counted regardless of IsSidechain flag.
	if got.TurnCount != 2 {
		t.Errorf("TurnCount: want 2, got %d", got.TurnCount)
	}

	// Both turns belong to the same canonical key "sonnet-4-6".
	counts, ok := got.PerModel["sonnet-4-6"]
	if !ok {
		t.Fatalf("PerModel[\"sonnet-4-6\"] not found; keys: %v", keysOf(got.PerModel))
	}
	if counts.Input != 300 {
		t.Errorf("PerModel[\"sonnet-4-6\"].Input: want 300 (both turns), got %d", counts.Input)
	}
	if counts.Output != 60 {
		t.Errorf("PerModel[\"sonnet-4-6\"].Output: want 60 (both turns), got %d", counts.Output)
	}
}
