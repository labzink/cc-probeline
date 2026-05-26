// Package statusline_test — Phase 6.d assembler config+ExtraCacheEvents tests
// (T-AC1..T-AC6).
//
// Tests cover:
//   - D1 guard: session-derived alerts are skipped when Turns == nil/empty.
//   - Config-error events (ExtraCacheEvents) ALWAYS surface, even on empty session.
//   - Nil Session defensive handling.
//   - Merge of all event sources.
//
// All tests in this file are intentionally RED until Phase 6.d GREEN lands,
// because assembler.hint() does not yet implement the D1 guard or ExtraCacheEvents merge.
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/hint"
	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// configAssembler returns a SuperCompact Assembler with default config for
// Phase 6.d tests.
func configAssembler() *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   mode.SuperCompact,
		Theme:  renderer.Theme{},
		Cols:   80,
		Config: probes.Config{},
	}
}

// oneTurn is a minimal Turn for tests that need a non-empty session.
var oneTurn = parser.Turn{
	Index: 1,
	Role:  "orch",
}

// ─── T-AC1: D1 guard — empty Turns → session-derived alerts skipped ──────────

// TestAssembler_HintSkipsAlertsOnEmptySession verifies the D1 guard (§11):
// when Session.Turns is nil/empty, session-derived CacheEvents (e.g. OrchTTL)
// must NOT appear in the hint output, even though CacheEvents is non-nil.
//
// Rationale: a newly opened session has no turns; surfacing cache alerts at
// turn-zero is noise (the cache hasn't been touched yet).
func TestAssembler_HintSkipsAlertsOnEmptySession(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	a := configAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			// Turns is nil — D1 guard must suppress CacheEvents below.
			CacheEvents: []parser.CacheEvent{
				{Type: parser.OrchTTL},
			},
		},
		SessionID:    "t-ac1-session",
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)

	// The OrchTTL alert text must NOT appear — D1 guard suppresses it.
	orchAlert := hint.AlertTexts[parser.OrchTTL]
	if strings.Contains(out, orchAlert) {
		t.Errorf("T-AC1: D1 guard: OrchTTL alert %q must be suppressed on empty session; got output: %q", orchAlert, out)
	}
	// "Cache rebuilt" is the common prefix of OrchTTL alert.
	if strings.Contains(out, "Cache rebuilt") {
		t.Errorf("T-AC1: D1 guard: 'Cache rebuilt' must not appear on empty session; got output: %q", out)
	}
}

// ─── T-AC2: non-empty Turns → session cache alert shown ─────────────────────

// TestAssembler_HintShowsCacheAlertOnNonEmptySession verifies that when the
// session HAS turns, the D1 guard does NOT suppress session CacheEvents.
// OrchTTL alert must appear in hint output.
func TestAssembler_HintShowsCacheAlertOnNonEmptySession(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	a := configAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			Turns: []parser.Turn{oneTurn}, // non-empty → D1 guard does NOT fire
			CacheEvents: []parser.CacheEvent{
				{Type: parser.OrchTTL},
			},
		},
		SessionID:    "t-ac2-session",
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)

	// OrchTTL alert must appear when session has turns.
	orchAlert := hint.AlertTexts[parser.OrchTTL]
	if !strings.Contains(out, orchAlert) {
		t.Errorf("T-AC2: OrchTTL alert %q must appear when session has turns; got output: %q", orchAlert, out)
	}
}

// ─── T-AC3: ExtraCacheEvents surface on EMPTY session ────────────────────────

// TestAssembler_HintShowsConfigErrorOnEmptySession verifies §11 (D1 T-13):
// ExtraCacheEvents (config-error injections) must ALWAYS surface, even when
// Session.Turns is nil/empty. Config errors are not session-derived — they
// originate from the config loader and must not be gated by D1.
func TestAssembler_HintShowsConfigErrorOnEmptySession(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	a := configAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			// Turns is nil — D1 guard fires for session events.
			CacheEvents: nil,
		},
		ExtraCacheEvents: []parser.CacheEvent{
			{Type: parser.ConfigError},
		},
		SessionID:    "t-ac3-session",
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)

	// ConfigError alert text must appear despite empty session.
	cfgAlert := hint.AlertTexts[parser.ConfigError]
	if cfgAlert == "" {
		// AlertTexts[ConfigError] not yet populated — GREEN scope will fix it.
		// We check for the "Config error" substring which is part of the planned text.
		if !strings.Contains(out, "Config error") {
			t.Errorf("T-AC3: 'Config error' must appear in output for ExtraCacheEvents on empty session; got: %q", out)
		}
	} else {
		if !strings.Contains(out, cfgAlert) {
			t.Errorf("T-AC3: ConfigError alert %q must appear for ExtraCacheEvents on empty session; got: %q", cfgAlert, out)
		}
	}
}

// ─── T-AC4: merge of all event sources → highest priority wins ───────────────

// TestAssembler_HintMergesAllSources verifies that when Turns is non-empty and
// all three sources (session events, subagent events, ExtraCacheEvents) are
// present, hint.BuildAlert receives a merged slice and returns the highest
// priority event (OrchTTL > ConfigError in criticalTypes order).
func TestAssembler_HintMergesAllSources(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// Subagent with SendMessageGap trigger (TurnCount>=2, span>5m).
	sa := parser.SubagentStats{
		AgentID:        "sub-merge",
		TurnCount:      2,
		FirstTimestamp: now.Add(-7 * time.Minute),
		LastTimestamp:  now,
	}

	a := configAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			Turns: []parser.Turn{oneTurn}, // non-empty — D1 does not fire
			CacheEvents: []parser.CacheEvent{
				{Type: parser.OrchTTL}, // highest priority in criticalTypes
			},
		},
		Subagents: []parser.SubagentStats{sa},
		ExtraCacheEvents: []parser.CacheEvent{
			{Type: parser.ConfigError},
		},
		SessionID:    "t-ac4-session",
		Now:          now,
		TerminalCols: 80,
	}

	out := a.Render(d)

	// OrchTTL is highest priority → its alert must appear.
	orchAlert := hint.AlertTexts[parser.OrchTTL]
	if !strings.Contains(out, orchAlert) {
		t.Errorf("T-AC4: merge: OrchTTL (highest priority) alert %q must appear; got: %q", orchAlert, out)
	}
}

// ─── T-AC5: nil Session with ExtraCacheEvents → config alert shown ───────────

// TestAssembler_HintNilSession_StillShowsExtraEvents verifies defensive handling:
// when d.Session is completely nil (not just empty), ExtraCacheEvents must still
// be processed and the ConfigError alert must appear. Prevents nil-dereference.
func TestAssembler_HintNilSession_StillShowsExtraEvents(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	a := configAssembler()
	d := probes.Data{
		Session: nil, // completely nil — defensive case
		ExtraCacheEvents: []parser.CacheEvent{
			{Type: parser.ConfigError},
		},
		SessionID:    "t-ac5-session",
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)

	// ConfigError alert must appear even when Session is nil.
	cfgAlert := hint.AlertTexts[parser.ConfigError]
	if cfgAlert == "" {
		if !strings.Contains(out, "Config error") {
			t.Errorf("T-AC5: 'Config error' must appear for ExtraCacheEvents when Session=nil; got: %q", out)
		}
	} else {
		if !strings.Contains(out, cfgAlert) {
			t.Errorf("T-AC5: ConfigError alert %q must appear when Session=nil; got: %q", cfgAlert, out)
		}
	}
}

// ─── T-AC6: empty ExtraCacheEvents + empty session → no false alert ──────────

// TestAssembler_HintEmptyExtraCacheEvents_NoFalseAlert verifies that when
// ExtraCacheEvents is nil AND the session is empty, no alert text appears at all.
// Guards against accidental injection of empty events producing garbage output.
func TestAssembler_HintEmptyExtraCacheEvents_NoFalseAlert(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	a := configAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			// Turns nil → D1 guard fires; CacheEvents nil.
		},
		ExtraCacheEvents: nil,
		SessionID:        "t-ac6-session",
		Now:              time.Now(),
		TerminalCols:     80,
	}

	out := a.Render(d)

	// None of the known alert texts must appear.
	for evType, alertText := range hint.AlertTexts {
		if alertText != "" && strings.Contains(out, alertText) {
			t.Errorf("T-AC6: no false alert: alert text for type %v (%q) must not appear with empty events; got output: %q", evType, alertText, out)
		}
	}
	// Specifically ensure "Config error" does not appear.
	if strings.Contains(out, "Config error") {
		t.Errorf("T-AC6: no false alert: 'Config error' must not appear with nil ExtraCacheEvents; got: %q", out)
	}
}
