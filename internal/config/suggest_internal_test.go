package config

// Whitebox tests for damerauLevenshtein and suggestFor.
// These live in package config (not config_test) so the unexported functions
// are reachable directly.
//
// T-S1..T-S6: edit-distance correctness, including the transposition case
//   that distinguishes Damerau-Levenshtein from plain Levenshtein.
// T-S7..T-S12: suggestion heuristics per concept §6.2.
//
// Expected distances pre-computed by hand:
//   dL("default",  "default") = 0   identical
//   dL("defaolt",  "default") = 1   one substitution (o→u at index 5)
//   dL("defaul",   "default") = 1   one insertion (missing 't')
//   dL("defaultt", "default") = 1   one deletion  (extra 't')
//   dL("defalut",  "default") = 1   adjacent transposition l↔u (DL ≠ Levenshtein which would be 2)
//   dL("alpha",    "omega")   >= 4  a→o, l→m, p→e, h→g = 4 substitutions

import (
	"errors"
	"strings"
	"testing"
)

// --- T-S1..T-S6: damerauLevenshtein ---

func TestDamerauLevenshtein_Identical(t *testing.T) {
	got := damerauLevenshtein("default", "default")
	if got != 0 {
		t.Errorf("dL('default','default') = %d, want 0", got)
	}
}

func TestDamerauLevenshtein_Substitution(t *testing.T) {
	// "defaolt" vs "default": position 5 o→u (one sub)
	got := damerauLevenshtein("defaolt", "default")
	if got != 1 {
		t.Errorf("dL('defaolt','default') = %d, want 1", got)
	}
}

func TestDamerauLevenshtein_Insertion(t *testing.T) {
	// "defaul" vs "default": missing 't' at end (one insertion)
	got := damerauLevenshtein("defaul", "default")
	if got != 1 {
		t.Errorf("dL('defaul','default') = %d, want 1", got)
	}
}

func TestDamerauLevenshtein_Deletion(t *testing.T) {
	// "defaultt" vs "default": extra 't' at end (one deletion)
	got := damerauLevenshtein("defaultt", "default")
	if got != 1 {
		t.Errorf("dL('defaultt','default') = %d, want 1", got)
	}
}

// T-S5: Adjacent transposition — the discriminator vs plain Levenshtein.
// "defalut" has 'l' and 'u' swapped relative to "default".
// Plain Levenshtein would score 2 (delete+insert); DL scores 1 (single transposition op).
func TestDamerauLevenshtein_Transposition(t *testing.T) {
	got := damerauLevenshtein("defalut", "default")
	if got != 1 {
		t.Errorf("dL('defalut','default') = %d, want 1 (adjacent transposition)", got)
	}
}

func TestDamerauLevenshtein_VeryDifferent(t *testing.T) {
	got := damerauLevenshtein("alpha", "omega")
	if got < 4 {
		t.Errorf("dL('alpha','omega') = %d, want >= 4", got)
	}
}

// --- T-S7..T-S12: suggestFor ---

// T-S7: Close typo of a known theme name → suggestion contains the correct name.
// "defalt" is DL=1 from "default" → should be suggested.
func TestSuggestFor_ThemeName_Levenshtein1(t *testing.T) {
	hint := suggestFor("theme.name", "defalt", nil)
	if !strings.Contains(hint, "default") {
		t.Errorf("suggestFor(theme.name, 'defalt') = %q, want to contain 'default'", hint)
	}
}

// T-S8: "neon" is DL >= 3 from all known themes → no suggestion.
func TestSuggestFor_ThemeName_TooFar_NoSuggestion(t *testing.T) {
	hint := suggestFor("theme.name", "neon", nil)
	if hint != "" {
		t.Errorf("suggestFor(theme.name, 'neon') = %q, want '' (too far)", hint)
	}
}

// T-S9: Hex without '#' prefix → hint mentions '#'.
func TestSuggestFor_Color_NoHash(t *testing.T) {
	hint := suggestFor("theme.colors.red", "FF00AA", nil)
	if !strings.Contains(hint, "#") {
		t.Errorf("suggestFor(theme.colors.red, 'FF00AA') = %q, want hint containing '#'", hint)
	}
}

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
