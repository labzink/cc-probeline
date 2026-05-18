// Package renderer_test contains black-box foundation tests for internal/renderer.
// Covers Theme and ColorScheme structural contracts from the Phase 4 concept.
package renderer_test

import (
	"reflect"
	"testing"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// TestColorScheme_AllFieldsPresent is a structural smoke test that verifies
// ColorScheme exposes all fields specified in the Phase 4 concept
// (section "Common interfaces"). Adding fields to ColorScheme is additive and
// non-breaking; removing any listed field will fail this test.
func TestColorScheme_AllFieldsPresent(t *testing.T) {
	// Expected field names from concept spec (section "Common interfaces").
	required := []string{
		"DimGrey",
		"Bold",
		"Reset",
		"Cyan",
		"Yellow",
		"Red",
		"Green",
		"Orange",
		"Magenta",
		"Italic",
		"BoldGreen",
		"BoldYellow",
		"BoldRed",
	}

	rt := reflect.TypeOf(renderer.ColorScheme{})
	for _, name := range required {
		if _, ok := rt.FieldByName(name); !ok {
			t.Errorf("ColorScheme is missing field %q (required by concept spec)", name)
		}
	}
}

// TestTheme_AnsiDisabled is a structural smoke test that verifies Theme can be
// constructed with AnsiEnabled=false. Probes use Theme to gate ANSI output;
// when AnsiEnabled is false the renderer must produce plain text.
// The test checks that Theme{AnsiEnabled: false} is a valid zero-ish state
// and that the field is accessible.
func TestTheme_AnsiDisabled(t *testing.T) {
	theme := renderer.Theme{
		AnsiEnabled: false,
		NerdFont:    false,
		Colors:      renderer.ColorScheme{},
	}

	if theme.AnsiEnabled {
		t.Error("Theme{AnsiEnabled: false}.AnsiEnabled: want false, got true")
	}
	if theme.NerdFont {
		t.Error("Theme{NerdFont: false}.NerdFont: want false, got true")
	}

	// All ColorScheme fields must be empty strings when zero-initialized
	// (no ANSI codes embedded in struct literals).
	rt := reflect.TypeOf(theme.Colors)
	rv := reflect.ValueOf(theme.Colors)
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		val := rv.Field(i).String()
		if val != "" {
			t.Errorf("ColorScheme.%s: want empty string for zero value, got %q", field.Name, val)
		}
	}
}
