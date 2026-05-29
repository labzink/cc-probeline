package probes

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// QuotaProbe renders quota usage for the 5-hour and 7-day rate-limit windows.
// Data is sourced from d.Stdin.RateLimits which is populated by the Claude Code
// statusLine hook payload. When rate_limits is absent the probe is hidden.
//
// Visible only when Config.QuotaEnabled=true AND d.Stdin.RateLimits != nil.
type QuotaProbe struct{}

func (p *QuotaProbe) Name() string  { return "quota" }
func (p *QuotaProbe) Priority() int { return 1 }
func (p *QuotaProbe) MinWidth() int { return len("0% · 0%") }

// Visible returns true only when QuotaEnabled is true and RateLimits data is
// available in the payload. Missing rate_limits → graceful hide (not a fake value).
func (p *QuotaProbe) Visible(d Data, c Config) bool {
	return c.QuotaEnabled && d.Stdin.RateLimits != nil
}

// Render formats the quota blocks using real RateLimits data from d.Stdin.
//
// Display levels:
//
//	Full:    "5h: <bar10_5h> <reset5h> · 7d: <bar10_7d> <reset7d>"
//	Compact: "<bar5_5h> <reset5h> · <bar5_7d> <reset7d>"
//	Minimal: "<pct5h>% · <pct7d>%"
func (p *QuotaProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	rl := d.Stdin.RateLimits
	if rl == nil {
		slog.Warn("quota.Render: called with nil RateLimits; returning empty")
		return ""
	}

	reset5h := formatReset(rl.FiveHour.ResetsAt, d.Now)
	reset7d := formatReset(rl.SevenDay.ResetsAt, d.Now)

	pct5h := int(rl.FiveHour.UsedPercentage)
	pct7d := int(rl.SevenDay.UsedPercentage)

	switch level {
	case LevelFull:
		bar5h := renderer.ProgressBar10(rl.FiveHour.UsedPercentage)
		bar7d := renderer.ProgressBar10(rl.SevenDay.UsedPercentage)
		return fmt.Sprintf("5h: %s %s · 7d: %s %s", bar5h, reset5h, bar7d, reset7d)
	case LevelCompact:
		bar5h := renderer.ProgressBar(rl.FiveHour.UsedPercentage)
		bar7d := renderer.ProgressBar(rl.SevenDay.UsedPercentage)
		return fmt.Sprintf("%s %s · %s %s", bar5h, reset5h, bar7d, reset7d)
	default: // LevelMinimal
		return fmt.Sprintf("%d%% · %d%%", pct5h, pct7d)
	}
}

// formatReset converts a raw resets_at JSON value and the current time into
// a reset-countdown string "↻ <h>h:<m>m" (<24h) or "↻ <d>d.<h>h" (≥24h).
// If parsing fails or the reset time is in the past, returns "↻ 0m".
func formatReset(raw []byte, now time.Time) string {
	t, ok := stdin.ParseResetsAt(raw)
	if !ok {
		slog.Debug("quota.formatReset: could not parse resets_at; omitting reset label")
		return "↻ 0m"
	}
	dur := t.Sub(now)
	if dur <= 0 {
		return "↻ 0m"
	}
	return formatDuration(dur)
}

// formatDuration renders a duration as "↻ <d>d.<h>h" when ≥24h,
// else "↻ <h>h:<m>m". Space after ↻, colon between h/m, dot between d/h.
func formatDuration(dur time.Duration) string {
	totalMin := int(dur.Minutes())
	totalHours := totalMin / 60
	mins := totalMin % 60

	if totalHours >= 24 {
		days := totalHours / 24
		hours := totalHours % 24
		return fmt.Sprintf("↻ %dd.%dh", days, hours)
	}
	return fmt.Sprintf("↻ %dh:%dm", totalHours, mins)
}
