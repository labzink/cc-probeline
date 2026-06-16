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

// Window lengths used to decide whether a persisted snapshot's used-percentage
// has rolled over. A snapshot is a point-in-time observation valid only until
// its window resets; past that moment the stored percentage is meaningless.
const (
	fiveHourWindow = 5 * time.Hour
	sevenDayWindow = 7 * 24 * time.Hour
)

// windowExpired reports whether a rate-limit window has rolled over since the
// snapshot was taken, in which case its stored used-percentage must read 0
// rather than the stale high value. Two independent signals:
//
//  1. reset known and in the past (reset>0 && now>=reset) — the window has
//     definitely reset.
//  2. reset unknown (reset==0, CC sent null) but the snapshot is older than the
//     window length — it must have rolled over at least once.
//
// A window whose reset is still in the future keeps its stored percentage.
func windowExpired(reset int64, snapTS time.Time, now time.Time, windowLen time.Duration) bool {
	if reset > 0 {
		return now.Unix() >= reset
	}
	return now.Sub(snapTS) > windowLen
}

// displayPctInt converts a used-percentage to the integer shown in the status
// line. By default it truncates (matching every other percentage display). When
// roundUp is true it rounds half-up, so the number flips to 100 at 99.5 instead
// of parking on 99 until a literal 100.0.
//
// Phase 7.45 B5: applied to the 5-hour window only. The 5h window is the one
// that throttles soonest, so a small nudge toward "100" right before the wall is
// worth the slight asymmetry; the 7-day window keeps truncation (its scale is
// long enough that an early 100 would mislead, not warn). Display-only: it never
// feeds the overage trigger or colour thresholds, which stay on the raw float —
// so the paid-overage badge cannot fire early off a rounded number.
func displayPctInt(pct float64, roundUp bool) int {
	if roundUp {
		return int(pct + 0.5) // pct is always ≥ 0, so +0.5+truncate == round-half-up
	}
	return int(pct)
}

// quotaUsageColor returns the raw ANSI colour code for a quota usage percentage
// using the window's configurable notice/warn/critical ratios: green below
// notice, yellow at notice, orange at warn, red at critical. It mirrors
// renderer.ProgressBarColor but with per-window thresholds; like that function it
// caps at Red — the >95% bold-red emphasis is applied separately by boldRedWrap.
// Returns "" when ANSI is disabled.
func quotaUsageColor(pct, notice, warn, critical float64, t renderer.Theme) string {
	if !t.AnsiEnabled {
		return ""
	}
	switch {
	case pct >= critical*100.0:
		return t.Colors.Red
	case pct >= warn*100.0:
		return t.Colors.Orange
	case pct >= notice*100.0:
		return t.Colors.Yellow
	default:
		return t.Colors.Green
	}
}

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
		// snapTime is the observation time (used below for window rollover).
		snapTime := time.UnixMilli(snap.TS)
		// Staleness age is measured from DataTS — the moment the numbers last
		// actually changed — not the write time, which the freshest-by-data
		// tie-break bumps to "now" every tick on unchanged data (Phase 7.45 B1).
		// Fall back to TS for snapshots written before DataTS existed.
		dataTime := snapTime
		if snap.DataTS != 0 {
			dataTime = time.UnixMilli(snap.DataTS)
		}
		age := d.Now.Sub(dataTime)
		if age > staleDuration {
			mins := int(age.Minutes())
			ageSuffix = fmt.Sprintf(" (as of %dm ago)", mins)
		}
		// A window that has rolled over since the snapshot was taken reads 0%:
		// the stored high percentage belongs to a window that has already reset.
		// This clears the stale-90% mirage seen after a laptop sleeps overnight
		// or right after /clear, before CC sends a fresh rate_limits payload.
		if windowExpired(snap.FiveHourReset, snapTime, d.Now, fiveHourWindow) {
			pct5h = 0
		}
		if windowExpired(snap.SevenDayReset, snapTime, d.Now, sevenDayWindow) {
			pct7d = 0
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

	// Per-window colour-flip ratios (notice/warn/critical), resolved with the
	// baked defaults 0.50/0.70/0.90 when unset or invalid.
	n5, w5, c5 := resolveRatios(c.Quota5hNoticeRatio, c.Quota5hWarnRatio, c.Quota5hCriticalRatio)
	n7, w7, c7 := resolveRatios(c.Quota7dNoticeRatio, c.Quota7dWarnRatio, c.Quota7dCriticalRatio)

	// boldRedWrap wraps a value string with {{color:bold_red}}...{{reset}} when
	// the percentage exceeds 95 and ANSI output is enabled. Markers are gated
	// on AnsiEnabled so that plain-text callers never receive raw markers.
	boldRedWrap := func(pct float64, s string) string {
		if t.AnsiEnabled && pct > 95.0 {
			return "{{color:bold_red}}" + s + "{{reset}}"
		}
		return s
	}

	// Reset countdown resolution: live stdin resets_at first, then the persisted
	// snapshot's reset unix (cross-session — lets an idle session show the countdown
	// once an active session has observed the new window), then "↻ ??m" when neither
	// is known (window just reset, next one not ticking yet).
	var live5h, live7d []byte
	if rl != nil {
		live5h = rl.FiveHour.ResetsAt
		live7d = rl.SevenDay.ResetsAt
	}
	var snap5hReset, snap7dReset int64
	if hasFresh {
		snap5hReset = snap.FiveHourReset
		snap7dReset = snap.SevenDayReset
	}
	reset5h := formatReset(live5h, snap5hReset, d.Now, fiveHourThresholds)
	reset7d := formatReset(live7d, snap7dReset, d.Now, sevenDayThresholds)

	// pctSuffix returns " NN%" for Full/Compact. The number appears once the
	// window reaches its critical ratio (and is below 100%); colour is red, or
	// bold_red above 95% (boldRedWrap also reddens the whole block there). Below
	// critical the number is omitted — the bar carries the signal. Phase 6.95.h:
	// at pct ≥ 100 the number is dropped and the extra-usage block stands in.
	pctSuffix := func(pct, critical float64, roundUp bool) string {
		if pct < critical*100.0 || pct >= 100.0 {
			return ""
		}
		pctStr := fmt.Sprintf("%d%%", displayPctInt(pct, roundUp))
		if !t.AnsiEnabled {
			// AnsiEnabled=false: emit plain suffix without colour markers.
			return " " + pctStr
		}
		if pct > 95.0 {
			return " {{color:bold_red}}" + pctStr + "{{reset}}"
		}
		return " {{color:red}}" + pctStr + "{{reset}}"
	}

	// Phase 6.95.h: decide which window (if any) carries the extra-usage block.
	// Active only when main armed the badge (ExtraActive) and the overage rounds
	// to at least one cent. The badge attaches to a window currently at ≥100%; if
	// both are maxed it goes to the one that resets later (holds overage longest).
	extraOn5h, extraOn7d := false, false
	if d.ExtraActive && d.ExtraUSD >= 0.01 {
		reset5hTime, known5h := resolveReset(live5h, snap5hReset)
		reset7dTime, known7d := resolveReset(live7d, snap7dReset)
		at5h := pct5h >= 100.0
		at7d := pct7d >= 100.0
		switch {
		case at5h && at7d:
			if laterReset(reset7dTime, known7d, reset5hTime, known5h) {
				extraOn7d = true
			} else {
				extraOn5h = true
			}
		case at5h:
			extraOn5h = true
		case at7d:
			extraOn7d = true
		}
	}

	// minimalPctColour wraps "NN%" in the window's usage colour using the
	// configurable notice/warn/critical ratios (green below notice · yellow ·
	// orange · red at critical); pct > 95 overrides to bold_red (same rule
	// boldRedWrap applies in Full/Compact). Markers are gated on AnsiEnabled so
	// plain callers receive an uncoloured "NN%".
	minimalPctColour := func(pct, notice, warn, critical float64, roundUp bool) string {
		pctStr := fmt.Sprintf("%d%%", displayPctInt(pct, roundUp))
		if !t.AnsiEnabled {
			return pctStr
		}
		var marker string
		switch {
		case pct > 95.0:
			marker = "{{color:bold_red}}"
		case pct >= critical*100.0:
			marker = "{{color:red}}"
		case pct >= warn*100.0:
			marker = "{{color:orange}}"
		case pct >= notice*100.0:
			marker = "{{color:yellow}}"
		default:
			marker = "{{color:green}}"
		}
		return marker + pctStr + "{{reset}}"
	}

	// extraBlock renders the red "+$X[ extra[ usage]]" badge for the given level.
	// Leading space separates it from the preceding reset countdown. Markers are
	// gated on AnsiEnabled (plain callers never receive raw markers).
	extraBlock := func(lvl Level) string {
		var tail string
		switch lvl {
		case LevelFull:
			tail = " extra usage"
		case LevelCompact:
			tail = " extra"
		}
		amt := fmt.Sprintf("+$%.2f%s", d.ExtraUSD, tail)
		if t.AnsiEnabled {
			return " {{color:red}}" + amt + "{{reset}}"
		}
		return " " + amt
	}

	switch level {
	case LevelFull:
		bar5h := quotaUsageColor(pct5h, n5, w5, c5, t) +
			renderer.ProgressBar10(pct5h) + colourReset
		bar7d := quotaUsageColor(pct7d, n7, w7, c7, t) +
			renderer.ProgressBar10(pct7d) + colourReset
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%s%s %s", bar5h, pctSuffix(pct5h, c5, true), reset5h))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%s%s %s", bar7d, pctSuffix(pct7d, c7, false), reset7d))
		if extraOn5h {
			val5h += extraBlock(LevelFull)
		}
		if extraOn7d {
			val7d += extraBlock(LevelFull)
		}
		return fmt.Sprintf("5h: %s · 7d: %s%s", val5h, val7d, ageSuffix)
	case LevelCompact:
		bar5h := quotaUsageColor(pct5h, n5, w5, c5, t) +
			renderer.ProgressBar(pct5h) + colourReset
		bar7d := quotaUsageColor(pct7d, n7, w7, c7, t) +
			renderer.ProgressBar(pct7d) + colourReset
		val5h := boldRedWrap(pct5h, fmt.Sprintf("%s%s %s", bar5h, pctSuffix(pct5h, c5, true), reset5h))
		val7d := boldRedWrap(pct7d, fmt.Sprintf("%s%s %s", bar7d, pctSuffix(pct7d, c7, false), reset7d))
		if extraOn5h {
			val5h += extraBlock(LevelCompact)
		}
		if extraOn7d {
			val7d += extraBlock(LevelCompact)
		}
		return fmt.Sprintf("%s · %s%s", val5h, val7d, ageSuffix)
	default: // LevelMinimal
		// Minimal drops the bar; the number stands in its place, coloured by the
		// same rules the bar would use, and the reset countdown is kept like
		// Compact (6.95.e). The number stays even at ≥100% — it is the only quota
		// signal at this level (6.95.h hides it only when a bar is present).
		val5h := minimalPctColour(pct5h, n5, w5, c5, true) + " " + reset5h
		val7d := minimalPctColour(pct7d, n7, w7, c7, false) + " " + reset7d
		if extraOn5h {
			val5h += extraBlock(LevelMinimal)
		}
		if extraOn7d {
			val7d += extraBlock(LevelMinimal)
		}
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

// formatReset resolves a reset countdown from the freshest known source and
// applies a gradient colour marker based on the remaining time. Resolution order:
//
//  1. live stdin resets_at (liveRaw) — session-local, most precise;
//  2. persisted snapshot reset unix (snapUnix > 0) — cross-session fallback so an
//     idle session can still show the countdown an active session has observed;
//  3. neither known → "↻ ??m" plain (no colour): the window just reset and the
//     next one has not started ticking, so the time is genuinely unknown.
//
// A parseable-but-past reset (dur <= 0) renders "↻ 0m" — distinct from the
// unknown "↻ ??m" case. Colour markers are emitted as {{color:X}}…{{reset}} text
// tokens; Apply()/renderer strip them when colour is off.
func formatReset(liveRaw []byte, snapUnix int64, now time.Time, thresholds resetThresholds) string {
	resetTime, known := resolveReset(liveRaw, snapUnix)
	if !known {
		slog.Debug("quota.formatReset: reset time unknown (live + snapshot both absent); rendering ??m")
		return "↻ ??m"
	}
	dur := resetTime.Sub(now)
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

// resolveReset resolves a reset time from the freshest known source: live stdin
// resets_at first, then the persisted snapshot reset unix. Returns (time, true)
// when known, (zero, false) when neither source is available. Shared by
// formatReset (countdown rendering) and the Phase 6.95.h extra-usage block
// placement (which window resets later carries the badge).
func resolveReset(liveRaw []byte, snapUnix int64) (time.Time, bool) {
	if t, ok := stdin.ParseResetsAt(liveRaw); ok {
		return t, true
	}
	if snapUnix > 0 {
		return time.Unix(snapUnix, 0), true
	}
	return time.Time{}, false
}

// laterReset reports whether window A's reset is later than window B's, used to
// decide which window carries the extra-usage badge when both are at 100%
// (draft §5: attach to the window that resets later — it holds the overage
// longest). A known reset always beats an unknown one; when both are unknown the
// caller's A (the 7-day window) wins as the longer-lived default.
func laterReset(a time.Time, aKnown bool, b time.Time, bKnown bool) bool {
	switch {
	case aKnown && bKnown:
		return a.After(b)
	case aKnown:
		return true
	case bKnown:
		return false
	default:
		return true
	}
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
