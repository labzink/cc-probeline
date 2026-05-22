// Package settingsfile handles reading, writing, and modifying
// ~/.claude/settings.json. It is shared between the uninstall (5.b) and
// install/merge-settings (5.e) subcommands.
package settingsfile

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("settingsfile.Load: %w", err)
	}
	return s, nil
}

// Save JSON-encodes s and atomically writes it to path via a temp file +
// os.Rename (same-filesystem atomic replace, per concept §4 / hint/state.go
// precedent).
func Save(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("settingsfile.Save: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("settingsfile.Save: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("settingsfile.Save: rename: %w", err)
	}
	return nil
}

// IsOurs reports whether s contains a statusLine.command that was written by
// cc-probeline (matched by ourCommandRegex).
func IsOurs(s Settings) bool {
	sl, ok := s["statusLine"].(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := sl["command"].(string)
	return ourCommandRegex.MatchString(cmd)
}

// RemoveStatusLine returns a new Settings map identical to s but with the
// "statusLine" key removed. Input is never mutated (immutable contract for
// shared-package extensibility, per concept §5.2 + plan §1 note).
func RemoveStatusLine(s Settings) Settings {
	out := make(Settings, len(s))
	for k, v := range s {
		if k != "statusLine" {
			out[k] = v
		}
	}
	return out
}

// BackupPath returns the path where a backup of path would be written for
// timestamp ts, following the naming scheme:
//
//	<path>.cc-probeline.bak.<YYYYMMDD-HHmmss>
func BackupPath(path string, ts time.Time) string {
	return path + ".cc-probeline.bak." + ts.Format("20060102-150405")
}

// Backup copies the file at path to BackupPath(path, ts) and returns the
// backup path. Returns an error when the source file cannot be read or the
// destination cannot be written.
func Backup(path string, ts time.Time) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("settingsfile.Backup: open source: %w", err)
	}
	defer src.Close() //nolint:errcheck

	bakPath := BackupPath(path, ts)
	dst, err := os.OpenFile(bakPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("settingsfile.Backup: create backup: %w", err)
	}
	defer dst.Close() //nolint:errcheck

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(bakPath)
		return "", fmt.Errorf("settingsfile.Backup: copy: %w", err)
	}
	return bakPath, nil
}
