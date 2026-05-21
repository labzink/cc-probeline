// Package renderer_test — Phase 4.4 spec-S4 tests for softWrap continuation marker.
//
// softWrap is unexported; exercised via FitLine with sep=" | " and cols<50.
// The continuation marker "↪ " (U+21AA + space, runewidth=2) is prepended to
// every wrapped chunk after the first.
package renderer_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// ---------------------------------------------------------------------------
// spec-S4: softWrap continuation marker
// ---------------------------------------------------------------------------

// TestSoftWrap_ContinuationMarker_OnWrappedChunks verifies that chunks
// after the first wrapped chunk begin with the "↪ " marker.
// Input: 3 P0 entries that produce "a | b | ccccc" (13 cols), cols=5.
// Expected wrap: line0="a | b", line1="↪ ccccc".
func TestSoftWrap_ContinuationMarker_OnWrappedChunks(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "a", "a", "a"),
		makeEntry(0, "b", "b", "b"),
		makeEntry(0, "ccccc", "ccccc", "ccccc"),
	}
	const sep = " | "
	const cols = 5
	out := renderer.FitLine(entries, cols, sep)

	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multi-line output for cols=%d, got: %q", cols, out)
	}
	// Every continuation line (index >= 1) must start with the marker.
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, "↪ ") {
			t.Errorf("wrapped line[%d] = %q: expected prefix '↪ ' (U+21AA + space)", i+1, line)
		}
	}
}

// TestSoftWrap_FirstChunk_NoMarker verifies that the first line of a wrapped
// result does NOT start with the continuation marker.
func TestSoftWrap_FirstChunk_NoMarker(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "first", "first", "first"),
		makeEntry(0, "second", "second", "second"),
		makeEntry(0, "thirdlong", "thirdlong", "thirdlong"),
	}
	const sep = " | "
	const cols = 10
	out := renderer.FitLine(entries, cols, sep)

	lines := strings.Split(out, "\n")
	if strings.HasPrefix(lines[0], "↪ ") {
		t.Errorf("first line must not start with continuation marker; got: %q", lines[0])
	}
}

// TestSoftWrap_MarkerVisualLen_2 verifies that the marker "↪ " has VisualLen=2.
// This confirms that format.VisualLen correctly measures the runewidth of U+21AA
// as 1 plus the space (1) = 2 total.
func TestSoftWrap_MarkerVisualLen_2(t *testing.T) {
	const marker = "↪ "
	got := format.VisualLen(marker)
	if got != 2 {
		t.Errorf("VisualLen(%q) = %d, want 2 (U+21AA runewidth=1 + space=1)", marker, got)
	}
}

// TestSoftWrap_RespectsTotalCols verifies that each wrapped line fits within
// the given cols, including the marker width on continuation lines.
func TestSoftWrap_RespectsTotalCols(t *testing.T) {
	// 5 entries of 8 chars each: "aaaaaaaa | bbbbbbbb | cccccccc | dddddddd | eeeeeeee"
	// Full = 5*8 + 4*3 = 52 cols, exceeds cols=12.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aaaaaaaa", "aaaaaaaa", "aaaaaaaa"),
		makeEntry(0, "bbbbbbbb", "bbbbbbbb", "bbbbbbbb"),
		makeEntry(0, "cccccccc", "cccccccc", "cccccccc"),
		makeEntry(0, "dddddddd", "dddddddd", "dddddddd"),
		makeEntry(0, "eeeeeeee", "eeeeeeee", "eeeeeeee"),
	}
	const sep = " | "
	const cols = 12
	out := renderer.FitLine(entries, cols, sep)

	for i, line := range strings.Split(out, "\n") {
		vl := format.VisualLen(line)
		if vl > cols {
			t.Errorf("wrapped line[%d] = %q: VisualLen=%d exceeds cols=%d", i, line, vl, cols)
		}
	}
}

// TestSoftWrap_SingleChunkUnchanged verifies that a single-chunk input is
// returned unchanged (no marker, no newline).
func TestSoftWrap_SingleChunkUnchanged(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aaa", "aaa", "aaa"),
	}
	const sep = " | "
	const cols = 80
	out := renderer.FitLine(entries, cols, sep)
	if out != "aaa" {
		t.Errorf("single-chunk FitLine() = %q, want %q", out, "aaa")
	}
	if strings.Contains(out, "↪") {
		t.Errorf("single-chunk must not contain marker; got: %q", out)
	}
}

// TestSoftWrap_WrappedChunks_AllHaveMarker is an affirmative regression test
// that verifies every continuation chunk (index >= 1) starts with "↪ " even
// when the input wraps into 3+ chunks. A regression that removes or skips the
// marker for intermediate chunks would be caught here.
func TestSoftWrap_WrappedChunks_AllHaveMarker(t *testing.T) {
	// 6 entries of 10 chars each at cols=12: forces 3+ wrapped chunks.
	// Full join: "aaaaaaaaaa | bbbbbbbbbb | cccccccccc | dddddddddd | eeeeeeeeee | ffffffffff"
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aaaaaaaaaa", "aaaaaaaaaa", "aaaaaaaaaa"),
		makeEntry(0, "bbbbbbbbbb", "bbbbbbbbbb", "bbbbbbbbbb"),
		makeEntry(0, "cccccccccc", "cccccccccc", "cccccccccc"),
		makeEntry(0, "dddddddddd", "dddddddddd", "dddddddddd"),
		makeEntry(0, "eeeeeeeeee", "eeeeeeeeee", "eeeeeeeeee"),
		makeEntry(0, "ffffffffff", "ffffffffff", "ffffffffff"),
	}
	const sep = " | "
	const cols = 12
	out := renderer.FitLine(entries, cols, sep)

	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 wrapped lines for cols=%d, got %d; output: %q", cols, len(lines), out)
	}
	// Every continuation line (index >= 1) must begin with the marker.
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, "↪ ") {
			t.Errorf("wrapped chunk[%d] = %q: missing '↪ ' marker", i+1, line)
		}
	}
	// First line must not have the marker.
	if strings.HasPrefix(lines[0], "↪ ") {
		t.Errorf("first chunk must not have marker; got: %q", lines[0])
	}
}
