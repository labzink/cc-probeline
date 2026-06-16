// Package config_test — strict-ordering tests for the per-window quota colour
// ratios (Phase 7.47 config-honesty). Each window's notice < warn < critical must
// hold strictly; violations surface as SeverityError and ApplyRangeFix resets the
// offending trio to defaults so the status line never renders a broken config.
package config_test

import (
	"testing"

	"github.com/labzink/cc-probeline/internal/config"
)

// A strictly-increasing trio on both windows is accepted without error.
func TestValidate_QuotaRatios_ValidTrios(t *testing.T) {
	cfg := config.Default()
	if errs := errsBySeverity(config.Validate(cfg), config.SeverityError); len(errs) != 0 {
		t.Fatalf("default quota trios produced %d SeverityError(s): %v", len(errs), errs)
	}
}

// 5h warn >= critical breaks strict ordering → SeverityError.
func TestValidate_QuotaRatios_5hNotIncreasing(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.Quota5hWarnRatio = 0.90
	cfg.Thresholds.Quota5hCriticalRatio = 0.90 // equal → not strictly increasing

	if errs := errsBySeverity(config.Validate(cfg), config.SeverityError); len(errs) == 0 {
		t.Error("expected SeverityError when quota_5h warn == critical, got none")
	}
}

// ApplyRangeFix resets only the broken 7d trio to defaults, leaving 5h untouched.
func TestApplyRangeFix_QuotaRatios_Resets7dTrio(t *testing.T) {
	cfg := config.Default()
	cfg.Thresholds.Quota5hNoticeRatio = 0.40 // valid custom 5h trio (0.40 < 0.70 < 0.90)
	cfg.Thresholds.Quota7dNoticeRatio = 0.95 // invalid: notice > warn → 7d trio broken

	config.ApplyRangeFix(cfg)
	def := config.Default()

	if cfg.Thresholds.Quota5hNoticeRatio != 0.40 {
		t.Errorf("valid 5h trio must survive: notice=%.2f", cfg.Thresholds.Quota5hNoticeRatio)
	}
	if cfg.Thresholds.Quota7dNoticeRatio != def.Thresholds.Quota7dNoticeRatio ||
		cfg.Thresholds.Quota7dWarnRatio != def.Thresholds.Quota7dWarnRatio ||
		cfg.Thresholds.Quota7dCriticalRatio != def.Thresholds.Quota7dCriticalRatio {
		t.Errorf("broken 7d trio not reset to defaults: notice=%.2f warn=%.2f critical=%.2f",
			cfg.Thresholds.Quota7dNoticeRatio, cfg.Thresholds.Quota7dWarnRatio, cfg.Thresholds.Quota7dCriticalRatio)
	}
}
