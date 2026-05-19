// Package renderer_test — RED tests for Phase 4.2.c ANSI marker-pass.
// All TestApply_AnsiEnabled_* and TestApply_AnsiDisabled_* tests FAIL on the
// stub because Apply returns text as-is (no marker resolution or stripping).
// TestApply_NoMarkers, TestDetectAnsi_NoColorEnv, TestDetectAnsi_NonTty
// PASS on the stub — see per-test comments.
package renderer_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// §4.2 ANSI pass — Theme{AnsiEnabled: false} must strip markers, leaving plain text.
// FAILS on stub: stub returns text as-is, leaving "{{color:cyan}}" and "{{reset}}" intact.
func TestApply_AnsiDisabled_StripsMarkers(t *testing.T) {
	theme := renderer.Theme{AnsiEnabled: false}
	input := "hello {{color:cyan}}world{{reset}}"
	want := "hello world"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(AnsiDisabled): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — marker {{color:NAME}}…{{reset}} resolves to escape codes from Theme.Colors.
// FAILS on stub: stub returns text as-is, not resolving markers to escape codes.
func TestApply_AnsiEnabled_ResolvesColor(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: true,
		Colors: renderer.ColorScheme{
			Cyan:  "\x1b[36m",
			Reset: "\x1b[0m",
		},
	}
	input := "foo{{color:cyan}}bar{{reset}}baz"
	want := "foo\x1b[36mbar\x1b[0mbaz"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(AnsiEnabled, cyan): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — {{dim}}…{{reset}} maps to \x1b[2m…\x1b[0m (dim grey for separators).
// FAILS on stub: stub returns text as-is, not resolving {{dim}} or {{reset}}.
func TestApply_AnsiEnabled_Dim(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: true,
		Colors: renderer.ColorScheme{
			DimGrey: "\x1b[2m",
			Reset:   "\x1b[0m",
		},
	}
	input := "{{dim}}sep{{reset}}"
	want := "\x1b[2msep\x1b[0m"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(AnsiEnabled, dim): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — {{bold}}…{{reset}} maps to \x1b[1m…\x1b[0m.
// FAILS on stub: stub returns text as-is.
func TestApply_AnsiEnabled_Bold(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: true,
		Colors: renderer.ColorScheme{
			Bold:  "\x1b[1m",
			Reset: "\x1b[0m",
		},
	}
	input := "{{bold}}X{{reset}}"
	want := "\x1b[1mX\x1b[0m"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(AnsiEnabled, bold): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — {{italic}}…{{reset}} maps to \x1b[3m…\x1b[0m (hint widget).
// FAILS on stub: stub returns text as-is.
func TestApply_AnsiEnabled_Italic(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: true,
		Colors: renderer.ColorScheme{
			Italic: "\x1b[3m",
			Reset:  "\x1b[0m",
		},
	}
	input := "{{italic}}h{{reset}}"
	want := "\x1b[3mh\x1b[0m"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(AnsiEnabled, italic): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — unknown color name not in Theme.Colors: strip the marker token,
// leave surrounding content verbatim (no escape code injected).
// FAILS on stub: stub returns text as-is, leaving "{{color:purple}}" in output.
func TestApply_AnsiEnabled_UnknownColor(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: true,
		Colors:      renderer.ColorScheme{Reset: "\x1b[0m"},
		// Note: no Purple field — intentionally absent to verify graceful fallback.
	}
	input := "before{{color:purple}}after"
	want := "beforeafter"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(AnsiEnabled, unknownColor): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — composition: {{bold}}{{color:red}}X{{reset}} must preserve
// the order of codes: bold code immediately followed by red code, then reset.
// FAILS on stub: stub returns text as-is, leaving all markers unresolved.
func TestApply_NestedMarkers(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: true,
		Colors: renderer.ColorScheme{
			Bold:  "\x1b[1m",
			Red:   "\x1b[31m",
			Reset: "\x1b[0m",
		},
	}
	input := "{{bold}}{{color:red}}X{{reset}}"
	want := "\x1b[1m\x1b[31mX\x1b[0m"
	got := renderer.Apply(input, theme)
	if got != want {
		t.Errorf("Apply(nested markers): got %s, want %s",
			strconv.Quote(got), strconv.Quote(want))
	}
}

// §4.2 ANSI pass — plain text with no markers must pass through unchanged.
// PASS on stub: intentional contract for empty-input/no-op path.
// (stub returns text as-is, which is correct for no-marker input)
func TestApply_NoMarkers(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"ansi_disabled", false},
		{"ansi_enabled", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			theme := renderer.Theme{AnsiEnabled: tc.enabled}
			input := "hello"
			want := "hello"
			got := renderer.Apply(input, theme)
			if got != want {
				t.Errorf("Apply(NoMarkers, %s): got %s, want %s",
					tc.name, strconv.Quote(got), strconv.Quote(want))
			}
		})
	}
}

// §4.2 ANSI detect — NO_COLOR env set → DetectAnsi must return false.
// PASS on stub: intentional contract for empty-input/no-op path.
// (stub always returns false, which happens to be correct when NO_COLOR=1)
func TestDetectAnsi_NoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Use os.Stdout as the file argument; NO_COLOR check must fire before tty check.
	got := renderer.DetectAnsi(os.Stdout)
	if got != false {
		t.Errorf("DetectAnsi with NO_COLOR=1: got %s, want false",
			fmt.Sprintf("%v", got))
	}
}

// §4.2 ANSI detect — non-tty file (regular temp file) → DetectAnsi must return false.
// PASS on stub: intentional contract for empty-input/no-op path.
// (stub always returns false, which is correct for non-tty)
func TestDetectAnsi_NonTty(t *testing.T) {
	// Create a regular file — definitely not a terminal.
	f, err := os.CreateTemp(t.TempDir(), "fake-out")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	t.Cleanup(func() { f.Close() })

	// Ensure NO_COLOR is absent so the tty check is exercised.
	t.Setenv("NO_COLOR", "")

	got := renderer.DetectAnsi(f)
	if got != false {
		t.Errorf("DetectAnsi(non-tty file): got %v, want false", got)
	}
}
