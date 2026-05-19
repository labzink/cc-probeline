// Package renderer_test — RED tests for Phase 4.3.a width detection.
//
// DetectCols resolution order (§4.3, specs.md §A4):
//  1. Environment variable $COLUMNS, if a positive integer.
//  2. Syscall via golang.org/x/term.GetSize on os.Stdout.
//  3. Fallback to 80.
//
// In `go test` stdout is a pipe (non-tty), so term.GetSize returns an error
// and tests reliably exercise either the env branch or the 80 fallback.
//
// Stub behaviour: DetectCols always returns 80. With the stub in place,
// FromEnv and EnvWins_OverTty FAIL; the other tests PASS by coincidence
// (their expected value matches the stub) — see per-test comments.
package renderer_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// §4.3 T-1 — $COLUMNS is honoured when it parses as a positive integer.
// FAILS on stub: stub returns 80, expected 143.
func TestDetectCols_FromEnv(t *testing.T) {
	t.Setenv("COLUMNS", "143")
	got := renderer.DetectCols()
	if got != 143 {
		t.Fatalf("DetectCols() = %d, want 143", got)
	}
}

// §4.3 T-2 — COLUMNS=0 is rejected (must be >0); fall through to fallback.
// PASSES on stub because stub already returns 80 — proper coverage arrives
// once the real implementation must explicitly reject zero.
func TestDetectCols_EnvZero_FallsThrough(t *testing.T) {
	t.Setenv("COLUMNS", "0")
	got := renderer.DetectCols()
	if got != 80 {
		t.Fatalf("DetectCols() = %d, want 80 (fallback after rejecting 0)", got)
	}
}

// §4.3 T-2 — negative COLUMNS is rejected.
// PASSES on stub (coincidence).
func TestDetectCols_EnvNegative_FallsThrough(t *testing.T) {
	t.Setenv("COLUMNS", "-5")
	got := renderer.DetectCols()
	if got != 80 {
		t.Fatalf("DetectCols() = %d, want 80 (fallback after rejecting -5)", got)
	}
}

// §4.3 T-2 — non-numeric COLUMNS is rejected.
// PASSES on stub (coincidence).
func TestDetectCols_EnvGarbage_FallsThrough(t *testing.T) {
	t.Setenv("COLUMNS", "abc")
	got := renderer.DetectCols()
	if got != 80 {
		t.Fatalf("DetectCols() = %d, want 80 (fallback after rejecting 'abc')", got)
	}
}

// §4.3 T-1 — empty COLUMNS is treated as unset; fall through.
// PASSES on stub (coincidence).
func TestDetectCols_EnvEmpty_FallsThrough(t *testing.T) {
	t.Setenv("COLUMNS", "")
	got := renderer.DetectCols()
	if got != 80 {
		t.Fatalf("DetectCols() = %d, want 80 (fallback after empty COLUMNS)", got)
	}
}

// §4.3 T-3 — when COLUMNS is unset and term.GetSize fails (pipe stdout in
// `go test`), DetectCols returns 80. PASSES on stub (coincidence).
func TestDetectCols_Fallback80(t *testing.T) {
	t.Setenv("COLUMNS", "")
	got := renderer.DetectCols()
	if got != 80 {
		t.Fatalf("DetectCols() = %d, want 80 (fallback)", got)
	}
}

// §4.3 T-1 — env takes precedence even when stdout would otherwise yield a
// TTY width. We cannot reliably create a TTY in `go test`, so this test
// documents the env-precedence contract: a valid $COLUMNS short-circuits the
// resolution before term.GetSize is consulted.
// FAILS on stub: stub returns 80, expected 200.
func TestDetectCols_EnvWins_OverTty(t *testing.T) {
	t.Setenv("COLUMNS", "200")
	got := renderer.DetectCols()
	if got != 200 {
		t.Fatalf("DetectCols() = %d, want 200 (env precedence)", got)
	}
}
