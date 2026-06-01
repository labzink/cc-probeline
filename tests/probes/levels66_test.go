// Package probes_test — Phase 6.6.b RED tests for probe priority/level changes.
//
// Tests T-4..T-12 assert the NEW contract from spec-common.md §2.2.
// All tests compile against existing probe code but FAIL on assertions
// because the implementation still has the OLD priorities and level logic.
//
// Failure reason (expected): assertion mismatch — not compile error.
package probes_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// ----------------------------------------------------------------------------
// T-4: EmailProbe — Priority==2 and Compact truncation to 16
// ----------------------------------------------------------------------------

// TestEmail66_PriorityAndLevels verifies:
//   - Priority()==2 (was 1)
//   - Compact: middleTruncate(email,16) — long email is truncated and contains "…"
//   - Minimal: middleTruncate(email,12) — unchanged from Phase 4
//
// RED: Priority() returns 1 (old); Compact returns full email (no truncation).
func TestEmail66_PriorityAndLevels(t *testing.T) {
	p := &probes.EmailProbe{}
	th := renderer.Theme{}
	// "averylongemail@example.com" has 26 runes — longer than 16 (Compact limit) and 12 (Minimal limit).
	longEmail := "averylongemail@example.com"
	d := probes.Data{Stdin: stdin.Payload{}}
	cfg := probes.Config{EmailEnabled: true, Email: longEmail}

	// Priority must be 2 (was 1).
	if got := p.Priority(); got != 2 {
		t.Errorf("EmailProbe.Priority(): want 2, got %d", got)
	}

	// Compact: middleTruncate(email, 16) — result must contain "…" and NOT be the full email.
	gotCompact := p.Render(d, cfg, th, probes.LevelCompact)
	if !strings.Contains(gotCompact, "…") {
		t.Errorf("Render(Compact, long email): want truncation marker '…', got %q", gotCompact)
	}
	if gotCompact == longEmail {
		t.Errorf("Render(Compact, long email): want truncated string, got full email %q", gotCompact)
	}
	// Rune length of truncated result must be <= len("…" adds 1) + head(13) + tail(3) = 17 for regime 1.
	// Exact: head=13, tail=max(16-13,2)=3 → "averylongemail@…com" = 17 runes.
	compactRunes := []rune(gotCompact)
	if len(compactRunes) > 17 {
		t.Errorf("Render(Compact, long email): want rune length <= 17, got %d in %q",
			len(compactRunes), gotCompact)
	}

	// Minimal: middleTruncate(email, 12) — already in Phase 4, still truncates.
	// 26 runes → regime 2: tail=(12-1)/2=5, head=12-1-5=6 → "averyl…e.com" (12 runes).
	gotMinimal := p.Render(d, cfg, th, probes.LevelMinimal)
	if !strings.Contains(gotMinimal, "…") {
		t.Errorf("Render(Minimal, long email): want truncation marker '…', got %q", gotMinimal)
	}
	wantMinimalRunes := 12
	if got := len([]rune(gotMinimal)); got != wantMinimalRunes {
		t.Errorf("Render(Minimal, long email): want %d runes, got %d in %q",
			wantMinimalRunes, got, gotMinimal)
	}
}

// ----------------------------------------------------------------------------
// T-5: ProjectProbe — Compact truncation to 12
// ----------------------------------------------------------------------------

// TestProject66_Compact verifies:
//   - Compact: middleTruncate(name, 12) for long project names (was: full name)
//   - Minimal: middleTruncate(name, 8) — unchanged from Phase 4
//
// RED: Compact currently returns full basename (no truncation).
func TestProject66_Compact(t *testing.T) {
	p := &probes.ProjectProbe{}
	th := renderer.Theme{}
	// "my-super-long-project-name" (26 runes) is longer than 12 (Compact limit) and 8 (Minimal limit).
	longCwd := "/home/user/my-super-long-project-name"
	d := probes.Data{Stdin: stdin.Payload{Cwd: longCwd}}
	cfg := probes.Config{ProjectEnabled: true}

	// Compact: middleTruncate(name, 12) — result must contain "…".
	// name="my-super-long-project-name" (26 runes):
	//   half=13, half >= minWidth-1=11 → regime 2; tail=(12-1)/2=5, head=12-1-5=6
	//   → "my-sup" + "…" + "-name" = "my-sup…-name" (12 runes)
	gotCompact := p.Render(d, cfg, th, probes.LevelCompact)
	if !strings.Contains(gotCompact, "…") {
		t.Errorf("Render(Compact, long name): want truncation marker '…', got %q", gotCompact)
	}
	if utf8.RuneCountInString(gotCompact) > 12 {
		t.Errorf("Render(Compact, long name): want rune length <= 12, got %d in %q",
			utf8.RuneCountInString(gotCompact), gotCompact)
	}

	// Minimal: middleTruncate(name, 8) — same regime 2: tail=3, head=4 → "my-s…ame" (8 runes).
	gotMinimal := p.Render(d, cfg, th, probes.LevelMinimal)
	if !strings.Contains(gotMinimal, "…") {
		t.Errorf("Render(Minimal, long name): want truncation marker '…', got %q", gotMinimal)
	}
	if utf8.RuneCountInString(gotMinimal) > 8 {
		t.Errorf("Render(Minimal, long name): want rune length <= 8, got %d in %q",
			utf8.RuneCountInString(gotMinimal), gotMinimal)
	}
}

// ----------------------------------------------------------------------------
// T-6: QuotaProbe — Priority==1; Full bar 10 segments, Compact bar 5 segments
// ----------------------------------------------------------------------------

// TestQuota66_PriorityAndBars verifies:
//   - Priority()==1 (was 3)
//   - Full: ProgressBar10 → 10-rune bar
//   - Compact: ProgressBar → 5-rune bar
//
// RED: Priority() returns 3 (old); Full uses ProgressBar (5 runes) instead of ProgressBar10 (10 runes).
func TestQuota66_PriorityAndBars(t *testing.T) {
	// C4: isolate from real quota file.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())
	p := &probes.QuotaProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{QuotaEnabled: true}

	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	futureUnix := json.RawMessage(fmt.Sprintf("%d", now.Add(2*time.Hour).Unix()))
	rl := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 40.0, ResetsAt: futureUnix},
		SevenDay: stdin.RateWindow{UsedPercentage: 40.0, ResetsAt: futureUnix},
	}
	d := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl}}

	// Priority must be 1 (was 3).
	if got := p.Priority(); got != 1 {
		t.Errorf("QuotaProbe.Priority(): want 1, got %d", got)
	}

	// Full: the bar segment for 5h must be 10 runes (ProgressBar10).
	// 40% → ProgressBar10 floors to 40% → ████░░░░░░ (4 filled of 10).
	gotFull := p.Render(d, cfg, th, probes.LevelFull)
	// Extract the bar: Full format is "5h: <bar> <reset> · 7d: <bar> <reset>"
	// The bar immediately follows "5h: " and ends before the space before reset.
	// We check rune length of the first bar by finding the segment between "5h: " and the next space.
	prefix5h := "5h: "
	if !strings.HasPrefix(gotFull, prefix5h) {
		t.Fatalf("Render(Full): want prefix %q, got %q", prefix5h, gotFull)
	}
	afterLabel := gotFull[len(prefix5h):]
	// Bar is everything up to the first space (reset starts with ↻).
	spaceIdx := strings.Index(afterLabel, " ")
	if spaceIdx < 0 {
		t.Fatalf("Render(Full): no space found after bar in %q", gotFull)
	}
	barFull := []rune(afterLabel[:spaceIdx])
	if len(barFull) != 10 {
		t.Errorf("Render(Full): want 10-rune bar (ProgressBar10), got %d runes %q",
			len(barFull), string(barFull))
	}

	// Compact: the bar segment for 5h must be 5 runes (ProgressBar).
	// Compact format is "<bar> <reset> · <bar> <reset>" — no "5h: " label.
	gotCompact := p.Render(d, cfg, th, probes.LevelCompact)
	spaceIdxC := strings.Index(gotCompact, " ")
	if spaceIdxC < 0 {
		t.Fatalf("Render(Compact): no space found after bar in %q", gotCompact)
	}
	barCompact := []rune(gotCompact[:spaceIdxC])
	if len(barCompact) != 5 {
		t.Errorf("Render(Compact): want 5-rune bar (ProgressBar), got %d runes %q",
			len(barCompact), string(barCompact))
	}
}

// ----------------------------------------------------------------------------
// T-7: QuotaProbe — reset format: ↻ 0h:59m (<24h), ↻ 3d.21h (≥24h), ↻ 0m (expired)
// ----------------------------------------------------------------------------

// TestQuota66_ResetFormat verifies the NEW formatReset output:
//
//	< 24h: "↻ <h>h:<m>m"  → "↻ 0h:59m"  (space after ↻, colon between h and m)
//	≥ 24h: "↻ <d>d.<h>h"  → "↻ 3d.21h"  (dot between d and h)
//	expired/parse-fail: "↻ 0m"
//
// Manual calculation:
//
//	Case <24h: ResetsAt − Now = 59 minutes → totalMin=59, totalHours=0, mins=59 → "↻ 0h:59m"
//	Case ≥24h: ResetsAt − Now = 93 hours   → days=3, hours=21              → "↻ 3d.21h"
//
// RED: current formatDuration produces "↻0h59m" (no space, no colon) and "↻3d12h" (no space, no dot).
func TestQuota66_ResetFormat(t *testing.T) {
	// C4: isolate from real quota file.
	t.Setenv("CC_PROBELINE_QUOTA_DIR", t.TempDir())
	p := &probes.QuotaProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{QuotaEnabled: true}

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Case 1: duration < 24h — ResetsAt = now + 59 minutes → "↻ 0h:59m".
	resetsAt59m := json.RawMessage(fmt.Sprintf("%d", now.Add(59*time.Minute).Unix()))
	rl59m := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 10.0, ResetsAt: resetsAt59m},
		SevenDay: stdin.RateWindow{UsedPercentage: 10.0, ResetsAt: resetsAt59m},
	}
	d59m := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl59m}}

	got59m := p.Render(d59m, cfg, th, probes.LevelFull)
	wantReset59m := "↻ 0h:59m"
	if !strings.Contains(got59m, wantReset59m) {
		t.Errorf("Render(Full, 59m): want reset %q in output, got %q", wantReset59m, got59m)
	}
	// Confirm space after ↻ (not just ↻0h...).
	if strings.Contains(got59m, "↻0") {
		t.Errorf("Render(Full, 59m): old format ↻0 without space found in %q", got59m)
	}

	// Case 2: duration ≥ 24h — ResetsAt = now + 93 hours = 3 days 21 hours → "↻ 3d.21h".
	resetsAt3d21h := json.RawMessage(fmt.Sprintf("%d", now.Add(93*time.Hour).Unix()))
	rl3d21h := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 10.0, ResetsAt: resetsAt3d21h},
		SevenDay: stdin.RateWindow{UsedPercentage: 10.0, ResetsAt: resetsAt3d21h},
	}
	d3d21h := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rl3d21h}}

	got3d21h := p.Render(d3d21h, cfg, th, probes.LevelFull)
	wantReset3d21h := "↻ 3d.21h"
	if !strings.Contains(got3d21h, wantReset3d21h) {
		t.Errorf("Render(Full, 3d21h): want reset %q in output, got %q", wantReset3d21h, got3d21h)
	}
	// Confirm dot separator (not old "3d21h").
	if strings.Contains(got3d21h, "3d21h") {
		t.Errorf("Render(Full, 3d21h): old format 3d21h (no dot) found in %q", got3d21h)
	}

	// Case 3: expired (ResetsAt in the past) → "↻ 0m".
	resetsAtPast := json.RawMessage(fmt.Sprintf("%d", now.Add(-1*time.Minute).Unix()))
	rlExpired := &stdin.RateLimits{
		FiveHour: stdin.RateWindow{UsedPercentage: 90.0, ResetsAt: resetsAtPast},
		SevenDay: stdin.RateWindow{UsedPercentage: 90.0, ResetsAt: resetsAtPast},
	}
	dExpired := probes.Data{Now: now, Stdin: stdin.Payload{RateLimits: rlExpired}}

	gotExpired := p.Render(dExpired, cfg, th, probes.LevelFull)
	wantExpired := "↻ 0m"
	if !strings.Contains(gotExpired, wantExpired) {
		t.Errorf("Render(Full, expired): want %q in output, got %q", wantExpired, gotExpired)
	}
}

// ----------------------------------------------------------------------------
// T-8: GitProbe — Compact: branch→12 + ⚠N; Minimal: branch→8 + no ⚠N
// ----------------------------------------------------------------------------

// TestGit66_CompactMinimal verifies:
//   - Compact: branch middle-truncated to 12, ⚠N preserved when > 0
//   - Minimal: branch middle-truncated to 8, ⚠N dropped (not present)
//
// RED: current Compact returns full branch (no truncation at Compact level).
func TestGit66_CompactMinimal(t *testing.T) {
	p := &probes.GitProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{GitEnabled: true}

	// Branch longer than 12 runes so truncation is observable at Compact level.
	// "agent/feature-dev/very-long-branch" has 34 runes.
	longBranch := "agent/feature-dev/very-long-branch"
	d := probes.Data{
		Git: &parser.GitStatus{
			Branch:        longBranch,
			ModifiedCount: 3,
		},
	}

	// Compact: branch truncated to 12 runes + "…" present + "⚠3" present.
	gotCompact := p.Render(d, cfg, th, probes.LevelCompact)
	if !strings.Contains(gotCompact, "…") {
		t.Errorf("Render(Compact, long branch): want truncation '…', got %q", gotCompact)
	}
	if !strings.Contains(gotCompact, "⚠3") {
		t.Errorf("Render(Compact, long branch): want '⚠3' preserved, got %q", gotCompact)
	}
	// The branch part (after "⎇ " and before " ⚠") must be <= 12 runes (truncated).
	// The whole output must contain "⎇ " as prefix for git.
	if !strings.Contains(gotCompact, "⎇") {
		t.Errorf("Render(Compact): want '⎇' in output, got %q", gotCompact)
	}

	// Minimal: branch truncated to 8 runes + NO ⚠N.
	gotMinimal := p.Render(d, cfg, th, probes.LevelMinimal)
	if !strings.Contains(gotMinimal, "…") {
		t.Errorf("Render(Minimal, long branch): want truncation '…', got %q", gotMinimal)
	}
	if strings.Contains(gotMinimal, "⚠") {
		t.Errorf("Render(Minimal, long branch): want NO '⚠N', got %q", gotMinimal)
	}
	// Branch is truncated to 8, output is "⎇ <8-rune-branch>".
	// Total rune count: 2 (⎇ ) + 8 = 10 runes max.
	minimalRunes := utf8.RuneCountInString(gotMinimal)
	if minimalRunes > 10 {
		t.Errorf("Render(Minimal, long branch): want rune length <= 10 (⎇+space+8), got %d in %q",
			minimalRunes, gotMinimal)
	}
}

// ----------------------------------------------------------------------------
// T-9: CtxProbe — Priority==1; Full bar 10 runes; Compact bar 5 runes, no %;
//       Minimal: only usedK/sizeK (no bar, no %)
// ----------------------------------------------------------------------------

// TestCtx66_Levels verifies:
//   - Priority()==1 (was 0)
//   - Full: ProgressBar10 (10-rune bar) + usedK/sizeK + (%)
//   - Compact: ProgressBar (5-rune bar) + usedK/sizeK, NO "%" present
//   - Minimal: only "usedK/sizeK" — no bar runes (█▒░), no "%"
//
// Setup: Size=200000, used=128000 → pct=64%
//
//	ProgressBar10(64%): rounds down to 60% → ██████░░░░ (10 runes)
//	ProgressBar(rounded 64%→60%): ███░░ (5 runes)
//
// RED: Priority() returns 0 (old); Full uses 5-rune bar instead of 10-rune bar.
func TestCtx66_Levels(t *testing.T) {
	p := &probes.CtxProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{CtxEnabled: true}

	d := probes.Data{Stdin: stdin.Payload{
		ContextWindow: stdin.ContextWindow{
			Size: 200000,
			CurrentUsage: map[string]int{
				"cache_read_input_tokens":     128000,
				"input_tokens":                0,
				"cache_creation_input_tokens": 0,
				"output_tokens":               0,
			},
		},
	}}

	// Priority must be 1 (was 0).
	if got := p.Priority(); got != 1 {
		t.Errorf("CtxProbe.Priority(): want 1, got %d", got)
	}

	// Full: bar must be 10 runes (ProgressBar10).
	// Full format: "ctx <bar> <usedK>/<sizeK> (<pct>%)"
	// Expected bar: 64% → floor to 60% → ██████░░░░
	gotFull := p.Render(d, cfg, th, probes.LevelFull)
	// The bar follows "ctx " and ends at the space before the label.
	const ctxPrefix = "ctx "
	if !strings.HasPrefix(gotFull, ctxPrefix) {
		t.Fatalf("Render(Full): want prefix %q, got %q", ctxPrefix, gotFull)
	}
	afterCtx := gotFull[len(ctxPrefix):]
	barEndIdx := strings.Index(afterCtx, " ")
	if barEndIdx < 0 {
		t.Fatalf("Render(Full): no space after bar in %q", gotFull)
	}
	barFull := []rune(afterCtx[:barEndIdx])
	if len(barFull) != 10 {
		t.Errorf("Render(Full): want 10-rune bar (ProgressBar10), got %d runes %q",
			len(barFull), string(barFull))
	}
	// Full must contain "(64%)" — percentage preserved.
	if !strings.Contains(gotFull, "(64%)") {
		t.Errorf("Render(Full): want '(64%%)' in output, got %q", gotFull)
	}

	// Compact: bar must be 5 runes (ProgressBar), NO "%" in output.
	// Compact format: "<bar> <usedK>/<sizeK>"
	gotCompact := p.Render(d, cfg, th, probes.LevelCompact)
	spaceIdxC := strings.Index(gotCompact, " ")
	if spaceIdxC < 0 {
		t.Fatalf("Render(Compact): no space after bar in %q", gotCompact)
	}
	barCompact := []rune(gotCompact[:spaceIdxC])
	if len(barCompact) != 5 {
		t.Errorf("Render(Compact): want 5-rune bar (ProgressBar), got %d runes %q",
			len(barCompact), string(barCompact))
	}
	if strings.Contains(gotCompact, "%") {
		t.Errorf("Render(Compact): must NOT contain '%%', got %q", gotCompact)
	}

	// Minimal: only "usedK/sizeK" — no bar chars (█ ▒ ░), no "%".
	// Expected exactly "128K/200K".
	gotMinimal := p.Render(d, cfg, th, probes.LevelMinimal)
	wantMinimal := "128K/200K"
	if gotMinimal != wantMinimal {
		t.Errorf("Render(Minimal): want %q, got %q", wantMinimal, gotMinimal)
	}
	// Extra guard: no bar block characters present.
	for _, r := range []rune{'█', '▒', '░'} {
		if strings.ContainsRune(gotMinimal, r) {
			t.Errorf("Render(Minimal): must not contain bar rune %q, got %q", string(r), gotMinimal)
		}
	}
	if strings.Contains(gotMinimal, "%") {
		t.Errorf("Render(Minimal): must not contain '%%', got %q", gotMinimal)
	}
}

// ----------------------------------------------------------------------------
// T-10: CostProbe — Priority==1 (render behaviour unchanged)
// ----------------------------------------------------------------------------

// TestCost66_Priority verifies:
//   - Priority()==1 (was 2)
//   - Render behaviour unchanged: Full "cost: $x", Compact/Minimal "$x"
//
// RED: Priority() returns 2 (old).
func TestCost66_Priority(t *testing.T) {
	p := &probes.CostProbe{}
	th := renderer.Theme{}
	cost := 7.42
	d := probes.Data{Stdin: stdin.Payload{Cost: stdin.Cost{TotalCostUSD: cost}}}
	cfg := probes.Config{CostEnabled: true}

	// Priority must be 1 (was 2).
	if got := p.Priority(); got != 1 {
		t.Errorf("CostProbe.Priority(): want 1, got %d", got)
	}

	// Render behaviour must remain identical — no regression.
	wantFull := "cost: $7.42"
	if got := p.Render(d, cfg, th, probes.LevelFull); got != wantFull {
		t.Errorf("Render(Full): want %q, got %q", wantFull, got)
	}
	wantCompact := "$7.42"
	if got := p.Render(d, cfg, th, probes.LevelCompact); got != wantCompact {
		t.Errorf("Render(Compact): want %q, got %q", wantCompact, got)
	}
	wantMinimal := "$7.42"
	if got := p.Render(d, cfg, th, probes.LevelMinimal); got != wantMinimal {
		t.Errorf("Render(Minimal): want %q, got %q", wantMinimal, got)
	}
}

// ----------------------------------------------------------------------------
// T-11: TimeProbe — Priority==1; Minimal returns MM:SS (not ""); MinWidth==5
// ----------------------------------------------------------------------------

// TestTime66_PriorityMinimal verifies:
//   - Priority()==1 (was 0)
//   - Minimal returns "MM:SS" — non-empty (was "")
//   - MinWidth()==5 (was 0)
//
// RED: Priority()=0, Minimal=""`, MinWidth()=0.
func TestTime66_PriorityMinimal(t *testing.T) {
	p := &probes.TimeProbe{}
	th := renderer.Theme{}
	cfg := probes.Config{TimeEnabled: true}
	// 2998000 ms = 49m58s → Minimal "49:58".
	d := makeTimeData(2998000)

	// Priority must be 1 (was 0).
	if got := p.Priority(); got != 1 {
		t.Errorf("TimeProbe.Priority(): want 1, got %d", got)
	}

	// MinWidth must be 5 (length of "MM:SS", was 0).
	if got := p.MinWidth(); got != 5 {
		t.Errorf("TimeProbe.MinWidth(): want 5, got %d", got)
	}

	// Minimal must return non-empty "MM:SS".
	gotMinimal := p.Render(d, cfg, th, probes.LevelMinimal)
	if gotMinimal == "" {
		t.Errorf("Render(Minimal): want non-empty MM:SS, got empty string")
	}
	// Must be exactly "MM:SS" format — 5 chars "49:58".
	wantMinimal := "49:58"
	if gotMinimal != wantMinimal {
		t.Errorf("Render(Minimal): want %q, got %q", wantMinimal, gotMinimal)
	}

	// Full and Compact behaviour unchanged.
	if got := p.Render(d, cfg, th, probes.LevelFull); got != "time: 49:58" {
		t.Errorf("Render(Full): want %q, got %q", "time: 49:58", got)
	}
	if got := p.Render(d, cfg, th, probes.LevelCompact); got != "49:58" {
		t.Errorf("Render(Compact): want %q, got %q", "49:58", got)
	}
}

// ----------------------------------------------------------------------------
// T-12: CacheProbe — Minimal contains TTL "⏱ Nm" when remaining > 0
// ----------------------------------------------------------------------------

// TestCache66_MinimalTTL verifies that CacheProbe.Render at LevelMinimal DOES
// contain "⏱ Nm" when OrchTTLMinutes is set and the cache window has not expired.
//
// Setup: OrchTTLMinutes=60, d.Now=10:08:00, LastTimestamp=10:00:00 (8 min elapsed),
// TurnCount=3. remaining = 60 − floor(8) = 52 → output must contain "⏱ 52m".
//
// RED: current CacheProbe always drops TTL from Minimal output.
//
// NOTE: This test conflicts with the EXISTING TestCacheProbe_TTL_Minimal which asserts
// ⏱ is ABSENT from Minimal. That existing test reflects Phase 4.1 behaviour.
// The GREEN dev must update TestCacheProbe_TTL_Minimal to reflect the new contract.
func TestCache66_MinimalTTL(t *testing.T) {
	p := &probes.CacheProbe{}
	cfg := cfgAllOn()
	cfg.OrchTTLMinutes = 60
	th := renderer.Theme{}

	now := time.Date(2024, 1, 1, 10, 8, 0, 0, time.UTC)
	lastTS := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	// elapsed = 8 min → remaining = 60 - 8 = 52 → "⏱ 52m"
	d := newCacheTTLData(1000, 2000, 500, 0.10, 60000, now, lastTS, 3)

	gotMinimal := p.Render(d, cfg, th, probes.LevelMinimal)

	// Minimal must now contain the TTL block.
	if !strings.Contains(gotMinimal, "⏱ 52m") {
		t.Errorf("Render(Minimal): want '⏱ 52m' in output, got %q", gotMinimal)
	}
	// Broader check: ⏱ glyph itself must be present.
	if !strings.Contains(gotMinimal, "⏱") {
		t.Errorf("Render(Minimal): want '⏱' TTL glyph in output, got %q", gotMinimal)
	}
}
