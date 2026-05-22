// Package settingsfile handles reading, writing, and modifying
// ~/.claude/settings.json. It is shared between the uninstall (5.b) and
// install/merge-settings (5.e) subcommands.
package settingsfile

import (
	"errors"
	"regexp"
	"time"
)

// Settings is a parsed JSON object from settings.json.
type Settings = map[string]any

// ourCommandRegex matches a command path that ends with cc-probeline or
// cc-probeline.exe, preceded by a path separator or start-of-string.
// This is the ownership marker per concept §5.3.
var ourCommandRegex = regexp.MustCompile(`(?:^|/|\\)cc-probeline(?:\.exe)?$`)

// Load reads and JSON-decodes the settings.json at path.
// Returns an error wrapping fs.ErrNotExist when the file is absent.
func Load(path string) (Settings, error) {
	return nil, errors.New("stub: 5.b GREEN not implemented")
}

// Save JSON-encodes s and atomically writes it to path via a temp file +
// os.Rename (same-filesystem atomic replace, per concept §4 / hint/state.go
// precedent).
func Save(path string, s Settings) error {
	return errors.New("stub: 5.b GREEN not implemented")
}

// IsOurs reports whether s contains a statusLine.command that was written by
// cc-probeline (matched by ourCommandRegex).
func IsOurs(s Settings) bool {
	return false
}

// RemoveStatusLine returns a new Settings map identical to s but with the
// "statusLine" key removed. Input is never mutated (immutable contract for
// shared-package extensibility, per concept §5.2 + plan §1 note).
func RemoveStatusLine(s Settings) Settings {
	return nil
}

// BackupPath returns the path where a backup of path would be written for
// timestamp ts, following the naming scheme:
//
//	<path>.cc-probeline.bak.<YYYYMMDD-HHmmss>
func BackupPath(path string, ts time.Time) string {
	return ""
}

// Backup copies the file at path to BackupPath(path, ts) and returns the
// backup path. Returns an error when the source file cannot be read or the
// destination cannot be written.
func Backup(path string, ts time.Time) (string, error) {
	return "", errors.New("stub: 5.b GREEN not implemented")
}
