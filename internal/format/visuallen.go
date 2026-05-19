// Package format — visual length helpers for status-line truncation.
package format

import (
	"regexp"

	"github.com/mattn/go-runewidth"
)

// markerRE matches our marker grammar: `{{` + a lowercase letter + zero or
// more of [a-z0-9:_-] + `}}`. The grammar is closed: the assembler emits
// only the recognised palette ({{color:NAME}}, {{dim}}, {{bold}},
// {{italic}}, {{reset}}), but the regex is generic so unknown tokens with
// the same shape also strip. Malformed sequences (single braces, missing
// closer, leading digit) pass through unchanged.
var markerRE = regexp.MustCompile(`\{\{[a-z][a-z0-9:_-]*\}\}`)

// StripMarkers removes our marker tokens from s.
func StripMarkers(s string) string {
	return markerRE.ReplaceAllString(s, "")
}

// VisualLen returns the terminal column width of s after StripMarkers, using
// runewidth so wide UTF-8 glyphs (CJK) count as 2.
//
// Invariant: callers must pass marker-form strings, never raw ANSI escape
// sequences. The assembler enforces this via §C-10 (Phase 4.2). Control
// bytes have runewidth 0, so a leaked escape would silently mis-count by
// the printable portion only — see tests/format/visuallen_test.go:
// TestVisualLen_NoEscapeInInput_Sanity for the pinned behaviour.
func VisualLen(s string) int {
	return runewidth.StringWidth(StripMarkers(s))
}
