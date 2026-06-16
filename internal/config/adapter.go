package config

import (
	"github.com/labzink/cc-probeline/internal/probes"
)

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

		CostBudgetUSD:        cfg.Thresholds.CostBudgetUSD,
		CtxNoticeRatio:       cfg.Thresholds.CtxNoticeRatio,
		CtxWarnRatio:         cfg.Thresholds.CtxWarnRatio,
		CtxCriticalRatio:     cfg.Thresholds.CtxCriticalRatio,
		Quota5hNoticeRatio:   cfg.Thresholds.Quota5hNoticeRatio,
		Quota5hWarnRatio:     cfg.Thresholds.Quota5hWarnRatio,
		Quota5hCriticalRatio: cfg.Thresholds.Quota5hCriticalRatio,
		Quota7dNoticeRatio:   cfg.Thresholds.Quota7dNoticeRatio,
		Quota7dWarnRatio:     cfg.Thresholds.Quota7dWarnRatio,
		Quota7dCriticalRatio: cfg.Thresholds.Quota7dCriticalRatio,
		OrchTTLMinutes:       cfg.Thresholds.OrchTTLMinutes,
		SubagentGapMinutes:   cfg.Thresholds.SubagentGapMinutes,
	}
}
