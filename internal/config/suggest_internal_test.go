package config

// Whitebox tests for suggestFor. These live in package config (not config_test)
// so the unexported functions are reachable directly.
//
// Heuristics covered (concept §6.2, minus the theme/colour cases removed when
// the theme config was cut in Phase 7.47):
//   - ratio value that looks like a percent → percent hint.
//   - bool field supplied as a string → true/false hint.
//   - unrecognised field → empty hint.

import (
	"errors"
	"strings"
	"testing"
)

// T-S10: Ratio value that looks like a percent (70.0) → hint mentions "percent".
func TestSuggestFor_RatioAsPercent(t *testing.T) {
	hint := suggestFor("thresholds.ctx_warn_ratio", 70.0, nil)
	if !strings.Contains(strings.ToLower(hint), "percent") {
		t.Errorf("suggestFor(ctx_warn_ratio, 70.0) = %q, want hint mentioning 'percent'", hint)
	}
}

// T-S11: Boolean field supplied as a string → hint mentions true/false.
// Simulate a TOML type-mismatch parse error.
func TestSuggestFor_BoolAsString_FromParseErr(t *testing.T) {
	typeErr := errors.New("toml: type mismatch for bool field")
	hint := suggestFor("general.tutorial_hints", "yes", typeErr)
	lower := strings.ToLower(hint)
	if !strings.Contains(lower, "true") && !strings.Contains(lower, "false") {
		t.Errorf("suggestFor(tutorial_hints, 'yes', typeErr) = %q, want hint mentioning true/false", hint)
	}
}

// T-S12: Unrecognised field with a non-special value → empty hint.
func TestSuggestFor_Unknown_ReturnsEmpty(t *testing.T) {
	hint := suggestFor("some.weird.field", 42, nil)
	if hint != "" {
		t.Errorf("suggestFor(some.weird.field, 42) = %q, want ''", hint)
	}
}
