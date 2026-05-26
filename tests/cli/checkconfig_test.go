// Package cli_test — e2e tests for `cc-probeline check-config` subcommand.
// Tests T-CC1..T-CC16 run against a compiled binary (shared via TestMain in
// render_test.go). Each test creates its own TOML fixtures in t.TempDir() for
// full isolation.
//
// All tests are intentionally RED until Phase 6.e GREEN lands: the stub
// runCheckConfigImpl always returns 0 with no output.
package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// writeConfig creates a TOML file at dir/name and returns its absolute path.
func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeConfig %s: %v", p, err)
	}
	return p
}

// runCheckConfig executes `cc-probeline check-config [args...]` with the given
// extra environment variables, returning stdout, stderr, exit code.
// Uses the run() helper from render_test.go (same package).
func runCheckConfig(t *testing.T, extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	allArgs := append([]string{"check-config"}, args...)
	return run(t, extraEnv, nil, allArgs...)
}

// runCheckConfigInDir executes `cc-probeline check-config [args...]` with the
// binary's working directory set to dir. Used by T-CC9 to test project-local
// cascade (which depends on os.Getwd() inside the binary).
func runCheckConfigInDir(t *testing.T, dir, home string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	allArgs := append([]string{"check-config"}, args...)
	cmd := exec.Command(binaryPath, allArgs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"HOME=" + home}, extraEnv...)...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("runCheckConfigInDir: unexpected error: %v", err)
		}
	}
	return
}

// ─── T-CC1: no config → exit 0, defaults ─────────────────────────────────────

// TestCheckConfig_NoConfig_ExitZero verifies that with a clean HOME (no config
// file at any cascade level) check-config exits 0 and stdout contains
// "Source: defaults" and the built-in defaults message.
// Concept §8.4, plan T-CC1.
func TestCheckConfig_NoConfig_ExitZero(t *testing.T) {
	// run() injects HOME=<fresh TempDir> so no ~/.config/cc-probeline/config.toml exists.
	stdout, _, exitCode := runCheckConfig(t, nil)

	if exitCode != 0 {
		t.Errorf("T-CC1: no config: expected exit 0, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "Source: defaults") {
		t.Errorf("T-CC1: no config: stdout should contain 'Source: defaults', got: %q", stdout)
	}
	if !strings.Contains(stdout, "Running with built-in defaults") {
		t.Errorf("T-CC1: no config: stdout should contain 'Running with built-in defaults', got: %q", stdout)
	}
}

// ─── T-CC2: no config → lists checked cascade locations ──────────────────────

// TestCheckConfig_NoConfig_ListsCheckedLocations verifies that when no config
// file is found, the output lists all three cascade locations.
// Concept §8.4, plan T-CC2.
func TestCheckConfig_NoConfig_ListsCheckedLocations(t *testing.T) {
	stdout, _, _ := runCheckConfig(t, nil)

	for _, want := range []string{"CC_PROBELINE_CONFIG env", "project-local", "global"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("T-CC2: no config: stdout should contain %q; got: %q", want, stdout)
		}
	}
}

// ─── T-CC3: valid global config → exit 0, "No errors" ────────────────────────

// TestCheckConfig_ValidGlobal_ExitZero places a valid config.toml at the global
// location ($HOME/.config/cc-probeline/config.toml) and expects exit 0 with
// "No errors" in stdout.
// Concept §8.2, plan T-CC3.
// Fixture: same content as tests/fixtures/config/valid-partial.toml.
func TestCheckConfig_ValidGlobal_ExitZero(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC3: MkdirAll: %v", err)
	}
	// valid-partial.toml content: [general] tutorial_hints = false
	writeConfig(t, cfgDir, "config.toml", "[general]\ntutorial_hints = false\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 0 {
		t.Errorf("T-CC3: valid config: expected exit 0, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "No errors") {
		t.Errorf("T-CC3: valid config: stdout should contain 'No errors', got: %q", stdout)
	}
}

// ─── T-CC4: broken TOML → exit 2, "error:" in output ────────────────────────

// TestCheckConfig_BrokenTOML_ExitTwo places a malformed TOML file at the global
// location and expects exit 2 with "error:" and the config path in stdout.
// Concept §8.3 / §8.7, plan T-CC4.
// Fixture: mirrors tests/fixtures/config/malformed.toml ("version = 1\n[broken\n").
func TestCheckConfig_BrokenTOML_ExitTwo(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC4: MkdirAll: %v", err)
	}
	// malformed.toml: "version = 1\n[broken\n"
	// Expected: DecodeError → Error{Severity: SeverityError, File: <path>, Line: 2}
	writeConfig(t, cfgDir, "config.toml", "version = 1\n[broken\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 2 {
		t.Errorf("T-CC4: broken TOML: expected exit 2, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "error:") {
		t.Errorf("T-CC4: broken TOML: stdout should contain 'error:', got: %q", stdout)
	}
	if !strings.Contains(stdout, "config.toml") {
		t.Errorf("T-CC4: broken TOML: stdout should mention 'config.toml' path, got: %q", stdout)
	}
}

// ─── T-CC5: parse error reports line:col ─────────────────────────────────────

// TestCheckConfig_ParseError_LineColumnReported ensures that a parse error on a
// specific line produces "N:M" (line:col) in output.
// Covers hypothesis insurance #1: pelletier gives line+column in DecodeError.
// Concept §8.3, plan T-CC5.
//
// Verify-RED: malformed.toml = "version = 1\n[broken\n"
//   - Line 1: "version = 1" — valid.
//   - Line 2: "[broken" — truncated table header, DecodeError at line 2.
//
// Expected: stdout contains "2:" (line 2 coordinate).
func TestCheckConfig_ParseError_LineColumnReported(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC5: MkdirAll: %v", err)
	}
	writeConfig(t, cfgDir, "config.toml", "version = 1\n[broken\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 2 {
		t.Errorf("T-CC5: parse error: expected exit 2, got %d; stdout: %q", exitCode, stdout)
	}
	// The malformed "[broken" line is at line 2. Output must contain "2:".
	if !strings.Contains(stdout, "2:") {
		t.Errorf("T-CC5: parse error: stdout should contain line number '2:', got: %q", stdout)
	}
}

// ─── T-CC6: unknown theme name → exit 2, suggestion ─────────────────────────

// TestCheckConfig_UnknownThemeName_ExitTwo_WithSuggestion writes
// [theme] name = "defalt" (typo) and expects exit 2 + "did you mean" in output.
//
// Verify-RED: Validate() for theme.name="defalt" produces:
//
//	Error{Severity: SeverityError, Field: "theme.name",
//	      Message: `unknown theme: "defalt". Valid: default, high-contrast, minimal`,
//	      Hint: `did you mean "default"?`}
//
// Levenshtein("defalt", "default") = 1 ≤ 2 → suggestion generated by suggest.go.
// Concept §6.2 / §8.3, plan T-CC6.
func TestCheckConfig_UnknownThemeName_ExitTwo_WithSuggestion(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC6: MkdirAll: %v", err)
	}
	writeConfig(t, cfgDir, "config.toml", "[theme]\nname = \"defalt\"\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 2 {
		t.Errorf("T-CC6: unknown theme: expected exit 2, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "did you mean") {
		t.Errorf("T-CC6: unknown theme: stdout should contain 'did you mean', got: %q", stdout)
	}
	if !strings.Contains(stdout, "theme.name") {
		t.Errorf("T-CC6: unknown theme: stdout should contain 'theme.name', got: %q", stdout)
	}
}

// ─── T-CC7: ratio looks like percent → exit 2, percent hint ─────────────────

// TestCheckConfig_InvalidRatio_ExitTwo_WithPercentHint sets ctx_warn_ratio = 70.0
// (looks like a percent) and expects exit 2 + "percent" in output.
//
// Verify-RED: Validate() for ctx_warn_ratio=70.0 produces:
//
//	Error{Severity: SeverityError, Field: "thresholds.ctx_warn_ratio",
//	      Message: "ratio must be in [0.0, 1.0]",
//	      Hint: "value 70 looks like a percent; ratios use 0.0-1.0 (try 0.70)"}
//
// suggestFor("thresholds.ctx_warn_ratio", 70.0, nil) detects value > 1.0 that
// looks like a percent and appends the hint.
// Concept §6.2 / §8.3, plan T-CC7.
func TestCheckConfig_InvalidRatio_ExitTwo_WithPercentHint(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC7: MkdirAll: %v", err)
	}
	writeConfig(t, cfgDir, "config.toml", "[thresholds]\nctx_warn_ratio = 70.0\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 2 {
		t.Errorf("T-CC7: invalid ratio: expected exit 2, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "percent") {
		t.Errorf("T-CC7: invalid ratio: stdout should contain 'percent' hint, got: %q", stdout)
	}
	if !strings.Contains(stdout, "ctx_warn_ratio") {
		t.Errorf("T-CC7: invalid ratio: stdout should mention 'ctx_warn_ratio', got: %q", stdout)
	}
}

// ─── T-CC8: unknown TOML key → exit 0, "warning" ────────────────────────────

// TestCheckConfig_UnknownKey_ExitZero_WithWarning sets an unknown field
// ctx_silly_ratio and expects exit 0 (SeverityWarning does not change exit code)
// with "warning" in stdout.
//
// Verify-RED: LoadCascade for this file produces:
//
//	Error{Severity: SeverityWarning, Field: "general.ctx_silly_ratio",
//	      Message: "unknown field"}
//
// Exit remains 0 because no SeverityError present (§8.7).
// Concept §6.1 / §8.7, plan T-CC8.
func TestCheckConfig_UnknownKey_ExitZero_WithWarning(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC8: MkdirAll: %v", err)
	}
	// unknown-field.toml content:
	//   [general]
	//   tutorial_hints = true
	//   ctx_silly_ratio = 0.5  ← unknown field → SeverityWarning
	writeConfig(t, cfgDir, "config.toml", "[general]\ntutorial_hints = true\nctx_silly_ratio = 0.5\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 0 {
		t.Errorf("T-CC8: unknown key: expected exit 0 (warning only), got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "warning") {
		t.Errorf("T-CC8: unknown key: stdout should contain 'warning', got: %q", stdout)
	}
}

// ─── T-CC9: project-local overrides global → "Source: project" ───────────────

// TestCheckConfig_ProjectOverridesGlobal_SourceLabel writes both a global config
// and a project-local .cc-probeline.toml, then runs the binary with cwd inside
// the project. Expects "Source: project" + the .cc-probeline.toml path in output.
//
// Uses runCheckConfigInDir to control the binary's cwd (which drives
// findProjectConfig via os.Getwd()).
// Concept §4.1, plan T-CC9.
func TestCheckConfig_ProjectOverridesGlobal_SourceLabel(t *testing.T) {
	home := t.TempDir()

	// Global config.
	globalDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("T-CC9: MkdirAll global: %v", err)
	}
	writeConfig(t, globalDir, "config.toml", "[general]\nno_color = true\n")

	// Project directory: has .git + .cc-probeline.toml.
	projDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projDir, ".git"), 0o755); err != nil {
		t.Fatalf("T-CC9: MkdirAll .git: %v", err)
	}
	writeConfig(t, projDir, ".cc-probeline.toml", "[general]\ntutorial_hints = false\n")

	stdout, _, exitCode := runCheckConfigInDir(t, projDir, home, nil)

	if exitCode != 0 {
		t.Errorf("T-CC9: project override: expected exit 0, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "Source: project") {
		t.Errorf("T-CC9: project override: stdout should contain 'Source: project', got: %q", stdout)
	}
	if !strings.Contains(stdout, ".cc-probeline.toml") {
		t.Errorf("T-CC9: project override: stdout should mention '.cc-probeline.toml', got: %q", stdout)
	}
}

// ─── T-CC10: CC_PROBELINE_CONFIG → missing file → exit 2 ────────────────────

// TestCheckConfig_EnvPointsToMissingFile_ExitTwo sets CC_PROBELINE_CONFIG to a
// non-existent path and expects exit 2 + the filename in stdout.
//
// Verify-RED: LoadCascade step 1 calls Load(envPath); parseFile returns:
//
//	Error{Severity: SeverityError, File: missingPath, Message: "config file not found"}
//
// No silent fallback to lower sources when env path is set (concept §4.1).
// Plan T-CC10.
func TestCheckConfig_EnvPointsToMissingFile_ExitTwo(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.toml")

	stdout, _, exitCode := runCheckConfig(t, []string{"CC_PROBELINE_CONFIG=" + missingPath})

	if exitCode != 2 {
		t.Errorf("T-CC10: missing env config: expected exit 2, got %d; stdout: %q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "does-not-exist.toml") {
		t.Errorf("T-CC10: missing env config: stdout should mention 'does-not-exist.toml', got: %q", stdout)
	}
}

// ─── T-CC11: --verbose dumps all fields including defaults ───────────────────

// TestCheckConfig_VerboseDumpsFullSchema runs check-config --verbose with a
// minimal valid config and verifies that default-valued fields (e.g.
// ctx_warn_ratio, tutorial_hints) also appear in the output.
// Concept §8.5, plan T-CC11.
func TestCheckConfig_VerboseDumpsFullSchema(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC11: MkdirAll: %v", err)
	}
	// Only one field overridden; --verbose must show all fields including defaults.
	writeConfig(t, cfgDir, "config.toml", "[general]\ntutorial_hints = false\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home}, "--verbose")

	if exitCode != 0 {
		t.Errorf("T-CC11: --verbose: expected exit 0, got %d; stdout: %q", exitCode, stdout)
	}
	// These are default-valued fields that must appear under --verbose.
	for _, field := range []string{"ctx_warn_ratio", "tutorial_hints", "no_color"} {
		if !strings.Contains(stdout, field) {
			t.Errorf("T-CC11: --verbose: stdout should contain default field %q, got: %q", field, stdout)
		}
	}
}

// ─── T-CC12: without --verbose only non-default fields ───────────────────────

// TestCheckConfig_NonVerboseOmitsDefaults verifies that without --verbose only
// overridden fields appear in the effective config section. Default-valued fields
// (like ctx_warn_ratio = 0.70) must be absent.
// Concept §8.2, plan T-CC12.
func TestCheckConfig_NonVerboseOmitsDefaults(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC12: MkdirAll: %v", err)
	}
	// Only tutorial_hints overridden (default = true → set to false).
	writeConfig(t, cfgDir, "config.toml", "[general]\ntutorial_hints = false\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 0 {
		t.Errorf("T-CC12: non-verbose: expected exit 0, got %d; stdout: %q", exitCode, stdout)
	}
	// Overridden field must be shown.
	if !strings.Contains(stdout, "tutorial_hints") {
		t.Errorf("T-CC12: non-verbose: stdout should show overridden 'tutorial_hints', got: %q", stdout)
	}
	// Default-valued ctx_warn_ratio = 0.70 must NOT appear without --verbose.
	if strings.Contains(stdout, "ctx_warn_ratio") {
		t.Errorf("T-CC12: non-verbose: stdout must NOT show default-valued 'ctx_warn_ratio', got: %q", stdout)
	}
}

// ─── T-CC13: --json output is valid JSON ─────────────────────────────────────

// TestCheckConfig_JSONOutput_ParsesAsJSON runs check-config --json with a valid
// config and verifies the output parses as JSON containing the schema keys:
// "source", "config", "errors", "valid".
// Concept §2.3, plan T-CC13.
func TestCheckConfig_JSONOutput_ParsesAsJSON(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC13: MkdirAll: %v", err)
	}
	writeConfig(t, cfgDir, "config.toml", "[general]\ntutorial_hints = false\n")

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home}, "--json")

	if exitCode != 0 {
		t.Errorf("T-CC13: --json: expected exit 0, got %d; stdout: %q", exitCode, stdout)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("T-CC13: --json: output is not valid JSON: %v; stdout: %q", err, stdout)
	}

	for _, key := range []string{"source", "config", "errors", "valid"} {
		if _, ok := out[key]; !ok {
			t.Errorf("T-CC13: --json: missing key %q in JSON; keys present: %v", key, keysOf(out))
		}
	}
}

// ─── T-CC14: --json output has no ANSI sequences ─────────────────────────────

// TestCheckConfig_JSONOutput_NoANSIColors verifies that --json never outputs
// ANSI escape sequences, even when errors are present.
// Concept §2.3 / §8.6, plan T-CC14.
func TestCheckConfig_JSONOutput_NoANSIColors(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC14: MkdirAll: %v", err)
	}
	writeConfig(t, cfgDir, "config.toml", "version = 1\n[broken\n")

	stdout, _, _ := runCheckConfig(t, []string{"HOME=" + home}, "--json")

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("T-CC14: --json: output must not contain ANSI escapes; got: %q", stdout)
	}
}

// ─── T-CC15: unknown flag → exit 64, stderr non-empty ────────────────────────

// TestCheckConfig_UnknownFlag_Exit64 passes --magic to check-config and expects
// exit 64 with a usage message on stderr.
// Concept §8.1 (exit 64 for unknown flags, consistent with top-level parseMode),
// plan T-CC15.
func TestCheckConfig_UnknownFlag_Exit64(t *testing.T) {
	_, stderr, exitCode := runCheckConfig(t, nil, "--magic")

	if exitCode != 64 {
		t.Errorf("T-CC15: unknown flag: expected exit 64, got %d; stderr: %q", exitCode, stderr)
	}
	if stderr == "" {
		t.Errorf("T-CC15: unknown flag: stderr should contain a usage hint, got empty")
	}
}

// ─── T-CC16: multiple errors all listed ──────────────────────────────────────

// TestCheckConfig_MultipleErrors_AllListed creates a config with three distinct
// SeverityError conditions and verifies that all three fields appear in stdout
// and the summary contains "3 errors".
//
// Verify-RED manual error count for config:
//
//	[theme]          name = "defalt"       → SeverityError (unknown theme)
//	[thresholds]     ctx_warn_ratio = 70.0 → SeverityError (> 1.0)
//	                 ctx_critical_ratio = -1.0 → SeverityError (< 0.0)
//
// Validate() returns at least 3 SeverityError entries.
// Concept §8.3, plan T-CC16.
func TestCheckConfig_MultipleErrors_AllListed(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-CC16: MkdirAll: %v", err)
	}
	content := "[theme]\nname = \"defalt\"\n\n[thresholds]\nctx_warn_ratio = 70.0\nctx_critical_ratio = -1.0\n"
	writeConfig(t, cfgDir, "config.toml", content)

	stdout, _, exitCode := runCheckConfig(t, []string{"HOME=" + home})

	if exitCode != 2 {
		t.Errorf("T-CC16: multiple errors: expected exit 2, got %d; stdout: %q", exitCode, stdout)
	}

	for _, field := range []string{"theme.name", "ctx_warn_ratio", "ctx_critical_ratio"} {
		if !strings.Contains(stdout, field) {
			t.Errorf("T-CC16: multiple errors: stdout should contain field %q; got: %q", field, stdout)
		}
	}
	if !strings.Contains(stdout, "3 errors") {
		t.Errorf("T-CC16: multiple errors: stdout should contain '3 errors' summary; got: %q", stdout)
	}
}

// ─── utility ─────────────────────────────────────────────────────────────────

// keysOf returns the keys of a map as a slice (for error messages).
func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
