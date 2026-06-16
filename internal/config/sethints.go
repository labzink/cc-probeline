package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// SetTutorialHints atomically updates only the [general].tutorial_hints field
// in the TOML at path. Atomicity via tmp+rename (precedent: settingsfile.Save).
//
// BL-15: the value is edited surgically (line-level), so all other lines —
// comments, blank lines, key order, whitespace and unknown sections — are
// preserved byte-for-byte. Only the single tutorial_hints value changes. This
// replaces the previous unmarshal→marshal round-trip, which dropped comments
// and reformatted the whole file.
//
// If path does not exist:
//  1. mkdir -p the parent directory.
//  2. Write a minimal config: `version = 1\n\n[general]\ntutorial_hints = <value>\n`
//     (no full template — the user explicitly asked for one toggle).
//
// If path exists but contains broken TOML:
//   - Returns error wrapping the parse failure.
//   - Does NOT overwrite (risk of clobbering user-edited fields).
//   - Caller (runHints) should print an actionable message and exit 2.
//
// Returns nil on success, or a wrapped io/fs/parse failure.
func SetTutorialHints(path string, value bool) error {
	// 1. Attempt to read the existing file.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return createMinimalHintsFile(path, value)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	// 2. Validate the existing TOML — must be valid, do not clobber on error.
	if err := toml.Unmarshal(data, Default()); err != nil {
		return fmt.Errorf("existing config is invalid; run 'cc-probeline check-config' for details, then fix or remove %s: %w", path, err)
	}

	// 3. Surgical edit: change only the tutorial_hints value, preserving
	// everything else (comments, formatting, key order).
	out := setGeneralTutorialHints(data, value)

	// 4. Atomic write via tmp+rename.
	return atomicWrite(path, out)
}

// setGeneralTutorialHints returns data with [general].tutorial_hints set to
// value, editing only that value in place. If the key is present its value is
// replaced (preserving indentation and any inline comment); if the [general]
// table exists without the key, the key is inserted right after the header; if
// no [general] table exists, one is appended. The input is assumed to be valid
// TOML (validated by the caller).
func setGeneralTutorialHints(data []byte, value bool) []byte {
	lines := strings.Split(string(data), "\n")
	val := strconv.FormatBool(value)

	inGeneral := false
	generalHeaderIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Top-level dotted form: general.tutorial_hints = ...
		if !inGeneral && isAssignment(trimmed, "general.tutorial_hints") {
			lines[i] = replaceTOMLValue(line, val)
			return []byte(strings.Join(lines, "\n"))
		}

		// Table header line: [general], [widgets], ...
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			inGeneral = name == "general"
			if inGeneral {
				generalHeaderIdx = i
			}
			continue
		}

		if inGeneral && isAssignment(trimmed, "tutorial_hints") {
			lines[i] = replaceTOMLValue(line, val)
			return []byte(strings.Join(lines, "\n"))
		}
	}

	// Key absent but [general] table present → insert after the header.
	if generalHeaderIdx >= 0 {
		insert := []string{"tutorial_hints = " + val}
		lines = append(lines[:generalHeaderIdx+1], append(insert, lines[generalHeaderIdx+1:]...)...)
		return []byte(strings.Join(lines, "\n"))
	}

	// No [general] table at all → append one.
	out := string(data)
	if len(out) > 0 && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += "\n[general]\ntutorial_hints = " + val + "\n"
	return []byte(out)
}

// isAssignment reports whether a trimmed line assigns to key (i.e. starts with
// key followed by optional spaces and '='), excluding comment lines.
func isAssignment(trimmed, key string) bool {
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	rest := strings.TrimPrefix(trimmed, key)
	if rest == trimmed {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	return strings.HasPrefix(rest, "=")
}

// replaceTOMLValue replaces the value after '=' on a key line with val,
// preserving the key, indentation, and any trailing inline comment. The
// existing value is assumed to be a scalar without '#' (tutorial_hints is bool).
func replaceTOMLValue(line, val string) string {
	eq := strings.Index(line, "=")
	if eq < 0 {
		return line
	}
	head := line[:eq+1]
	comment := ""
	if h := strings.Index(line[eq+1:], "#"); h >= 0 {
		comment = " " + strings.TrimSpace(line[eq+1+h:])
	}
	return head + " " + val + comment
}

// GlobalConfigPath returns the platform-appropriate global config location
// (XDG / HOME / APPDATA cascade). Re-exports the unexported globalConfigPath
// from path.go. Returns "" when no location can be determined.
func GlobalConfigPath() string { return globalConfigPath() }

// createMinimalHintsFile creates parent directories and writes a minimal config
// file containing only version and [general].tutorial_hints. Called when the
// config file does not yet exist.
func createMinimalHintsFile(path string, value bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", path, err)
	}
	content := fmt.Sprintf("version = 1\n\n[general]\ntutorial_hints = %t\n", value)
	return atomicWrite(path, []byte(content))
}

// atomicWrite writes content to path via a .tmp sibling, then renames.
// os.Rename is atomic on POSIX (same-FS) and uses MoveFileEx on Windows.
// If WriteFile fails, path is not touched. If Rename fails, the .tmp file is
// removed (best-effort) to avoid leaving orphaned temporaries on disk.
// KISS: no flock (TOML editing is rare).
func atomicWrite(path string, content []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup; ignore error
		return err
	}
	return nil
}
