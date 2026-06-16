package config

import (
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// Built-in colour palettes. Kept as package-level vars so they are easy to
// read and modify without touching renderer/theme.go.
//
// high-contrast: saturated true-colour ANSI equivalents for maximum readability.
var highContrastPalette = renderer.ColorScheme{
	Cyan:    "#00FFFF",
	Yellow:  "#FFFF00",
	Red:     "#FF0000",
	Green:   "#00FF00",
	Orange:  "#FF8800",
	Magenta: "#FF00FF",
	Dim:     "#888888",
}

// minimal: monochrome — semantic colours are suppressed (empty strings) so
// output is plain-text; only Dim retains a muted value for separators.
var minimalPalette = renderer.ColorScheme{
	Dim: "#666666",
	// All other fields remain empty strings (no colour output).
}

// ToProbesConfig maps the high-level Config into the per-probe runtime config
// consumed by Probe.Visible / Probe.Render. Pure function; no errors.
func ToProbesConfig(cfg Config) probes.Config {
	return probes.Config{
		ModelEnabled:   cfg.Widgets.Model,
		EffortEnabled:  cfg.Widgets.Effort,
		CostEnabled:    cfg.Widgets.Cost,
		ProjectEnabled: cfg.Widgets.Project,
		EmailEnabled:   cfg.Widgets.Email,
		TimeEnabled:    cfg.Widgets.Time,
		CtxEnabled:     cfg.Widgets.Ctx,
		// CacheEnabled and SubagentEnabled are hardcoded true: the widget toggle
		// fields were removed from Widgets (dead config) but probes still read
		// these flags internally (cache.go:46, subagent.go:34). Hardcoding true
		// keeps probe visibility unchanged without cascading deletes into probes/.
		CacheEnabled:    true,
		QuotaEnabled:    cfg.Widgets.Quota,
		GitEnabled:      cfg.Widgets.Git,
		SubagentEnabled: true,

		TableRows: cfg.General.TableRows,

		Email: cfg.Probes.Email.Address,

		CostBudgetUSD:      cfg.Thresholds.CostBudgetUSD,
		CtxNoticeRatio:     cfg.Thresholds.CtxNoticeRatio,
		CtxWarnRatio:       cfg.Thresholds.CtxWarnRatio,
		CtxCriticalRatio:   cfg.Thresholds.CtxCriticalRatio,
		OrchTTLMinutes:     cfg.Thresholds.OrchTTLMinutes,
		SubagentGapMinutes: cfg.Thresholds.SubagentGapMinutes,
	}
}

// ToTheme overlays cfg.Theme overrides on top of base. Pure function; no errors.
//
//  1. If cfg.Theme.Name is "high-contrast" or "minimal", the corresponding
//     built-in palette replaces the colour base. Any other name (including
//     "default" and unknown names) leaves base.Colors unchanged — the adapter
//     is pure and delegates validation to the separate validator.
//  2. For each non-empty hex string in cfg.Theme.Colors, the matching semantic
//     colour in the result is overridden. Empty strings keep the palette value.
//  3. base.AnsiEnabled and base.NerdFont are always preserved; they are set by
//     the caller from ENV and cfg.General before this call (see concept §7.1).
func ToTheme(cfg Config, base renderer.Theme) renderer.Theme {
	result := base

	// Step 1: palette switch.
	switch cfg.Theme.Name {
	case "high-contrast":
		result.Colors = highContrastPalette
	case "minimal":
		result.Colors = minimalPalette
	default:
		// "default" and unknown names: keep base.Colors (no palette override).
		// result.Colors is already a copy of base.Colors from `result := base`.
	}

	// Step 2: per-field hex overlays. Non-empty strings override the palette.
	ov := cfg.Theme.Colors
	if ov.Cyan != "" {
		result.Colors.Cyan = ov.Cyan
	}
	if ov.Yellow != "" {
		result.Colors.Yellow = ov.Yellow
	}
	if ov.Red != "" {
		result.Colors.Red = ov.Red
	}
	if ov.Green != "" {
		result.Colors.Green = ov.Green
	}
	if ov.Orange != "" {
		result.Colors.Orange = ov.Orange
	}
	if ov.Magenta != "" {
		result.Colors.Magenta = ov.Magenta
	}
	if ov.Dim != "" {
		result.Colors.Dim = ov.Dim
	}

	// Step 3: AnsiEnabled / NerdFont already preserved (copied via result := base).
	return result
}
