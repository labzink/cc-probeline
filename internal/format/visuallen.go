// Package format — visual length helpers for status-line truncation.
//
// StripMarkers and VisualLen implementations land in Phase 4.3.b. The stubs
// keep the public API stable so truncate.FitLine and tests can compile against
// them during the foundation step. We import runewidth here so `go mod tidy`
// retains the dependency before 4.3.b lands; VisualLen calls it as a partial
// stub (no marker stripping yet), which guarantees the future RED tests fail
// on the StripMarkers contract rather than on a missing dependency.
package format

import "github.com/mattn/go-runewidth"

// StripMarkers removes our marker tokens of the form `{{name}}` or
// `{{name:value}}` from s. Recognised tokens: {{color:NAME}}, {{dim}},
// {{bold}}, {{italic}}, {{reset}}.
//
// Stub: returns s unchanged. Real implementation in 4.3.b.
func StripMarkers(s string) string {
	return s
}

// VisualLen returns the terminal column width of s after StripMarkers, using
// runewidth so wide UTF-8 glyphs (CJK, certain icons) count as 2.
//
// Invariant: callers must pass marker-form strings, never raw ANSI escape
// sequences. The assembler enforces this via §C-10 (Phase 4.2).
//
// Stub: skips StripMarkers, returns runewidth.StringWidth(s). 4.3.b RED tests
// expecting marker-stripped widths will fail until the real StripMarkers lands.
func VisualLen(s string) int {
	return runewidth.StringWidth(s)
}
