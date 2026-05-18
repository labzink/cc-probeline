// Package renderer provides Theme and ColorScheme types used by probes to
// gate ANSI output. Actual ANSI codes are populated in Phase 4.2; zero
// values (empty strings) produce plain-text output.
package renderer

// ColorScheme holds the ANSI escape sequences for each semantic colour role.
// All fields are empty strings in the zero value — safe for plain-text output.
type ColorScheme struct {
	DimGrey string // dim grey for separators
	Bold    string
	Reset   string
	Cyan    string // git branch, "orch" label
	Yellow  string // warnings, TTL <30m, agent IDs
	Red     string // cache miss, TTL <5m, cost >90% budget
	Green   string // progress <50%, healthy cost
	Orange  string // progress 70-90%
	Magenta string // [high] effort indicator
	Italic  string // hint widget

	// Pre-computed combinations used frequently by probes.
	BoldGreen  string
	BoldYellow string
	BoldRed    string
}

// Theme carries the active colour palette and terminal feature flags.
// NerdFont and AnsiEnabled are detected at startup (Phase 4.2); both default
// to false so that the zero value produces safe plain-text output.
type Theme struct {
	Colors      ColorScheme
	NerdFont    bool // true when a Nerd Font is detected in the terminal
	AnsiEnabled bool // false when NO_COLOR=1 or stdout is not a tty
}
