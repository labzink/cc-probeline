// Package config_test contains integration tests for LoadCascade.
// Tests T-C1..T-C8 per phase-6-plan-6.b.md §3.3.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// T-C1: LoadCascade with no env, no project config, no global → returns
// Default(), source=SourceDefaults, no errors.
func TestLoadCascade_DefaultsWhenNothingExists(t *testing.T) {
	// Clear env var and use a cwd with no .cc-probeline.toml anywhere.
	t.Setenv("CC_PROBELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir()) // fresh home dir with no config file

	cfg, source, errs := config.LoadCascade(t.TempDir())
	if cfg == nil {
		t.Fatal("T-C1: cfg is nil")
	}
	if source != config.SourceDefaults {
		t.Errorf("T-C1: source: got %v, want SourceDefaults", source)
	}
	if len(errs) != 0 {
		t.Errorf("T-C1: expected 0 errors, got %d: %v", len(errs), errs)
	}
	// Spot-check Default values.
	def := config.Default()
	if cfg.Thresholds.CtxWarnRatio != def.Thresholds.CtxWarnRatio {
		t.Errorf("T-C1: CtxWarnRatio: got %v, want %v", cfg.Thresholds.CtxWarnRatio, def.Thresholds.CtxWarnRatio)
	}
}

// T-C2: When CC_PROBELINE_CONFIG is set to an existing file, LoadCascade loads
// that file regardless of project or global configs.
func TestLoadCascade_EnvWinsOverAll(t *testing.T) {
	// Write env config with a distinctive value.
	envDir := t.TempDir()
	envCfg := filepath.Join(envDir, "env.toml")
	const envContent = `version = 1
[general]
refresh_interval_hint = 42
`
	if err := os.WriteFile(envCfg, []byte(envContent), 0o644); err != nil {
		t.Fatalf("T-C2: WriteFile: %v", err)
	}
	t.Setenv("CC_PROBELINE_CONFIG", envCfg)

	// Also create a project config — must NOT be used.
	projDir := t.TempDir()
	projCfg := filepath.Join(projDir, ".cc-probeline.toml")
	if err := os.WriteFile(projCfg, []byte("[general]\nrefresh_interval_hint = 99\n"), 0o644); err != nil {
		t.Fatalf("T-C2: WriteFile proj: %v", err)
	}

	cfg, source, errs := config.LoadCascade(projDir)
	if cfg == nil {
		t.Fatal("T-C2: cfg is nil")
	}
	if source != config.SourceEnv {
		t.Errorf("T-C2: source: got %v, want SourceEnv", source)
	}
	// env config sets refresh_interval_hint=42; project would give 99.
	if cfg.General.RefreshIntervalHint != 42 {
		t.Errorf("T-C2: RefreshIntervalHint: got %d, want 42 (from env config)", cfg.General.RefreshIntervalHint)
	}
	// No fatal errors expected.
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-C2: unexpected SeverityError: %v", e)
		}
	}
}

// T-C3: When CC_PROBELINE_CONFIG points to a missing file, LoadCascade returns
// an error with source=SourceEnv and does NOT fall through to global or defaults
// (explicit user intent — missing file is suspicious).
func TestLoadCascade_EnvMissingFile_Error(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "/tmp/cc-probeline-env-does-not-exist-99999.toml")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())

	cfg, source, errs := config.LoadCascade(t.TempDir())
	if cfg == nil {
		t.Fatal("T-C3: cfg is nil")
	}
	if source != config.SourceEnv {
		t.Errorf("T-C3: source: got %v, want SourceEnv (no fall-through)", source)
	}
	if len(errs) == 0 {
		t.Fatal("T-C3: expected 1+ errors, got 0")
	}
	hasSevError := false
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			hasSevError = true
		}
	}
	if !hasSevError {
		t.Errorf("T-C3: no SeverityError; errs=%v", errs)
	}
}

// T-C4: With no env, a project config wins over a global config.
func TestLoadCascade_ProjectWinsOverGlobal(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")

	// Set up a HOME with a global config.
	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", homeDir)
	globalDir := filepath.Join(homeDir, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-C4: MkdirAll: %v", err)
	}
	globalCfg := filepath.Join(globalDir, "config.toml")
	if err := os.WriteFile(globalCfg, []byte("[general]\nrefresh_interval_hint = 77\n"), 0o644); err != nil {
		t.Fatalf("T-C4: WriteFile global: %v", err)
	}

	// Set up a project config with different value.
	projDir := t.TempDir()
	projCfg := filepath.Join(projDir, ".cc-probeline.toml")
	if err := os.WriteFile(projCfg, []byte("[general]\nrefresh_interval_hint = 33\n"), 0o644); err != nil {
		t.Fatalf("T-C4: WriteFile proj: %v", err)
	}

	cfg, source, errs := config.LoadCascade(projDir)
	if cfg == nil {
		t.Fatal("T-C4: cfg is nil")
	}
	if source != config.SourceProject {
		t.Errorf("T-C4: source: got %v, want SourceProject", source)
	}
	if cfg.General.RefreshIntervalHint != 33 {
		t.Errorf("T-C4: RefreshIntervalHint: got %d, want 33 (from project config)", cfg.General.RefreshIntervalHint)
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-C4: unexpected SeverityError: %v", e)
		}
	}
}

// T-C5: With no env and no project config, the global config is used.
func TestLoadCascade_GlobalWhenNoProject(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")

	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", homeDir)
	globalDir := filepath.Join(homeDir, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-C5: MkdirAll: %v", err)
	}
	globalCfg := filepath.Join(globalDir, "config.toml")
	if err := os.WriteFile(globalCfg, []byte("[general]\nnerd_font = true\n"), 0o644); err != nil {
		t.Fatalf("T-C5: WriteFile global: %v", err)
	}

	// cwd has no .cc-probeline.toml.
	cfg, source, errs := config.LoadCascade(t.TempDir())
	if cfg == nil {
		t.Fatal("T-C5: cfg is nil")
	}
	if source != config.SourceGlobal {
		t.Errorf("T-C5: source: got %v, want SourceGlobal", source)
	}
	if cfg.General.NerdFont != true {
		t.Errorf("T-C5: NerdFont: got false, want true (from global config)")
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-C5: unexpected SeverityError: %v", e)
		}
	}
}

// T-C6: A broken project config → returns errors, source=SourceProject,
// does NOT fall through to the valid global config.
func TestLoadCascade_BrokenProject_NoFallthrough(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")

	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", homeDir)
	// Write a valid global config.
	globalDir := filepath.Join(homeDir, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-C6: MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("[general]\nnerd_font = true\n"), 0o644); err != nil {
		t.Fatalf("T-C6: WriteFile global: %v", err)
	}

	// Write a broken project config.
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projDir, ".cc-probeline.toml"),
		[]byte("version = 1\n[broken\n"), 0o644); err != nil {
		t.Fatalf("T-C6: WriteFile proj: %v", err)
	}

	_, source, errs := config.LoadCascade(projDir)
	if source != config.SourceProject {
		t.Errorf("T-C6: source: got %v, want SourceProject (no fall-through to global)", source)
	}
	if len(errs) == 0 {
		t.Fatal("T-C6: expected 1+ errors from broken project config, got 0")
	}
	hasSevError := false
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			hasSevError = true
		}
	}
	if !hasSevError {
		t.Errorf("T-C6: no SeverityError; errs=%v", errs)
	}
}

// T-C7: A broken global config → returns errors, source=SourceGlobal.
func TestLoadCascade_BrokenGlobal_Error(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")

	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", homeDir)
	globalDir := filepath.Join(homeDir, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-C7: MkdirAll: %v", err)
	}
	// Write broken global config.
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("[broken\n"), 0o644); err != nil {
		t.Fatalf("T-C7: WriteFile global: %v", err)
	}

	_, source, errs := config.LoadCascade(t.TempDir())
	if source != config.SourceGlobal {
		t.Errorf("T-C7: source: got %v, want SourceGlobal", source)
	}
	if len(errs) == 0 {
		t.Fatal("T-C7: expected 1+ errors, got 0")
	}
}

// T-C8: When cwd is empty, LoadCascade skips project search and uses global (if
// present) or falls back to defaults.
func TestLoadCascade_EmptyCwd_SkipsProjectSearch(t *testing.T) {
	t.Setenv("CC_PROBELINE_CONFIG", "")

	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", homeDir)
	globalDir := filepath.Join(homeDir, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-C8: MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.toml"),
		[]byte("[general]\nno_color = true\n"), 0o644); err != nil {
		t.Fatalf("T-C8: WriteFile global: %v", err)
	}

	// cwd="" must skip project search.
	cfg, source, errs := config.LoadCascade("")
	if cfg == nil {
		t.Fatal("T-C8: cfg is nil")
	}
	if source != config.SourceGlobal {
		t.Errorf("T-C8: source: got %v, want SourceGlobal (project search skipped for empty cwd)", source)
	}
	if cfg.General.NoColor != true {
		t.Errorf("T-C8: NoColor: got false, want true (from global config)")
	}
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Errorf("T-C8: unexpected SeverityError: %v", e)
		}
	}
}
