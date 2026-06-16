// Package config_test — strict-ordering tests for the three ctx colour ratios
// (Phase 7.47 config-honesty). notice < warn < critical must hold strictly;
// violations surface as SeverityError and ApplyRangeFix resets the whole trio to
// the baked defaults so the status line never renders a broken configuration.
package config_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// A strictly-increasing trio is accepted without error.
func TestValidate_CtxRatios_ValidTrio(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxNoticeRatio = 0.50
	cfg.Thresholds.CtxWarnRatio = 0.70
	cfg.Thresholds.CtxCriticalRatio = 0.90

	if errs := errsBySeverity(config.Validate(cfg), config.SeverityError); len(errs) != 0 {
		t.Fatalf("valid trio produced %d SeverityError(s): %v", len(errs), errs)
	}
}

// notice >= warn breaks the strict ordering → SeverityError.
func TestValidate_CtxRatios_NoticeNotBelowWarn(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxNoticeRatio = 0.75 // >= warn
	cfg.Thresholds.CtxWarnRatio = 0.70
	cfg.Thresholds.CtxCriticalRatio = 0.90

	if errs := errsBySeverity(config.Validate(cfg), config.SeverityError); len(errs) == 0 {
		t.Error("expected SeverityError when notice >= warn, got none")
	}
}

// Equal warn == critical is rejected (must strictly increase, not just be sorted).
func TestValidate_CtxRatios_WarnEqualsCritical(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxNoticeRatio = 0.50
	cfg.Thresholds.CtxWarnRatio = 0.80
	cfg.Thresholds.CtxCriticalRatio = 0.80 // equal → not strictly increasing

	if errs := errsBySeverity(config.Validate(cfg), config.SeverityError); len(errs) == 0 {
		t.Error("expected SeverityError when warn == critical, got none")
	}
}

// ApplyRangeFix resets all three ratios to defaults when the trio is not strictly
// increasing, listing each field as fixed.
func TestApplyRangeFix_CtxRatios_ResetsTrio(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.CtxNoticeRatio = 0.80 // notice > warn → invalid trio
	cfg.Thresholds.CtxWarnRatio = 0.70
	cfg.Thresholds.CtxCriticalRatio = 0.90

	fixed := config.ApplyRangeFix(cfg)
	def := config.Default()

	if cfg.Thresholds.CtxNoticeRatio != def.Thresholds.CtxNoticeRatio ||
		cfg.Thresholds.CtxWarnRatio != def.Thresholds.CtxWarnRatio ||
		cfg.Thresholds.CtxCriticalRatio != def.Thresholds.CtxCriticalRatio {
		t.Errorf("trio not reset to defaults: notice=%.2f warn=%.2f critical=%.2f",
			cfg.Thresholds.CtxNoticeRatio, cfg.Thresholds.CtxWarnRatio, cfg.Thresholds.CtxCriticalRatio)
	}

	for _, want := range []string{
		"thresholds.ctx_notice_ratio",
		"thresholds.ctx_warn_ratio",
		"thresholds.ctx_critical_ratio",
	} {
		found := false
		for _, f := range fixed {
			if f == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in fixed fields %v", want, fixed)
		}
	}
}
