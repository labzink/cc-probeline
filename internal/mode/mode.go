// Package mode stores the user's display preference (super-compact vs
// standard) in a global file under XDG_CONFIG_HOME (or $HOME/.config).
// Phase 4.2.a fills in the real implementation; this file is a foundation
// stub that defines public types and signatures so RED tests can compile.
package mode

// Mode names the rendering mode selected by the user.
type Mode string

const (
	SuperCompact Mode = "super-compact"
	Standard     Mode = "standard"
)

// Default is returned by Load when the file is missing or its contents
// do not match a known Mode.
const Default = Standard

// Path returns the absolute path of the mode storage file.
// Stub: returns an empty string until 4.2.a implements XDG/HOME resolution.
func Path() string { return "" }

// Load reads the persisted Mode from disk. Stub returns Default.
func Load() Mode { return Default }

// Save persists the Mode atomically (.tmp + rename) under flock.
// Stub returns nil without touching the filesystem.
func Save(_ Mode) error { return nil }

// Toggle flips the current Mode and persists it.
// Stub returns (Default, nil).
func Toggle() (Mode, error) { return Default, nil }
