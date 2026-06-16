package config

import (
	"fmt"
	"strings"
)

// suggestFor returns an optional UI hint to accompany a config Error.
// Returns "" when no useful suggestion can be made. Pure function.
//
// Heuristics:
//   - field contains "ratio" + value > 1.0 (looks like percent) → percent hint.
//   - bool key + parseErr type-mismatch → true/false hint.
func suggestFor(field string, value any, parseErr error) string {
	// Boolean type-mismatch hint (check parseErr first — most specific).
	if parseErr != nil && strings.Contains(strings.ToLower(parseErr.Error()), "bool") {
		return fmt.Sprintf("%v expects a boolean (true/false), not a string", field)
	}

	if strings.Contains(field, "ratio") {
		return suggestRatio(field, value)
	}

	return ""
}

// suggestRatio returns a hint when a ratio value looks like it was given as a
// percentage (e.g. 70 instead of 0.70).
func suggestRatio(field string, value any) string {
	var f float64
	switch v := value.(type) {
	case float64:
		f = v
	case float32:
		f = float64(v)
	case int:
		f = float64(v)
	case int64:
		f = float64(v)
	default:
		return ""
	}
	if f > 1.0 {
		return fmt.Sprintf("value %.4g looks like a percent; ratios use 0.0-1.0 (try %.2f)", f, f/100)
	}
	return ""
}
