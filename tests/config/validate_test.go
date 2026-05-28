package config_test

// Tests for Validate(cfg *Config) []Error and ApplyRangeFix(cfg *Config) []string.
// All T-V* tests are RED by design: the production stub returns nil.
// T-V1 (Default_NoErrors) passes legitimately — stub returns nil == valid.

import (
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// helper: find errors matching a given severity in the returned slice.
func errsBySeverity(errs []config.Error, sev config.Severity) []config.Error {
	var out []config.Error
	for _, e := range errs {
		if e.Severity == sev {
			out = append(out, e)
		}
	}
	return out
}

// T-V1: Validate(Default()) must return nil/empty — the zero config is always valid.
func TestValidate_Default_NoErrors(t *testing.T) {
	errs := config.Validate(config.Default())
	if len(errs) != 0 {
		t.Errorf("expected no errors for Default(), got %d: %v", len(errs), errs)
	}
}

// T-V2: Version != 0 && != 1 must produce a SeverityError mentioning "unsupported version".
func TestValidate_VersionUnsupported(t *testing.T) {
	cfg := config.Default()
	cfg.Version = 2
	errs := config.Validate(cfg)
	severityErrs := errsBySeverity(errs, config.SeverityError)
	if len(severityErrs) == 0 {
		t.Fatal("expected at least 1 SeverityError for Version=2, got none")
	}
	found := false
	for _, e := range severityErrs {
		if strings.Contains(strings.ToLower(e.Message), "unsupported version") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error message to contain 'unsupported version', got: %v", severityErrs)
	}
}

// T-V3: Unknown theme name triggers SeverityError with non-empty Hint (suggestion).
func TestValidate_ThemeNameUnknown(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "neon"
	errs := config.Validate(cfg)
	severityErrs := errsBySeverity(errs, config.SeverityError)
	if len(severityErrs) == 0 {
		t.Fatal("expected SeverityError for Theme.Name='neon', got none")
	}
	hasHint := false
	for _, e := range severityErrs {
		if e.Hint != "" {
			hasHint = true
			break
		}
	}
	if !hasHint {
		t.Errorf("expected non-empty Hint for unknown theme name, got: %v", severityErrs)
	}
}

// T-V4: Close typo "defalt" (DL=1 from "default") → Hint contains "default".
func TestValidate_ThemeNameTypo_LevenshteinSuggestion(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "defalt"
	errs := config.Validate(cfg)
	severityErrs := errsBySeverity(errs, config.SeverityError)
	if len(severityErrs) == 0 {
		t.Fatal("expected SeverityError for Theme.Name='defalt', got none")
	}
	found := false
	for _, e := range severityErrs {
		if strings.Contains(e.Hint, "default") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Hint to suggest 'default', errors: %v", severityErrs)
	}
}

// T-V5: All recognised theme names must produce no errors.
func TestValidate_ThemeNameAllowed(t *testing.T) {
	allowed := []string{"", "default", "high-contrast", "minimal"}
	for _, name := range allowed {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Theme.Name = name
			errs := config.Validate(cfg)
			if len(errsBySeverity(errs, config.SeverityError)) > 0 {
				t.Errorf("Theme.Name=%q expected no SeverityError, got: %v", name, errs)
			}
		})
	}
}

// T-V6: Valid 6-digit hex color with '#' prefix must produce no error.
func TestValidate_HexColor_Valid(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Colors.Red = "#FF00AA"
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) > 0 {
		t.Errorf("expected no SeverityError for valid hex '#FF00AA', got: %v", errs)
	}
}

// T-V7: Non-hex color name must produce a SeverityError.
func TestValidate_HexColor_Invalid(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Colors.Red = "purple"
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) == 0 {
		t.Error("expected SeverityError for Colors.Red='purple', got none")
	}
}

// T-V8: 6-digit hex without '#' must produce SeverityError + Hint about missing '#'.
func TestValidate_HexColor_MissingHash(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Colors.Red = "FF00AA"
	errs := config.Validate(cfg)
	severityErrs := errsBySeverity(errs, config.SeverityError)
	if len(severityErrs) == 0 {
		t.Fatal("expected SeverityError for Colors.Red='FF00AA' (no #), got none")
	}
	found := false
	for _, e := range severityErrs {
		if strings.Contains(strings.ToLower(e.Hint), "#") || strings.Contains(strings.ToLower(e.Hint), "prefix") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Hint to mention '#' or 'prefix', got: %v", severityErrs)
	}
}

// T-V9: Empty color string is valid (keep palette default).
func TestValidate_HexColor_EmptyOK(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Colors.Red = ""
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) > 0 {
		t.Errorf("expected no SeverityError for empty Colors.Red, got: %v", errs)
	}
}

// T-V10: CtxWarnRatio=70.0 (looks like a percent) → SeverityError + Hint mentioning percent.
func TestValidate_CtxWarnRatio_OutOfRange_AsPercent(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxWarnRatio = 70.0
	// Also bump critical so the critical<warn rule does not fire additionally.
	cfg.Thresholds.CtxCriticalRatio = 90.0
	errs := config.Validate(cfg)
	severityErrs := errsBySeverity(errs, config.SeverityError)
	if len(severityErrs) == 0 {
		t.Fatal("expected SeverityError for CtxWarnRatio=70.0, got none")
	}
	found := false
	for _, e := range severityErrs {
		if strings.Contains(strings.ToLower(e.Hint), "percent") || strings.Contains(strings.ToLower(e.Hint), "0.70") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Hint to mention percent/0.70, got: %v", severityErrs)
	}
}

// T-V11: CtxWarnRatio negative → SeverityError.
func TestValidate_CtxWarnRatio_Negative(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxWarnRatio = -0.1
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) == 0 {
		t.Error("expected SeverityError for CtxWarnRatio=-0.1, got none")
	}
}

// T-V12: CtxCriticalRatio < CtxWarnRatio → SeverityError.
func TestValidate_CtxCriticalLowerThanWarn(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxWarnRatio = 0.8
	cfg.Thresholds.CtxCriticalRatio = 0.7
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) == 0 {
		t.Error("expected SeverityError when CtxCriticalRatio < CtxWarnRatio, got none")
	}
}

// T-V13: OrchTTLMinutes < 0 → SeverityError.
func TestValidate_OrchTTLNegative(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.OrchTTLMinutes = -1
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) == 0 {
		t.Error("expected SeverityError for OrchTTLMinutes=-1, got none")
	}
}

// T-V14: OrchTTLMinutes=0 is treated as "use default" and must not error.
func TestValidate_OrchTTLZero_OK(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.OrchTTLMinutes = 0
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) > 0 {
		t.Errorf("expected no SeverityError for OrchTTLMinutes=0, got: %v", errs)
	}
}

// T-V15: SubagentGapMinutes < 0 → SeverityError.
func TestValidate_SubagentGapNegative(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.SubagentGapMinutes = -1
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) == 0 {
		t.Error("expected SeverityError for SubagentGapMinutes=-1, got none")
	}
}

// T-V16: CostBudgetUSD < 0 → SeverityError.
func TestValidate_CostBudgetNegative(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CostBudgetUSD = -1
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) == 0 {
		t.Error("expected SeverityError for CostBudgetUSD=-1, got none")
	}
}

// T-V17: RefreshIntervalHint=10000 → SeverityWarning (not Error).
func TestValidate_RefreshIntervalHintHigh_Warning(t *testing.T) {
	cfg := config.Default()
	cfg.General.RefreshIntervalHint = 10000
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) > 0 {
		t.Error("RefreshIntervalHint=10000 must not produce SeverityError")
	}
	if len(errsBySeverity(errs, config.SeverityWarning)) == 0 {
		t.Error("expected SeverityWarning for RefreshIntervalHint=10000, got none")
	}
}

// T-V18: Malformed email address → SeverityWarning.
func TestValidate_EmailMalformed_Warning(t *testing.T) {
	cfg := config.Default()
	cfg.Probes.Email.Address = "not-an-email"
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityError)) > 0 {
		t.Error("malformed email must not produce SeverityError (only Warning)")
	}
	if len(errsBySeverity(errs, config.SeverityWarning)) == 0 {
		t.Error("expected SeverityWarning for malformed email, got none")
	}
}

// T-V19: Empty email address is valid (auto-detect from session).
func TestValidate_EmailEmptyOK(t *testing.T) {
	cfg := config.Default()
	cfg.Probes.Email.Address = ""
	errs := config.Validate(cfg)
	if len(errsBySeverity(errs, config.SeverityWarning)) > 0 {
		for _, e := range errsBySeverity(errs, config.SeverityWarning) {
			if strings.Contains(strings.ToLower(e.Message), "email") {
				t.Errorf("unexpected email Warning for empty address: %v", e)
			}
		}
	}
}

// T-V20: ApplyRangeFix mutates invalid ratio to default and reports the field path.
func TestApplyRangeFix_RestoresDefaults(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxWarnRatio = -0.5 // invalid
	// Raise critical ratio above warn to avoid double-firing the critical<warn rule
	// (that would also need fixing). Keep critical at a valid value for this test.
	cfg.Thresholds.CtxCriticalRatio = 0.9

	fixed := config.ApplyRangeFix(cfg)

	// The field path for CtxWarnRatio must appear in the returned list.
	const wantField = "thresholds.ctx_warn_ratio"
	found := false
	for _, f := range fixed {
		if f == wantField {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in fixed fields %v", wantField, fixed)
	}

	// cfg must be mutated to the default value (0.70).
	const wantRatio = 0.70
	if cfg.Thresholds.CtxWarnRatio != wantRatio {
		t.Errorf("expected CtxWarnRatio reset to %.2f, got %.2f", wantRatio, cfg.Thresholds.CtxWarnRatio)
	}
}

// T-V21: ApplyRangeFix only mutates invalid fields; valid fields are untouched.
func TestApplyRangeFix_LeavesValidFields(t *testing.T) {
	cfg := config.Default()
	cfg.General.TutorialHints = false     // valid, must survive
	cfg.Thresholds.CtxWarnRatio = -0.5    // invalid, will be fixed
	cfg.Thresholds.CtxCriticalRatio = 0.9 // valid, must survive

	config.ApplyRangeFix(cfg)

	if cfg.General.TutorialHints != false {
		t.Error("ApplyRangeFix must not change TutorialHints=false")
	}
	if cfg.Thresholds.CtxCriticalRatio != 0.9 {
		t.Errorf("ApplyRangeFix must not change valid CtxCriticalRatio, got %.2f", cfg.Thresholds.CtxCriticalRatio)
	}
}

// T-VX: suggestRatio coverage — Validate calls suggestRatio for out-of-range ratios.
// CtxWarnRatio=70 looks like a percentage → hint must mention "percent".
func TestValidate_CtxCriticalRatio_AsPercent_Hint(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxCriticalRatio = 90.0
	cfg.Thresholds.CtxWarnRatio = 70.0 // also out-of-range to avoid critical<warn
	errs := config.Validate(cfg)
	severityErrs := errsBySeverity(errs, config.SeverityError)
	found := false
	for _, e := range severityErrs {
		if strings.Contains(strings.ToLower(e.Hint), "percent") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected suggestRatio hint (percent) for CtxCriticalRatio=90.0, got errs: %v", severityErrs)
	}
}

// T-V-APF0: ApplyRangeFix resets unsupported Version to default (1).
func TestApplyRangeFix_Version_Reset(t *testing.T) {
	cfg := config.Default()
	cfg.Version = 99 // unsupported

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'version' in fixed fields %v", fixed)
	}
	if cfg.Version != 1 {
		t.Errorf("expected Version reset to 1, got %d", cfg.Version)
	}
}

// T-V-APF1: ApplyRangeFix resets CtxWarnRatio when > 1.0.
func TestApplyRangeFix_CtxWarnRatio_TooHigh(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxWarnRatio = 1.5 // > 1.0

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "thresholds.ctx_warn_ratio" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thresholds.ctx_warn_ratio' in fixed fields %v", fixed)
	}
}

// T-V-APF2: ApplyRangeFix resets CtxCriticalRatio when > 1.0.
func TestApplyRangeFix_CtxCriticalRatio_TooHigh(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxCriticalRatio = 1.5 // > 1.0

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "thresholds.ctx_critical_ratio" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thresholds.ctx_critical_ratio' in fixed fields %v", fixed)
	}
}

// T-V-APF3: ApplyRangeFix resets OrchTTLMinutes when negative.
func TestApplyRangeFix_OrchTTL_Negative_Reset(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.OrchTTLMinutes = -1

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "thresholds.orch_ttl_minutes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thresholds.orch_ttl_minutes' in fixed fields %v", fixed)
	}
	def := config.Default()
	if cfg.Thresholds.OrchTTLMinutes != def.Thresholds.OrchTTLMinutes {
		t.Errorf("expected reset to default %d, got %d", def.Thresholds.OrchTTLMinutes, cfg.Thresholds.OrchTTLMinutes)
	}
}

// T-V22: ApplyRangeFix resets invalid hex color to empty string (palette default).
func TestApplyRangeFix_InvalidColor_Reset(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Colors.Red = "purple" // invalid — not #RRGGBB

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "theme.colors.red" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'theme.colors.red' in fixed fields %v", fixed)
	}
	// After fix, the value must be the default (empty string == use palette default).
	def := config.Default()
	if cfg.Theme.Colors.Red != def.Theme.Colors.Red {
		t.Errorf("ApplyRangeFix did not reset Theme.Colors.Red; got %q, want %q",
			cfg.Theme.Colors.Red, def.Theme.Colors.Red)
	}
}

// T-V23: ApplyRangeFix resets CtxCriticalRatio to default when critical < warn.
func TestApplyRangeFix_CriticalLowerThanWarn_Reset(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxWarnRatio = 0.8
	cfg.Thresholds.CtxCriticalRatio = 0.5 // invalid: < warn

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "thresholds.ctx_critical_ratio" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thresholds.ctx_critical_ratio' in fixed fields %v", fixed)
	}
	def := config.Default()
	if cfg.Thresholds.CtxCriticalRatio != def.Thresholds.CtxCriticalRatio {
		t.Errorf("ApplyRangeFix did not reset CtxCriticalRatio; got %.2f, want %.2f",
			cfg.Thresholds.CtxCriticalRatio, def.Thresholds.CtxCriticalRatio)
	}
}

// T-V24: ApplyRangeFix resets CostBudgetUSD to default (0.0) when negative.
func TestApplyRangeFix_CostBudgetNegative_Reset(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CostBudgetUSD = -5.0 // invalid

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "thresholds.cost_budget_usd" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thresholds.cost_budget_usd' in fixed fields %v", fixed)
	}
	def := config.Default()
	if cfg.Thresholds.CostBudgetUSD != def.Thresholds.CostBudgetUSD {
		t.Errorf("ApplyRangeFix did not reset CostBudgetUSD; got %.2f, want %.2f",
			cfg.Thresholds.CostBudgetUSD, def.Thresholds.CostBudgetUSD)
	}
}

// T-V25: ApplyRangeFix resets SubagentGapMinutes to default when negative.
func TestApplyRangeFix_SubagentGapNegative_Reset(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.SubagentGapMinutes = -1 // invalid

	fixed := config.ApplyRangeFix(cfg)

	found := false
	for _, f := range fixed {
		if f == "thresholds.subagent_gap_minutes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'thresholds.subagent_gap_minutes' in fixed fields %v", fixed)
	}
	def := config.Default()
	if cfg.Thresholds.SubagentGapMinutes != def.Thresholds.SubagentGapMinutes {
		t.Errorf("ApplyRangeFix did not reset SubagentGapMinutes; got %d, want %d",
			cfg.Thresholds.SubagentGapMinutes, def.Thresholds.SubagentGapMinutes)
	}
}
