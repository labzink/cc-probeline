// Package renderer — terminal width detection.
//
// DetectCols implementation lands in Phase 4.3.a. This stub returns the
// 80-column fallback so callers can wire the call site early.
package renderer

// DetectCols returns the terminal width in columns. Resolution order
// (§4.3, specs.md §A4):
//
//  1. Environment variable $COLUMNS, if a positive integer.
//  2. Syscall via golang.org/x/term.GetSize on os.Stdout.
//  3. Fallback to 80.
//
// Stub: always returns 80. Real implementation in 4.3.a.
func DetectCols() int {
	return 80
}
