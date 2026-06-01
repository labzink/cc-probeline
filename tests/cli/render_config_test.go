// Package cli_test — Phase 6.d render+config integration tests (T-RC1..T-RC6).
//
// These tests verify that runRender correctly wires config.LoadCascade into the
// render pipeline: lenient behaviour on broken configs, no_color propagation,
// ENV override, and widget toggles.
//
// All tests in this file are intentionally RED until Phase 6.d GREEN lands:
//   - runRender does not yet call config.LoadCascade.
//   - ExtraCacheEvents injection is not wired.
//   - config.no_color override is not applied.
package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── T-RC1: no config file → default behaviour ───────────────────────────────

// TestRender_NoConfig_DefaultBehavior verifies that when no config file exists
// anywhere in the cascade, the binary renders successfully (exit 0) and does
// NOT show "Config error" in the output. All widgets default to on.
func TestRender_NoConfig_DefaultBehavior(t *testing.T) {
	// Isolate HOME and XDG_CONFIG_HOME so no real config is found.
	home := t.TempDir()
	xdgCfg := t.TempDir()

	stdout, _, exitCode := run(t,
		[]string{
			"HOME=" + home,
			"XDG_CONFIG_HOME=" + xdgCfg,
		},
		[]byte(minimalPayload),
	)

	// GREEN expectation: exit 0 + non-empty output + no "Config error".
	if exitCode != 0 {
		t.Errorf("T-RC1: expected exit 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("T-RC1: stdout should be non-empty (default render); got empty")
	}
	if strings.Contains(stdout, "Config error") {
		t.Errorf("T-RC1: stdout must NOT contain 'Config error' with no config file; got: %q", stdout)
	}
}

// ─── T-RC2: broken TOML in global config → lenient, alert shown ──────────────

// TestRender_BrokenTOML_LenientWithAlert verifies §5.3 lenient render policy:
// a broken config.toml must NOT crash the binary (exit must be 0) and the
// output MUST contain the "Config error" alert text injected via ExtraCacheEvents.
func TestRender_BrokenTOML_LenientWithAlert(t *testing.T) {
	xdgCfg := t.TempDir()

	// Write an invalid TOML to the global config path.
	cfgDir := filepath.Join(xdgCfg, "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-RC2: MkdirAll: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	// Verify fixture: "this is not valid toml" → TOML parser returns SeverityError.
	if err := os.WriteFile(cfgPath, []byte("this is not valid toml\n"), 0o644); err != nil {
		t.Fatalf("T-RC2: WriteFile: %v", err)
	}

	stdout, _, exitCode := run(t,
		[]string{
			"HOME=" + t.TempDir(),
			"XDG_CONFIG_HOME=" + xdgCfg,
		},
		[]byte(minimalPayload),
	)

	// Lenient: binary must NOT crash on broken config.
	if exitCode != 0 {
		t.Errorf("T-RC2: expected exit 0 (lenient), got %d", exitCode)
	}
	// Alert must be injected into output.
	if !strings.Contains(stdout, "Config error") {
		t.Errorf("T-RC2: expected 'Config error' alert in output for broken TOML; got: %q", stdout)
	}
}

// ─── T-RC3: config no_color = true → no ANSI in output ──────────────────────

// TestRender_NoColorConfigOverridesAuto verifies §7.3 ENV/config precedence:
// when [general] no_color = true in config (and NO_COLOR env not set),
// the output must contain no ANSI escape sequences.
func TestRender_NoColorConfigOverridesAuto(t *testing.T) {
	xdgCfg := t.TempDir()

	// Write a config that disables colour.
	cfgDir := filepath.Join(xdgCfg, "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-RC3: MkdirAll: %v", err)
	}
	// Verify fixture: version=1 + no_color=true → valid TOML, SeverityError=0.
	noColorCfg := `version = 1

[general]
no_color = true
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(noColorCfg), 0o644); err != nil {
		t.Fatalf("T-RC3: WriteFile: %v", err)
	}

	// Explicitly unset NO_COLOR so the test exercises config, not env.
	env := []string{
		"HOME=" + t.TempDir(),
		"XDG_CONFIG_HOME=" + xdgCfg,
		"NO_COLOR=",           // explicit empty → unset semantics via override
		"TERM=xterm-256color", // encourage ANSI detection in auto mode
	}

	stdout, _, exitCode := run(t, env, []byte(minimalPayload))

	if exitCode != 0 {
		t.Errorf("T-RC3: expected exit 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("T-RC3: stdout should be non-empty; got empty")
	}
	// No ANSI escape sequences allowed when no_color = true.
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("T-RC3: stdout contains ANSI escapes despite no_color=true in config; got: %q", stdout)
	}
}

// ─── T-RC4: CC_PROBELINE_CONFIG points to nonexistent file → lenient + alert ─

// TestRender_ENVConfigMissingFile_LenientWithAlert verifies insurance #10:
// when CC_PROBELINE_CONFIG points to a nonexistent file the binary must exit 0
// (lenient) and the "Config error" alert must appear in output.
func TestRender_ENVConfigMissingFile_LenientWithAlert(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist.toml")
	// Verify: file must not exist.
	if _, err := os.Stat(nonexistent); !os.IsNotExist(err) {
		t.Fatalf("T-RC4: precondition: file should not exist at %s", nonexistent)
	}

	stdout, _, exitCode := run(t,
		[]string{
			"HOME=" + t.TempDir(),
			"CC_PROBELINE_CONFIG=" + nonexistent,
		},
		[]byte(minimalPayload),
	)

	// Lenient: missing explicit path must not crash the binary.
	if exitCode != 0 {
		t.Errorf("T-RC4: expected exit 0 (lenient), got %d", exitCode)
	}
	// Alert must be injected because Load() returns SeverityError for missing files.
	if !strings.Contains(stdout, "Config error") {
		t.Errorf("T-RC4: expected 'Config error' alert in output for missing ENV config; got: %q", stdout)
	}
}

// ─── T-RC5: [widgets] cost = false → no '$' in output ────────────────────────

// TestRender_ValidConfig_CostWidgetOff_NoCostInOutput verifies that setting
// [widgets] cost = false in config causes the cost probe to be hidden.
// The cost probe output normally contains '$'.
func TestRender_ValidConfig_CostWidgetOff_NoCostInOutput(t *testing.T) {
	xdgCfg := t.TempDir()

	cfgDir := filepath.Join(xdgCfg, "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-RC5: MkdirAll: %v", err)
	}
	// Verify fixture: cost = false, all others default (true).
	// cost_budget_usd = 0 → probe renders $0.0042 without a budget label.
	// When cost widget disabled, $ symbol should not appear.
	costOffCfg := `version = 1

[widgets]
cost = false
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(costOffCfg), 0o644); err != nil {
		t.Fatalf("T-RC5: WriteFile: %v", err)
	}

	stdout, _, exitCode := run(t,
		[]string{
			"HOME=" + t.TempDir(),
			"XDG_CONFIG_HOME=" + xdgCfg,
			"NO_COLOR=1", // strip ANSI for reliable string matching
		},
		[]byte(minimalPayload),
	)

	if exitCode != 0 {
		t.Errorf("T-RC5: expected exit 0, got %d", exitCode)
	}
	// With NO_COLOR=1 and cost=false, '$' must not appear in output.
	// minimalPayload has total_cost_usd = 0.0042 which renders as "$0.0042" when enabled.
	if strings.Contains(stdout, "$") {
		t.Errorf("T-RC5: output contains '$' (cost probe) despite [widgets] cost=false; got: %q", stdout)
	}
}

// ─── T-RC6: ctx_warn_ratio threshold → warn color applied ────────────────────

// TestRender_CtxThreshold_AppliedToColorPicking verifies that a custom
// ctx_warn_ratio = 0.5 causes the ctx probe to apply warn colour when ctx
// usage ratio exceeds 0.5. This is a smoke check: we verify the output is
// non-empty and exits 0 (deeper colour checking is renderer-layer territory).
//
// minimalPayload has input_tokens=1000 + output_tokens=500 = 1500 used
// out of context_window_size=200000 → ratio=0.0075 which is below 0.5.
// We use a tighter window payload to push above 0.5.
func TestRender_CtxThreshold_AppliedToColorPicking(t *testing.T) {
	xdgCfg := t.TempDir()

	cfgDir := filepath.Join(xdgCfg, "cc-probeline")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("T-RC6: MkdirAll: %v", err)
	}
	// ctx_warn_ratio = 0.5: warn fires when usage/window > 50%.
	thresholdCfg := `version = 1

[thresholds]
ctx_warn_ratio = 0.5
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(thresholdCfg), 0o644); err != nil {
		t.Fatalf("T-RC6: WriteFile: %v", err)
	}

	// Payload: window=2000, usage=1200 → ratio=0.60 > 0.5 → warn colour.
	// Verify arithmetic: 1200/2000 = 0.60, default ctx_warn_ratio=0.70 would NOT
	// trigger warn, but 0.50 threshold WOULD → colour change observable.
	highCtxPayload := `{
  "model": {"id": "claude-opus-4-5", "display_name": "Claude Opus 4.5"},
  "effort": {"level": "off"},
  "session_id": "test-session-ctx",
  "transcript_path": "/nonexistent/path.jsonl",
  "cwd": "/tmp",
  "context_window": {"context_window_size": 2000, "current_usage": {"input_tokens": 1100, "output_tokens": 100}},
  "cost": {"total_cost_usd": 0.001, "total_api_duration_ms": 500}
}`

	stdout, _, exitCode := run(t,
		[]string{
			"HOME=" + t.TempDir(),
			"XDG_CONFIG_HOME=" + xdgCfg,
		},
		[]byte(highCtxPayload),
	)

	// Smoke: binary must not crash; output must be non-empty.
	if exitCode != 0 {
		t.Errorf("T-RC6: expected exit 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("T-RC6: stdout should be non-empty; got empty")
	}
	// Phase 6.8.e (T-22): ctx Full no longer emits "%" — the used-K number is
	// coloured instead. Verify the bar glyph is present (proves ctx probe rendered).
	if !strings.Contains(stdout, "ctx") {
		t.Errorf("T-RC6: expected ctx label in output (probe is visible); got: %q", stdout)
	}
}
