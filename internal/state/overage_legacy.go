//go:build legacy_overage

// This file holds the session-local overage estimator that drove the
// "+$X extra usage" badge from Phase 6.95.h until Phase 7.48.
//
// Why it is behind a build tag rather than deleted: the estimator is correct
// arithmetic for the question it answered ("how much has THIS session spent
// since a window hit 100%"), including the proportional crossing tail added in
// Phase 7.45 B4. It stopped shipping because the badge now shows Anthropic's own
// month-to-date figure from the usage cache in ~/.claude.json — account-wide,
// like every other number in the quota block, and not an estimate at all.
//
// Kept compilable so it can come back by deleting one line if the official
// figure ever proves unavailable (it depends on a cache that only Claude Code's
// /usage screen refreshes). Build and test it with:
//
//	go test -tags legacy_overage ./tests/state/
//
// The Session fields it owns (OverageBaseline, OverageActive, PrevQuotaPct,
// PrevQuotaTotal) stay in state.go: they are persisted JSON, and removing them
// would break state files written by earlier versions.

package state

// ExtraUsageTick advances the extra-usage (paid overage) state for one refresh
// and returns whether the badge is active and the overage USD to display.
//
// pct is the binding rate-limit percentage (max of the 5h/7d windows); the badge
// arms when pct ≥ 100 AND hasExtra (~/.claude.json hasExtraUsageEnabled).
//
// Phase 7.45 B4 — proportional crossing tail. On the first refresh that crosses
// 100%, the cost added this tick (sessionTotal − prevTotal) is the crossing
// turn's cost; only the fraction of it that lies above the 100% line counts as
// extra:
//
//	tail = (sessionTotal − prevTotal) × (pct − 100) / (pct − prevPct)
//
// so the baseline is sessionTotal − tail (not the full sessionTotal as before,
// which silently dropped the crossing turn's overage). If CC clips pct at 100 the
// fraction is 0 → tail 0 → identical to the old behaviour. The tail is only taken
// when a genuine sub-100 previous reading exists (prevPct in (0,100)); a cold
// start (prevPct == 0) or an already-over window takes no tail.
//
// When the trigger is false the badge clears and the baseline resets to 0 —
// recomputed every refresh, never sticky.
func (s *Session) ExtraUsageTick(sessionTotal, pct float64, hasExtra bool) (active bool, usd float64) {
	prevPct, prevTotal := s.PrevQuotaPct, s.PrevQuotaTotal
	// Record this tick for the next call — always, so the reading immediately
	// before a crossing is available regardless of badge state.
	s.PrevQuotaPct, s.PrevQuotaTotal = pct, sessionTotal

	if pct >= 100 && hasExtra {
		if !s.OverageActive {
			tail := 0.0
			if prevPct > 0 && prevPct < 100 && pct > prevPct {
				if turnCost := sessionTotal - prevTotal; turnCost > 0 {
					tail = turnCost * (pct - 100) / (pct - prevPct)
				}
			}
			s.OverageBaseline = sessionTotal - tail
			s.OverageActive = true
		}
		over := sessionTotal - s.OverageBaseline
		if over < 0 {
			over = 0
		}
		return true, over
	}
	s.OverageActive = false
	s.OverageBaseline = 0
	return false, 0
}
