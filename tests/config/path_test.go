// Package config_test contains unit tests for path resolution (findProjectConfig,
// globalConfigPath). Tests T-P1..T-P12 per phase-6-plan-6.b.md §3.2.
//
// Since findProjectConfig and globalConfigPath are unexported, and tests live in
// the external tests/config/ package, behaviour is verified through LoadCascade:
// the cascade falls through to project/global sources iff path resolution finds
// the file. This is the correct external-black-box approach.
//
// DRIFT NOTE: GREEN must add internal/config/export_test.go with exported shims
// FindProjectConfig / GlobalConfigPath if direct unit-test access is desired.
// Until then, T-P1..T-P12 test observable behaviour via LoadCascade.
package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// T-P1: findProjectConfig returns the path when .cc-probeline.toml exists
// directly in cwd. Verified by LoadCascade returning SourceProject.
func TestFindProjectConfig_DirectMatch(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir()) // fresh home, no global config

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".cc-probeline.toml"),
		[]byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("T-P1: WriteFile: %v", err)
	}

	_, source, _ := config.LoadCascade(dir)
	if source != config.SourceProject {
		t.Errorf("T-P1: source: got %v, want SourceProject (direct match in cwd)", source)
	}
}

// T-P2: findProjectConfig walks one level up and finds .cc-probeline.toml in
// the parent directory.
func TestFindProjectConfig_OneLevelUp(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())

	parent := t.TempDir()
	sub := filepath.Join(parent, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("T-P2: Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".cc-probeline.toml"),
		[]byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("T-P2: WriteFile: %v", err)
	}

	_, source, _ := config.LoadCascade(sub)
	if source != config.SourceProject {
		t.Errorf("T-P2: source: got %v, want SourceProject (found in parent dir)", source)
	}
}

// T-P3: findProjectConfig stops at a .git directory when no .cc-probeline.toml
// is present — project search returns "not found" (insurance #3).
// Verified by LoadCascade not returning SourceProject.
func TestFindProjectConfig_StopOnGit(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir()) // fresh home, no global config

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("T-P3: Mkdir .git: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("T-P3: Mkdir sub: %v", err)
	}
	// No .cc-probeline.toml anywhere.

	_, source, _ := config.LoadCascade(sub)
	if source == config.SourceProject {
		t.Errorf("T-P3: source: got SourceProject, want anything else (should stop at .git)")
	}
}

// T-P4: findProjectConfig finds .cc-probeline.toml in a directory that also
// has .git — config found before stop condition fires.
func TestFindProjectConfig_GitWithConfig(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("T-P4: Mkdir .git: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("T-P4: Mkdir sub: %v", err)
	}
	// .cc-probeline.toml co-located with .git.
	if err := os.WriteFile(filepath.Join(dir, ".cc-probeline.toml"),
		[]byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("T-P4: WriteFile: %v", err)
	}

	_, source, _ := config.LoadCascade(sub)
	if source != config.SourceProject {
		t.Errorf("T-P4: source: got %v, want SourceProject (config co-located with .git)", source)
	}
}

// T-P5: findProjectConfig returns "" for empty cwd — project search is skipped
// and LoadCascade falls through to global or defaults.
func TestFindProjectConfig_EmptyCwd(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir()) // no global config in fresh home

	_, source, _ := config.LoadCascade("")
	if source == config.SourceProject {
		t.Errorf("T-P5: source: got SourceProject, want anything else (empty cwd must skip project search)")
	}
}

// T-P6: findProjectConfig returns "" for a non-existent cwd — project search
// returns "not found" gracefully.
func TestFindProjectConfig_InvalidCwd(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())

	_, source, _ := config.LoadCascade("/does/not/exist/cc-probeline-test-path-12345")
	if source == config.SourceProject {
		t.Errorf("T-P6: source: got SourceProject, want anything else (invalid cwd must not find project config)")
	}
}

// T-P7: findProjectConfig stops after maxDepth=20 levels and returns "" without
// panicking or infinite-looping.
func TestFindProjectConfig_MaxDepth(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())

	// Build a 25-level deep tree, no .cc-probeline.toml, no .git.
	base := t.TempDir()
	cur := base
	for i := 0; i < 25; i++ {
		cur = filepath.Join(cur, "d")
		if err := os.Mkdir(cur, 0o755); err != nil {
			t.Fatalf("T-P7: Mkdir level %d: %v", i, err)
		}
	}

	// Must complete without hanging. Source should not be SourceProject.
	_, source, _ := config.LoadCascade(cur)
	if source == config.SourceProject {
		t.Errorf("T-P7: source: got SourceProject, want anything else (maxDepth exceeded)")
	}
}

// T-P8: findProjectConfig with cwd="/" returns "" — reached filesystem root.
func TestFindProjectConfig_AtFilesystemRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("T-P8: skipped on Windows (different root path semantics)")
	}
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())

	_, source, _ := config.LoadCascade("/")
	if source == config.SourceProject {
		t.Errorf("T-P8: source: got SourceProject at filesystem root, want anything else")
	}
}

// T-P9: globalConfigPath returns $XDG_CONFIG_HOME/cc-probeline/config.toml
// when XDG_CONFIG_HOME is set. Verified by LoadCascade finding the file at
// the expected path.
func TestGlobalConfigPath_XDGSet(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")

	xdgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgHome)

	// Create the file at the expected XDG path.
	globalDir := filepath.Join(xdgHome, "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-P9: MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("[general]\nno_color = true\n"), 0o644); err != nil {
		t.Fatalf("T-P9: WriteFile: %v", err)
	}

	cfg, source, errs := config.LoadCascade(t.TempDir())
	if source != config.SourceGlobal {
		t.Errorf("T-P9: source: got %v, want SourceGlobal (XDG path must be used)", source)
	}
	if cfg.General.NoColor != true {
		t.Errorf("T-P9: NoColor: got false, want true (from XDG global config)")
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-P9: unexpected SeverityError: %v", e)
		}
	}
}

// T-P10: globalConfigPath returns $HOME/.config/cc-probeline/config.toml
// when XDG_CONFIG_HOME is not set.
func TestGlobalConfigPath_HOMEFallback(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create the file at the expected HOME fallback path.
	globalDir := filepath.Join(homeDir, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-P10: MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("[general]\nnerd_font = true\n"), 0o644); err != nil {
		t.Fatalf("T-P10: WriteFile: %v", err)
	}

	cfg, source, errs := config.LoadCascade(t.TempDir())
	if source != config.SourceGlobal {
		t.Errorf("T-P10: source: got %v, want SourceGlobal (HOME fallback path)", source)
	}
	if cfg.General.NerdFont != true {
		t.Errorf("T-P10: NerdFont: got false, want true (from HOME fallback global config)")
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-P10: unexpected SeverityError: %v", e)
		}
	}
}

// T-P11: globalConfigPath returns "" when XDG_CONFIG_HOME, HOME, and APPDATA
// are all unset — LoadCascade falls back to SourceDefaults.
func TestGlobalConfigPath_NeitherSet(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("APPDATA", "")

	_, source, errs := config.LoadCascade(t.TempDir())
	if source != config.SourceDefaults {
		t.Errorf("T-P11: source: got %v, want SourceDefaults (no HOME/APPDATA/XDG)", source)
	}
	if len(errs) != 0 {
		t.Errorf("T-P11: expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// T-P12: globalConfigPath returns %APPDATA%\cc-probeline\config.toml on Windows.
// Skipped on non-Windows platforms.
func TestGlobalConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("T-P12: skipped on non-Windows platform")
	}
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	appData := t.TempDir()
	t.Setenv("APPDATA", appData)

	// Create the file at the expected APPDATA path.
	globalDir := filepath.Join(appData, "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-P12: MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("[general]\ntutorial_hints = false\n"), 0o644); err != nil {
		t.Fatalf("T-P12: WriteFile: %v", err)
	}

	_, source, errs := config.LoadCascade(t.TempDir())
	if source != config.SourceGlobal {
		t.Errorf("T-P12: source: got %v, want SourceGlobal (Windows APPDATA path)", source)
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-P12: unexpected SeverityError: %v", e)
		}
	}
}
