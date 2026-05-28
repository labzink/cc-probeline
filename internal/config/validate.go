package config

import (
	"fmt"
	"regexp"
)

// hexColorRe matches a valid #RRGGBB hex colour string.
var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// emailRe is a simple sanity check: at least one non-@ on each side of '@'.
var emailRe = regexp.MustCompile(`^[^@]+@[^@]+$`)

// Validate runs semantic checks on cfg. Returns []Error (empty == valid).
// cfg is treated as read-only; use ApplyRangeFix to mutate invalid fields.
func Validate(cfg *Config) []Error {
	var errs []Error

	// Version: 0 (unset) and 1 are both acceptable.
	if cfg.Version != 0 && cfg.Version != 1 {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "version",
			Message:  fmt.Sprintf("unsupported version: %d (Phase 6 supports version 1)", cfg.Version),
		})
	}

	// Theme.Name: must be one of the known values.
	validThemeNames := map[string]bool{
		"":              true,
		"default":       true,
		"high-contrast": true,
		"minimal":       true,
	}
	if !validThemeNames[cfg.Theme.Name] {
		hint := suggestFor("theme.name", cfg.Theme.Name, nil)
		if hint == "" {
			// Always provide a hint listing valid options when no Levenshtein match.
			hint = "valid theme names: default, high-contrast, minimal"
		}
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "theme.name",
			Message:  fmt.Sprintf("unknown theme: %q. Valid: default, high-contrast, minimal", cfg.Theme.Name),
			Hint:     hint,
		})
	}

	// Theme.Colors: each non-empty color must be a valid #RRGGBB hex string.
	type colorField struct {
		name  string
		value string
	}
	colorFields := []colorField{
		{"theme.colors.cyan", cfg.Theme.Colors.Cyan},
		{"theme.colors.yellow", cfg.Theme.Colors.Yellow},
		{"theme.colors.red", cfg.Theme.Colors.Red},
		{"theme.colors.green", cfg.Theme.Colors.Green},
		{"theme.colors.orange", cfg.Theme.Colors.Orange},
		{"theme.colors.magenta", cfg.Theme.Colors.Magenta},
		{"theme.colors.dim", cfg.Theme.Colors.Dim},
	}
	for _, cf := range colorFields {
		if cf.value == "" {
			continue // empty == use palette default, OK
		}
		if !hexColorRe.MatchString(cf.value) {
			hint := suggestFor(cf.name, cf.value, nil)
			errs = append(errs, Error{
				Severity: SeverityError,
				Field:    cf.name,
				Message:  fmt.Sprintf("invalid hex color: %q (expected #RRGGBB)", cf.value),
				Hint:     hint,
			})
		}
	}

	// Thresholds.CtxWarnRatio: must be in [0.0, 1.0].
	if cfg.Thresholds.CtxWarnRatio < 0.0 || cfg.Thresholds.CtxWarnRatio > 1.0 {
		hint := suggestFor("thresholds.ctx_warn_ratio", cfg.Thresholds.CtxWarnRatio, nil)
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.ctx_warn_ratio",
			Message:  "ratio must be in [0.0, 1.0]",
			Hint:     hint,
		})
	}

	// Thresholds.CtxCriticalRatio: must be in [0.0, 1.0] and >= CtxWarnRatio.
	if cfg.Thresholds.CtxCriticalRatio < 0.0 || cfg.Thresholds.CtxCriticalRatio > 1.0 {
		hint := suggestFor("thresholds.ctx_critical_ratio", cfg.Thresholds.CtxCriticalRatio, nil)
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.ctx_critical_ratio",
			Message:  "ratio must be in [0.0, 1.0]",
			Hint:     hint,
		})
	} else if cfg.Thresholds.CtxCriticalRatio < cfg.Thresholds.CtxWarnRatio {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.ctx_critical_ratio",
			Message: fmt.Sprintf(
				"critical ratio must be >= warn ratio (%.2f < %.2f)",
				cfg.Thresholds.CtxCriticalRatio,
				cfg.Thresholds.CtxWarnRatio,
			),
		})
	}

	// Thresholds.OrchTTLMinutes: 0 == "use default"; only < 0 is an error.
	if cfg.Thresholds.OrchTTLMinutes < 0 {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.orch_ttl_minutes",
			Message:  "must be positive",
		})
	}

	// Thresholds.SubagentGapMinutes: same semantics.
	if cfg.Thresholds.SubagentGapMinutes < 0 {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.subagent_gap_minutes",
			Message:  "must be positive",
		})
	}

	// Thresholds.CostBudgetUSD: must be >= 0 (0 == disabled).
	if cfg.Thresholds.CostBudgetUSD < 0 {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.cost_budget_usd",
			Message:  "cost budget must be non-negative",
		})
	}

	// General.RefreshIntervalHint: warn if outside [1, 3600].
	if cfg.General.RefreshIntervalHint < 1 || cfg.General.RefreshIntervalHint > 3600 {
		errs = append(errs, Error{
			Severity: SeverityWarning,
			Field:    "general.refresh_interval_hint",
			Message:  fmt.Sprintf("refresh interval %d is unusual (typical: 1-60)", cfg.General.RefreshIntervalHint),
		})
	}

	// Probes.Email.Address: warn when non-empty but malformed.
	if cfg.Probes.Email.Address != "" && !emailRe.MatchString(cfg.Probes.Email.Address) {
		errs = append(errs, Error{
			Severity: SeverityWarning,
			Field:    "probes.email.address",
			Message:  fmt.Sprintf("email looks malformed: %q", cfg.Probes.Email.Address),
		})
	}

	return errs
}

// ApplyRangeFix mutates cfg in place, replacing each field that Validate
// would flag as SeverityError with the default value. Used by lenient callers
// (runRender) before passing cfg to ToProbesConfig/ToTheme.
// Returns the list of fields that were fixed (for slog debugging).
func ApplyRangeFix(cfg *Config) []string {
	def := Default()
	var fixed []string

	// Version.
	if cfg.Version != 0 && cfg.Version != 1 {
		cfg.Version = def.Version
		fixed = append(fixed, "version")
	}

	// Theme.Name.
	validThemeNames := map[string]bool{
		"":              true,
		"default":       true,
		"high-contrast": true,
		"minimal":       true,
	}
	if !validThemeNames[cfg.Theme.Name] {
		cfg.Theme.Name = def.Theme.Name
		fixed = append(fixed, "theme.name")
	}

	// Theme.Colors — fix individual invalid color overrides.
	type colorPtr struct {
		name string
		ptr  *string
		defv string
	}
	colorPtrs := []colorPtr{
		{"theme.colors.cyan", &cfg.Theme.Colors.Cyan, def.Theme.Colors.Cyan},
		{"theme.colors.yellow", &cfg.Theme.Colors.Yellow, def.Theme.Colors.Yellow},
		{"theme.colors.red", &cfg.Theme.Colors.Red, def.Theme.Colors.Red},
		{"theme.colors.green", &cfg.Theme.Colors.Green, def.Theme.Colors.Green},
		{"theme.colors.orange", &cfg.Theme.Colors.Orange, def.Theme.Colors.Orange},
		{"theme.colors.magenta", &cfg.Theme.Colors.Magenta, def.Theme.Colors.Magenta},
		{"theme.colors.dim", &cfg.Theme.Colors.Dim, def.Theme.Colors.Dim},
	}
	for _, cp := range colorPtrs {
		if *cp.ptr != "" && !hexColorRe.MatchString(*cp.ptr) {
			*cp.ptr = cp.defv
			fixed = append(fixed, cp.name)
		}
	}

	// Thresholds.CtxWarnRatio.
	if cfg.Thresholds.CtxWarnRatio < 0.0 || cfg.Thresholds.CtxWarnRatio > 1.0 {
		cfg.Thresholds.CtxWarnRatio = def.Thresholds.CtxWarnRatio
		fixed = append(fixed, "thresholds.ctx_warn_ratio")
	}

	// Thresholds.CtxCriticalRatio.
	if cfg.Thresholds.CtxCriticalRatio < 0.0 || cfg.Thresholds.CtxCriticalRatio > 1.0 {
		cfg.Thresholds.CtxCriticalRatio = def.Thresholds.CtxCriticalRatio
		fixed = append(fixed, "thresholds.ctx_critical_ratio")
	} else if cfg.Thresholds.CtxCriticalRatio < cfg.Thresholds.CtxWarnRatio {
		cfg.Thresholds.CtxCriticalRatio = def.Thresholds.CtxCriticalRatio
		fixed = append(fixed, "thresholds.ctx_critical_ratio")
	}

	// Thresholds.OrchTTLMinutes.
	if cfg.Thresholds.OrchTTLMinutes < 0 {
		cfg.Thresholds.OrchTTLMinutes = def.Thresholds.OrchTTLMinutes
		fixed = append(fixed, "thresholds.orch_ttl_minutes")
	}

	// Thresholds.SubagentGapMinutes.
	if cfg.Thresholds.SubagentGapMinutes < 0 {
		cfg.Thresholds.SubagentGapMinutes = def.Thresholds.SubagentGapMinutes
		fixed = append(fixed, "thresholds.subagent_gap_minutes")
	}

	// Thresholds.CostBudgetUSD.
	if cfg.Thresholds.CostBudgetUSD < 0 {
		cfg.Thresholds.CostBudgetUSD = def.Thresholds.CostBudgetUSD
		fixed = append(fixed, "thresholds.cost_budget_usd")
	}

	return fixed
}
