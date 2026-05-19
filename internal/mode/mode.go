// Package mode stores the user's display preference (super-compact vs
// standard) in a global file under XDG_CONFIG_HOME (or $HOME/.config).
// Phase 4.2.a: real implementation — XDG-aware path, atomic write, flock.
package mode

import (
	"fmt"
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
// Returns "" when both XDG_CONFIG_HOME and HOME are empty.
// Decision C-1: global single file, not per-session/per-cwd.
func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline", "mode")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "cc-probeline", "mode")
}

// Load reads the persisted Mode from disk.
// Returns Default (Standard) when the file does not exist, Path() is empty,
// or the file contains an unrecognised value. Whitespace around the stored
// value is trimmed.
// Decision C-3: Default = Standard on any error or unknown value.
func Load() Mode {
	p := Path()
	if p == "" {
		return Default
	}
	b, err := os.ReadFile(p)
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
	if m != Standard && m != SuperCompact {
		return fmt.Errorf("mode.Save: unknown mode %q", m)
	}

	p := Path()
	if p == "" {
		return fmt.Errorf("mode: HOME not set")
	}
	dir := filepath.Dir(p)

	// Ensure the parent directory exists before acquiring the lock.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Acquire an exclusive file lock on <path>.lock (not the mode file itself).
	// Note: <path>.lock is never removed by design — flock advisory
	// requires a stable inode. Cleanup happens at uninstall (Phase 7).
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
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
		return current, err
	}
	return next, nil
}
