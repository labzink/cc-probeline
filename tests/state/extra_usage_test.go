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
	active, usd := s.ExtraUsageTick(5.00, true, true)
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
	if active, usd = s.ExtraUsageTick(5.07, true, true); !active || usd < 0.0699 || usd > 0.0701 {
		t.Errorf("after +0.07: active=%v overage=%.4f; want true ~0.07", active, usd)
	}
	// And again: $5.14 → overage $0.14. Baseline must stay pinned at 5.00.
	if active, usd = s.ExtraUsageTick(5.14, true, true); usd < 0.1399 || usd > 0.1401 {
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
	active, usd := s.ExtraUsageTick(5.00, false, true)
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
	if active, _ := s.ExtraUsageTick(5.00, true, false); active {
		t.Errorf("at 100%% but hasExtra=false: active = true; want false")
	}
}

// TestExtraUsage_ClearsAndRebaselines verifies the "never sticky" rule: once the
// windows drop below 100% the baseline resets, and a later re-crossing snapshots
// a fresh baseline (overage counts from the new crossing, not the old one).
func TestExtraUsage_ClearsAndRebaselines(t *testing.T) {
	s := &state.Session{}

	// Arm at $5.00, accrue to $5.10.
	s.ExtraUsageTick(5.00, true, true)
	if _, usd := s.ExtraUsageTick(5.10, true, true); usd < 0.0999 || usd > 0.1001 {
		t.Fatalf("accrue: overage=%.4f; want ~0.10", usd)
	}

	// Windows reset below 100% → badge clears, baseline zeroed.
	if active, usd := s.ExtraUsageTick(5.10, false, true); active || usd != 0 {
		t.Errorf("clear: active=%v overage=%.4f; want false 0", active, usd)
	}
	if s.OverageBaseline != 0 {
		t.Errorf("clear: baseline=%.4f; want 0", s.OverageBaseline)
	}

	// Re-cross later at $8.00: fresh baseline, overage counts from here.
	if active, usd := s.ExtraUsageTick(8.00, true, true); !active || usd != 0 {
		t.Errorf("re-cross: active=%v overage=%.4f; want true 0 (new baseline)", active, usd)
	}
	if s.OverageBaseline != 8.00 {
		t.Errorf("re-cross baseline=%.4f; want 8.00", s.OverageBaseline)
	}
}
