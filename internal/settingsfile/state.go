package settingsfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InstallState records what cc-probeline did to the user's environment so that
// uninstall can reverse it. Persisted to StatePath() between install and
// uninstall runs.
type InstallState struct {
	Version          int       `json:"version"`
	PreInstallBackup string    `json:"pre_install_backup,omitempty"`
	HadForeign       bool      `json:"had_foreign"`
	CreatedAt        time.Time `json:"created_at"`
}

const installStateVersion = 1

// StatePath returns the absolute path to install-state.json under
// $XDG_CONFIG_HOME/cc-probeline/ (fallback ~/.config/cc-probeline/).
// Returns "" when neither XDG_CONFIG_HOME nor HOME is set.
func StatePath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "cc-probeline", "install-state.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "cc-probeline", "install-state.json")
}

// LoadState reads the install state from path. Returns (nil, nil) when the
// file is missing (uninstall must treat this as "no state, fall back to
// remove-our-block behaviour"). Returns an error on parse failure.
func LoadState(path string) (*InstallState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("settingsfile.LoadState: %w", err)
	}
	var s InstallState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("settingsfile.LoadState: parse: %w", err)
	}
	return &s, nil
}

// SaveState atomically writes s to path (tmp + rename), creating parent
// directories as needed.
func SaveState(path string, s InstallState) error {
	if s.Version == 0 {
		s.Version = installStateVersion
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("settingsfile.SaveState: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("settingsfile.SaveState: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("settingsfile.SaveState: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("settingsfile.SaveState: rename: %w", err)
	}
	return nil
}

// RemoveState deletes the state file at path. Missing file is not an error
// (idempotent uninstall).
func RemoveState(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("settingsfile.RemoveState: %w", err)
	}
	return nil
}
