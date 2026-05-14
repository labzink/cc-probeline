// Package parser_test — RED tests for Dedup function.
// Tests cover dedup policy from plans/concepts/phase-3-step1-concept.md §3.
// API: func Dedup(records []Record) []Record  (internal/parser/dedup.go)
//
// Dedup priority (Phase 3.1 scope): RequestID → UUID.
// message.id fallback is deferred to Phase 4 (not extracted into Record).
// Records without any dedup key pass through unchanged.
package parser_test

import (
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
)

// makeRecord is a test helper that builds a minimal Record.
func makeRecord(requestID, uuid string, ts time.Time) parser.Record {
	return parser.Record{
		Type:      "assistant",
		RequestID: requestID,
		UUID:      uuid,
		Timestamp: ts,
	}
}

var (
	t0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
)

// TestDedup_RequestID verifies that among records sharing a RequestID
// the one with the smallest Timestamp wins, and the winner occupies
// the position of the first occurrence in the output slice.
// Ref: concept §3 rule 1.
func TestDedup_RequestID(t *testing.T) {
	// earlier record appears second in the input — it must still win
	input := []parser.Record{
		makeRecord("req-1", "uuid-a", t1), // later ts  → loser
		makeRecord("req-1", "uuid-b", t0), // earlier ts → winner
	}

	got := parser.Dedup(input)

	if len(got) != 1 {
		t.Fatalf("want 1 record after dedup, got %d", len(got))
	}
	if got[0].Timestamp != t0 {
		t.Errorf("want winner Timestamp=%v, got %v", t0, got[0].Timestamp)
	}
}

// TestDedup_RequestID_OrderPreserved checks that the winner occupies
// the position of the *first* occurrence of the key (stable position).
// Ref: concept §3 "Deterministic ordering".
func TestDedup_RequestID_OrderPreserved(t *testing.T) {
	r1 := makeRecord("req-A", "uuid-1", t1) // first occurrence of key; later ts
	r2 := makeRecord("req-B", "uuid-2", t0) // different key
	r3 := makeRecord("req-A", "uuid-3", t0) // duplicate key; earlier ts → wins

	input := []parser.Record{r1, r2, r3}
	got := parser.Dedup(input)

	// r3 wins over r1 (smaller ts), but it must sit at position 0 (r1's slot).
	if len(got) != 2 {
		t.Fatalf("want 2 records, got %d", len(got))
	}
	// Position 0 must be the winner for req-A.
	if got[0].UUID != "uuid-3" {
		t.Errorf("want winner uuid-3 at position 0, got %v", got[0].UUID)
	}
	// Position 1 must be req-B (unchanged).
	if got[1].RequestID != "req-B" {
		t.Errorf("want req-B at position 1, got %v", got[1].RequestID)
	}
}

// TestDedup_UUIDFallback checks that when RequestID is empty the UUID is used
// as the dedup key. Winner = smaller Timestamp.
// Ref: concept §3 rule 2.
func TestDedup_UUIDFallback(t *testing.T) {
	input := []parser.Record{
		makeRecord("", "uuid-x", t2), // empty RequestID, later ts → loser
		makeRecord("", "uuid-x", t1), // same UUID, earlier ts → winner
	}

	got := parser.Dedup(input)

	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	if got[0].Timestamp != t1 {
		t.Errorf("want winner Timestamp=%v, got %v", t1, got[0].Timestamp)
	}
}

// TestDedup_NoKey verifies that a record with both RequestID and UUID empty
// is NOT deduplicated — it passes through as a unique entry regardless of
// any other records.
// Ref: concept §3 rule 4.
func TestDedup_NoKey(t *testing.T) {
	r1 := makeRecord("", "", t0) // no dedup key
	r2 := makeRecord("", "", t0) // also no dedup key — treated as separate
	r3 := makeRecord("req-1", "uuid-a", t1)

	input := []parser.Record{r1, r2, r3}
	got := parser.Dedup(input)

	// r1 and r2 are both kept (no key → no dedup between them).
	// r3 is unique by key, kept.
	if len(got) != 3 {
		t.Fatalf("want 3 records (2 no-key + 1 keyed), got %d", len(got))
	}
}

// TestDedup_EqualTimestamp_StableFirstWins verifies that when two records
// share the same RequestID and the same Timestamp, the first encountered
// in the input survives (stable / first-wins rule).
// Ref: concept §3 "Equal Timestamp — stable sort by input index".
func TestDedup_EqualTimestamp_StableFirstWins(t *testing.T) {
	first := makeRecord("req-eq", "uuid-first", t0)
	second := makeRecord("req-eq", "uuid-second", t0)

	input := []parser.Record{first, second}
	got := parser.Dedup(input)

	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	if got[0].UUID != "uuid-first" {
		t.Errorf("want first record to survive equal-ts tie, got UUID=%v", got[0].UUID)
	}
}

// TestDedup_MixedKeys exercises RequestID-keyed, UUID-keyed, and keyless
// records in a single call to verify they interact correctly.
func TestDedup_MixedKeys(t *testing.T) {
	noKey1 := makeRecord("", "", t0)
	noKey2 := makeRecord("", "", t1)
	byReq1 := makeRecord("req-1", "", t2)
	byReq2 := makeRecord("req-1", "", t0) // earlier ts → wins over byReq1
	byUUID1 := makeRecord("", "u-1", t1)
	byUUID2 := makeRecord("", "u-1", t2) // later ts → loser

	input := []parser.Record{noKey1, noKey2, byReq1, byReq2, byUUID1, byUUID2}
	got := parser.Dedup(input)

	// Expected: noKey1, noKey2 (both kept), winner(req-1), winner(u-1) = 4 total.
	if len(got) != 4 {
		t.Fatalf("want 4 records, got %d: %v", len(got), got)
	}

	// Winner for req-1 must have earlier ts (t0 = byReq2).
	var reqWinner *parser.Record
	for i := range got {
		if got[i].RequestID == "req-1" {
			reqWinner = &got[i]
			break
		}
	}
	if reqWinner == nil {
		t.Fatal("no record with RequestID=req-1 in result")
	}
	if reqWinner.Timestamp != t0 {
		t.Errorf("req-1 winner: want ts=%v, got %v", t0, reqWinner.Timestamp)
	}

	// Winner for uuid u-1 must have earlier ts (t1 = byUUID1).
	var uuidWinner *parser.Record
	for i := range got {
		if got[i].UUID == "u-1" {
			uuidWinner = &got[i]
			break
		}
	}
	if uuidWinner == nil {
		t.Fatal("no record with UUID=u-1 in result")
	}
	if uuidWinner.Timestamp != t1 {
		t.Errorf("u-1 winner: want ts=%v, got %v", t1, uuidWinner.Timestamp)
	}
}

// TestDedup_ThreeWayDup verifies that among three records with the same key
// the earliest timestamp wins regardless of input order.
func TestDedup_ThreeWayDup(t *testing.T) {
	input := []parser.Record{
		makeRecord("req-x", "u", t2),  // latest → loser
		makeRecord("req-x", "u", t0),  // earliest → winner
		makeRecord("req-x", "u", t1),  // middle → loser
	}

	got := parser.Dedup(input)

	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	if got[0].Timestamp != t0 {
		t.Errorf("want earliest ts=%v, got %v", t0, got[0].Timestamp)
	}
}

// TestDedup_EmptyInput ensures Dedup is safe with an empty slice.
func TestDedup_EmptyInput(t *testing.T) {
	got := parser.Dedup([]parser.Record{})
	if len(got) != 0 {
		t.Fatalf("want empty result, got %d records", len(got))
	}
}

// TestDedup_NilInput ensures Dedup is safe with a nil slice.
func TestDedup_NilInput(t *testing.T) {
	got := parser.Dedup(nil)
	if len(got) != 0 {
		t.Fatalf("want empty result for nil input, got %d records", len(got))
	}
}

// TestDedup_AllUnique confirms that records with distinct keys all survive.
func TestDedup_AllUnique(t *testing.T) {
	input := []parser.Record{
		makeRecord("req-1", "u1", t0),
		makeRecord("req-2", "u2", t1),
		makeRecord("req-3", "u3", t2),
	}

	got := parser.Dedup(input)

	if len(got) != 3 {
		t.Fatalf("want 3 unique records, got %d", len(got))
	}
}
