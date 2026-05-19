package renderer

import (
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// markerRe matches any {{...}} token in the input text.
var markerRe = regexp.MustCompile(`\{\{[a-z][a-z0-9:_-]*\}\}`)

// resolveMarker returns the ANSI escape code for a marker token (without the
// surrounding braces), or an empty string when the marker is unknown or the
// color name has no mapping in the theme.
// Color markers MUST use the {{color:NAME}} form. Style markers ({{dim}}, {{bold}}, {{italic}}, {{reset}}) are bare.
func resolveMarker(token string, cs ColorScheme) string {
	switch token {
	case "dim":
		return cs.DimGrey
	case "bold":
		return cs.Bold
	case "italic":
		return cs.Italic
	case "reset":
		return cs.Reset
	}
	// Handle {{color:NAME}} tokens.
	if strings.HasPrefix(token, "color:") {
		name := strings.TrimPrefix(token, "color:")
		switch name {
		case "cyan":
			return cs.Cyan
		case "yellow":
			return cs.Yellow
		case "red":
			return cs.Red
		case "green":
			return cs.Green
		case "orange":
			return cs.Orange
		case "magenta":
			return cs.Magenta
		case "bold_green":
			return cs.BoldGreen
		case "bold_yellow":
			return cs.BoldYellow
		case "bold_red":
			return cs.BoldRed
		}
		// Unknown color name — strip the marker, return empty string.
		return ""
	}
	// Unknown marker type — strip it.
	return ""
}

// Apply converts {{marker}} tokens in text to ANSI escape sequences using the
// colours defined in t. When t.AnsiEnabled is false all markers are stripped
// and the plain text is returned.
func Apply(text string, t Theme) string {
	return markerRe.ReplaceAllStringFunc(text, func(match string) string {
		// Strip the surrounding {{ and }}.
		token := match[2 : len(match)-2]
		if !t.AnsiEnabled {
			// Strip mode: always return empty string for any marker.
			return ""
		}
		return resolveMarker(token, t.Colors)
	})
}

// DetectAnsi reports whether ANSI colour output is appropriate for stdout.
// Returns false when the NO_COLOR environment variable is non-empty (C-8) or
// when the provided file is not a terminal (C-7).
func DetectAnsi(stdout *os.File) bool {
	// C-8: respect NO_COLOR convention.
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// C-7: only enable ANSI when writing to an actual terminal.
	if !term.IsTerminal(int(stdout.Fd())) {
		return false
	}
	return true
}
