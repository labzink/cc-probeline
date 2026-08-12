// Package probes_test — Phase 7.48: the model-scoped weekly window and the
// official overage badge inside QuotaProbe.Render.
//
// What these lock down, beyond what the golden snapshots show: the bracket
// grouping (a model window is part of the week, not a third peer window), the
// single-countdown rule and its escape hatch, and the staleness gate that makes
// both blocks vanish rather than lie. All tests are hermetic — the quota
// snapshot directory is a t.TempDir.
package probes_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/claudejson"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

var modelNow = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

// modelData builds a render input with 5h at 42%, 7d at 95% and a model window
// whose reset lands at the offset given (measured from the 7-day reset, so the
// caller can make the two coincide or diverge).
func modelData(t *testing.T, modelPct float64, modelResetSkew time.Duration, age time.Duration) probes.Data {
	t.Helper()
	reset7d := modelNow.Add(5 * 24 * time.Hour)
	raw := func(ts time.Time) json.RawMessage {
		b, _ := json.Marshal(ts.UTC().Format(time.RFC3339))
		return b
	}
	return probes.Data{
		Now: modelNow,
		Stdin: stdin.Payload{RateLimits: &stdin.RateLimits{
			FiveHour: stdin.RateWindow{UsedPercentage: 42, ResetsAt: raw(modelNow.Add(3 * time.Hour))},
			SevenDay: stdin.RateWindow{UsedPercentage: 95, ResetsAt: raw(reset7d)},
		}},
		ModelWindow: &claudejson.ModelWindow{
			Name:      "fable",
			Pct:       modelPct,
			ResetUnix: reset7d.Add(modelResetSkew).Unix(),
		},
		UsageAge: age,
	}
}

func renderQuota(t *testing.T, d probes.Data, level probes.Level) string {
	t.Helper()
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())
	p := &probes.QuotaProbe{}
	return p.Render(d, probes.Config{QuotaEnabled: true}, renderer.Theme{}, level)
}

// ---------------------------------------------------------------------------
// Property: the model window renders inside the 7-day block, in brackets, and
// the two weekly windows share one countdown placed after the bracket. Bracket
// rather than " · " is the point: " · " separates peer windows, and a model
// window is a slice of the same week, not a third week.
// ---------------------------------------------------------------------------

func TestQuotaProbe_ModelWindow_BracketedWithSharedCountdown(t *testing.T) {
	got := renderQuota(t, modelData(t, 62, 300*time.Millisecond, 2*time.Minute), probes.LevelFull)

	if !strings.Contains(got, "(fable: ") {
		t.Fatalf("expected a bracketed model block labelled with the full name, got %q", got)
	}
	// Exactly two countdowns in the whole line: one for 5h, one for the weekly
	// group. A third would mean the 7-day block kept its own.
	if n := strings.Count(got, "↻"); n != 2 {
		t.Errorf("countdown count = %d, want 2 (5h + one shared weekly), got %q", n, got)
	}
	// The shared countdown must sit AFTER the closing bracket, so it visibly
	// covers the group rather than the model window alone.
	closing := strings.Index(got, ")")
	last := strings.LastIndex(got, "↻")
	if closing < 0 || last < closing {
		t.Errorf("shared countdown must follow the bracket, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Property: if the model window ever resets on its own schedule, each block
// carries its own countdown — one timer must never speak for two moments.
// ---------------------------------------------------------------------------

func TestQuotaProbe_ModelWindow_DivergentResetKeepsBothCountdowns(t *testing.T) {
	got := renderQuota(t, modelData(t, 62, 48*time.Hour, 2*time.Minute), probes.LevelFull)

	if n := strings.Count(got, "↻"); n != 3 {
		t.Errorf("countdown count = %d, want 3 (5h + 7d + model), got %q", n, got)
	}
}

// ---------------------------------------------------------------------------
// Property: the tighter levels keep a label — Compact and Minimal print no
// "5h:"/"7d:" prefixes at all, so an unlabelled third value would be readable
// only by position.
// ---------------------------------------------------------------------------

func TestQuotaProbe_ModelWindow_InitialLabelWhenTight(t *testing.T) {
	for _, lvl := range []struct {
		name  string
		level probes.Level
	}{
		{"compact", probes.LevelCompact},
		{"minimal", probes.LevelMinimal},
	} {
		t.Run(lvl.name, func(t *testing.T) {
			got := renderQuota(t, modelData(t, 62, 0, 2*time.Minute), lvl.level)
			if !strings.Contains(got, "(f: ") {
				t.Errorf("expected the initial label \"(f: \" at this level, got %q", got)
			}
			if strings.Contains(got, "fable") {
				t.Errorf("full model name has no room at this level, got %q", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Property: a stale usage cache hides BOTH cache-sourced blocks. Everything else
// in the line is live payload data and must be untouched — this is the "no model
// window, no badge, exactly the old line" guarantee.
// ---------------------------------------------------------------------------

func TestQuotaProbe_StaleUsageHidesModelAndBadge(t *testing.T) {
	d := modelData(t, 62, 0, 90*time.Minute) // older than the one-hour gate
	d.Overage = &claudejson.ExtraUsage{Enabled: true, UsedUSD: 20.40, LimitUSD: 120}

	got := renderQuota(t, d, probes.LevelFull)

	if strings.Contains(got, "fable") || strings.Contains(got, "(f") {
		t.Errorf("stale cache must not render the model window, got %q", got)
	}
	if strings.Contains(got, "+$") {
		t.Errorf("stale cache must not render the overage badge, got %q", got)
	}
	if !strings.Contains(got, "5h: ") || !strings.Contains(got, "7d: ") {
		t.Errorf("live payload blocks must survive a stale cache, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Property: the badge appears only for an account that actually has paid
// overage switched on, and only with a known ceiling. "Enabled" alone with a
// zero ceiling means we do not know the shape of the plan — say nothing.
// ---------------------------------------------------------------------------

func TestQuotaProbe_OverageBadgeGates(t *testing.T) {
	cases := []struct {
		name    string
		overage *claudejson.ExtraUsage
		want    bool
	}{
		{"enabled with ceiling", &claudejson.ExtraUsage{Enabled: true, UsedUSD: 20.40, LimitUSD: 120}, true},
		{"disabled", &claudejson.ExtraUsage{Enabled: false, UsedUSD: 20.40, LimitUSD: 120}, false},
		{"no ceiling", &claudejson.ExtraUsage{Enabled: true, UsedUSD: 20.40, LimitUSD: 0}, false},
		{"absent", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := modelData(t, 62, 0, 2*time.Minute)
			d.Overage = tc.overage

			got := renderQuota(t, d, probes.LevelFull)

			if has := strings.Contains(got, "+$"); has != tc.want {
				t.Errorf("badge present = %v, want %v; got %q", has, tc.want, got)
			}
			if tc.want && !strings.Contains(got, "⧗ 2m") {
				t.Errorf("badge must carry its dim age marker, got %q", got)
			}
			if tc.want && !strings.Contains(got, "+$20.40 / $120.00") {
				t.Errorf("Full level shows spent and ceiling, got %q", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Property: with no model window the quota block is byte-for-byte what it was
// before this feature — the guarantee for every account that has no such limit.
// ---------------------------------------------------------------------------

func TestQuotaProbe_NoModelWindow_UnchangedShape(t *testing.T) {
	d := modelData(t, 62, 0, 2*time.Minute)
	d.ModelWindow = nil

	got := renderQuota(t, d, probes.LevelFull)

	if strings.Contains(got, "(") {
		t.Errorf("no bracket group without a model window, got %q", got)
	}
	if n := strings.Count(got, "↻"); n != 2 {
		t.Errorf("countdown count = %d, want 2 (one per window), got %q", n, got)
	}
}

// ---------------------------------------------------------------------------
// Property: on a narrow terminal the cache-sourced blocks give way first. The
// 5h/7d pair is what every account has; the model window and the badge are
// extras. Without this the quota block measured 75 visible columns at a width
// of 40 — level degradation stops at Minimal and the first line never wraps, so
// the overflow would push the whole status line past the terminal edge.
// ---------------------------------------------------------------------------

func TestQuotaProbe_NarrowTerminalDropsCacheBlocks(t *testing.T) {
	d := modelData(t, 62, 0, 2*time.Minute)
	d.Overage = &claudejson.ExtraUsage{Enabled: true, UsedUSD: 20.40, LimitUSD: 120}
	d.TerminalCols = 40

	got := renderQuota(t, d, probes.LevelMinimal)

	if strings.Contains(got, "(f") || strings.Contains(got, "+$") {
		t.Errorf("at 40 columns neither block may render, got %q", got)
	}
	if !strings.Contains(got, "42%") || !strings.Contains(got, "95%") {
		t.Errorf("the 5h/7d pair must survive every width, got %q", got)
	}
	if n := len([]rune(got)); n > 40 {
		t.Errorf("quota block is %d columns at width 40: %q", n, got)
	}
}

// ---------------------------------------------------------------------------
// Property: an account with extra usage switched on but nothing spent gets no
// badge. Otherwise the line carries a permanent red "+$0.00 / $120.00" for an
// event that has not happened.
// ---------------------------------------------------------------------------

func TestQuotaProbe_NoBadgeUntilSomethingIsSpent(t *testing.T) {
	d := modelData(t, 62, 0, 2*time.Minute)
	d.Overage = &claudejson.ExtraUsage{Enabled: true, UsedUSD: 0, LimitUSD: 120}

	if got := renderQuota(t, d, probes.LevelFull); strings.Contains(got, "+$") {
		t.Errorf("no badge before the first cent is spent, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Property: a snapshot stamped in the FUTURE counts as stale, not as fresh. A
// backwards clock jump must not be the one thing that disables the staleness
// gate.
// ---------------------------------------------------------------------------

func TestQuotaProbe_FutureSnapshotIsNotFresh(t *testing.T) {
	d := modelData(t, 62, 0, -30*time.Minute)
	d.Overage = &claudejson.ExtraUsage{Enabled: true, UsedUSD: 20.40, LimitUSD: 120}

	got := renderQuota(t, d, probes.LevelFull)

	if strings.Contains(got, "fable") || strings.Contains(got, "+$") {
		t.Errorf("a future-stamped snapshot must render neither block, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Property: an unknown model reset inherits the weekly countdown rather than
// printing "↻ ??m" — the weekly reset is the best information available, and
// the question marks would cost columns to say nothing.
// ---------------------------------------------------------------------------

func TestQuotaProbe_UnknownModelResetSharesTheCountdown(t *testing.T) {
	d := modelData(t, 62, 0, 2*time.Minute)
	d.ModelWindow.ResetUnix = 0

	got := renderQuota(t, d, probes.LevelFull)

	if strings.Contains(got, "??") {
		t.Errorf("unknown model reset must not print ??m, got %q", got)
	}
	if n := strings.Count(got, "↻"); n != 2 {
		t.Errorf("countdown count = %d, want 2 (5h + shared weekly), got %q", n, got)
	}
}

// ---------------------------------------------------------------------------
// Property: a model window whose own reset has passed is not rendered — its
// percentage belongs to a week that already rolled over, the same trap
// windowExpired guards for the 5h/7d windows.
// ---------------------------------------------------------------------------

func TestQuotaProbe_ExpiredModelWindowDropped(t *testing.T) {
	// The skew is measured from the 7-day reset (now + 5d), so -6d puts the
	// model window's own reset a day in the past.
	d := modelData(t, 88, -6*24*time.Hour, 2*time.Minute)

	if got := renderQuota(t, d, probes.LevelFull); strings.Contains(got, "fable") {
		t.Errorf("a rolled-over model window must not render, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Property: the model window follows the same number rules as its neighbours —
// the percentage appears from the critical ratio up, and disappears again at
// 100%, where the filled bar is the signal.
// ---------------------------------------------------------------------------

func TestQuotaProbe_ModelWindowNumberRules(t *testing.T) {
	cases := []struct {
		name     string
		pct      float64
		wantText string
		wantIn   bool
	}{
		{"below critical: bar only", 62, "62%", false},
		{"past critical: number shows", 97, "97%", true},
		{"at 100: number gone", 100, "100%", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderQuota(t, modelData(t, tc.pct, 0, 2*time.Minute), probes.LevelFull)
			if has := strings.Contains(got, tc.wantText); has != tc.wantIn {
				t.Errorf("%q present = %v, want %v; got %q", tc.wantText, has, tc.wantIn, got)
			}
			if !strings.Contains(got, "(fable:") {
				t.Errorf("the model block itself must render, got %q", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Property: a non-USD account is not given a dollar sign. We do not guess
// symbols — the ISO code trails the figures instead.
// ---------------------------------------------------------------------------

func TestQuotaProbe_NonUSDBadge(t *testing.T) {
	d := modelData(t, 62, 0, 2*time.Minute)
	d.Overage = &claudejson.ExtraUsage{Enabled: true, UsedUSD: 20.40, LimitUSD: 120, Currency: "eur"}

	got := renderQuota(t, d, probes.LevelFull)

	if strings.Contains(got, "$") {
		t.Errorf("no dollar sign on a EUR account, got %q", got)
	}
	if !strings.Contains(got, "+20.40 / 120.00 EUR") {
		t.Errorf("want the ISO code after the figures, got %q", got)
	}
}
