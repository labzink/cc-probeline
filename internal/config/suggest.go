package config

import (
	"fmt"
	"strings"
)

// knownThemeNames is the list of valid theme name strings.
var knownThemeNames = []string{"default", "high-contrast", "minimal"}

// damerauLevenshtein computes the Damerau-Levenshtein edit distance between
// strings a and b, counting insertions, deletions, substitutions, and adjacent
// transpositions each as a single operation. Hand-rolled per project Q1 (no
// external dependencies). O(len(a)*len(b)) time and space.
func damerauLevenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la := len(ra)
	lb := len(rb)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// d[i][j] = distance between ra[:i] and rb[:j].
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
	}
	for i := 0; i <= la; i++ {
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := d[i-1][j] + 1
			ins := d[i][j-1] + 1
			sub := d[i-1][j-1] + cost
			best := del
			if ins < best {
				best = ins
			}
			if sub < best {
				best = sub
			}
			// Adjacent transposition: swap ra[i-2] and ra[i-1] to get rb[j-2]..rb[j-1].
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				t := d[i-2][j-2] + cost
				if t < best {
					best = t
				}
			}
			d[i][j] = best
		}
	}
	return d[la][lb]
}

// suggestFor returns an optional UI hint to accompany a config Error.
// Returns "" when no useful suggestion can be made. Pure function.
//
// Heuristics (concept §6.2):
//   - field "theme.name"         + unknown value → Levenshtein vs known palettes (threshold ≤ 2).
//   - field contains "color"     + 6-hex without '#' → missing '#' prefix hint.
//   - field contains "ratio"     + value > 1.0 (looks like percent) → percent hint.
//   - field contains bool key    + parseErr type-mismatch → true/false hint.
func suggestFor(field string, value any, parseErr error) string {
	// Boolean type-mismatch hint (check parseErr first — most specific).
	if parseErr != nil && strings.Contains(strings.ToLower(parseErr.Error()), "bool") {
		return fmt.Sprintf("%v expects a boolean (true/false), not a string", field)
	}

	switch {
	case field == "theme.name":
		s, ok := value.(string)
		if !ok {
			return ""
		}
		return suggestThemeName(s)

	case strings.Contains(field, "color"):
		s, ok := value.(string)
		if !ok {
			return ""
		}
		return suggestColor(s)

	case strings.Contains(field, "ratio"):
		return suggestRatio(field, value)
	}

	return ""
}

// suggestThemeName returns a Levenshtein-based "did you mean?" hint for an
// unknown theme name. Returns "" when the closest candidate is more than 2
// edits away (clearly not a typo).
func suggestThemeName(input string) string {
	best := ""
	bestDist := 3 // threshold: accept only dist ≤ 2
	for _, name := range knownThemeNames {
		d := damerauLevenshtein(input, name)
		if d < bestDist {
			bestDist = d
			best = name
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf(`did you mean %q?`, best)
}

// suggestColor returns a hint when the value looks like a 6-digit hex colour
// missing the '#' prefix.
func suggestColor(input string) string {
	if len(input) != 6 {
		return ""
	}
	// Check all characters are hex digits.
	for _, c := range input {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return fmt.Sprintf(`did you forget the '#' prefix? Try '#%s'`, input)
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
