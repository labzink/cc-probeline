// Package renderer provides Theme and ColorScheme types used by probes to
// gate ANSI output. Zero values (empty strings) produce plain-text output.
package renderer

// ColorScheme holds the ANSI escape sequences for each semantic colour role.
// All fields are empty strings in the zero value — safe for plain-text output.
type ColorScheme struct {
	DimGrey string // dim grey for separators
	Bold    string
	Reset   string
	Cyan    string // git branch, "orchestrator" label
	Yellow  string // warnings, TTL <30m, agent IDs
	Red     string // cache miss, TTL <5m, cost >90% budget
	Green   string // progress <50%, healthy cost
	Orange  string // progress 70-90%; 256-colour code (only exception to 16-colour rule)
	Magenta string // [high] effort indicator
	Italic  string // hint widget
	Dim     string // muted colour for secondary text

	// Pre-computed combinations used frequently by probes.
	BoldGreen  string
	BoldYellow string
	BoldRed    string
}

// Theme carries the active colour palette and terminal feature flags.
// NerdFont and AnsiEnabled are detected at startup; both default to false so
// that the zero value produces safe plain-text output.
type Theme struct {
	Colors      ColorScheme
	NerdFont    bool // true when a Nerd Font is detected in the terminal
	AnsiEnabled bool // false when NO_COLOR is non-empty; true otherwise
}

// DefaultPalette returns the standard 16-colour ANSI palette used by cc-probeline.
//
// All colours are standard SGR codes that respect the terminal's own colour
// theme, so output is readable on both light and dark backgrounds.
// The single exception is Orange (progress bar 70–90%), which uses the
// 256-colour code \x1b[38;5;208m because 16-colour ANSI has no orange.
func DefaultPalette() ColorScheme {
	return ColorScheme{
		// Style attributes.
		Bold:   "\x1b[1m",
		Dim:    "\x1b[2m",
		Italic: "\x1b[3m",
		Reset:  "\x1b[0m",

		// Named colours (16-colour ANSI SGR).
		Cyan:    "\x1b[36m",
		Yellow:  "\x1b[33m",
		Red:     "\x1b[31m",
		Green:   "\x1b[32m",
		Magenta: "\x1b[35m",

		// Orange: only 256-colour exception — no orange in standard 16-colour ANSI.
		// Code 208 is a mid-brightness orange, readable on light and dark backgrounds.
		Orange: "\x1b[38;5;208m",

		// DimGrey maps to the same dim attribute; terminal theme determines the shade.
		DimGrey: "\x1b[2m",

		// Pre-computed bold+colour combinations.
		BoldGreen:  "\x1b[1;32m",
		BoldYellow: "\x1b[1;33m",
		BoldRed:    "\x1b[1;31m",
	}
}
