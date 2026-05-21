// Package statusline_test — Phase 4.4.d tests for hint widget integration in Assembler.
//
// Tests cover:
//   - Hint row appended when widget returns text (fresh state).
//   - Hint row absent when all hints shown (AllShown).
//   - Alert text surfaced when CacheEvent is present.
//   - Alert overrides rotation even when all hints shown.
//   - Memory-only mode (SessionID="") works without writing a file.
//   - Compile-time check that Assembler implements statusline.Renderer.
package statusline_test

import (
	"os"
	"path/filepath"
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

// ---------------------------------------------------------------------------
// compile-time guard: Assembler must implement Renderer (spec-S6)
// ---------------------------------------------------------------------------

var _ statusline.Renderer = (*statusline.Assembler)(nil)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeHintData builds probes.Data with a session that has the given sessionID
// and cache events. CacheHome is set so hint.Save writes to a temp dir.
func makeHintData(t *testing.T, sessionID string, events []parser.CacheEvent) probes.Data {
	t.Helper()
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	session := &parser.SessionStats{
		TurnCount:   0,
		CacheEvents: events,
	}
	return probes.Data{
		Session:      session,
		SessionID:    sessionID,
		Now:          time.Now(),
		TerminalCols: 80,
	}
}

// makeHintAssembler returns a SuperCompact Assembler (no table) so the hint
// row, if present, is always the last line.
func makeHintAssembler() *statusline.Assembler {
	return &statusline.Assembler{
		Mode:   mode.SuperCompact,
		Theme:  renderer.Theme{},
		Cols:   80,
		Config: probes.Config{},
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_HintAppended_WhenAvailable
// Fresh state (no file) → DefaultHints[0].Text must appear as the last line.
// ---------------------------------------------------------------------------

func TestAssembler_Render_HintAppended_WhenAvailable(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "c"}})

	a := makeHintAssembler()
	d := makeHintData(t, "test-session-1", nil)

	out := a.Render(d)
	lines := strings.Split(out, "\n")

	// Must have at least 4 lines: line0 + line1 + line2 + hint.
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (3 header + hint), got %d; output: %q", len(lines), out)
	}
	// Last line must be the first default hint text.
	lastLine := lines[len(lines)-1]
	wantHint := hint.DefaultHints[0].Text
	if lastLine != wantHint {
		t.Errorf("last line = %q, want hint text %q", lastLine, wantHint)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_HintHidden_WhenAllShown
// State with all 8 hints shown → output must NOT contain a hint row beyond line2.
// ---------------------------------------------------------------------------

func TestAssembler_Render_HintHidden_WhenAllShown(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "c"}})

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	// Pre-populate state with all 8 shown indices.
	shown := make([]int, len(hint.DefaultHints))
	for i := range shown {
		shown[i] = i
	}
	state := hint.State{
		ShownIndices: shown,
		CurrentIndex: len(hint.DefaultHints) - 1,
		LastSwitch:   time.Now(),
	}
	const sid = "test-session-2"
	if err := hint.Save(sid, state); err != nil {
		t.Fatalf("hint.Save: %v", err)
	}

	a := makeHintAssembler()
	d := probes.Data{
		Session:      &parser.SessionStats{},
		SessionID:    sid,
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)
	lines := strings.Split(out, "\n")

	// SuperCompact with no table and no hint: exactly 3 lines (2 newlines).
	if len(lines) != 3 {
		t.Errorf("expected 3 lines when all hints shown, got %d; output: %q", len(lines), out)
	}
	// None of the hint texts must appear.
	for _, h := range hint.DefaultHints {
		if strings.Contains(out, h.Text) {
			t.Errorf("expected hint text %q to be absent (all shown), but found it in output: %q", h.Text, out)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_HintAlert_OnCacheEvent
// CacheEvent{OrchTTL} → output must contain the alert text.
// ---------------------------------------------------------------------------

func TestAssembler_Render_HintAlert_OnCacheEvent(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "c"}})

	a := makeHintAssembler()
	d := makeHintData(t, "test-session-3", []parser.CacheEvent{
		{Type: parser.OrchTTL},
	})

	out := a.Render(d)
	wantAlert := hint.AlertTexts[parser.OrchTTL]
	if !strings.Contains(out, wantAlert) {
		t.Errorf("expected alert text %q in output; got: %q", wantAlert, out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_HintAlert_OverridesRotation
// All hints shown + ModelSwitched event → alert text must appear (not empty).
// ---------------------------------------------------------------------------

func TestAssembler_Render_HintAlert_OverridesRotation(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "c"}})

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	// All hints shown — normally hint row is hidden.
	shown := make([]int, len(hint.DefaultHints))
	for i := range shown {
		shown[i] = i
	}
	state := hint.State{
		ShownIndices: shown,
		CurrentIndex: len(hint.DefaultHints) - 1,
		LastSwitch:   time.Now(),
	}
	const sid = "test-session-4"
	if err := hint.Save(sid, state); err != nil {
		t.Fatalf("hint.Save: %v", err)
	}

	a := makeHintAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			CacheEvents: []parser.CacheEvent{
				{Type: parser.ModelSwitched, Detail: "opus -> sonnet"},
			},
		},
		SessionID:    sid,
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)
	// Alert must override the "all shown" hide.
	if strings.Count(out, "\n") < 3 {
		t.Fatalf("expected at least 4 lines (3 header + alert), got output: %q", out)
	}
	// Alert text for ModelSwitched includes "opus -> sonnet" interpolated.
	if !strings.Contains(out, "Cache rebuilt") {
		t.Errorf("expected alert 'Cache rebuilt' in output; got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// TestAssembler_Render_HintMemoryOnly_EmptySessionID
// SessionID="" → hint still works (in-memory), no file written.
// ---------------------------------------------------------------------------

func TestAssembler_Render_HintMemoryOnly_EmptySessionID(t *testing.T) {
	swapLine0(t, []probes.Probe{&fakeProbe{name: "e", visible: true, out: "e"}})
	swapLine1(t, []probes.Probe{&fakeProbe{name: "m", visible: true, out: "m"}})
	swapLine2(t, []probes.Probe{&fakeProbe{name: "c", visible: true, out: "c"}})

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	a := makeHintAssembler()
	d := probes.Data{
		Session:      &parser.SessionStats{},
		SessionID:    "", // memory-only
		Now:          time.Now(),
		TerminalCols: 80,
	}

	out := a.Render(d)

	// Hint must still appear (memory-only rotation).
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Errorf("expected hint row even with empty SessionID, got %d lines; output: %q", len(lines), out)
	}

	// No hint-*.json file must have been created in the cache dir.
	ccCacheDir := filepath.Join(cacheHome, "cc-probeline")
	entries, err := os.ReadDir(ccCacheDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "hint-") && strings.HasSuffix(e.Name(), ".json") {
			t.Errorf("unexpected hint state file written for empty SessionID: %s", e.Name())
		}
	}
}
