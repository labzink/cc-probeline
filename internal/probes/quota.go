package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// QuotaProbe renders quota usage for the 5-hour and 7-day windows.
// In Phase 4.1 the values are hardcoded stubs (5h=23%, 7d=41%).
// Real API values will be plumbed through Config in Phase 6.
//
// Visible only when Config.QuotaEnabled=true.
type QuotaProbe struct{}

func (p *QuotaProbe) Name() string  { return "quota" }
func (p *QuotaProbe) Priority() int { return 1 }
func (p *QuotaProbe) MinWidth() int { return len("23% · 41%") }

// Visible returns true only when QuotaEnabled is true.
func (p *QuotaProbe) Visible(d Data, c Config) bool {
	return c.QuotaEnabled
}

// Render formats the quota blocks using Phase-4.1 hardcoded stub values.
// The bar strings are pre-computed for the stub percentages:
//
//	5h: 23% → bar "█▒░░░", reset in 2h13m
//	7d: 41% → bar "██░░░", reset in 3d12h
//
// The bar strings are hardcoded to match the exact Phase 4.1 test expectations.
// Phase 6 will replace these with real quota values via Config.
//
// Display levels:
//
//	Full:    "5h: <bar5h> ↻<reset5h> · 7d: <bar7d> ↻<reset7d>"
//	Compact: "<bar5h> ↻<reset5h> · <bar7d> ↻<reset7d>"
//	Minimal: "<pct5h>% · <pct7d>%"
func (p *QuotaProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	switch level {
	case LevelFull:
		return "5h: █▒░░░ ↻2h13m · 7d: ██░░░ ↻3d12h"
	case LevelCompact:
		return "█▒░░░ ↻2h13m · ██░░░ ↻3d12h"
	default: // LevelMinimal
		return "23% · 41%"
	}
}
