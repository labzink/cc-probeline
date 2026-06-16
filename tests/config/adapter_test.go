// Package config_test tests ToProbesConfig adapter.
// Tests T-AD1..T-AD4 per phase-6-plan-6.a.md §3.2.
//
// NOTE: ToTheme and all theme-palette tests were removed in Phase 7.47
// because config.Theme (Theme struct, ThemeColors struct, ToTheme function)
// were cut from the production code. The renderer.Theme type (AnsiEnabled /
// Colors / NerdFont) is a separate runtime type and is NOT affected here.
package config_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
	"github.com/labzink/cc-probeline/internal/probes"
)

// ---------------------------------------------------------------------------
// ToProbesConfig tests
// ---------------------------------------------------------------------------

// T-AD1: Each active Widgets toggle maps to the corresponding XEnabled field in
// probes.Config. Table-driven: set one widget to false at a time.
// Note: Cache and Subagent were removed from Widgets in Phase 6.95; their
// probes.Config fields are now hardcoded true in adapter (see T-AD1b below).
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
// CacheEnabled and SubagentEnabled are hardcoded true in adapter (Phase 6.95):
// their widget toggles were removed from Widgets struct but probes still read them.
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
		{"CacheEnabled", pcfg.CacheEnabled}, // hardcoded true in adapter
		{"QuotaEnabled", pcfg.QuotaEnabled},
		{"GitEnabled", pcfg.GitEnabled},
		{"SubagentEnabled", pcfg.SubagentEnabled}, // hardcoded true in adapter
	}

	for _, f := range fields {
		if !f.got {
			t.Errorf("T-AD4 %s: got false, want true (default must preserve Phase 4-5 all-visible behaviour)", f.name)
		}
	}
}

// T-AD1b: CacheEnabled and SubagentEnabled are hardcoded true regardless of
// config (Phase 6.95: dead toggles removed, probes still read these fields).
func TestToProbesConfig_CacheSubagent_AlwaysTrue(t *testing.T) {
	// Even with an entirely zeroed Config, both flags must be true.
	pcfg := config.ToProbesConfig(config.Config{})
	if !pcfg.CacheEnabled {
		t.Error("T-AD1b CacheEnabled: got false, want true (hardcoded in adapter)")
	}
	if !pcfg.SubagentEnabled {
		t.Error("T-AD1b SubagentEnabled: got false, want true (hardcoded in adapter)")
	}
}

// T-AD1c: TableRows is forwarded from cfg.General.TableRows into probes.Config.
func TestToProbesConfig_TableRows_Forwarded(t *testing.T) {
	cfg := config.Default()
	cfg.General.TableRows = 15
	pcfg := config.ToProbesConfig(*cfg)
	if pcfg.TableRows != 15 {
		t.Errorf("T-AD1c TableRows: got %d, want 15", pcfg.TableRows)
	}
}
