// Package format holds small string/number formatting helpers shared by the
// probes and the renderer. Living here breaks the would-be import cycle
// (probes ↔ renderer) without forcing either side to duplicate the logic.
package format

import "fmt"

// MiddleTruncate returns s unchanged when len([]rune(s)) <= minWidth.
// Otherwise it middle-truncates s with "…", using two regimes:
//
// Regime 1 — string is relatively short (floor(len/2) < minWidth-1):
//
//	head = floor(len/2)
//	tail = max(minWidth - head, 2)   // no -1 for ellipsis
//	Total output > minWidth.
//
// Regime 2 — string is long (floor(len/2) >= minWidth-1):
//
//	tail = floor((minWidth-1)/2)
//	head = minWidth - 1 - tail
//	Total output == minWidth.
//
// Concrete examples (minWidth=8):
//
//	"cc-probeline" (12)               → regime 1 → "cc-pro…ne"  (9 runes)
//	"my-super-long-project-name" (26) → regime 2 → "my-s…ame"   (8 runes)
func MiddleTruncate(s string, minWidth int) string {
	runes := []rune(s)
	n := len(runes)
	if n <= minWidth {
		return s
	}
	half := n / 2
	var head, tail int
	if half < minWidth-1 {
		head = half
		tail = minWidth - head
	} else {
		tail = (minWidth - 1) / 2
		head = minWidth - 1 - tail
	}
	return string(runes[:head]) + "…" + string(runes[n-tail:])
}

// FormatK converts a token count to a "K" abbreviated string (e.g. 128000 → "128K").
// Values < 1000 are returned as-is (e.g. 500 → "500").
func FormatK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%dK", n/1000)
	}
	return fmt.Sprintf("%d", n)
}

// FormatMMSS converts a duration in milliseconds to "MM:SS" format.
// Example: 3661000 ms → "61:01".
func FormatMMSS(ms int64) string {
	totalSec := ms / 1000
	return fmt.Sprintf("%02d:%02d", totalSec/60, totalSec%60)
}
