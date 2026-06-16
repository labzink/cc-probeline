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

	// Thresholds.CtxNoticeRatio: must be in [0.0, 1.0].
	if cfg.Thresholds.CtxNoticeRatio < 0.0 || cfg.Thresholds.CtxNoticeRatio > 1.0 {
		hint := suggestFor("thresholds.ctx_notice_ratio", cfg.Thresholds.CtxNoticeRatio, nil)
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.ctx_notice_ratio",
			Message:  "ratio must be in [0.0, 1.0]",
			Hint:     hint,
		})
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

	// Thresholds.CtxCriticalRatio: must be in [0.0, 1.0].
	if cfg.Thresholds.CtxCriticalRatio < 0.0 || cfg.Thresholds.CtxCriticalRatio > 1.0 {
		hint := suggestFor("thresholds.ctx_critical_ratio", cfg.Thresholds.CtxCriticalRatio, nil)
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.ctx_critical_ratio",
			Message:  "ratio must be in [0.0, 1.0]",
			Hint:     hint,
		})
	}

	// Strict ordering notice < warn < critical. Only checked when all three are
	// individually in range, so out-of-range values do not double-fire here.
	if inUnitInterval(cfg.Thresholds.CtxNoticeRatio) &&
		inUnitInterval(cfg.Thresholds.CtxWarnRatio) &&
		inUnitInterval(cfg.Thresholds.CtxCriticalRatio) &&
		!(cfg.Thresholds.CtxNoticeRatio < cfg.Thresholds.CtxWarnRatio &&
			cfg.Thresholds.CtxWarnRatio < cfg.Thresholds.CtxCriticalRatio) {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    "thresholds.ctx_critical_ratio",
			Message: fmt.Sprintf(
				"ctx ratios must strictly increase: notice (%.2f) < warn (%.2f) < critical (%.2f)",
				cfg.Thresholds.CtxNoticeRatio,
				cfg.Thresholds.CtxWarnRatio,
				cfg.Thresholds.CtxCriticalRatio,
			),
		})
	}

	// Thresholds.Quota5h / Quota7d colour-ratio trios.
	errs = validateRatioTrio(errs, "thresholds.quota_5h",
		cfg.Thresholds.Quota5hNoticeRatio, cfg.Thresholds.Quota5hWarnRatio, cfg.Thresholds.Quota5hCriticalRatio)
	errs = validateRatioTrio(errs, "thresholds.quota_7d",
		cfg.Thresholds.Quota7dNoticeRatio, cfg.Thresholds.Quota7dWarnRatio, cfg.Thresholds.Quota7dCriticalRatio)

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

// inUnitInterval reports whether v lies within the closed unit interval [0, 1].
func inUnitInterval(v float64) bool { return v >= 0.0 && v <= 1.0 }

// validateRatioTrio appends a SeverityError for each ratio outside [0, 1] under
// the given TOML field prefix (e.g. "thresholds.quota_5h"), plus one error when
// the three are in range but do not strictly increase (notice < warn < critical).
func validateRatioTrio(errs []Error, prefix string, notice, warn, critical float64) []Error {
	for _, f := range []struct {
		suffix string
		v      float64
	}{{"notice_ratio", notice}, {"warn_ratio", warn}, {"critical_ratio", critical}} {
		if f.v < 0.0 || f.v > 1.0 {
			field := prefix + "_" + f.suffix
			errs = append(errs, Error{
				Severity: SeverityError,
				Field:    field,
				Message:  "ratio must be in [0.0, 1.0]",
				Hint:     suggestFor(field, f.v, nil),
			})
		}
	}
	if inUnitInterval(notice) && inUnitInterval(warn) && inUnitInterval(critical) &&
		!(notice < warn && warn < critical) {
		errs = append(errs, Error{
			Severity: SeverityError,
			Field:    prefix + "_critical_ratio",
			Message: fmt.Sprintf(
				"ratios must strictly increase: notice (%.2f) < warn (%.2f) < critical (%.2f)",
				notice, warn, critical),
		})
	}
	return errs
}

// fixRatioTrio clamps each out-of-range ratio to its default and, when the trio
// is not strictly increasing, resets all three to defaults (a known-good
// monotonic set). Mutates through the pointers; appends fixed field paths.
func fixRatioTrio(prefix string, notice, warn, critical *float64, defN, defW, defC float64, fixed []string) []string {
	if *notice < 0.0 || *notice > 1.0 {
		*notice = defN
		fixed = append(fixed, prefix+"_notice_ratio")
	}
	if *warn < 0.0 || *warn > 1.0 {
		*warn = defW
		fixed = append(fixed, prefix+"_warn_ratio")
	}
	if *critical < 0.0 || *critical > 1.0 {
		*critical = defC
		fixed = append(fixed, prefix+"_critical_ratio")
	}
	if !(*notice < *warn && *warn < *critical) {
		*notice, *warn, *critical = defN, defW, defC
		fixed = append(fixed, prefix+"_notice_ratio", prefix+"_warn_ratio", prefix+"_critical_ratio")
	}
	return fixed
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

	// Thresholds.CtxNoticeRatio.
	if cfg.Thresholds.CtxNoticeRatio < 0.0 || cfg.Thresholds.CtxNoticeRatio > 1.0 {
		cfg.Thresholds.CtxNoticeRatio = def.Thresholds.CtxNoticeRatio
		fixed = append(fixed, "thresholds.ctx_notice_ratio")
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
	}

	// Strict ordering notice < warn < critical. The three are interdependent, so
	// when (after the individual range clamps above) they are not strictly
	// increasing, reset the whole trio to defaults — a known-good monotonic set.
	if !(cfg.Thresholds.CtxNoticeRatio < cfg.Thresholds.CtxWarnRatio &&
		cfg.Thresholds.CtxWarnRatio < cfg.Thresholds.CtxCriticalRatio) {
		cfg.Thresholds.CtxNoticeRatio = def.Thresholds.CtxNoticeRatio
		cfg.Thresholds.CtxWarnRatio = def.Thresholds.CtxWarnRatio
		cfg.Thresholds.CtxCriticalRatio = def.Thresholds.CtxCriticalRatio
		fixed = append(fixed, "thresholds.ctx_notice_ratio",
			"thresholds.ctx_warn_ratio", "thresholds.ctx_critical_ratio")
	}

	// Thresholds.Quota5h / Quota7d colour-ratio trios.
	fixed = fixRatioTrio("thresholds.quota_5h",
		&cfg.Thresholds.Quota5hNoticeRatio, &cfg.Thresholds.Quota5hWarnRatio, &cfg.Thresholds.Quota5hCriticalRatio,
		def.Thresholds.Quota5hNoticeRatio, def.Thresholds.Quota5hWarnRatio, def.Thresholds.Quota5hCriticalRatio, fixed)
	fixed = fixRatioTrio("thresholds.quota_7d",
		&cfg.Thresholds.Quota7dNoticeRatio, &cfg.Thresholds.Quota7dWarnRatio, &cfg.Thresholds.Quota7dCriticalRatio,
		def.Thresholds.Quota7dNoticeRatio, def.Thresholds.Quota7dWarnRatio, def.Thresholds.Quota7dCriticalRatio, fixed)

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
