package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// SetTutorialHints atomically updates only the [general].tutorial_hints field
// in the TOML at path. Atomicity via tmp+rename (precedent: settingsfile.Save).
//
// Other keys are preserved through pelletier round-trip. NOTE: comments are
// LOST — pelletier/go-toml/v2 does not preserve TOML comments on round-trip.
// This is a known limitation documented in concept §10.2 + insurance #4.
// BL-15: move to AST-based edit to preserve TOML comments (Phase 7).
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

	// 2. Parse the existing TOML — must be valid, do not clobber on error.
	// Start from Default() so sections omitted from the file keep their default
	// values instead of being round-tripped to zero, which would write a
	// dishonest config (e.g. ctx_*_ratio = 0.0 or table_rows = 0) that no longer
	// reflects the effective behaviour.
	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("existing config is invalid; run 'cc-probeline check-config' for details, then fix or remove %s: %w", path, err)
	}

	// 3. Mutate the single target field.
	cfg.General.TutorialHints = value

	// 4. Marshal back. Comments are lost here — see godoc above.
	out, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// 5. Atomic write via tmp+rename.
	return atomicWrite(path, out)
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
