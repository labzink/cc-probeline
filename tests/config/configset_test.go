// Package config_test — hermetic write-tests for config setters (Phase 6.95.cfg).
// Each test uses t.TempDir() for full file-system isolation.
// Tests verify: field changed, round-trip of other keys intact (no clobber),
// cap boundary (SetTableRows), and error on invalid input (SetMode).
package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// seedConfig writes a minimal multi-field TOML to cfgPath and returns the path.
func seedConfig(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("seedConfig: %v", err)
	}
	return p
}

// loadField reads the TOML at path and returns the parsed Config.
// Fails the test on any SeverityError.
func loadField(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, errs := config.Load(path)
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Fatalf("Load after setter returned error: %v", e.Message)
		}
	}
	return cfg
}

// multiFieldSeed is a TOML with several distinct fields so round-trip tests can
// verify that unrelated keys are preserved.
const multiFieldSeed = `version = 1

[general]
tutorial_hints = true
no_color = false
nerd_font = true
refresh_interval_hint = 10
table_rows = 5
mode = "standard"

[theme]
name = "high-contrast"

[widgets]
model = true
quota = false
`

// ─── SetMode ──────────────────────────────────────────────────────────────────

// T-SM1: SetMode("standard") on a fresh path creates the file with mode = "standard".
func TestSetMode_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetMode(p, "standard"); err != nil {
		t.Fatalf("SetMode returned error: %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.Mode != "standard" {
		t.Errorf("Mode: got %q, want %q", cfg.General.Mode, "standard")
	}
}

// T-SM2: SetMode("super-compact") updates an existing file.
func TestSetMode_UpdatesExisting(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	if err := config.SetMode(p, "super-compact"); err != nil {
		t.Fatalf("SetMode returned error: %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.Mode != "super-compact" {
		t.Errorf("Mode: got %q, want %q", cfg.General.Mode, "super-compact")
	}
}

// T-SM3: SetMode with an unknown value returns an error and does not touch the file.
func TestSetMode_InvalidValue_ErrorNoChange(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	origData, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	setErr := config.SetMode(p, "turbo")
	if setErr == nil {
		t.Fatal("expected error for unknown mode, got nil")
	}
	if !strings.Contains(setErr.Error(), "invalid mode") {
		t.Errorf("error should mention 'invalid mode', got: %v", setErr)
	}

	afterData, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read after failed SetMode: %v", err)
	}
	if string(afterData) != string(origData) {
		t.Errorf("file was modified despite invalid mode\nbefore: %q\nafter:  %q",
			string(origData), string(afterData))
	}
}

// T-SM4: SetMode preserves other keys (round-trip).
func TestSetMode_RoundTrip_OtherKeysPreserved(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	if err := config.SetMode(p, "super-compact"); err != nil {
		t.Fatalf("SetMode: %v", err)
	}

	cfg := loadField(t, p)
	// mode changed
	if cfg.General.Mode != "super-compact" {
		t.Errorf("Mode: got %q, want super-compact", cfg.General.Mode)
	}
	// other general fields preserved
	if !cfg.General.TutorialHints {
		t.Error("TutorialHints should be preserved (true)")
	}
	if !cfg.General.NerdFont {
		t.Error("NerdFont should be preserved (true)")
	}
	if cfg.General.RefreshIntervalHint != 10 {
		t.Errorf("RefreshIntervalHint: got %d, want 10", cfg.General.RefreshIntervalHint)
	}
	// theme preserved
	if cfg.Theme.Name != "high-contrast" {
		t.Errorf("Theme.Name: got %q, want high-contrast", cfg.Theme.Name)
	}
	// widget preserved
	if cfg.Widgets.Quota {
		t.Error("Widgets.Quota: got true, want false (preserved)")
	}
}

// ─── SetNoColor ───────────────────────────────────────────────────────────────

// T-SNC1: SetNoColor(true) updates the field and preserves other keys.
func TestSetNoColor_True_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	if err := config.SetNoColor(p, true); err != nil {
		t.Fatalf("SetNoColor: %v", err)
	}

	cfg := loadField(t, p)
	if !cfg.General.NoColor {
		t.Error("NoColor: got false, want true")
	}
	// other fields preserved
	if !cfg.General.TutorialHints {
		t.Error("TutorialHints should be preserved (true)")
	}
	if cfg.Theme.Name != "high-contrast" {
		t.Errorf("Theme.Name: got %q, want high-contrast", cfg.Theme.Name)
	}
}

// T-SNC2: SetNoColor(false) on a clean path creates the config file.
func TestSetNoColor_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetNoColor(p, false); err != nil {
		t.Fatalf("SetNoColor: %v", err)
	}

	if _, err := os.Stat(p); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	cfg := loadField(t, p)
	if cfg.General.NoColor {
		t.Error("NoColor: got true, want false")
	}
}

// ─── SetWidget ────────────────────────────────────────────────────────────────

// T-SW1: SetWidget("quota", false) updates quota and preserves other widgets.
func TestSetWidget_Quota_False_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	// Start with quota=true (default).
	p := seedConfig(t, tmp, "version = 1\n\n[widgets]\nquota = true\nmodel = true\n")

	if err := config.SetWidget(p, "quota", false); err != nil {
		t.Fatalf("SetWidget: %v", err)
	}

	cfg := loadField(t, p)
	if cfg.Widgets.Quota {
		t.Error("Widgets.Quota: got true, want false")
	}
	if !cfg.Widgets.Model {
		t.Error("Widgets.Model: got false, want true (preserved)")
	}
}

// T-SW2: SetWidget with unknown name returns error and does not modify file.
func TestSetWidget_UnknownName_ErrorNoChange(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	origData, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	setErr := config.SetWidget(p, "cache", true)
	if setErr == nil {
		t.Fatal("expected error for unknown widget 'cache', got nil")
	}

	afterData, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(afterData) != string(origData) {
		t.Errorf("file was modified despite unknown widget")
	}
}

// T-SW3: SetWidget covers all 9 active widget names without error.
func TestSetWidget_AllActiveNames(t *testing.T) {
	names := []string{"model", "effort", "cost", "project", "email", "time", "ctx", "quota", "git"}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			p := filepath.Join(tmp, "config.toml")
			if err := config.SetWidget(p, name, false); err != nil {
				t.Errorf("SetWidget(%q, false): unexpected error: %v", name, err)
			}
		})
	}
}

// ─── SetRefreshInterval ───────────────────────────────────────────────────────

// T-SRI1: SetRefreshInterval updates the field and preserves others.
func TestSetRefreshInterval_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	if err := config.SetRefreshInterval(p, 30); err != nil {
		t.Fatalf("SetRefreshInterval: %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.RefreshIntervalHint != 30 {
		t.Errorf("RefreshIntervalHint: got %d, want 30", cfg.General.RefreshIntervalHint)
	}
	// other fields preserved
	if !cfg.General.TutorialHints {
		t.Error("TutorialHints should be preserved (true)")
	}
	if cfg.Theme.Name != "high-contrast" {
		t.Errorf("Theme.Name: got %q, want high-contrast", cfg.Theme.Name)
	}
}

// ─── SetTableRows ─────────────────────────────────────────────────────────────

// T-STR1: SetTableRows(15) sets table_rows = 15.
func TestSetTableRows_Normal(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetTableRows(p, 15); err != nil {
		t.Fatalf("SetTableRows: %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.TableRows != 15 {
		t.Errorf("TableRows: got %d, want 15", cfg.General.TableRows)
	}
}

// T-STR2: SetTableRows(100) is silently capped to 40 (cap boundary).
func TestSetTableRows_Cap_100_Becomes_40(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetTableRows(p, 100); err != nil {
		t.Fatalf("SetTableRows(100): %v", err)
	}

	// Check via raw file content — the TOML must contain table_rows = 40.
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "table_rows = 40") {
		t.Errorf("expected 'table_rows = 40' in file (cap applied), got:\n%s", string(data))
	}

	// Also verify via round-trip Load.
	cfg := loadField(t, p)
	if cfg.General.TableRows != 40 {
		t.Errorf("TableRows after cap: got %d, want 40", cfg.General.TableRows)
	}
}

// T-STR3: SetTableRows(40) — exactly at cap, no change.
func TestSetTableRows_ExactlyAtCap(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetTableRows(p, 40); err != nil {
		t.Fatalf("SetTableRows(40): %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.TableRows != 40 {
		t.Errorf("TableRows: got %d, want 40", cfg.General.TableRows)
	}
}

// T-STR5: SetTableRows(0) is raised to the floor (1).
func TestSetTableRows_Floor_0_Becomes_1(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetTableRows(p, 0); err != nil {
		t.Fatalf("SetTableRows(0): %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.TableRows != 1 {
		t.Errorf("TableRows after floor: got %d, want 1", cfg.General.TableRows)
	}
}

// T-STR6: SetTableRows(-5) (negative) is raised to the floor (1).
func TestSetTableRows_Floor_Negative_Becomes_1(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetTableRows(p, -5); err != nil {
		t.Fatalf("SetTableRows(-5): %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.TableRows != 1 {
		t.Errorf("TableRows after floor: got %d, want 1", cfg.General.TableRows)
	}
}

// T-STR7: SetTableRows(1) — exactly at floor, no change.
func TestSetTableRows_ExactlyAtFloor(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")

	if err := config.SetTableRows(p, 1); err != nil {
		t.Fatalf("SetTableRows(1): %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.TableRows != 1 {
		t.Errorf("TableRows: got %d, want 1", cfg.General.TableRows)
	}
}

// T-STR4: SetTableRows preserves other keys (round-trip).
func TestSetTableRows_RoundTrip_OtherKeysPreserved(t *testing.T) {
	tmp := t.TempDir()
	p := seedConfig(t, tmp, multiFieldSeed)

	if err := config.SetTableRows(p, 20); err != nil {
		t.Fatalf("SetTableRows: %v", err)
	}

	cfg := loadField(t, p)
	if cfg.General.TableRows != 20 {
		t.Errorf("TableRows: got %d, want 20", cfg.General.TableRows)
	}
	// other keys preserved
	if cfg.General.RefreshIntervalHint != 10 {
		t.Errorf("RefreshIntervalHint: got %d, want 10 (preserved)", cfg.General.RefreshIntervalHint)
	}
	if cfg.Theme.Name != "high-contrast" {
		t.Errorf("Theme.Name: got %q, want high-contrast (preserved)", cfg.Theme.Name)
	}
}
