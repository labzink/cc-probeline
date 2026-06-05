package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// tableRowsCap is the maximum value accepted by SetTableRows.
const tableRowsCap = 40

// validModes lists the accepted values for SetMode.
var validModes = []string{"standard", "super-compact"}

// SetMode atomically updates [general].mode in the TOML at path.
// Accepted values: "standard", "super-compact". Any other value returns an
// error and leaves the file unchanged.
// Round-trip semantics and file-creation behaviour mirror SetTutorialHints.
func SetMode(path, mode string) error {
	if !isValidMode(mode) {
		return fmt.Errorf("invalid mode %q: accepted values are %s",
			mode, strings.Join(validModes, ", "))
	}
	cfg, err := readOrDefault(path)
	if err != nil {
		return err
	}
	cfg.General.Mode = mode
	return marshalAndWrite(path, cfg)
}

// SetNoColor atomically updates [general].no_color in the TOML at path.
// Round-trip semantics and file-creation behaviour mirror SetTutorialHints.
func SetNoColor(path string, value bool) error {
	cfg, err := readOrDefault(path)
	if err != nil {
		return err
	}
	cfg.General.NoColor = value
	return marshalAndWrite(path, cfg)
}

// SetWidget atomically updates the named widget toggle in [widgets].
// name must be one of the Widgets field TOML names (e.g. "model", "ctx").
// Unknown names return an error and leave the file unchanged.
// Round-trip semantics and file-creation behaviour mirror SetTutorialHints.
func SetWidget(path, name string, value bool) error {
	cfg, err := readOrDefault(path)
	if err != nil {
		return err
	}
	switch name {
	case "model":
		cfg.Widgets.Model = value
	case "effort":
		cfg.Widgets.Effort = value
	case "cost":
		cfg.Widgets.Cost = value
	case "project":
		cfg.Widgets.Project = value
	case "email":
		cfg.Widgets.Email = value
	case "time":
		cfg.Widgets.Time = value
	case "ctx":
		cfg.Widgets.Ctx = value
	case "quota":
		cfg.Widgets.Quota = value
	case "git":
		cfg.Widgets.Git = value
	default:
		return fmt.Errorf("unknown widget %q: accepted names are model, effort, cost, project, email, time, ctx, quota, git", name)
	}
	return marshalAndWrite(path, cfg)
}

// SetRefreshInterval atomically updates [general].refresh_interval_hint in the
// TOML at path.
// Round-trip semantics and file-creation behaviour mirror SetTutorialHints.
func SetRefreshInterval(path string, seconds int) error {
	cfg, err := readOrDefault(path)
	if err != nil {
		return err
	}
	cfg.General.RefreshIntervalHint = seconds
	return marshalAndWrite(path, cfg)
}

// SetTableRows atomically updates [general].table_rows in the TOML at path.
// Values greater than tableRowsCap (40) are silently capped to tableRowsCap.
// Round-trip semantics and file-creation behaviour mirror SetTutorialHints.
func SetTableRows(path string, rows int) error {
	if rows > tableRowsCap {
		rows = tableRowsCap
	}
	cfg, err := readOrDefault(path)
	if err != nil {
		return err
	}
	cfg.General.TableRows = rows
	return marshalAndWrite(path, cfg)
}

// ─── shared helpers ───────────────────────────────────────────────────────────

// readOrDefault reads and parses the TOML at path (starting from Default() so
// omitted keys keep their default values), or creates a minimal new file when
// the file does not yet exist. Returns an error when the existing file is
// invalid so callers do not clobber user-edited content.
func readOrDefault(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Return Default(); the caller will write the file via marshalAndWrite.
			return Default(), nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("existing config is invalid; run 'cc-probeline check-config' for details, then fix or remove %s: %w", path, err)
	}
	return cfg, nil
}

// marshalAndWrite marshals cfg to TOML and atomically writes it to path,
// creating parent directories as needed.
func marshalAndWrite(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", path, err)
	}
	out, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return atomicWrite(path, out)
}

// isValidMode reports whether mode is one of the accepted mode strings.
func isValidMode(mode string) bool {
	for _, v := range validModes {
		if mode == v {
			return true
		}
	}
	return false
}
