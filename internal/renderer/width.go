// Package renderer — terminal width detection.
package renderer

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

// DetectCols returns the terminal width in columns. Resolution order
// (§4.3, specs.md §A4):
//
//  1. Environment variable $COLUMNS, if it parses as a positive integer.
//  2. Syscall via golang.org/x/term.GetSize on os.Stdout.
//  3. Fallback to 80.
//
// Each step only commits when its result is strictly positive; anything else
// falls through to the next step.
func DetectCols() int {
	if s := os.Getenv("COLUMNS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}
