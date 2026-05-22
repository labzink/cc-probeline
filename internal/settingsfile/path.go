package settingsfile

import (
	"os"
	"path/filepath"
)

// Path returns the absolute path to ~/.claude/settings.json.
// Resolves via os.UserHomeDir(); returns "" when home directory is unavailable.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}
