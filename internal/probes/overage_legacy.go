//go:build legacy_overage

// laterReset picked which rate-limit window carried the "+$X extra usage" badge
// when both windows sat at 100%: the one resetting later, since it holds the
// overage longest. Phase 7.48 moved the badge out of any single window — the
// official figure is account-wide — so nothing calls this. Kept under the same
// tag as the estimator it belonged to (internal/state/overage_legacy.go).

package probes

import "time"

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
