package probes

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/labzink/cc-probeline/internal/quota"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// staleDuration is the threshold beyond which a quota snapshot is considered
// stale. When the snapshot age exceeds this, an "as of Xm ago" suffix is added.
const staleDuration = 10 * time.Minute

// QuotaProbe renders quota usage for the 5-hour and 7-day rate-limit windows.
// Data is sourced from quota.Freshest() (cross-session persistent file) with
// d.Stdin.RateLimits as a fallback when no snapshot has been stored yet.
//
// Visible only when Config.QuotaEnabled=true AND either quota.Freshest() returns
// a snapshot OR d.Stdin.RateLimits != nil.
type QuotaProbe struct{}

func (p *QuotaProbe) Name() string  { return "quota" }
func (p *QuotaProbe) Priority() int { return 1 }
func (p *QuotaProbe) MinWidth() int { return len("0% · 0%") }

// Visible returns true only when QuotaEnabled is true and quota data is available
// (either from the persistent Freshest snapshot or from the current payload).
func (p *QuotaProbe) Visible(d Data, c Config) bool {
	if !c.QuotaEnabled {
		return false
	}
	_, hasFresh := quota.Freshest()
	return hasFresh || d.Stdin.RateLimits != nil
}

// Render formats the quota blocks.
//
// Data source priority:
//  1. quota.Freshest() — cross-session persistent snapshot (account-wide freshest).
//  2. d.Stdin.RateLimits — current payload fallback when no snapshot exists.
//
// When the snapshot is older than staleDuration (10m), an "as of Xm ago" suffix
// is appended to signal freshness decay.
//
// Colour markers applied per B3 §5:
//   - progress bars wrapped in ProgressBarColor(pct, th) + bar + Reset
//   - ↻ reset countdown wrapped in {{color:yellow}} when time-to-reset < 30m
//   - percentage value wrapped in {{color:bold_red}} when pct > 95
//
// Display levels:
//
//	Full:    "5h: <bar10_5h> <reset5h> · 7d: <bar10_7d> <reset7d>" [+ age suffix]
//	Compact: "<bar5_5h> <reset5h> · <bar5_7d> <reset7d>" [+ age suffix]
//	Minimal: "<pct5h>% · <pct7d>%" [+ age suffix]
func (p *QuotaProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	// Resolve data source: freshest-wins cross-session snapshot takes priority.
	snap, hasFresh := quota.Freshest()

	var pct5h, pct7d float64
	var rl *stdin.RateLimits
	var ageSuffix string

	if hasFresh {
		pct5h = snap.FiveHourPct
		pct7d = snap.SevenDayPct
		// Compute age suffix when snapshot is stale.
		snapTime := time.UnixMilli(snap.TS)
		age := d.Now.Sub(snapTime)
		if age > staleDuration {
			mins := int(age.Minutes())
			ageSuffix = fmt.Sprintf(" (as of %dm ago)", mins)
		}
		// Use current payload rate_limits for reset-countdown times (they are
		// session-local and not stored in the snapshot).
		rl = d.Stdin.RateLimits
	} else if d.Stdin.RateLimits != nil {
		// Fallback to current payload when no persistent snapshot exists.
		rl = d.Stdin.RateLimits
		pct5h = rl.FiveHour.UsedPercentage
		pct7d = rl.SevenDay.UsedPercentage
	} else {
		slog.Warn("quota.Render: called with no snapshot and nil RateLimits; returning empty")
		return ""
	}

	// colourReset returns the reset escape only when AnsiEnabled.
	colourReset := ""
	if t.AnsiEnabled {
		colourReset = t.Colors.Reset
	}

	// boldRedWrap wraps a value string with {{color:bold_red}}...{{reset}} when
	// the percentage exceeds 95 and ANSI output is enabled. Markers are gated
	// on AnsiEnabled so that plain-text callers never receive raw markers.
	boldRedWrap := func(pct float64, s string) string {
		if t.AnsiEnabled && pct > 95.0 {
			return "{{color:bold_red}}" + s + "{{reset}}"
		}
		return s
	}

	var reset5h, reset7d string
	if rl != nil {
		reset5h = formatResetColoured(rl.FiveHour.ResetsAt, d.Now, t.AnsiEnabled)
		reset7d = formatResetColoured(rl.SevenDay.ResetsAt, d.Now, t.AnsiEnabled)
	}

	pct5hInt := int(pct5h)
	pct7dInt := int(pct7d)

	switch level {
	case LevelFull:
		bar5h := renderer.ProgressBarColor(pct5h, t) +
			renderer.ProgressBar10(pct5h) + colourReset
		bar7d := renderer.ProgressBarColor(pct7d, t) +
			renderer.ProgressBar10(pct7d) + colourReset
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%s %s", bar5h, reset5h))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%s %s", bar7d, reset7d))
		return fmt.Sprintf("5h: %s · 7d: %s%s", val5h, val7d, ageSuffix)
	case LevelCompact:
		bar5h := renderer.ProgressBarColor(pct5h, t) +
			renderer.ProgressBar(pct5h) + colourReset
		bar7d := renderer.ProgressBarColor(pct7d, t) +
			renderer.ProgressBar(pct7d) + colourReset
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%s %s", bar5h, reset5h))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%s %s", bar7d, reset7d))
		return fmt.Sprintf("%s · %s%s", val5h, val7d, ageSuffix)
	default: // LevelMinimal
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%d%%", pct5hInt))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%d%%", pct7dInt))
		return fmt.Sprintf("%s · %s%s", val5h, val7d, ageSuffix)
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

// formatResetColoured is like formatReset but wraps the result in
// {{color:yellow}}…{{reset}} when the time-to-reset is less than 30 minutes
// and ansiEnabled is true.
func formatResetColoured(raw []byte, now time.Time, ansiEnabled bool) string {
	t, ok := stdin.ParseResetsAt(raw)
	if !ok {
		slog.Debug("quota.formatResetColoured: could not parse resets_at; omitting reset label")
		return "↻ 0m"
	}
	dur := t.Sub(now)
	if dur <= 0 {
		return "↻ 0m"
	}
	text := formatDuration(dur)
	// Wrap in yellow marker when reset is imminent (< 30 minutes) and colour is on.
	if ansiEnabled && dur < 30*time.Minute {
		return "{{color:yellow}}" + text + "{{reset}}"
	}
	return text
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
