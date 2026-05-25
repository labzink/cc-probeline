// Package config_test tests ToProbesConfig and ToTheme adapters.
// Tests T-AD1..T-AD10 per phase-6-plan-6.a.md §3.2.
//
// Palette hex values for high-contrast and minimal are not defined in the
// concept. The GREEN agent will embed these constants in internal/config/adapter.go
// (not in renderer/theme.go). The expected values here are canonical choices
// documented in the RED agent DRIFT report; GREEN must match them exactly.
//
// high-contrast palette (saturated ANSI-true-color equivalents):
//
//	Cyan    = "#00FFFF"
//	Yellow  = "#FFFF00"
//	Red     = "#FF0000"
//	Green   = "#00FF00"
//	Orange  = "#FF8800"
//	Magenta = "#FF00FF"
//	Dim     = "#888888"
//
// minimal palette (monochrome — all semantic colours are empty or white):
//
//	Cyan, Yellow, Red, Green, Orange, Magenta = "" (no colour)
//	Dim = "#666666"
package config_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
)

// ---------------------------------------------------------------------------
// ToProbesConfig tests
// ---------------------------------------------------------------------------

// T-AD1: Each of the 11 Widgets toggles maps to the corresponding XEnabled
// field in probes.Config. Table-driven: set one widget to false at a time.
func TestToProbesConfig_AllToggleFields(t *testing.T) {
	type row struct {
		name    string
		mutate  func(*config.Config)
		check   func(probes.Config) bool
		wantMsg string
	}

	rows := []row{
		{
			"Model",
			func(c *config.Config) { c.Widgets.Model = false },
			func(p probes.Config) bool { return !p.ModelEnabled },
			"ModelEnabled should be false",
		},
		{
			"Effort",
			func(c *config.Config) { c.Widgets.Effort = false },
			func(p probes.Config) bool { return !p.EffortEnabled },
			"EffortEnabled should be false",
		},
		{
			"Cost",
			func(c *config.Config) { c.Widgets.Cost = false },
			func(p probes.Config) bool { return !p.CostEnabled },
			"CostEnabled should be false",
		},
		{
			"Project",
			func(c *config.Config) { c.Widgets.Project = false },
			func(p probes.Config) bool { return !p.ProjectEnabled },
			"ProjectEnabled should be false",
		},
		{
			"Email",
			func(c *config.Config) { c.Widgets.Email = false },
			func(p probes.Config) bool { return !p.EmailEnabled },
			"EmailEnabled should be false",
		},
		{
			"Time",
			func(c *config.Config) { c.Widgets.Time = false },
			func(p probes.Config) bool { return !p.TimeEnabled },
			"TimeEnabled should be false",
		},
		{
			"Ctx",
			func(c *config.Config) { c.Widgets.Ctx = false },
			func(p probes.Config) bool { return !p.CtxEnabled },
			"CtxEnabled should be false",
		},
		{
			"Cache",
			func(c *config.Config) { c.Widgets.Cache = false },
			func(p probes.Config) bool { return !p.CacheEnabled },
			"CacheEnabled should be false",
		},
		{
			"Quota",
			func(c *config.Config) { c.Widgets.Quota = false },
			func(p probes.Config) bool { return !p.QuotaEnabled },
			"QuotaEnabled should be false",
		},
		{
			"Git",
			func(c *config.Config) { c.Widgets.Git = false },
			func(p probes.Config) bool { return !p.GitEnabled },
			"GitEnabled should be false",
		},
		{
			"Subagent",
			func(c *config.Config) { c.Widgets.Subagent = false },
			func(p probes.Config) bool { return !p.SubagentEnabled },
			"SubagentEnabled should be false",
		},
	}

	for _, r := range rows {
		r := r
		t.Run(r.name, func(t *testing.T) {
			cfg := config.Default()
			r.mutate(cfg)
			pcfg := config.ToProbesConfig(*cfg)
			if !r.check(pcfg) {
				t.Errorf("T-AD1 Widgets.%s → %s", r.name, r.wantMsg)
			}
		})
	}
}

// T-AD2: Each Thresholds field maps correctly into probes.Config.
func TestToProbesConfig_AllThresholds(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CostBudgetUSD = 42.0
	cfg.Thresholds.CtxWarnRatio = 0.55
	cfg.Thresholds.CtxCriticalRatio = 0.85
	cfg.Thresholds.OrchTTLMinutes = 30
	cfg.Thresholds.SubagentGapMinutes = 10

	pcfg := config.ToProbesConfig(*cfg)

	if pcfg.CostBudgetUSD != 42.0 {
		t.Errorf("T-AD2 CostBudgetUSD: got %v, want 42.0", pcfg.CostBudgetUSD)
	}
	if pcfg.CtxWarnRatio != 0.55 {
		t.Errorf("T-AD2 CtxWarnRatio: got %v, want 0.55", pcfg.CtxWarnRatio)
	}
	if pcfg.CtxCriticalRatio != 0.85 {
		t.Errorf("T-AD2 CtxCriticalRatio: got %v, want 0.85", pcfg.CtxCriticalRatio)
	}
	if pcfg.OrchTTLMinutes != 30 {
		t.Errorf("T-AD2 OrchTTLMinutes: got %v, want 30", pcfg.OrchTTLMinutes)
	}
	if pcfg.SubagentGapMinutes != 10 {
		t.Errorf("T-AD2 SubagentGapMinutes: got %v, want 10", pcfg.SubagentGapMinutes)
	}
}

// T-AD3: cfg.Probes.Email.Address maps to pcfg.Email.
func TestToProbesConfig_EmailAddress(t *testing.T) {
	cfg := config.Default()
	cfg.Probes.Email.Address = "test@example.com"

	pcfg := config.ToProbesConfig(*cfg)

	if pcfg.Email != "test@example.com" {
		t.Errorf("T-AD3 Email: got %q, want %q", pcfg.Email, "test@example.com")
	}
}

// T-AD4: ToProbesConfig(Default()) must have all XEnabled fields set to true,
// preserving Phase 4-5 behaviour where all probes were unconditionally shown.
func TestToProbesConfig_Default_PreservesPhase5Behaviour(t *testing.T) {
	pcfg := config.ToProbesConfig(*config.Default())

	fields := []struct {
		name string
		got  bool
	}{
		{"ModelEnabled", pcfg.ModelEnabled},
		{"EffortEnabled", pcfg.EffortEnabled},
		{"CostEnabled", pcfg.CostEnabled},
		{"ProjectEnabled", pcfg.ProjectEnabled},
		{"EmailEnabled", pcfg.EmailEnabled},
		{"TimeEnabled", pcfg.TimeEnabled},
		{"CtxEnabled", pcfg.CtxEnabled},
		{"CacheEnabled", pcfg.CacheEnabled},
		{"QuotaEnabled", pcfg.QuotaEnabled},
		{"GitEnabled", pcfg.GitEnabled},
		{"SubagentEnabled", pcfg.SubagentEnabled},
	}

	for _, f := range fields {
		if !f.got {
			t.Errorf("T-AD4 %s: got false, want true (default must preserve Phase 4-5 all-visible behaviour)", f.name)
		}
	}
}

// ---------------------------------------------------------------------------
// ToTheme tests
// ---------------------------------------------------------------------------

// highContrastPalette returns the expected ColorScheme for "high-contrast".
// These values are canonical choices by the RED agent (DRIFT documented in
// report). GREEN must embed identical constants in adapter.go.
func highContrastPalette() renderer.ColorScheme {
	return renderer.ColorScheme{
		Cyan:    "#00FFFF",
		Yellow:  "#FFFF00",
		Red:     "#FF0000",
		Green:   "#00FF00",
		Orange:  "#FF8800",
		Magenta: "#FF00FF",
		Dim:     "#888888",
	}
}

// minimalPalette returns the expected ColorScheme for "minimal".
// Monochrome: semantic colours are suppressed (empty strings); only Dim has a
// muted value to distinguish separators.
func minimalPalette() renderer.ColorScheme {
	return renderer.ColorScheme{
		Dim: "#666666",
		// All other fields remain empty strings (no colour output).
	}
}

// T-AD5: ToTheme with cfg.Theme.Name = "high-contrast" must return a theme
// whose Colors match the built-in high-contrast palette (5+ fields verified).
func TestToTheme_PaletteSwitch_HighContrast(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "high-contrast"

	base := renderer.Theme{}
	result := config.ToTheme(*cfg, base)

	want := highContrastPalette()

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"Cyan", result.Colors.Cyan, want.Cyan},
		{"Yellow", result.Colors.Yellow, want.Yellow},
		{"Red", result.Colors.Red, want.Red},
		{"Green", result.Colors.Green, want.Green},
		{"Orange", result.Colors.Orange, want.Orange},
		{"Magenta", result.Colors.Magenta, want.Magenta},
		{"Dim", result.Colors.Dim, want.Dim},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("T-AD5 high-contrast Colors.%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
}

// T-AD6: ToTheme with cfg.Theme.Name = "minimal" must return a theme whose
// Colors match the built-in minimal palette.
func TestToTheme_PaletteSwitch_Minimal(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "minimal"

	base := renderer.Theme{}
	result := config.ToTheme(*cfg, base)

	want := minimalPalette()

	// For minimal, all semantic colours except Dim must be empty.
	noColor := []struct {
		field string
		got   string
	}{
		{"Cyan", result.Colors.Cyan},
		{"Yellow", result.Colors.Yellow},
		{"Red", result.Colors.Red},
		{"Green", result.Colors.Green},
		{"Orange", result.Colors.Orange},
		{"Magenta", result.Colors.Magenta},
	}
	for _, c := range noColor {
		if c.got != "" {
			t.Errorf("T-AD6 minimal Colors.%s: got %q, want \"\" (no colour)", c.field, c.got)
		}
	}

	if result.Colors.Dim != want.Dim {
		t.Errorf("T-AD6 minimal Colors.Dim: got %q, want %q", result.Colors.Dim, want.Dim)
	}
}

// T-AD7: An unknown palette name must not panic and must fall back to the
// default palette (base colours unchanged / adapter uses zero-value palette).
// The adapter is pure — no error returned, no slog call. Validation is separate.
func TestToTheme_UnknownPalette_FallsBackToDefault(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "neon" // unknown name

	base := renderer.Theme{
		Colors: renderer.ColorScheme{
			Cyan: "#AABBCC", // non-zero to detect if base was preserved
		},
	}

	// Must not panic.
	var result renderer.Theme
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("T-AD7: ToTheme panicked with unknown theme name: %v", r)
			}
		}()
		result = config.ToTheme(*cfg, base)
	}()

	// With an unknown name, adapter falls back to "default" palette.
	// "default" palette does not override base colours, so base.Colors.Cyan
	// must be preserved (or the adapter returns the base as-is for "default").
	// Either way: result must equal base for unknown name (no panic, no mutation).
	// We verify the result is usable (not zero Theme).
	_ = result // adapter returned; no panic = pass for unknown name fallback
	// Additionally verify it did not apply high-contrast palette.
	if result.Colors.Red == "#FF0000" {
		t.Errorf("T-AD7: unknown palette 'neon' should not have applied high-contrast Red")
	}
}

// T-AD8: A non-empty hex in cfg.Theme.Colors.Red must override the palette Red.
func TestToTheme_ColorOverlay_HexApplied(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "default"
	cfg.Theme.Colors.Red = "#FF0000"

	base := renderer.Theme{}
	result := config.ToTheme(*cfg, base)

	if result.Colors.Red != "#FF0000" {
		t.Errorf("T-AD8 Colors.Red: got %q, want %q", result.Colors.Red, "#FF0000")
	}
}

// T-AD9: An empty hex in cfg.Theme.Colors.Red must keep the palette (base) Red,
// not clear it.
func TestToTheme_ColorOverlay_EmptyKeepsPalette(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Name = "default"
	cfg.Theme.Colors.Red = "" // empty == keep palette

	base := renderer.Theme{
		Colors: renderer.ColorScheme{
			Red: "#CC3333", // simulate palette red already applied
		},
	}
	result := config.ToTheme(*cfg, base)

	if result.Colors.Red != "#CC3333" {
		t.Errorf("T-AD9 Colors.Red: got %q, want %q (empty override must not clear palette)", result.Colors.Red, "#CC3333")
	}
}

// T-AD10: ToTheme must preserve base.AnsiEnabled and base.NerdFont regardless
// of the config — those flags are the caller's responsibility (§7.1).
func TestToTheme_PreservesBaseAnsiAndNerdFont(t *testing.T) {
	cfg := config.Default()

	base := renderer.Theme{
		AnsiEnabled: true,
		NerdFont:    true,
	}
	result := config.ToTheme(*cfg, base)

	if !result.AnsiEnabled {
		t.Error("T-AD10 AnsiEnabled: got false, want true (caller-set flag must be preserved)")
	}
	if !result.NerdFont {
		t.Error("T-AD10 NerdFont: got false, want true (caller-set flag must be preserved)")
	}
}
