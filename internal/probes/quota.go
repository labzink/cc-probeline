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

	// Resolve reset times from the snapshot (single consistent source).
	// snap.FiveHourReset / snap.SevenDayReset are populated by quota.Update and
	// represent the same observation as pct5h/pct7d. Using RateLimits for reset
	// while using the snapshot for pct causes desync after a window reset (F6).
	// Guard: value == 0 means unknown → formatResetFromUnix returns "".
	var reset5h, reset7d string
	if hasFresh {
		reset5h = formatResetFromUnix(snap.FiveHourReset, d.Now, fiveHourThresholds)
		reset7d = formatResetFromUnix(snap.SevenDayReset, d.Now, sevenDayThresholds)
	} else if rl != nil {
		// Fallback: no snapshot — use session-local payload reset times.
		reset5h = formatResetColoured(rl.FiveHour.ResetsAt, d.Now, t.AnsiEnabled, fiveHourThresholds)
		reset7d = formatResetColoured(rl.SevenDay.ResetsAt, d.Now, t.AnsiEnabled, sevenDayThresholds)
	}

	pct5hInt := int(pct5h)
	pct7dInt := int(pct7d)

	// pctSuffix returns " NN%" coloured with ProgressBarColor when pct ≥ 90.
	// At pct > 95 the existing boldRedWrap handles the bold-red colouring of
	// the entire block; the suffix itself uses the bar-colour marker.
	// Spec §2.3: colour = ProgressBarColor(pct,t); >95 → bold_red.
	pctSuffix := func(pct float64) string {
		if pct < 90.0 {
			return ""
		}
		pctStr := fmt.Sprintf("%d%%", int(pct))
		if pct > 95.0 {
			return " {{color:bold_red}}" + pctStr + "{{reset}}"
		}
		// ≥ 90 and ≤ 95: colour matches the progress bar colour (always red at ≥90%).
		color := renderer.ProgressBarColor(pct, t)
		if color != "" {
			// ProgressBarColor returned a raw ANSI code; wrap in text marker instead.
			// At pct ≥ 90, ProgressBarColor returns Red. Use the text marker form.
			return " {{color:red}}" + pctStr + "{{reset}}"
		}
		// AnsiEnabled=false: emit plain suffix without colour markers.
		return " " + pctStr
	}

	switch level {
	case LevelFull:
		bar5h := renderer.ProgressBarColor(pct5h, t) +
			renderer.ProgressBar10(pct5h) + colourReset
		bar7d := renderer.ProgressBarColor(pct7d, t) +
			renderer.ProgressBar10(pct7d) + colourReset
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%s%s %s", bar5h, pctSuffix(pct5h), reset5h))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%s%s %s", bar7d, pctSuffix(pct7d), reset7d))
		return fmt.Sprintf("5h: %s · 7d: %s%s", val5h, val7d, ageSuffix)
	case LevelCompact:
		bar5h := renderer.ProgressBarColor(pct5h, t) +
			renderer.ProgressBar(pct5h) + colourReset
		bar7d := renderer.ProgressBarColor(pct7d, t) +
			renderer.ProgressBar(pct7d) + colourReset
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%s%s %s", bar5h, pctSuffix(pct5h), reset5h))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%s%s %s", bar7d, pctSuffix(pct7d), reset7d))
		return fmt.Sprintf("%s · %s%s", val5h, val7d, ageSuffix)
	default: // LevelMinimal
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%d%%", pct5hInt))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%d%%", pct7dInt))
		return fmt.Sprintf("%s · %s%s", val5h, val7d, ageSuffix)
	}
}

// resetThresholds defines the gradient colour thresholds for a reset countdown.
// Durations are checked in order; the first matching threshold wins.
// When no threshold matches (remaining > all limits) no marker is applied.
type resetThresholds struct {
	red    time.Duration // ≤ this → red
	orange time.Duration // ≤ this → orange
	green  time.Duration // ≤ this → green
	// > green → no marker
}

// fiveHourThresholds holds the 5-hour reset colour thresholds (spec §2.3):
//
//	≤ 10m → red
//	≤ 30m → orange
//	≤ 60m → green
//	> 60m → no marker
var fiveHourThresholds = resetThresholds{
	red:    10 * time.Minute,
	orange: 30 * time.Minute,
	green:  60 * time.Minute,
}

// sevenDayThresholds holds the 7-day reset colour thresholds (spec §2.3):
//
//	≤ 5h  → red
//	≤ 24h → orange
//	≤ 2d  → green
//	> 2d  → no marker
var sevenDayThresholds = resetThresholds{
	red:    5 * time.Hour,
	orange: 24 * time.Hour,
	green:  48 * time.Hour,
}

// resetColourMarker returns the {{color:X}} marker for the given remaining duration
// using the supplied threshold table. Returns "" when no marker applies.
// Markers are always returned as text tokens; Apply converts them to ANSI.
func resetColourMarker(remaining time.Duration, th resetThresholds) string {
	switch {
	case remaining <= th.red:
		return "{{color:red}}"
	case remaining <= th.orange:
		return "{{color:orange}}"
	case remaining <= th.green:
		return "{{color:green}}"
	default:
		return ""
	}
}

// formatResetColoured converts a raw resets_at value into a reset-countdown
// string and applies a gradient colour marker based on the remaining time.
// The colour thresholds are supplied by the caller (5h vs 7d differ).
// Markers are always emitted as {{color:X}}…{{reset}} text tokens regardless
// of ansiEnabled; Apply()/renderer strip them when colour is off.
func formatResetColoured(raw []byte, now time.Time, _ bool, thresholds resetThresholds) string {
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
	marker := resetColourMarker(dur, thresholds)
	if marker == "" {
		return text
	}
	return marker + text + "{{reset}}"
}

// formatResetFromUnix converts a unix-second timestamp to a reset-countdown
// string with gradient colour markers. It reads from the snapshot directly
// (not from a raw JSON field) so pct and reset-time always share the same
// source (F6 fix). Returns "" when unixSec == 0 (unknown).
func formatResetFromUnix(unixSec int64, now time.Time, thresholds resetThresholds) string {
	if unixSec == 0 {
		slog.Debug("quota.formatResetFromUnix: reset unix == 0; omitting reset label")
		return ""
	}
	t := time.Unix(unixSec, 0)
	dur := t.Sub(now)
	if dur <= 0 {
		return "↻ 0m"
	}
	text := formatDuration(dur)
	marker := resetColourMarker(dur, thresholds)
	if marker == "" {
		return text
	}
	return marker + text + "{{reset}}"
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
