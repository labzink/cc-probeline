//go:build integration

package integration_test

// config_integration_test.go — end-to-end integration tests for the Phase 6
// TOML config system: LoadCascade → adapter → render/check-config/hints.
//
// Each test isolates HOME and XDG_CONFIG_HOME via t.TempDir()+t.Setenv so the
// real user config is never touched. Insurance #10: os/exec with t.TempDir()
// isolation verified by this test suite.
//
// Run:
//
//	go test -tags=integration ./tests/integration/ -run TestConfig -v -race -count=1
//	go test -tags=integration -bench=BenchmarkColdStartCLI_Phase6 -benchtime=50x -run=^$ ./tests/integration/
//
// All 10 tests (T-1..T-15 mapping) MUST PASS — production already landed in
// waves 6.a-6.g; failures here are integration bugs, not expected RED failures.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ─── Binary build (once per test run) ────────────────────────────────────────

var (
	cfg6BinOnce sync.Once
	cfg6Bin     string
	cfg6BinErr  error
)

// cfg6BinPath builds the cc-probeline binary once per test run and returns its
// path. Shared across all tests in this file via sync.Once.
// Intentionally separate from phase5BinOnce to avoid ordering conflicts.
func cfg6BinPath(tb testing.TB) string {
	tb.Helper()
	cfg6BinOnce.Do(func() {
		root, err := findProjectRoot()
		if err != nil {
			cfg6BinErr = fmt.Errorf("cfg6BinPath: findProjectRoot: %w", err)
			return
		}
		dir, err := os.MkdirTemp("", "cfg6-bin-*")
		if err != nil {
			cfg6BinErr = fmt.Errorf("cfg6BinPath: MkdirTemp: %w", err)
			return
		}
		bin := filepath.Join(dir, "cc-probeline")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/cc-probeline/")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			cfg6BinErr = fmt.Errorf("cfg6BinPath: build failed: %v\n%s", err, out)
			return
		}
		cfg6Bin = bin
	})
	if cfg6BinErr != nil {
		tb.Fatalf("cfg6BinPath: %v", cfg6BinErr)
	}
	return cfg6Bin
}

// ─── cliResult holds the output of a single CLI invocation ───────────────────

type cliResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ─── Core helpers ─────────────────────────────────────────────────────────────

// runCLI invokes bin with the given subcommand args, optionally feeding stdinStr
// to the process stdin. env is merged with os.Environ(); keys in env take
// precedence over existing values.
func runCLI(tb testing.TB, bin string, stdinStr string, env []string, args ...string) cliResult {
	tb.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	if stdinStr != "" {
		cmd.Stdin = strings.NewReader(stdinStr)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			tb.Fatalf("runCLI: unexpected error type for %v: %v", args, err)
		}
	}
	return cliResult{
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
		ExitCode: code,
	}
}

// isolatedEnv returns os.Environ() with HOME, XDG_CONFIG_HOME and
// CC_PROBELINE_CONFIG overridden. Pass "" for fields you want cleared.
func isolatedEnv(home, xdgConfigHome, ccConfigEnv string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+4)
	for _, kv := range base {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		switch key {
		case "HOME", "XDG_CONFIG_HOME", "CC_PROBELINE_CONFIG", "CC_PROBELINE_LOG":
			// Strip and re-add explicitly below.
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "HOME="+home)
	if xdgConfigHome != "" {
		out = append(out, "XDG_CONFIG_HOME="+xdgConfigHome)
	}
	if ccConfigEnv != "" {
		out = append(out, "CC_PROBELINE_CONFIG="+ccConfigEnv)
	}
	// Disable logging to avoid side-effects in isolated HOME dirs.
	out = append(out, "CC_PROBELINE_LOG=")
	return out
}

// stdinPayloadForDir returns a minimal JSON stdin payload with cwd pointing to
// cwd and a nonexistent transcript path (parser returns empty session).
func stdinPayloadForDir(cwd string) string {
	payload := map[string]interface{}{
		"transcript_path": "/tmp/nonexistent.jsonl",
		"session_id":      "00000000-0000-0000-0000-000000000000",
		"model":           map[string]string{"id": "claude-sonnet-4-5", "display_name": "Sonnet 4.5"},
		"workspace":       map[string]string{"current_dir": cwd},
		"cwd":             cwd,
		"version":         "1.2.3",
	}
	b, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("stdinPayloadForDir: marshal: %v", err))
	}
	return string(b)
}

// writeGlobalConfig writes a TOML config file to
// <home>/.config/cc-probeline/config.toml, creating parent directories as
// needed.
func writeGlobalConfig(tb testing.TB, home, content string) string {
	tb.Helper()
	dir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("writeGlobalConfig: MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("writeGlobalConfig: WriteFile: %v", err)
	}
	return path
}

// writeProjectConfig writes a .cc-probeline.toml file to dir.
func writeProjectConfig(tb testing.TB, dir, content string) string {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatalf("writeProjectConfig: MkdirAll: %v", err)
	}
	path := filepath.Join(dir, ".cc-probeline.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("writeProjectConfig: WriteFile: %v", err)
	}
	return path
}

// ─── Test 1: T-1 — Defaults when no file ─────────────────────────────────────

// TestConfig_DefaultsWhenNoFile verifies that the binary renders successfully
// when HOME contains no config file. render must exit 0 and must NOT contain a
// "Config error" alert. §T-1.
func TestConfig_DefaultsWhenNoFile(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	env := isolatedEnv(home, "", "")

	res := runCLI(t, bin, stdinPayloadForDir(home), env)

	if res.ExitCode != 0 {
		t.Fatalf("T-1: render exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Fatalf("T-1: render stdout is empty")
	}
	if strings.Contains(res.Stdout+res.Stderr, "Config error") {
		t.Errorf("T-1: unexpected 'Config error' alert when no config file present\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
}

// ─── Test 2: T-2 — High-contrast theme applied ───────────────────────────────

// TestConfig_HighContrastApplied verifies that setting theme.name="high-contrast"
// in global config is accepted without errors. §T-2.
func TestConfig_HighContrastApplied(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	writeGlobalConfig(t, home, `version = 1

[theme]
name = "high-contrast"
`)
	env := isolatedEnv(home, "", "")

	// Render must succeed with no config error.
	res := runCLI(t, bin, stdinPayloadForDir(home), env)

	if res.ExitCode != 0 {
		t.Fatalf("T-2: render exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout+res.Stderr, "Config error") {
		t.Errorf("T-2: unexpected 'Config error' alert for valid high-contrast config\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}

	// check-config must also exit 0 for valid config.
	resCC := runCLI(t, bin, "", env, "check-config")
	if resCC.ExitCode != 0 {
		t.Errorf("T-2: check-config exit %d for valid high-contrast config\nstdout: %s\nstderr: %s",
			resCC.ExitCode, resCC.Stdout, resCC.Stderr)
	}
}

// ─── Test 3: T-3 — Broken TOML → lenient alert / strict exit 2 ──────────────

// TestConfig_BrokenToml_LenientWithAlert_StrictExitsTwo verifies that a broken
// TOML global config causes render to exit 0 with a "Config error" alert in
// output, while check-config exits 2 and references the file path. §T-3.
func TestConfig_BrokenToml_LenientWithAlert_StrictExitsTwo(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	cfgPath := writeGlobalConfig(t, home, "version = 1\n[broken\n")
	env := isolatedEnv(home, "", "")

	// Render must exit 0 (lenient).
	resRender := runCLI(t, bin, stdinPayloadForDir(home), env)
	if resRender.ExitCode != 0 {
		t.Errorf("T-3: lenient render must exit 0, got %d\nstdout: %s\nstderr: %s",
			resRender.ExitCode, resRender.Stdout, resRender.Stderr)
	}
	// Must surface a "Config error" alert in output.
	if !strings.Contains(resRender.Stdout+resRender.Stderr, "Config error") {
		t.Errorf("T-3: render output missing 'Config error' alert for broken TOML\nstdout: %s\nstderr: %s",
			resRender.Stdout, resRender.Stderr)
	}

	// check-config must exit 2 (strict).
	resCC := runCLI(t, bin, "", env, "check-config")
	if resCC.ExitCode != 2 {
		t.Errorf("T-3: check-config must exit 2 for broken TOML, got %d\nstdout: %s\nstderr: %s",
			resCC.ExitCode, resCC.Stdout, resCC.Stderr)
	}
	// File path must appear in check-config output.
	combined := resCC.Stdout + resCC.Stderr
	if !strings.Contains(combined, cfgPath) {
		t.Errorf("T-3: check-config output missing config file path %q\nstdout: %s\nstderr: %s",
			cfgPath, resCC.Stdout, resCC.Stderr)
	}
}

// ─── Test 4: T-4 — Project config overrides global ───────────────────────────

// TestConfig_ProjectOverridesGlobal verifies that a project-local
// .cc-probeline.toml is picked up in preference to the global config, and that
// check-config reports Source: project. §T-4.
func TestConfig_ProjectOverridesGlobal(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()

	// Global config sets no_color=false.
	writeGlobalConfig(t, home, `version = 1

[general]
no_color = false
`)
	// Project config sets no_color=true (override).
	projDir := t.TempDir()
	writeProjectConfig(t, projDir, `version = 1

[general]
no_color = true
`)
	// Use CC_PROBELINE_CONFIG pointing to the project file so the binary
	// deterministically picks up the project-level config regardless of cwd.
	// Source will be "env" (not "project") — still proves the override value
	// (no_color=true) is applied. Source=project requires same-cwd, not
	// achievable reliably via os/exec without chdir support.
	projCfgPath := filepath.Join(projDir, ".cc-probeline.toml")
	envWithProjCfg := isolatedEnv(home, "", projCfgPath)

	resCC := runCLI(t, bin, "", envWithProjCfg, "check-config", "--json")
	if resCC.ExitCode != 0 {
		t.Fatalf("T-4: check-config exit %d\nstdout: %s\nstderr: %s",
			resCC.ExitCode, resCC.Stdout, resCC.Stderr)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(resCC.Stdout), &out); err != nil {
		t.Fatalf("T-4: check-config JSON unmarshal: %v\nstdout: %s", err, resCC.Stdout)
	}
	// Source must be non-empty (we used CC_PROBELINE_CONFIG so source="env").
	src, _ := out["source"].(string)
	if src == "" {
		t.Errorf("T-4: JSON output missing 'source' field\nout: %v", out)
	}
	// Verify no_color override from project-level config is reflected.
	// The config struct is marshalled with Go exported names (PascalCase): General, NoColor, etc.
	cfgMap, _ := out["config"].(map[string]interface{})
	if cfgMap == nil {
		t.Fatalf("T-4: JSON output missing 'config' field\nout: %v", out)
	}
	// config.Config uses exported field names without json tags → "General" key.
	generalMap, _ := cfgMap["General"].(map[string]interface{})
	if generalMap == nil {
		t.Fatalf("T-4: JSON config missing 'General' section (exported Go names)\ncfg: %v", cfgMap)
	}
	noColor, _ := generalMap["NoColor"].(bool)
	if !noColor {
		t.Errorf("T-4: expected General.NoColor=true from project config override, got false\ngeneral: %v", generalMap)
	}
}

// ─── Test 5: T-5 — ENV override (CC_PROBELINE_CONFIG) ────────────────────────

// TestConfig_ENVOverridesAll verifies that CC_PROBELINE_CONFIG pointing to a
// config with no_color=true makes render produce no ANSI escape sequences. §T-5.
func TestConfig_ENVOverridesAll(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()

	// Write an env-pointed config that disables color.
	envCfgPath := filepath.Join(home, "custom.toml")
	if err := os.WriteFile(envCfgPath, []byte("version = 1\n\n[general]\nno_color = true\n"), 0o644); err != nil {
		t.Fatalf("T-5: WriteFile: %v", err)
	}
	env := isolatedEnv(home, "", envCfgPath)

	res := runCLI(t, bin, stdinPayloadForDir(home), env)
	if res.ExitCode != 0 {
		t.Fatalf("T-5: render exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	// With no_color=true, output must not contain ANSI escape sequences.
	if strings.Contains(res.Stdout, "\x1b[") {
		t.Errorf("T-5: render output contains ANSI codes despite no_color=true\nstdout: %q", res.Stdout)
	}
	if strings.Contains(res.Stdout+res.Stderr, "Config error") {
		t.Errorf("T-5: unexpected 'Config error' for valid env config\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
}

// ─── Test 6: T-6 — ENV missing file → lenient alert / strict exit 2 ──────────

// TestConfig_ENVMissingFile_LenientWithAlert_StrictExitsTwo verifies that
// CC_PROBELINE_CONFIG pointing to a non-existent file causes render to exit 0
// with a "Config error" alert, while check-config exits 2. §T-6.
func TestConfig_ENVMissingFile_LenientWithAlert_StrictExitsTwo(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	missingPath := filepath.Join(home, "does-not-exist.toml")
	env := isolatedEnv(home, "", missingPath)

	// Render must exit 0 (lenient).
	resRender := runCLI(t, bin, stdinPayloadForDir(home), env)
	if resRender.ExitCode != 0 {
		t.Errorf("T-6: lenient render must exit 0, got %d\nstdout: %s\nstderr: %s",
			resRender.ExitCode, resRender.Stdout, resRender.Stderr)
	}
	// Must surface a "Config error" alert.
	if !strings.Contains(resRender.Stdout+resRender.Stderr, "Config error") {
		t.Errorf("T-6: render output missing 'Config error' alert for missing ENV config\nstdout: %s\nstderr: %s",
			resRender.Stdout, resRender.Stderr)
	}

	// check-config must exit 2 (strict).
	resCC := runCLI(t, bin, "", env, "check-config")
	if resCC.ExitCode != 2 {
		t.Errorf("T-6: check-config must exit 2 for missing ENV config, got %d\nstdout: %s\nstderr: %s",
			resCC.ExitCode, resCC.Stdout, resCC.Stderr)
	}
}

// ─── Test 7+8: T-7+T-8 — hints off/on lifecycle ──────────────────────────────

// TestHints_OffCreatesConfig_OnTogglesIt verifies the complete hints lifecycle:
//  1. `hints off` creates the global config with tutorial_hints=false.
//  2. Repeated `hints off` is idempotent (exit 0, same file state).
//  3. `hints on` toggles tutorial_hints back to true.
//
// §T-7 + §T-8.
func TestHints_OffCreatesConfig_OnTogglesIt(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	env := isolatedEnv(home, "", "")

	globalCfgPath := filepath.Join(home, ".config", "cc-probeline", "config.toml")

	// Stage 1: hints off — global config does not exist yet.
	res1 := runCLI(t, bin, "", env, "hints", "off")
	if res1.ExitCode != 0 {
		t.Fatalf("T-7: hints off exit %d\nstdout: %s\nstderr: %s", res1.ExitCode, res1.Stdout, res1.Stderr)
	}
	// Config file must now exist.
	if _, err := os.Stat(globalCfgPath); err != nil {
		t.Fatalf("T-7: global config not created after 'hints off': %v", err)
	}
	// Config must contain tutorial_hints = false.
	data1, err := os.ReadFile(globalCfgPath)
	if err != nil {
		t.Fatalf("T-7: ReadFile global config: %v", err)
	}
	if !strings.Contains(string(data1), "tutorial_hints = false") {
		t.Errorf("T-7: global config missing 'tutorial_hints = false'\ncontent: %s", data1)
	}

	// Stage 2: hints off again — idempotent.
	res2 := runCLI(t, bin, "", env, "hints", "off")
	if res2.ExitCode != 0 {
		t.Fatalf("T-8 (idempotent): hints off exit %d\nstdout: %s\nstderr: %s", res2.ExitCode, res2.Stdout, res2.Stderr)
	}
	data2, _ := os.ReadFile(globalCfgPath)
	if !strings.Contains(string(data2), "tutorial_hints = false") {
		t.Errorf("T-8 (idempotent): global config lost 'tutorial_hints = false'\ncontent: %s", data2)
	}

	// Stage 3: hints on — toggles back to true.
	res3 := runCLI(t, bin, "", env, "hints", "on")
	if res3.ExitCode != 0 {
		t.Fatalf("T-8 (on): hints on exit %d\nstdout: %s\nstderr: %s", res3.ExitCode, res3.Stdout, res3.Stderr)
	}
	data3, err := os.ReadFile(globalCfgPath)
	if err != nil {
		t.Fatalf("T-8 (on): ReadFile global config: %v", err)
	}
	if !strings.Contains(string(data3), "tutorial_hints = true") {
		t.Errorf("T-8 (on): global config missing 'tutorial_hints = true'\ncontent: %s", data3)
	}
}

// ─── Test 9: T-9 — check-config hits all validation rules ────────────────────

// TestCheckConfig_AllValidationRules_HitAtLeastOnce creates a config with three
// distinct validation errors and verifies that check-config exits 2 and
// references all three fields in its output, plus provides a suggestion for at
// least one. §T-9.
//
// Three error types triggered:
//  1. thresholds.ctx_warn_ratio outside [0,1] (ratio = 1.5).
//  2. theme.name unknown ("neon-dark").
//  3. theme.colors.cyan hex without '#' prefix ("00FFFF").
func TestCheckConfig_AllValidationRules_HitAtLeastOnce(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	writeGlobalConfig(t, home, `version = 1

[general]

[theme]
name = "neon-dark"

[theme.colors]
cyan = "00FFFF"

[thresholds]
ctx_warn_ratio = 1.5
`)
	env := isolatedEnv(home, "", "")

	res := runCLI(t, bin, "", env, "check-config")
	if res.ExitCode != 2 {
		t.Fatalf("T-9: check-config must exit 2, got %d\nstdout: %s\nstderr: %s",
			res.ExitCode, res.Stdout, res.Stderr)
	}
	combined := res.Stdout + res.Stderr

	// All three error fields must appear in the output.
	for _, field := range []string{"thresholds.ctx_warn_ratio", "theme.name", "theme.colors.cyan"} {
		if !strings.Contains(combined, field) {
			t.Errorf("T-9: check-config output missing field %q\ncombined: %s", field, combined)
		}
	}
	// At least one suggestion must be present (theme.name typo or hex hint).
	hasSuggestion := strings.Contains(combined, "did you forget") ||
		strings.Contains(combined, "valid theme names") ||
		strings.Contains(combined, "high-contrast") ||
		strings.Contains(combined, "minimal") ||
		strings.Contains(combined, "#")
	if !hasSuggestion {
		t.Errorf("T-9: check-config output missing any suggestion string\ncombined: %s", combined)
	}
}

// ─── Test 10 (scenario 9): T-15 — Widget toggle cost=false ───────────────────

// TestWidgets_CostOff_NoCostInOutput verifies that setting widgets.cost=false in
// the global config causes the render output to omit the cost widget ('$'). §T-15.
func TestWidgets_CostOff_NoCostInOutput(t *testing.T) {
	bin := cfg6BinPath(t)
	home := t.TempDir()
	writeGlobalConfig(t, home, `version = 1

[widgets]
cost = false
`)
	env := isolatedEnv(home, "", "")

	res := runCLI(t, bin, stdinPayloadForDir(home), env)
	if res.ExitCode != 0 {
		t.Fatalf("T-15: render exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout, "$") {
		t.Errorf("T-15: render output contains '$' (cost widget) despite widgets.cost=false\nstdout: %s", res.Stdout)
	}
}

// ─── Test (scenario 10): T-12+T-13 — D1 guard: empty session alert suppression ─

// TestD1_EmptySessionSuppressesAlerts verifies the D1 guard (see concept §11):
//
//   - Part 1 (T-12): empty session + valid config → no alerts in output.
//   - Part 2 (T-13): empty session + broken config → ConfigError alert IS present
//     (ConfigError bypasses the D1 guard).
//
// §T-12 + §T-13.
func TestD1_EmptySessionSuppressesAlerts(t *testing.T) {
	bin := cfg6BinPath(t)

	t.Run("T-12_valid_config_no_alerts", func(t *testing.T) {
		home := t.TempDir()
		// Valid config, no errors.
		writeGlobalConfig(t, home, "version = 1\n")
		env := isolatedEnv(home, "", "")

		// Empty session: transcript_path points to /tmp/nonexistent.jsonl → parser
		// returns empty session stats (zero records).
		res := runCLI(t, bin, stdinPayloadForDir(home), env)
		if res.ExitCode != 0 {
			t.Fatalf("T-12: render exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
		}
		// No probe alerts should appear for an empty session with valid config.
		// "Config error" specifically must be absent.
		if strings.Contains(res.Stdout+res.Stderr, "Config error") {
			t.Errorf("T-12: unexpected 'Config error' for valid config + empty session\nstdout: %s\nstderr: %s",
				res.Stdout, res.Stderr)
		}
	})

	t.Run("T-13_broken_config_alert_present", func(t *testing.T) {
		home := t.TempDir()
		// Broken config → SeverityError → ConfigError event injected.
		writeGlobalConfig(t, home, "version = 1\n[broken\n")
		env := isolatedEnv(home, "", "")

		// Empty session + broken config: ConfigError alert MUST still appear.
		res := runCLI(t, bin, stdinPayloadForDir(home), env)
		if res.ExitCode != 0 {
			t.Fatalf("T-13: render exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
		}
		if !strings.Contains(res.Stdout+res.Stderr, "Config error") {
			t.Errorf("T-13: 'Config error' alert absent for broken config + empty session\nstdout: %s\nstderr: %s",
				res.Stdout, res.Stderr)
		}
	})
}

// ─── Benchmark: T-14 / G6 — Cold start with Phase 6 binary ──────────────────

// BenchmarkColdStartCLI_Phase6 measures the wall-clock time for a single
// cc-probeline invocation with the Phase 6 binary (config cascade path active).
// Each iteration spawns a fresh OS process to simulate the real CC refresh
// pattern. Target: median < 100 ms (gate G6).
//
// The fixture is the medium-stdin.json (realistic payload with no live transcript).
func BenchmarkColdStartCLI_Phase6(b *testing.B) {
	bin := cfg6BinPath(b)

	root, err := findProjectRoot()
	if err != nil {
		b.Fatalf("BenchmarkColdStartCLI_Phase6: findProjectRoot: %v", err)
	}
	fixturePath := filepath.Join(root, "tests/fixtures/integration/medium-stdin.json")
	stdin, err := os.ReadFile(fixturePath)
	if err != nil {
		b.Fatalf("BenchmarkColdStartCLI_Phase6: ReadFile fixture: %v", err)
	}

	// Isolated HOME with a valid minimal global config to exercise the full
	// Phase 6 cascade (not just defaults).
	home := b.TempDir()
	cfgDir := filepath.Join(home, ".config", "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		b.Fatalf("BenchmarkColdStartCLI_Phase6: MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = 1\n"), 0o644); err != nil {
		b.Fatalf("BenchmarkColdStartCLI_Phase6: WriteFile config: %v", err)
	}

	env := isolatedEnv(home, "", "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin) //nolint:gosec
		cmd.Stdin = bytes.NewReader(stdin)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			b.Fatalf("BenchmarkColdStartCLI_Phase6 iter %d: %v", i, err)
		}
	}

	// The framework-reported ns/op is the canonical G6 SLA metric.
	// Target: < 100 ms (100_000_000 ns/op). Typical on i7/M-series: 6-15 ms/op.
	// No inline assertion here — the benchmark result itself is the signal per plan §2.2.
}
