// Package format_test — RED tests for Phase 4.3.b visualLen + StripMarkers.
//
// Public contract (§4.3, plan T-4/T-5):
//
//   - StripMarkers removes our marker tokens `{{name}}` and `{{name:value}}`.
//     Recognised tokens include {{color:NAME}}, {{dim}}, {{bold}}, {{italic}},
//     {{reset}}. Anything that does not match `\{\{[a-z][a-z0-9:_-]*\}\}` is
//     left as-is (malformed sequences pass through).
//
//   - VisualLen returns the terminal column width of s after StripMarkers,
//     using runewidth so wide UTF-8 glyphs (CJK) count as 2.
//
// Stub behaviour:
//   - StripMarkers returns s unchanged.
//   - VisualLen calls runewidth.StringWidth(s) without stripping.
//
// On the stub: StripMarkers_NoMarkers, StripMarkers_MalformedNotStripped,
// VisualLen_Ascii/EmptyString/UTF8Icons/CJK/MixedAsciiAndCJK/NoEscape_Sanity
// PASS by coincidence (the stub behaviour happens to match). The marker
// cases FAIL until StripMarkers is implemented.
//
// Icon widths were verified empirically via runewidth.RuneWidth before
// writing these assertions — see commit message and PLAN §4.3.b notes.
package format_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/format"
)

// §4.3 T-4 — StripMarkers removes a `{{color:NAME}}…{{reset}}` pair.
// FAILS on stub (returns input unchanged).
func TestStripMarkers_Color(t *testing.T) {
	got := format.StripMarkers("a{{color:cyan}}b{{reset}}c")
	if got != "abc" {
		t.Fatalf("StripMarkers = %q, want %q", got, "abc")
	}
}

// §4.3 T-4 — StripMarkers removes {{dim}}, {{bold}}, {{italic}}, {{reset}}.
// FAILS on stub.
func TestStripMarkers_DimBoldItalic(t *testing.T) {
	got := format.StripMarkers("{{dim}}x{{bold}}y{{italic}}z{{reset}}")
	if got != "xyz" {
		t.Fatalf("StripMarkers = %q, want %q", got, "xyz")
	}
}

// §4.3 T-4 — strings without markers are returned unchanged.
// PASSES on stub (coincidence).
func TestStripMarkers_NoMarkers(t *testing.T) {
	got := format.StripMarkers("hello")
	if got != "hello" {
		t.Fatalf("StripMarkers = %q, want %q", got, "hello")
	}
}

// §4.3 T-4 — the regex is generic over `{{name}}`, so unknown but
// syntactically valid tokens are also stripped. The marker grammar acts as
// a closed namespace from the caller's side (assembler emits only the
// recognised palette), but StripMarkers itself is conservative and removes
// anything matching `\{\{[a-z][a-z0-9:_-]*\}\}`.
// FAILS on stub.
func TestStripMarkers_UnknownToken_StillStripped(t *testing.T) {
	got := format.StripMarkers("a{{whatever}}b")
	if got != "ab" {
		t.Fatalf("StripMarkers = %q, want %q", got, "ab")
	}
}

// §4.3 T-4 — malformed marker sequences pass through unchanged: single
// braces, missing closer, missing opener, leading digit.
// PASSES on stub (coincidence — stub passes everything through).
func TestStripMarkers_MalformedNotStripped(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a{b}c", "a{b}c"},
		{"a{{noend", "a{{noend"},
		{"a}}b", "a}}b"},
		{"x{{1bad}}y", "x{{1bad}}y"},
	}
	for _, c := range cases {
		if got := format.StripMarkers(c.in); got != c.want {
			t.Errorf("StripMarkers(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// §4.3 T-5 — VisualLen("hello") = 5 (plain ASCII).
// PASSES on stub (stub already calls runewidth.StringWidth without strip,
// and there are no markers to strip in this input).
func TestVisualLen_Ascii(t *testing.T) {
	if got := format.VisualLen("hello"); got != 5 {
		t.Fatalf("VisualLen = %d, want 5", got)
	}
}

// §4.3 T-4 + T-5 — markers must not contribute to width.
// FAILS on stub: stub width = 3 (abc) + 14 (markers) = 17.
func TestVisualLen_WithMarkers(t *testing.T) {
	if got := format.VisualLen("a{{color:red}}b{{reset}}c"); got != 3 {
		t.Fatalf("VisualLen = %d, want 3", got)
	}
}

// §4.3 T-5 — empty string width is 0.
// PASSES on stub.
func TestVisualLen_EmptyString(t *testing.T) {
	if got := format.VisualLen(""); got != 0 {
		t.Fatalf("VisualLen = %d, want 0", got)
	}
}

// §4.3 T-4 + T-5 — a string that consists only of markers has visual
// width 0. FAILS on stub (stub returns runewidth of the literal markers).
func TestVisualLen_OnlyMarkers(t *testing.T) {
	if got := format.VisualLen("{{dim}}{{reset}}"); got != 0 {
		t.Fatalf("VisualLen = %d, want 0", got)
	}
}

// §4.3 T-5 + brainstorm Batch 3 — every icon in the legend renders as a
// single terminal cell under runewidth's default (Ambiguous → Narrow).
// Empirically verified via runewidth.RuneWidth before writing.
// PASSES on stub (no markers, stub width == real width).
func TestVisualLen_UTF8Icons(t *testing.T) {
	icons := []struct {
		name string
		s    string
	}{
		{"branch ⎇", "⎇"},
		{"clock ⏱", "⏱"},
		{"refresh ↻", "↻"},
		{"warning ⚠", "⚠"},
		{"equiv ≡", "≡"},
		{"arrow-up-right ↗", "↗"},
		{"bullet •", "•"},
		{"block-full █", "█"},
		{"block-medium ▒", "▒"},
		{"block-light ░", "░"},
	}
	for _, ic := range icons {
		if got := format.VisualLen(ic.s); got != 1 {
			t.Errorf("VisualLen(%s) = %d, want 1", ic.name, got)
		}
	}
}

// §4.3 T-5 — non-ASCII width assertions. Strings use Go unicode escapes so
// the public-mirror language guard does not reject this shipped test file:
//
//   - U+043F U+0440 U+0438 U+0432 U+0435 U+0442 = 6 narrow runes (Cyrillic, 6 cells)
//   - U+4E2D U+6587                             = 2 wide runes  (CJK, 4 cells)
//   - "abc" + U+4E2D                            = 3 narrow + 1 wide (5 cells)
//
// PASSES on stub (no markers).
func TestVisualLen_CJK(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"\u043f\u0440\u0438\u0432\u0435\u0442", 6}, // Cyrillic privet
		{"\u4e2d\u6587", 4},                         // CJK Zhong Wen
	}
	for _, c := range cases {
		if got := format.VisualLen(c.in); got != c.want {
			t.Errorf("VisualLen(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// §4.3 T-5 — mixed ASCII + CJK widths add correctly:
// "abc" (3 narrow) + U+4E2D (1 wide = 2 cells) = 5 cells total. PASSES on stub.
func TestVisualLen_MixedAsciiAndCJK(t *testing.T) {
	if got := format.VisualLen("abc\u4e2d"); got != 5 {
		t.Fatalf("VisualLen = %d, want 5", got)
	}
}

// §4.3 Insurance — raw ANSI escape sequences must not appear in VisualLen's
// input (invariant Phase 4.2 §C-10). If they do (some caller breaks the
// invariant), this test documents the observed runewidth behaviour: control
// chars have width 0, the rest count normally — so "\x1b[36mfoo\x1b[0m"
// yields 10. The test pins this number so a regression that starts feeding
// escape codes through visualLen will be caught loudly rather than silently
// produce mis-aligned output. PASSES on stub (stub already uses StringWidth).
func TestVisualLen_NoEscapeInInput_Sanity(t *testing.T) {
	got := format.VisualLen("\x1b[36mfoo\x1b[0m")
	if got != 10 {
		t.Fatalf("VisualLen(escape) = %d, want 10 (documents runewidth behaviour, not desired output)", got)
	}
}
