// Package config_test — unit tests for SetTutorialHints (T-SH1..T-SH7).
//
// Tests are intentionally RED until Phase 6.f GREEN lands:
// sethints.go stub returns nil without writing anything.
package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// globalConfigPathUnder returns the path that GlobalConfigPath() would return
// when XDG_CONFIG_HOME is set to xdgRoot.
func globalConfigPathUnder(xdgRoot string) string {
	return filepath.Join(xdgRoot, "cc-probeline", "config.toml")
}

// T-SH1: SetTutorialHints on a non-existent path creates a minimal config file
// containing version, [general] section, and tutorial_hints field.
func TestSetTutorialHints_NonExistent_Creates(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfgPath := globalConfigPathUnder(tmp)

	// Precondition: file must not exist.
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("precondition failed: config file already exists")
	}

	if err := config.SetTutorialHints(cfgPath, false); err != nil {
		t.Fatalf("SetTutorialHints returned error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "version = 1") {
		t.Errorf("expected 'version = 1' in created config, got:\n%s", content)
	}
	if !strings.Contains(content, "[general]") {
		t.Errorf("expected '[general]' section in created config, got:\n%s", content)
	}
	if !strings.Contains(content, "tutorial_hints = false") {
		t.Errorf("expected 'tutorial_hints = false' in created config, got:\n%s", content)
	}

	// Round-trip: Load should read back tutorial_hints = false.
	cfg, errs := config.Load(cfgPath)
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Fatalf("Load after SetTutorialHints returned error: %v", e.Message)
		}
	}
	if cfg.General.TutorialHints != false {
		t.Errorf("round-trip: expected TutorialHints=false, got %v", cfg.General.TutorialHints)
	}
}

// T-SH2: SetTutorialHints creates parent directories when they don't exist.
func TestSetTutorialHints_NonExistent_CreatesParentDir(t *testing.T) {
	tmp := t.TempDir()
	// Use a deeply nested path that does not exist yet.
	cfgPath := filepath.Join(tmp, "deeply", "nested", "path", "config.toml")

	if err := config.SetTutorialHints(cfgPath, true); err != nil {
		t.Fatalf("SetTutorialHints with nested path returned error: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created at nested path: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("cannot read created config: %v", err)
	}
	if !strings.Contains(string(data), "tutorial_hints = true") {
		t.Errorf("expected tutorial_hints = true, got:\n%s", string(data))
	}
}

// T-SH3: Atomicity — after a normal write there is no residual .tmp file.
// The atomic pattern writes a .tmp then renames; a stale .tmp would indicate
// the rename never ran or a previous write crashed mid-way.
func TestSetTutorialHints_AtomicRename_NoPartial(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	// Seed an existing config so we exercise the update path.
	seed := "version = 1\n\n[general]\ntutorial_hints = true\n"
	if err := os.WriteFile(cfgPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := config.SetTutorialHints(cfgPath, false); err != nil {
		t.Fatalf("SetTutorialHints returned error: %v", err)
	}

	// No .tmp residue should remain.
	tmpFile := cfgPath + ".tmp"
	if _, err := os.Stat(tmpFile); err == nil {
		t.Errorf("stale .tmp file found after SetTutorialHints: %s", tmpFile)
	}

	// The final file must be readable and contain the correct value.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("cannot read config after SetTutorialHints: %v", err)
	}
	if !strings.Contains(string(data), "tutorial_hints = false") {
		t.Errorf("expected tutorial_hints = false after update, got:\n%s", string(data))
	}
}

// T-SH4: SetTutorialHints preserves non-hints fields when updating an existing config.
func TestSetTutorialHints_PreservesNonHintsFields(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	// Write a config that has theme name and a widget setting.
	initial := `version = 1

[general]
tutorial_hints = true
no_color = true

[theme]
name = "high-contrast"

[widgets]
model = false
`
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	if err := config.SetTutorialHints(cfgPath, false); err != nil {
		t.Fatalf("SetTutorialHints returned error: %v", err)
	}

	// Round-trip via Load to verify non-hints fields are intact.
	cfg, errs := config.Load(cfgPath)
	for _, e := range errs {
		if e.Severity == config.SeverityError {
			t.Fatalf("Load after SetTutorialHints returned error: %v", e.Message)
		}
	}

	if cfg.General.TutorialHints != false {
		t.Errorf("TutorialHints: expected false, got %v", cfg.General.TutorialHints)
	}
	if cfg.General.NoColor != true {
		t.Errorf("NoColor: expected true (preserved), got %v", cfg.General.NoColor)
	}
	if cfg.Theme.Name != "high-contrast" {
		t.Errorf("Theme.Name: expected 'high-contrast' (preserved), got %q", cfg.Theme.Name)
	}
	if cfg.Widgets.Model != false {
		t.Errorf("Widgets.Model: expected false (preserved), got %v", cfg.Widgets.Model)
	}
}

// T-SH5: Comment loss is a documented known limitation of the pelletier round-trip.
// This test asserts that comments ARE lost after SetTutorialHints, documenting
// insurance #4 from Phase 6 concept §10.2. Phase 7 will fix via AST-based edit.
func TestSetTutorialHints_CommentLoss_Documented(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	// Config with a meaningful inline comment.
	withComment := `version = 1

[general]
# toggle this to hide inline hints
tutorial_hints = true
`
	if err := os.WriteFile(cfgPath, []byte(withComment), 0o644); err != nil {
		t.Fatalf("write config with comment: %v", err)
	}

	if err := config.SetTutorialHints(cfgPath, false); err != nil {
		t.Fatalf("SetTutorialHints returned error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("cannot read config after SetTutorialHints: %v", err)
	}

	// Known limitation: comments are NOT preserved by pelletier round-trip.
	// This assertion documents the behaviour; do NOT fix here — Phase 7.
	if strings.Contains(string(data), "# toggle this to hide inline hints") {
		t.Errorf("comment was unexpectedly preserved — " +
			"if pelletier now preserves comments, update Phase 7 plan and remove this assertion")
	}

	// The value must still be updated correctly despite comment loss.
	if !strings.Contains(string(data), "tutorial_hints = false") {
		t.Errorf("tutorial_hints value not updated, got:\n%s", string(data))
	}
}

// T-SH6: SetTutorialHints returns an error and does NOT overwrite a broken TOML file.
func TestSetTutorialHints_BrokenTOML_ErrorWrappedNotClobbered(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	broken := "this is not valid toml = [ unclosed\n"
	if err := os.WriteFile(cfgPath, []byte(broken), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}

	origData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read original broken config: %v", err)
	}

	setErr := config.SetTutorialHints(cfgPath, false)
	if setErr == nil {
		t.Fatal("expected error on broken TOML, got nil")
	}

	// File must be unchanged after the error.
	afterData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config after failed SetTutorialHints: %v", err)
	}
	if string(afterData) != string(origData) {
		t.Errorf("file was modified despite parse error\nbefore: %q\nafter:  %q",
			string(origData), string(afterData))
	}

	// Error message must mention "fix or remove" to be actionable.
	if !strings.Contains(setErr.Error(), "fix or remove") {
		t.Errorf("error should mention 'fix or remove', got: %v", setErr)
	}
}

// T-SH7: Calling SetTutorialHints multiple times correctly toggles the value.
func TestSetTutorialHints_TogglesValueBetweenCalls(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")

	for _, want := range []bool{true, false, true} {
		if err := config.SetTutorialHints(cfgPath, want); err != nil {
			t.Fatalf("SetTutorialHints(%v) returned error: %v", want, err)
		}
		cfg, errs := config.Load(cfgPath)
		for _, e := range errs {
			if e.Severity == config.SeverityError {
				t.Fatalf("Load after SetTutorialHints(%v) returned error: %v", want, e.Message)
			}
		}
		if cfg.General.TutorialHints != want {
			t.Errorf("after SetTutorialHints(%v): Load().TutorialHints = %v", want, cfg.General.TutorialHints)
		}
	}
}

// TestGlobalConfigPath_XDG: GlobalConfigPath respects XDG_CONFIG_HOME.
func TestGlobalConfigPath_XDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	// Clear HOME so the fallback is not used.
	t.Setenv("HOME", "")

	got := config.GlobalConfigPath()
	want := filepath.Join(tmp, "cc-probeline", "config.toml")
	if got != want {
		t.Errorf("GlobalConfigPath() = %q, want %q", got, want)
	}
}

// TestGlobalConfigPath_HOME: GlobalConfigPath falls back to $HOME/.config when
// XDG_CONFIG_HOME is unset.
func TestGlobalConfigPath_HOME(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")

	got := config.GlobalConfigPath()
	want := filepath.Join(tmp, ".config", "cc-probeline", "config.toml")
	if got != want {
		t.Errorf("GlobalConfigPath() = %q, want %q", got, want)
	}
}
