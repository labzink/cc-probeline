// Package renderer_test — RED tests for Phase 4.3.c truncation pipeline.
//
// FitLine progressively downgrades probe render levels by Priority group
// (§4.3 Truncation pipeline) until the assembled string fits in cols.
// softWrap is an unexported helper; it is exercised indirectly through
// FitLine with sep=" | " and cols<50.
//
// All assertions use format.VisualLen to measure rendered width, because
// strings may contain {{marker}} tokens that have zero terminal width.
//
// Stub behaviour: FitLine always returns "".  Most tests will FAIL on the
// stub; TestFitLine_EmptyProbes accidentally passes (empty input → "" == "").
package renderer_test

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/format"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeEntry builds a ProbeEntry that returns a different string per level.
// level 0 = Full, 1 = Compact, 2 = Minimal; anything else = "".
func makeEntry(priority int, full, compact, minimal string) renderer.ProbeEntry {
	return renderer.ProbeEntry{
		Priority: priority,
		Render: func(level int) string {
			switch level {
			case 0:
				return full
			case 1:
				return compact
			case 2:
				return minimal
			}
			return ""
		},
	}
}

// invisibleEntry returns a ProbeEntry whose Render always returns "".
// Models a probe that is not visible at any level (§4.3 AllInvisible).
func invisibleEntry(priority int) renderer.ProbeEntry {
	return renderer.ProbeEntry{
		Priority: priority,
		Render:   func(_ int) string { return "" },
	}
}

// ---------------------------------------------------------------------------
// T-6: FitLine — basic fit / downgrade tests
// ---------------------------------------------------------------------------

// §4.3 T-6 — three short probes, cols=80. All render at Full; no downgrade.
// FAILS on stub: stub returns "", want "gpt-4o | 1h | 99%".
func TestFitLine_NoTruncation_FitsAtFull(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "gpt-4o", "gpt", ""),
		makeEntry(1, "1h", "1h", ""),
		makeEntry(2, "99%", "~", ""),
	}
	const sep = " | "
	const cols = 80
	out := renderer.FitLine(entries, cols, sep)
	want := "gpt-4o | 1h | 99%"
	if out != want {
		t.Fatalf("FitLine() = %q, want %q", out, want)
	}
	if vl := format.VisualLen(out); vl > cols {
		t.Fatalf("VisualLen(%q) = %d, exceeds cols=%d", out, vl, cols)
	}
}

// §4.3 T-6 — P0 stays Full, P2 entries downgrade to Compact when cols=40.
// FAILS on stub: stub returns "".
func TestFitLine_DowngradeP2ToCompact(t *testing.T) {
	// Full output: "model-name-long | project-x-long | cache-y-long" ~48 cols
	// After P2→Compact: "model-name-long | px | cy" which fits in 40.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "model-name-long", "model-name-long", ""),
		makeEntry(2, "project-x-long", "px", ""),
		makeEntry(2, "cache-y-long", "cy", ""),
	}
	const sep = " | "
	const cols = 40
	out := renderer.FitLine(entries, cols, sep)
	// P0 must remain Full.
	if !strings.Contains(out, "model-name-long") {
		t.Fatalf("FitLine() = %q: P0 entry not at Full level (missing 'model-name-long')", out)
	}
	// P2 must be Compact, not Full.
	if strings.Contains(out, "project-x-long") {
		t.Fatalf("FitLine() = %q: P2 entry 'project-x-long' should be downgraded to Compact 'px'", out)
	}
	if strings.Contains(out, "cache-y-long") {
		t.Fatalf("FitLine() = %q: P2 entry 'cache-y-long' should be downgraded to Compact 'cy'", out)
	}
	if !strings.Contains(out, "px") {
		t.Fatalf("FitLine() = %q: expected Compact 'px' for P2 entry", out)
	}
	if !strings.Contains(out, "cy") {
		t.Fatalf("FitLine() = %q: expected Compact 'cy' for P2 entry", out)
	}
}

// §4.3 T-6 — with cols=5, every pass through 3 overflows so FitLine reaches
// pass=4 where P0 stays Full and P1/P2/P3 render at Minimal ("" → dropped).
// pass calculations for the inputs below (sep=" | " = 3 visual cols):
//
//	pass 0: "M | quota-long | proj-long | email-long" = 39 > 5
//	pass 1: "M | quota-long | pl | el"                = 24 > 5
//	pass 2: "M | ql | pl | el"                        = 16 > 5
//	pass 3: "M | ql"                                  =  6 > 5
//	pass 4: "M"                                       =  1 ≤ 5 → returned
//
// FAILS on stub: stub returns "".
func TestFitLine_DowngradeAllNonP0ToMinimal(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "M", "M", ""),           // P0: always Full → "M"
		makeEntry(1, "quota-long", "ql", ""), // P1: Minimal="" → dropped
		makeEntry(2, "proj-long", "pl", ""),  // P2: Minimal="" → dropped
		makeEntry(3, "email-long", "el", ""), // P3: Minimal="" → dropped
	}
	const sep = " | "
	const cols = 5
	out := renderer.FitLine(entries, cols, sep)
	// P0 must be present.
	if !strings.Contains(out, "M") {
		t.Fatalf("FitLine() = %q: P0 entry missing", out)
	}
	// P1/P2/P3 with Minimal="" should be dropped from output.
	for _, forbidden := range []string{"quota-long", "proj-long", "email-long", "ql", "pl", "el"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("FitLine() = %q: expected %q dropped at Minimal level", out, forbidden)
		}
	}
}

// §4.3 T-7 — P0 is inviolable. Even if its Full content overflows cols, FitLine
// returns the Full string (not downgraded, not truncated).
// FAILS on stub: stub returns "".
func TestFitLine_P0Never_Downgrades(t *testing.T) {
	const veryLong = "VERY LONG STRING THAT IS WAY TOO WIDE"
	entries := []renderer.ProbeEntry{
		makeEntry(0, veryLong, "short", ""),
	}
	const sep = " | "
	const cols = 10
	out := renderer.FitLine(entries, cols, sep)
	// P0 must stay Full; overflow accepted.
	if out != veryLong {
		t.Fatalf("FitLine() = %q, want %q (P0 inviolable, overflow accepted)", out, veryLong)
	}
}

// ---------------------------------------------------------------------------
// T-8: FitLine — soft wrap tests (via sep=" | " and cols<50)
// ---------------------------------------------------------------------------

// §4.3 T-8 — line2 with sep=" | " and cols=40: result must contain a newline
// because there are enough chunks to overflow 40 cols and trigger soft wrap.
// FAILS on stub: stub returns "".
func TestFitLine_SoftWrap_Line2_Under50Cols(t *testing.T) {
	// 4 probes all P0 (inviolable), each rendering a 12-char Full string.
	// Joined: "aaaaaaaaaaaa | bbbbbbbbbbbb | cccccccccccc | dddddddddddd" = 58 chars > 40.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aaaaaaaaaaaa", "aaaaaaaaaaaa", "aaaaaaaaaaaa"),
		makeEntry(0, "bbbbbbbbbbbb", "bbbbbbbbbbbb", "bbbbbbbbbbbb"),
		makeEntry(0, "cccccccccccc", "cccccccccccc", "cccccccccccc"),
		makeEntry(0, "dddddddddddd", "dddddddddddd", "dddddddddddd"),
	}
	const sep = " | "
	const cols = 40
	out := renderer.FitLine(entries, cols, sep)
	if !strings.Contains(out, "\n") {
		t.Fatalf("FitLine() = %q: expected soft-wrap newline for cols=%d sep=%q", out, cols, sep)
	}
	// Each line of the wrapped output must be <= cols.
	for i, line := range strings.Split(out, "\n") {
		if vl := format.VisualLen(line); vl > cols {
			t.Fatalf("FitLine() wrapped line[%d] = %q: VisualLen=%d exceeds cols=%d", i, line, vl, cols)
		}
	}
}

// §4.3 T-8 — line0 uses sep="{{dim}} • {{reset}}" (not " | ").
// Even when cols is tiny, FitLine must NOT soft-wrap (wrap is only for sep=" | ").
// FAILS on stub: stub returns "".
func TestFitLine_SoftWrap_Line0_NotWrapped(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aaa", "aaa", "aaa"),
		makeEntry(0, "bbb", "bbb", "bbb"),
		makeEntry(0, "ccc", "ccc", "ccc"),
	}
	const sep = "{{dim}} • {{reset}}"
	const cols = 5
	out := renderer.FitLine(entries, cols, sep)
	if strings.Contains(out, "\n") {
		t.Fatalf("FitLine() = %q: must not soft-wrap when sep=%q (not ' | ')", out, sep)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

// §4.3 — empty entries slice returns "".
// PASSES on stub accidentally (stub returns "" regardless).
// Will remain passing in GREEN. Documented here for completeness.
func TestFitLine_EmptyProbes(t *testing.T) {
	out := renderer.FitLine([]renderer.ProbeEntry{}, 80, " | ")
	if out != "" {
		t.Fatalf("FitLine([]) = %q, want empty string", out)
	}
}

// §4.3 — all entries return "" at every level (invisible probes).
// FitLine must return "" (no separator-only output).
// FAILS on stub: stub returns "" which matches, so this PASSES on stub.
// Actual assertion is that the output contains no separators from empty probes.
// Will continue passing in GREEN.
func TestFitLine_AllInvisible(t *testing.T) {
	entries := []renderer.ProbeEntry{
		invisibleEntry(0),
		invisibleEntry(1),
		invisibleEntry(2),
	}
	out := renderer.FitLine(entries, 80, " | ")
	if out != "" {
		t.Fatalf("FitLine(all-invisible) = %q, want empty string (no separator leakage)", out)
	}
}

// §4.3 — sep contains markers (e.g. "{{dim}} • {{reset}}"); VisualLen of sep
// is 3 (" • "), not len("{{dim}} • {{reset}}") = 19. FitLine must use
// VisualLen for comparison so the fit decision is correct.
// Set cols so that without proper VisualLen the result would NOT fit,
// but with proper VisualLen it does.
// FAILS on stub: stub returns "".
func TestFitLine_SeparatorMarkersStripped(t *testing.T) {
	// Two P0 entries: "aa" and "bb". sep="{{dim}} • {{reset}}" (visual width 3).
	// Assembled Full: "aa{{dim}} • {{reset}}bb" → VisualLen = 2+3+2 = 7.
	// cols=10: must fit (7 <= 10), so FitLine returns Full assembly.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aa", "a", ""),
		makeEntry(0, "bb", "b", ""),
	}
	const sep = "{{dim}} • {{reset}}"
	const cols = 10
	out := renderer.FitLine(entries, cols, sep)
	// Full content present (no downgrade needed).
	if !strings.Contains(out, "aa") {
		t.Fatalf("FitLine() = %q: expected Full 'aa', VisualLen(sep) not counted correctly", out)
	}
	if !strings.Contains(out, "bb") {
		t.Fatalf("FitLine() = %q: expected Full 'bb'", out)
	}
	// VisualLen of output must be <= cols.
	if vl := format.VisualLen(out); vl > cols {
		t.Fatalf("FitLine() = %q: VisualLen=%d exceeds cols=%d (marker sep over-counted?)", out, vl, cols)
	}
}

// §4.3 — probe returns a CJK string (U+4E2D U+6587, visual width 4, byte length 6).
// FitLine must use VisualLen (runewidth-based), not len().
// CJK literals live as Go unicode escapes to keep the file ASCII-only
// (language guard on the public mirror rejects raw CJK / Cyrillic).
// FAILS on stub: stub returns "".
func TestFitLine_VisualLen_CjkProbe(t *testing.T) {
	const cjk = "\u4e2d\u6587" // U+4E2D U+6587 -- Go unicode escape; runtime equals the raw CJK pair
	// cjk has VisualLen=4. With sep=" | " and P0 probe "X" (width 1):
	// assembled Full = "X | <cjk>" -> VisualLen = 1 + 3 + 4 = 8.
	// cols=8 must fit.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "X", "X", "X"),
		makeEntry(1, cjk, "CJ", ""),
	}
	const sep = " | "
	const cols = 8
	out := renderer.FitLine(entries, cols, sep)
	if !strings.Contains(out, cjk) {
		t.Fatalf("FitLine() = %q: expected CJK probe at Full level (VisualLen=4 fits in cols=8)", out)
	}
	if vl := format.VisualLen(out); vl > cols {
		t.Fatalf("FitLine() = %q: VisualLen=%d exceeds cols=%d (CJK width mis-counted?)", out, vl, cols)
	}
}

// ---------------------------------------------------------------------------
// T-8: softWrap — indirect tests via FitLine
// ---------------------------------------------------------------------------

// §4.3 T-8 — softWrap basic: "a | b | ccccc" with cols=5 should produce
// at least two lines. First chunk "a" fits; "a | b" = 5 fits; "a | b | ccccc" = 13 > 5.
// So wrap at "a | b" then "\nccccc".
// We exercise this by constructing equivalent ProbeEntry input and asserting
// the wrapped structure.
// FAILS on stub: stub returns "".
func TestSoftWrap_Basic(t *testing.T) {
	// Three P0 entries so no downgrade occurs. Full output "a | b | ccccc" is
	// 13 chars, exceeds cols=5, so softWrap is triggered (sep=" | ", cols<50).
	entries := []renderer.ProbeEntry{
		makeEntry(0, "a", "a", "a"),
		makeEntry(0, "b", "b", "b"),
		makeEntry(0, "ccccc", "ccccc", "ccccc"),
	}
	const sep = " | "
	const cols = 5
	out := renderer.FitLine(entries, cols, sep)
	// Must have wrapped to multiple lines.
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("softWrap: FitLine() = %q: expected multi-line output for cols=%d, got %d line(s)", out, cols, len(lines))
	}
	// Each line must not exceed cols.
	for i, line := range lines {
		if vl := format.VisualLen(line); vl > cols {
			t.Fatalf("softWrap: line[%d] = %q: VisualLen=%d exceeds cols=%d", i, line, vl, cols)
		}
	}
	// All content must be preserved.
	for _, chunk := range []string{"a", "b", "ccccc"} {
		if !strings.Contains(out, chunk) {
			t.Fatalf("softWrap: FitLine() = %q: missing chunk %q after wrap", out, chunk)
		}
	}
}

// §4.3 T-8 — softWrap single chunk: single P0 probe with cols<50 and sep=" | ".
// If there is only one chunk, no splitting occurs. Output must equal the single
// probe's Full render (possibly overflowing, since P0 is inviolable).
// FAILS on stub: stub returns "".
func TestSoftWrap_SingleChunk(t *testing.T) {
	entries := []renderer.ProbeEntry{
		makeEntry(0, "aaa", "aaa", "aaa"),
	}
	const sep = " | "
	const cols = 40
	out := renderer.FitLine(entries, cols, sep)
	if out != "aaa" {
		t.Fatalf("FitLine(single entry) = %q, want %q (no sep, no wrap)", out, "aaa")
	}
}

// ---------------------------------------------------------------------------
// Priority group boundary tests (indirect via FitLine)
// ---------------------------------------------------------------------------

// §4.3 — pass=0 (large cols): all probes render at Full regardless of Priority.
// Verifies that levelsForPass(entries, 0) returns Full for every Priority
// including Priority=99 (clamped to group 3+).
// FAILS on stub: stub returns "".
func TestFitLine_AllFull_LargeCols(t *testing.T) {
	// cols=1000 ensures pass=0 always fits.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "p0-full", "p0-compact", ""),
		makeEntry(1, "p1-full", "p1-compact", ""),
		makeEntry(2, "p2-full", "p2-compact", ""),
		makeEntry(3, "p3-full", "p3-compact", ""),
		makeEntry(99, "p99-full", "p99-compact", ""),
	}
	const sep = " | "
	const cols = 1000
	out := renderer.FitLine(entries, cols, sep)
	for _, want := range []string{"p0-full", "p1-full", "p2-full", "p3-full", "p99-full"} {
		if !strings.Contains(out, want) {
			t.Fatalf("FitLine(cols=1000) = %q: missing Full-level %q (pass=0 must use Full for all)", out, want)
		}
	}
}

// §4.3 — pass=4 (tiny cols, final pass): P0=Full, P1/P2/P3/P99=Minimal.
// Verifies that levelsForPass(entries, 4) matches the table row for pass=4:
//
//	P0=Full | P1=Minimal | P2=Minimal | P3=Minimal | P4+=Minimal
//
// FAILS on stub: stub returns "".
func TestFitLine_OnlyP0Full_TinyCols(t *testing.T) {
	// cols=3 so the full output never fits; FitLine falls through to pass=4.
	// P0 renders "X" (1 char). P1/P2/P3/P99 render "" at Minimal → dropped.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "X", "X", "X"),           // P0: Full at every pass
		makeEntry(1, "p1-full", "p1c", ""),    // P1: Minimal="" → dropped at pass 4
		makeEntry(2, "p2-full", "p2c", ""),    // P2: Minimal="" → dropped at pass 4
		makeEntry(3, "p3-full", "p3c", ""),    // P3: Minimal="" → dropped at pass 4
		makeEntry(99, "p99-full", "p99c", ""), // P4+: Minimal="" → dropped at pass 4
	}
	const sep = " | "
	const cols = 3
	out := renderer.FitLine(entries, cols, sep)
	// P0 must be present.
	if !strings.Contains(out, "X") {
		t.Fatalf("FitLine(cols=3) = %q: P0 must remain Full", out)
	}
	// P1/P2/P3/P99 must be absent (Minimal="" → dropped by assemble).
	for _, forbidden := range []string{"p1", "p2", "p3", "p99"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("FitLine(cols=3) = %q: %q must be dropped (Minimal='' at pass=4)", out, forbidden)
		}
	}
}

// §4.3 — boundary check: Priority=0/1/2/3/99 all handled in pass=1.
// pass=1 table: P0=Full, P1=Full, P2=Compact, P3=Compact, P4+=Compact.
// Design: set cols so pass=0 overflows but pass=1 fits.
// FAILS on stub: stub returns "".
func TestPriorityGroup_Boundaries(t *testing.T) {
	// pass=0 (all Full): "p0-full | p1-full | p2-full | p3-full | p99-full"
	//   widths: 7 + 3 + 7 + 3 + 7 + 3 + 7 + 3 + 8 = 48
	// pass=1 (P2/P3/P99 Compact): "p0-full | p1-full | p2-c | p3-c | p99-c"
	//   widths: 7 + 3 + 7 + 3 + 4 + 3 + 4 + 3 + 5 = 39
	// cols=47: pass=0 (48) overflows by 1, pass=1 (39) fits.
	entries := []renderer.ProbeEntry{
		makeEntry(0, "p0-full", "p0-compact", ""),
		makeEntry(1, "p1-full", "p1-compact", ""),
		makeEntry(2, "p2-full", "p2-c", ""),
		makeEntry(3, "p3-full", "p3-c", ""),
		makeEntry(99, "p99-full", "p99-c", ""),
	}
	const sep = " | "
	const cols = 47
	out := renderer.FitLine(entries, cols, sep)
	// P0 and P1 stay Full (pass=1 boundary: P0/P1 are Full).
	if !strings.Contains(out, "p0-full") {
		t.Fatalf("FitLine() = %q: P0 should be Full at pass=1", out)
	}
	if !strings.Contains(out, "p1-full") {
		t.Fatalf("FitLine() = %q: P1 should be Full at pass=1", out)
	}
	// P2/P3/P99 downgrade to Compact at pass=1.
	for _, full := range []string{"p2-full", "p3-full", "p99-full"} {
		if strings.Contains(out, full) {
			t.Fatalf("FitLine() = %q: %q should be Compact at pass=1 (not Full)", out, full)
		}
	}
	if !strings.Contains(out, "p2-c") {
		t.Fatalf("FitLine() = %q: P2 should be Compact 'p2-c' at pass=1", out)
	}
	// Result must fit.
	if vl := format.VisualLen(out); vl > cols {
		t.Fatalf("FitLine() = %q: VisualLen=%d exceeds cols=%d", out, vl, cols)
	}
}
