package renderer

import "os"

// Apply converts the {{color:NAME}}/{{dim}}/{{bold}}/{{italic}}/{{reset}}
// markers in text into ANSI escape codes using Theme.Colors. When
// theme.AnsiEnabled is false the markers are stripped instead.
// Phase 4.2.c fills in the real implementation; stub returns text as-is.
func Apply(text string, _ Theme) string { return text }

// DetectAnsi reports whether ANSI colour output is appropriate for the
// given stream. False when NO_COLOR is set or when the stream is not a
// terminal. Phase 4.2.c fills in the real detection; stub returns false.
func DetectAnsi(_ *os.File) bool { return false }
