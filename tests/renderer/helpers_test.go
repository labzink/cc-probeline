// Package renderer_test — shared test helpers for the renderer test suite.
package renderer_test

import (
	"regexp"
)

// markerRE matches all {{marker}} tokens in raw renderer output.
var markerRE = regexp.MustCompile(`\{\{[a-z][a-z0-9:_-]*\}\}`)

// stripMk removes all {{marker}} tokens from a raw renderer output string,
// returning plain text that can be inspected for structural content.
// Equivalent to format.StripMarkers but local to the test package.
func stripMk(s string) string {
	return markerRE.ReplaceAllString(s, "")
}
