package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// maxSearchDepth is the maximum number of directory levels findProjectConfig
// will traverse before giving up (insurance #3 sanity bound).
const maxSearchDepth = 20

// findProjectConfig walks up from cwd looking for .cc-probeline.toml.
//
// Stop conditions (whichever comes first):
//   - Found .cc-probeline.toml → return its absolute path.
//   - Reached filesystem root → return "".
//   - Found a .git/ directory WITHOUT .cc-probeline.toml at the same level → return "".
//   - Traversed more than maxSearchDepth levels → return "".
//
// Empty or invalid cwd → return "" (per concept §4.3).
func findProjectConfig(cwd string) string {
	if cwd == "" {
		return ""
	}

	// Resolve to an absolute path; bail on error (invalid cwd).
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}

	// Verify the starting directory actually exists.
	if _, err := os.Stat(abs); err != nil {
		return ""
	}

	cur := abs
	for depth := 0; depth < maxSearchDepth; depth++ {
		cfgPath := filepath.Join(cur, ".cc-probeline.toml")
		gitPath := filepath.Join(cur, ".git")

		cfgExists := fileExists(cfgPath)
		gitExists := dirExists(gitPath)

		if cfgExists {
			// Config found — return it regardless of .git presence.
			return cfgPath
		}
		if gitExists {
			// Reached repo root without finding config — stop to avoid leaking
			// outside the repository boundary.
			return ""
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached filesystem root.
			return ""
		}
		cur = parent
	}

	// Exceeded maxSearchDepth.
	return ""
}

// globalConfigPath returns the platform-appropriate global config location:
//   - $XDG_CONFIG_HOME/cc-probeline/config.toml   (if XDG_CONFIG_HOME is set)
//   - $HOME/.config/cc-probeline/config.toml       (Linux/macOS default)
//   - %APPDATA%\cc-probeline\config.toml            (Windows)
//
// Returns "" if no suitable home directory can be determined.
// NOTE: the returned path is independent of whether the file actually exists —
// callers are responsible for checking existence.
func globalConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-probeline", "config.toml")
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "cc-probeline", "config.toml")
		}
		return ""
	}

	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "cc-probeline", "config.toml")
	}

	return ""
}

// fileExists reports whether path refers to a regular file (or symlink to one).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether path refers to a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
