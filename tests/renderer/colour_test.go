// Package renderer_test — RED tests for Phase 6.7.a: DefaultPalette + DetectAnsi rework.
//
// T-1: DefaultPalette() returns a fully-populated 16-colour ANSI palette
//      (Cyan, Red, Orange verified by value; all other fields non-empty).
// T-2: DetectAnsi(f) returns true for non-tty file when NO_COLOR is absent,
//      and false when NO_COLOR=1.
//
// Both tests FAIL until dev implements DefaultPalette() and reworks DetectAnsi.
package renderer_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// TestDefaultPalette_Values verifies that DefaultPalette() returns the correct
// ANSI escape sequences for spot-checked colour fields (T-1, value assertions).
func TestDefaultPalette_Values(t *testing.T) {
	p := renderer.DefaultPalette()

	cases := []struct {
		field string
		got   string
		want  string
	}{
		{"Cyan", p.Cyan, "\x1b[36m"},
		{"Red", p.Red, "\x1b[31m"},
		{"Orange", p.Orange, "\x1b[38;5;208m"},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("DefaultPalette().%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

// TestDefaultPalette_AllFieldsNonEmpty verifies that DefaultPalette() leaves no
// field empty — a zero field means that colour category silently drops markers (T-1).
// The fields checked here are the ones required by spec-common §2.1.
func TestDefaultPalette_AllFieldsNonEmpty(t *testing.T) {
	p := renderer.DefaultPalette()

	// All required fields must be non-empty strings.
	required := []struct {
		name string
		val  string
	}{
		{"Bold", p.Bold},
		{"Italic", p.Italic},
		{"Reset", p.Reset},
		{"Dim", p.Dim},
		{"DimGrey", p.DimGrey},
		{"Cyan", p.Cyan},
		{"Yellow", p.Yellow},
		{"Red", p.Red},
		{"Green", p.Green},
		{"Orange", p.Orange},
		{"Magenta", p.Magenta},
		{"BoldGreen", p.BoldGreen},
		{"BoldYellow", p.BoldYellow},
		{"BoldRed", p.BoldRed},
	}

	for _, f := range required {
		if f.val == "" {
			t.Errorf("DefaultPalette().%s is empty, want non-empty ANSI code", f.name)
		}
	}
}

// TestDefaultPalette_TypeIsColorScheme is a compile-time + runtime sanity check
// that DefaultPalette() returns renderer.ColorScheme (not a pointer or alias).
func TestDefaultPalette_TypeIsColorScheme(t *testing.T) {
	p := renderer.DefaultPalette()
	_ = reflect.TypeOf(p) // will not compile if type is wrong
	// Verify it's a value type (not nil-able pointer).
	var _ renderer.ColorScheme = p
}

// TestDetectAnsi_NoColor_Absent verifies that DetectAnsi returns true for a
// non-tty temp file when NO_COLOR is empty (new colour-by-default semantics, T-2).
// FAILS on current implementation: tty-check returns false for non-tty files.
func TestDetectAnsi_NoColor_Absent(t *testing.T) {
	// Given: NO_COLOR is unset.
	t.Setenv("NO_COLOR", "")

	// Given: file is definitely not a terminal.
	f, err := os.CreateTemp(t.TempDir(), "non-tty-out")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	t.Cleanup(func() { f.Close() })

	// When: DetectAnsi is called.
	got := renderer.DetectAnsi(f)

	// Then: must return true — colour-by-default, tty-check removed.
	if got != true {
		t.Errorf("DetectAnsi(non-tty, NO_COLOR=''): got %v, want true", got)
	}
}

// TestDetectAnsi_NoColor_Set verifies that DetectAnsi returns false when
// NO_COLOR is non-empty, regardless of tty status (T-2).
// This behaviour is unchanged from the previous implementation.
func TestDetectAnsi_NoColor_Set(t *testing.T) {
	// Given: NO_COLOR=1 is set.
	t.Setenv("NO_COLOR", "1")

	f, err := os.CreateTemp(t.TempDir(), "non-tty-out")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	t.Cleanup(func() { f.Close() })

	// When: DetectAnsi is called.
	got := renderer.DetectAnsi(f)

	// Then: must return false — NO_COLOR opt-out.
	if got != false {
		t.Errorf("DetectAnsi(NO_COLOR=1): got %v, want false", got)
	}
}
