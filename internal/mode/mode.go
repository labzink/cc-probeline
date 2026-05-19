// Package mode stores the user's display preference (super-compact vs
// standard) in a global file under XDG_CONFIG_HOME (or $HOME/.config).
// Phase 4.2.a: real implementation — XDG-aware path, atomic write, flock.
package mode

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

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
// Resolves via XDG_CONFIG_HOME when set, otherwise falls back to
// $HOME/.config/cc-probeline/mode.
// Decision C-1: global single file, not per-session/per-cwd.
func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline", "mode")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "cc-probeline", "mode")
}

// Load reads the persisted Mode from disk.
// Returns Default (Standard) when the file does not exist or contains an
// unrecognised value. Whitespace around the stored value is trimmed.
// Decision C-3: Default = Standard on any error or unknown value.
func Load() Mode {
	b, err := os.ReadFile(Path())
	if err != nil {
		return Default
	}
	m := Mode(strings.TrimSpace(string(b)))
	if m != SuperCompact && m != Standard {
		return Default
	}
	return m
}

// Save persists m atomically to the mode storage file.
// Write sequence: MkdirAll → write to <path>.tmp → rename to <path>.
// The entire write is guarded by a flock on <path>.lock (a separate lock
// file so that concurrent readers on the mode file itself are never blocked).
// Decision C-2: atomic .tmp+rename; flock on dedicated .lock file.
func Save(m Mode) error {
	p := Path()
	dir := filepath.Dir(p)

	// Ensure the parent directory exists before acquiring the lock.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Acquire an exclusive file lock on <path>.lock (not the mode file itself).
	fl := flock.New(p + ".lock")
	if err := fl.Lock(); err != nil {
		return err
	}
	defer fl.Unlock() //nolint:errcheck

	// Write to a temporary file then atomically rename into place.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(m), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Toggle reads the current Mode, flips it, saves it, and returns the new value.
// Standard → SuperCompact; SuperCompact → Standard.
// Decision C-2: persistence via Save (atomic write + flock).
func Toggle() (Mode, error) {
	current := Load()
	var next Mode
	if current == Standard {
		next = SuperCompact
	} else {
		next = Standard
	}
	if err := Save(next); err != nil {
		return Default, err
	}
	return next, nil
}
