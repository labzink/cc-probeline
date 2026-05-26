// Package cli_test — e2e tests for `cc-probeline hints on|off` subcommand.
// Tests T-H1..T-H10 run against a compiled binary (shared via TestMain in
// render_test.go). Each test uses t.TempDir() for full HOME/XDG isolation.
//
// All tests are intentionally RED until Phase 6.f GREEN lands:
// the stub runHints is not wired in main.go dispatch yet, so the binary
// treats "hints" as an unknown positional arg and falls through to render mode.
package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runHintsCmd executes `cc-probeline hints <args...>` with isolated HOME and
// XDG_CONFIG_HOME pointing to tmpHome. Returns stdout, stderr, exit code.
func runHintsCmd(t *testing.T, tmpHome string, extraEnv []string, hintsArgs ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	env := append([]string{
		"HOME=" + tmpHome,
		"XDG_CONFIG_HOME=" + tmpHome,
	}, extraEnv...)
	allArgs := append([]string{"hints"}, hintsArgs...)
	return run(t, env, nil, allArgs...)
}

// globalConfigFile returns the path that the binary uses for the global config
// when XDG_CONFIG_HOME is set to xdgRoot.
func globalConfigFile(xdgRoot string) string {
	return filepath.Join(xdgRoot, "cc-probeline", "config.toml")
}

// T-H1: hints off on a clean HOME creates the global config with tutorial_hints = false.
func TestHints_OffCreatesConfig(t *testing.T) {
	tmp := t.TempDir()

	stdout, _, exitCode := runHintsCmd(t, tmp, nil, "off")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout=%q", exitCode, stdout)
	}

	cfgPath := globalConfigFile(tmp)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("global config not created: %v", err)
	}
	if !strings.Contains(string(data), "tutorial_hints = false") {
		t.Errorf("expected tutorial_hints = false in config, got:\n%s", string(data))
	}
}

// T-H2: hints on on a clean HOME creates the global config with tutorial_hints = true.
func TestHints_OnCreatesConfig(t *testing.T) {
	tmp := t.TempDir()

	stdout, _, exitCode := runHintsCmd(t, tmp, nil, "on")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout=%q", exitCode, stdout)
	}

	cfgPath := globalConfigFile(tmp)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("global config not created: %v", err)
	}
	if !strings.Contains(string(data), "tutorial_hints = true") {
		t.Errorf("expected tutorial_hints = true in config, got:\n%s", string(data))
	}
}

// T-H3: hints off followed by hints on — last write wins (true).
func TestHints_TogglePersists(t *testing.T) {
	tmp := t.TempDir()

	if _, _, code := runHintsCmd(t, tmp, nil, "off"); code != 0 {
		t.Fatalf("first call (off) failed with exit %d", code)
	}
	if _, _, code := runHintsCmd(t, tmp, nil, "on"); code != 0 {
		t.Fatalf("second call (on) failed with exit %d", code)
	}

	cfgPath := globalConfigFile(tmp)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("cannot read config: %v", err)
	}
	if !strings.Contains(string(data), "tutorial_hints = true") {
		t.Errorf("expected final tutorial_hints = true, got:\n%s", string(data))
	}
}

// T-H4: hints off called twice — idempotent (exit 0 both times, no duplicate key).
func TestHints_Idempotent(t *testing.T) {
	tmp := t.TempDir()

	for i := 0; i < 2; i++ {
		if _, _, code := runHintsCmd(t, tmp, nil, "off"); code != 0 {
			t.Fatalf("call %d: expected exit 0, got %d", i+1, code)
		}
	}

	cfgPath := globalConfigFile(tmp)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("cannot read config: %v", err)
	}

	// There must be exactly one tutorial_hints line (no duplicate keys).
	count := strings.Count(string(data), "tutorial_hints")
	if count != 1 {
		t.Errorf("expected exactly 1 'tutorial_hints' line, found %d:\n%s", count, string(data))
	}
}

// T-H5: hints off with an existing config that has other fields — other fields preserved.
func TestHints_PreservesOtherFields(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := globalConfigFile(tmp)

	// Create parent dir and write existing config with a theme setting.
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := `version = 1

[general]
tutorial_hints = true

[theme]
name = "high-contrast"
`
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	if _, _, code := runHintsCmd(t, tmp, nil, "off"); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("cannot read config after hints off: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "tutorial_hints = false") {
		t.Errorf("tutorial_hints not updated to false:\n%s", content)
	}
	// theme.name must still be present.
	if !strings.Contains(content, "high-contrast") {
		t.Errorf("theme.name 'high-contrast' was lost:\n%s", content)
	}
}

// T-H6: hints off with a broken config file — exit 2, stderr mentions "fix or remove"
// and the config file path.
func TestHints_BrokenConfigRefuses(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := globalConfigFile(tmp)

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("not valid toml = [ unclosed\n"), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}

	_, stderr, exitCode := runHintsCmd(t, tmp, nil, "off")
	if exitCode != 2 {
		t.Fatalf("expected exit 2 for broken config, got %d; stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "fix or remove") {
		t.Errorf("stderr should mention 'fix or remove', got: %q", stderr)
	}
	// Stderr should include the config path so the user knows what to edit.
	if !strings.Contains(stderr, cfgPath) {
		t.Errorf("stderr should contain config path %q, got: %q", cfgPath, stderr)
	}
}

// T-H7: cc-probeline hints (no subcommand) — exit 64, stderr contains Usage.
func TestHints_NoArg_Exit64(t *testing.T) {
	tmp := t.TempDir()

	_, stderr, exitCode := runHintsCmd(t, tmp, nil /* no sub-args */)
	if exitCode != 64 {
		t.Fatalf("expected exit 64, got %d; stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "Usage") {
		t.Errorf("stderr should contain 'Usage', got: %q", stderr)
	}
}

// T-H8: cc-probeline hints maybe — unknown arg, exit 64.
func TestHints_UnknownArg_Exit64(t *testing.T) {
	tmp := t.TempDir()

	_, stderr, exitCode := runHintsCmd(t, tmp, nil, "maybe")
	if exitCode != 64 {
		t.Fatalf("expected exit 64, got %d; stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "Usage") {
		t.Errorf("stderr should contain 'Usage', got: %q", stderr)
	}
}

// T-H9: hints off stdout contains the resolved global config path.
func TestHints_StdoutMentionsConfigPath(t *testing.T) {
	tmp := t.TempDir()

	stdout, _, exitCode := runHintsCmd(t, tmp, nil, "off")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stdout=%q", exitCode, stdout)
	}

	cfgPath := globalConfigFile(tmp)
	if !strings.Contains(stdout, cfgPath) {
		t.Errorf("stdout should contain config path %q, got: %q", cfgPath, stdout)
	}
}

// T-H10: hints off does NOT edit a project-local .cc-probeline.toml even when
// the binary's cwd is the project directory (Q2 decision: global config only).
func TestHints_DoesNotEditProjectLocal(t *testing.T) {
	tmp := t.TempDir()

	// Create a project directory with a local config.
	projectDir := filepath.Join(tmp, "myproject")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	localCfg := filepath.Join(projectDir, ".cc-probeline.toml")
	localContent := "version = 1\n\n[general]\ntutorial_hints = true\n"
	if err := os.WriteFile(localCfg, []byte(localContent), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	// Record the local config mtime before running hints off.
	statBefore, err := os.Stat(localCfg)
	if err != nil {
		t.Fatalf("stat local config before: %v", err)
	}

	// Run hints off with HOME isolated to tmp; HOME != projectDir.
	homeDir := filepath.Join(tmp, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	env := []string{
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + homeDir,
	}
	_, _, exitCode := run(t, env, nil, "hints", "off")
	if exitCode != 0 {
		t.Fatalf("hints off failed with exit %d", exitCode)
	}

	// The project-local config must be untouched.
	statAfter, err := os.Stat(localCfg)
	if err != nil {
		t.Fatalf("stat local config after: %v", err)
	}
	if !statAfter.ModTime().Equal(statBefore.ModTime()) {
		t.Errorf("project-local config was modified by hints off (mtime changed)")
	}

	// The local config content must also be byte-identical.
	afterData, err := os.ReadFile(localCfg)
	if err != nil {
		t.Fatalf("read local config after: %v", err)
	}
	if string(afterData) != localContent {
		t.Errorf("project-local config content changed:\nbefore: %q\nafter:  %q",
			localContent, string(afterData))
	}
}
