// Package state_test — Phase 6.95.h extra-usage (paid overage) transition tests.
// ExtraUsageTick is a pure state transition: it snapshots SessionTotal as the
// baseline on the first refresh where a rate-limit window is at ≥100% AND
// hasExtraUsageEnabled, then reports the overage (SessionTotal − baseline) on
// each subsequent refresh. When the trigger clears the baseline resets to 0.
package state_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/state"
)

// TestExtraUsage_BaselineThenAccrues is the core contract: the first crossing
// snapshots the baseline (overage 0), and the overage then grows with
// SessionTotal. Numbers mirror live capture (~$0.06–0.08/turn growth past 100%).
func TestExtraUsage_BaselineThenAccrues(t *testing.T) {
	s := &state.Session{}

	// First refresh past 100%: SessionTotal = $5.00 becomes the baseline.
	// pct exactly 100 ⇒ crossing tail is 0 ⇒ baseline = full SessionTotal.
	active, usd := s.ExtraUsageTick(5.00, 100.0, true)
	if !active {
		t.Fatalf("first crossing: active = false; want true")
	}
	if usd != 0 {
		t.Errorf("first crossing: overage = %.4f; want 0 (baseline just set)", usd)
	}
	if s.OverageBaseline != 5.00 {
		t.Errorf("baseline = %.4f; want 5.00", s.OverageBaseline)
	}

	// Next refresh: spend grew to $5.07 → overage $0.07 (float tolerance).
	if active, usd = s.ExtraUsageTick(5.07, 100.0, true); !active || usd < 0.0699 || usd > 0.0701 {
		t.Errorf("after +0.07: active=%v overage=%.4f; want true ~0.07", active, usd)
	}
	// And again: $5.14 → overage $0.14. Baseline must stay pinned at 5.00.
	if active, usd = s.ExtraUsageTick(5.14, 100.0, true); usd < 0.1399 || usd > 0.1401 {
		t.Errorf("after +0.14: overage=%.4f; want ~0.14", usd)
	}
	if s.OverageBaseline != 5.00 {
		t.Errorf("baseline drifted to %.4f; want pinned 5.00", s.OverageBaseline)
	}
}

// TestExtraUsage_NotTriggeredWhenBelow100 verifies that a window below 100%
// (even with hasExtra) does not arm the badge and keeps the baseline at 0.
func TestExtraUsage_NotTriggeredWhenBelow100(t *testing.T) {
	s := &state.Session{}
	active, usd := s.ExtraUsageTick(5.00, 50.0, true)
	if active || usd != 0 {
		t.Errorf("below 100%%: active=%v overage=%.4f; want false 0", active, usd)
	}
	if s.OverageBaseline != 0 || s.OverageActive {
		t.Errorf("below 100%%: baseline=%.4f active=%v; want 0 false", s.OverageBaseline, s.OverageActive)
	}
}

// TestExtraUsage_NotTriggeredWithoutExtraEnabled verifies that being at 100%
// without hasExtraUsageEnabled never arms the badge.
func TestExtraUsage_NotTriggeredWithoutExtraEnabled(t *testing.T) {
	s := &state.Session{}
	if active, _ := s.ExtraUsageTick(5.00, 100.0, false); active {
		t.Errorf("at 100%% but hasExtra=false: active = true; want false")
	}
}

// TestExtraUsage_ClearsAndRebaselines verifies the "never sticky" rule: once the
// windows drop below 100% the baseline resets, and a later re-crossing snapshots
// a fresh baseline (overage counts from the new crossing, not the old one).
func TestExtraUsage_ClearsAndRebaselines(t *testing.T) {
	s := &state.Session{}

	// Arm at $5.00, accrue to $5.10.
	s.ExtraUsageTick(5.00, 100.0, true)
	if _, usd := s.ExtraUsageTick(5.10, 100.0, true); usd < 0.0999 || usd > 0.1001 {
		t.Fatalf("accrue: overage=%.4f; want ~0.10", usd)
	}

	// Windows reset below 100% → badge clears, baseline zeroed.
	if active, usd := s.ExtraUsageTick(5.10, 50.0, true); active || usd != 0 {
		t.Errorf("clear: active=%v overage=%.4f; want false 0", active, usd)
	}
	if s.OverageBaseline != 0 {
		t.Errorf("clear: baseline=%.4f; want 0", s.OverageBaseline)
	}

	// Re-cross later at $8.00: fresh baseline, overage counts from here.
	if active, usd := s.ExtraUsageTick(8.00, 100.0, true); !active || usd != 0 {
		t.Errorf("re-cross: active=%v overage=%.4f; want true 0 (new baseline)", active, usd)
	}
	if s.OverageBaseline != 8.00 {
		t.Errorf("re-cross baseline=%.4f; want 8.00", s.OverageBaseline)
	}
}

// TestExtraUsage_CrossingTailProportional is the B4 contract: when the quota
// crosses from 99% to 103% in a single tick, only the fraction of the crossing
// turn's cost that lies above the 100% line counts as extra. With the turn
// costing $0.40 and the window moving 99→103, the above-100 fraction is
// (103−100)/(103−99) = 3/4, so extra = 3/4 × $0.40 = $0.30. The pre-B4 code set
// the baseline to the full SessionTotal at the crossing tick, reporting $0 and
// silently dropping this $0.30 tail.
func TestExtraUsage_CrossingTailProportional(t *testing.T) {
	s := &state.Session{}

	// Tick 1: still under quota at 99%, SessionTotal $10.00 — records prev reading.
	if active, usd := s.ExtraUsageTick(10.00, 99.0, true); active || usd != 0 {
		t.Fatalf("pre-cross at 99%%: active=%v usd=%.4f; want false 0", active, usd)
	}

	// Tick 2: the crossing turn costs $0.40, taking the window 99→103.
	active, usd := s.ExtraUsageTick(10.40, 103.0, true)
	if !active {
		t.Fatalf("crossing: active=false; want true")
	}
	const wantTail = 0.30 // 3/4 × 0.40
	if usd < wantTail-0.0001 || usd > wantTail+0.0001 {
		t.Errorf("crossing tail: extra=%.4f; want %.4f (3/4 × $0.40)", usd, wantTail)
	}

	// Tick 3: spend grows by $0.10 → extra accrues to $0.40 (tail $0.30 + new $0.10).
	if active, usd = s.ExtraUsageTick(10.50, 103.0, true); !active || usd < 0.3999 || usd > 0.4001 {
		t.Errorf("accrue after crossing: active=%v extra=%.4f; want true ~0.40", active, usd)
	}
}

// TestExtraUsage_ClippedAt100NoTail verifies that when CC clips the reported
// percentage at exactly 100 (pct never exceeds 100), the proportional tail is 0
// and the badge behaves as before B4: baseline = full SessionTotal, extra $0 at
// the crossing tick.
func TestExtraUsage_ClippedAt100NoTail(t *testing.T) {
	s := &state.Session{}
	s.ExtraUsageTick(10.00, 98.0, true) // prev reading under quota
	if active, usd := s.ExtraUsageTick(10.40, 100.0, true); !active || usd != 0 {
		t.Errorf("clipped crossing: active=%v extra=%.4f; want true 0 (no visible tail)", active, usd)
	}
	if s.OverageBaseline != 10.40 {
		t.Errorf("clipped crossing baseline=%.4f; want 10.40 (full)", s.OverageBaseline)
	}
}
