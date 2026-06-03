// Package statusline_test — Phase 6.9 FIXES, group G-ttl.
//
// F3: Cache-TTL window is 5 minutes (Claude API prompt-cache), independent of
// Config.OrchTTLMinutes (which is the orchestrator idle-warning timeout, default 60).
// GREEN will introduce Config.CacheTTLMinutes (default 5). These tests assert the
// 5-min-window outcome and are RED until GREEN sets that default.
//
// F10: The # cell of a subagent row uses Unicode subscript digits (U+2080..2089)
// instead of ASCII digits. Current code emits "↳5" (ASCII); contract is "↳₅".
// Tests are RED until GREEN replaces fmt.Sprintf("↳%d",...) with subscript mapping.
//
// All tests go through Assembler.Render (production path).
package statusline_test

import (
	"strings"
	"testing"
	"time"

	"github.com/labzink/cc-probeline/internal/mode"
	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/statusline"
)

// f3DefaultAssembler returns a Standard-mode Assembler with DEFAULT config:
// OrchTTLMinutes=60 (idle-warning timeout), CacheTTLMinutes not set yet.
// F3 contract: the CACHE window must be 5 min regardless of OrchTTLMinutes.
// GREEN must add CacheTTLMinutes field (default 5) to probes.Config.
func f3DefaultAssembler() *statusline.Assembler {
	return &statusline.Assembler{
		Mode:  mode.Standard,
		Theme: renderer.Theme{AnsiEnabled: false},
		Cols:  80,
		// OrchTTLMinutes=60 is the production default (orchestrator idle timeout).
		// CacheTTLMinutes is intentionally NOT set — GREEN will add it with default=5.
		Config: probes.Config{OrchTTLMinutes: 60},
	}
}

// f3RenderSubagent renders a single orch turn + one subagent via the default
// (OrchTTLMinutes=60) assembler and returns the full output.
func f3RenderSubagent(t *testing.T, subStats parser.SubagentStats, now time.Time) string {
	t.Helper()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	orchT := orchTurn(1, "orch1", 1, base, "BashOrch", "")
	a := f3DefaultAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount:     1,
			Turns:         []parser.Turn{orchT},
			LastTimestamp: base,
		},
		Subagents:    []parser.SubagentStats{subStats},
		Now:          now,
		TerminalCols: 80,
	}
	return a.Render(d)
}

// ---------------------------------------------------------------------------
// F3(a): TestF3_SubagentTTLReflects5MinWindow
//
// Spec F3: a subagent whose last turn was ~1 min before now must show a TTL
// suffix reflecting the 5-minute cache window (remaining ≈ 4m, i.e. "⏱ 4m").
// It must NOT show "⏱ 59m" or "⏱ 60m" (which would mean the 60-min orch timeout
// is used for the cache window instead of the 5-min cache window).
//
// Manual calculation (CacheTTLMinutes=5, elapsed=1min):
//
//	remaining = 5 − 1 = 4 → "⏱ 4m"
//
// Current code passes OrchTTLMinutes=60 → remaining=60−1=59 → "⏱ 59m" → RED.
// GREEN sets CacheTTLMinutes=5 → remaining=4 → "⏱ 4m" → passes.
// ---------------------------------------------------------------------------
func TestF3_SubagentTTLReflects5MinWindow(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	// Subagent last turn at base; now = base+1min → elapsed=1min.
	lastTS := base
	now := base.Add(1 * time.Minute)

	subTurn := parser.Turn{
		Role:        "agent",
		UUID:        "f3a-t1",
		GroupID:     0,
		Timestamp:   lastTS,
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     "F3ATool",
		IsSidechain: true,
	}
	subStats := parser.SubagentStats{
		AgentID:         "agent-f3a",
		AgentType:       "agent",
		ActivationStart: lastTS,
		LastTimestamp:   lastTS,
		CurrentTurnNum:  1,
		TurnCount:       1,
		Turns:           []parser.Turn{subTurn},
	}

	out := f3RenderSubagent(t, subStats, now)

	// Locate the subagent row (identified by "↳" in the # cell).
	subRowLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "↳") {
			subRowLine = l
			break
		}
	}
	if subRowLine == "" {
		t.Fatalf("F3(a): subagent row (with '↳') not found\noutput:\n%s", out)
	}

	// Contract: TTL must reflect 5-min cache window → remaining=4 → "⏱ 4m".
	if !strings.Contains(subRowLine, "⏱ 4m") {
		t.Errorf("F3(a): subagent row must contain '⏱ 4m' (5-min cache window, elapsed=1min);\n"+
			"got row: %s\nNote: '⏱ 59m' or '⏱ 60m' indicates OrchTTLMinutes=60 used for cache (bug).\nfull output:\n%s",
			subRowLine, out)
	}

	// Explicitly guard against the 60-min bug.
	if strings.Contains(subRowLine, "⏱ 59m") || strings.Contains(subRowLine, "⏱ 60m") {
		t.Errorf("F3(a): subagent TTL must NOT be '⏱ 59m'/'⏱ 60m' — OrchTTLMinutes=60 must not control cache window;\n"+
			"row: %s", subRowLine)
	}
}

// ---------------------------------------------------------------------------
// F3(b): TestF3_SubagentRedCacheWriteAt5MinGap
//
// Spec F3: two subagent turns ~6 min apart → the subagent row's cache_create
// is wrapped in {{color:red}} (gap ≥ 5-min cache window → cache collapse).
// Current code checks gap ≥ OrchTTLMinutes=60 → 6min < 60min → no red → RED.
// GREEN sets CacheTTLMinutes=5 → 6min ≥ 5min → red → passes.
//
// Manual (CacheTTLMinutes=5): gap=6min ≥ 5min → CacheExpiredAt → true → red.
// ---------------------------------------------------------------------------
func TestF3_SubagentRedCacheWriteAt5MinGap(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Two subagent turns: turn1 at base, turn2 at base+6min.
	// Gap between them = 6min ≥ 5-min cache window → red cache_create on subagent row.
	turn1 := parser.Turn{
		Role:        "agent",
		UUID:        "f3b-t1",
		GroupID:     0,
		Timestamp:   base,
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     "F3BTool1",
		IsSidechain: true,
	}
	turn2 := parser.Turn{
		Role:        "agent",
		UUID:        "f3b-t2",
		GroupID:     0,
		Timestamp:   base.Add(6 * time.Minute),
		Tokens:      parser.TokenCounts{Output: 75, CacheCreate: 750},
		ToolUse:     "F3BTool2",
		IsSidechain: true,
	}
	subStats := parser.SubagentStats{
		AgentID:         "agent-f3b",
		AgentType:       "agent",
		ActivationStart: base,
		LastTimestamp:   base.Add(6 * time.Minute),
		CurrentTurnNum:  2,
		TurnCount:       2,
		Turns:           []parser.Turn{turn1, turn2},
	}

	// now = last turn + 1min (subagent TTL alive within 5-min window → 4m left).
	now := base.Add(7 * time.Minute)
	out := f3RenderSubagent(t, subStats, now)

	// Locate subagent row (contains "↳").
	subRowLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "↳") {
			subRowLine = l
			break
		}
	}
	if subRowLine == "" {
		t.Fatalf("F3(b): subagent row (with '↳') not found\noutput:\n%s", out)
	}

	// Contract: gap=6min ≥ CacheTTLMinutes=5 → cache_create must be {{color:red}}.
	if !strings.Contains(subRowLine, "{{color:red}}") {
		t.Errorf("F3(b): subagent row must contain '{{color:red}}' on cache_create (gap=6min ≥ 5-min cache window);\n"+
			"got row: %s\nNote: absence means OrchTTLMinutes=60 is used (gap=6 < 60 → no red).\nfull output:\n%s",
			subRowLine, out)
	}

	// TTL must NOT be frozen "⏱ 0m" — subagents never freeze (spec §2.3).
	if strings.Contains(subRowLine, "⏱ 0m") {
		t.Errorf("F3(b): subagent row must NOT show frozen '⏱ 0m' (no freeze for subagents per spec);\n"+
			"row: %s", subRowLine)
	}
}

// ---------------------------------------------------------------------------
// F3(c): TestF3_OrchFreezAt5MinGap
//
// Spec F3: an older orch turn whose gap to the next-newer orch turn is ≥ 5min
// → its TTL is frozen "⏱ 0m". Current code checks gap ≥ OrchTTLMinutes=60 →
// 6min < 60min → no freeze → RED.
// GREEN sets CacheTTLMinutes=5 → 6min ≥ 5min → freeze → passes.
//
// Manual (CacheTTLMinutes=5, gap=6min):
//
//	remaining = 5 − 6 = −1 ≤ 0 → FREEZE prev row: "⏱ 0m".
//
// Contrast with OrchTTLMinutes=60:
//
//	remaining = 60 − 6 = 54 > 0 → no freeze (bug).
//
// ---------------------------------------------------------------------------
func TestF3_OrchFreezeAt5MinGap(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// prev orch turn at base, cur orch turn at base+6min.
	prevTS := base
	curTS := base.Add(6 * time.Minute)

	// Newest-first order: cur, prev.
	turns := []parser.Turn{
		orchTurn(2, "f3c-cur", 2, curTS, "F3CTool_cur", ""),
		orchTurn(1, "f3c-prev", 1, prevTS, "F3CTool_prev", ""),
	}

	// now = curTS+1s (so fresh cur row has a live TTL, not expired).
	now := curTS.Add(time.Second)

	a := f3DefaultAssembler()
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount:     2,
			Turns:         turns,
			LastTimestamp: curTS,
		},
		Now:          now,
		TerminalCols: 80,
	}
	out := a.Render(d)

	// Locate the prev row (identified by "F3CTool_prev" in the tool cell).
	prevRowLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "F3CTool_prev") {
			prevRowLine = l
			break
		}
	}
	if prevRowLine == "" {
		t.Fatalf("F3(c): prev row ('F3CTool_prev') not found\noutput:\n%s", out)
	}

	// Contract: gap=6min ≥ CacheTTLMinutes=5 → prev row frozen → "⏱ 0m".
	if !strings.Contains(prevRowLine, "⏱ 0m") {
		t.Errorf("F3(c): prev orch row must contain '⏱ 0m' (frozen; gap=6min ≥ 5-min cache window);\n"+
			"got row: %s\nNote: absence means OrchTTLMinutes=60 used (gap=6 < 60 → no freeze).\nfull output:\n%s",
			prevRowLine, out)
	}
}

// ---------------------------------------------------------------------------
// F10(a): TestF10_SubagentHashCellSubscriptSingle
//
// Spec F10: the # cell of a subagent row = "↳" + Unicode subscript digit(s).
// Single-digit N=5 → "↳₅" (U+2085).
// Current code: fmt.Sprintf("↳%d", 5) → "↳5" (ASCII '5') → RED.
// GREEN maps each digit to subscript rune U+2080..2089.
//
// Route: via Assembler.Render (production path). The # cell content is read
// from the first column cell of the subagent row (line containing "↳").
// ---------------------------------------------------------------------------
func TestF10_SubagentHashCellSubscriptSingle(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	orchT := orchTurn(1, "orch1", 1, base, "BashOrch", "")

	// Subagent with CurrentTurnNum=5 → hash cell must be "↳₅".
	subTurn := parser.Turn{
		Role:        "agent",
		UUID:        "f10a-t1",
		GroupID:     0,
		Timestamp:   base.Add(time.Second),
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     "F10ATool",
		IsSidechain: true,
	}
	subStats := parser.SubagentStats{
		AgentID:         "agent-f10a",
		AgentType:       "agent",
		ActivationStart: base.Add(time.Second),
		LastTimestamp:   base.Add(time.Second),
		CurrentTurnNum:  5, // → subscript "₅"
		TurnCount:       5,
		Turns:           []parser.Turn{subTurn},
	}

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount:     1,
			Turns:         []parser.Turn{orchT},
			LastTimestamp: base,
		},
		Subagents:    []parser.SubagentStats{subStats},
		Now:          base.Add(2 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	// Locate the subagent row by "↳".
	subRowLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "↳") {
			subRowLine = l
			break
		}
	}
	if subRowLine == "" {
		t.Fatalf("F10(a): subagent row (with '↳') not found\noutput:\n%s", out)
	}

	// Contract: # cell must contain "↳₅" (Unicode subscript 5, U+2085), not "↳5".
	wantSubscript := "↳₅" // ↳ + U+2085
	if !strings.Contains(subRowLine, wantSubscript) {
		t.Errorf("F10(a): subagent # cell must contain %q (subscript '₅', U+2085);\n"+
			"got row: %s\nNote: '↳5' (ASCII) means subscript mapping not yet implemented.",
			wantSubscript, subRowLine)
	}

	// Ensure ASCII digit is NOT present in place of the subscript (disambiguation).
	// "↳5" without subscript = the bug we are fixing.
	if strings.Contains(subRowLine, "↳5") {
		t.Errorf("F10(a): subagent # cell must NOT contain ASCII '↳5'; must use subscript '↳₅';\n"+
			"row: %s", subRowLine)
	}
}

// ---------------------------------------------------------------------------
// F10(b): TestF10_SubagentHashCellSubscriptMultiDigit
//
// Spec F10: multi-digit N=12 → "↳₁₂" (U+2081 U+2082).
// Current code: "↳12" (ASCII) → RED.
// ---------------------------------------------------------------------------
func TestF10_SubagentHashCellSubscriptMultiDigit(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	orchT := orchTurn(1, "orch2", 1, base, "BashOrch2", "")

	// Subagent with CurrentTurnNum=12 → hash cell must be "↳₁₂".
	subTurn := parser.Turn{
		Role:        "agent",
		UUID:        "f10b-t1",
		GroupID:     0,
		Timestamp:   base.Add(time.Second),
		Tokens:      parser.TokenCounts{Output: 50, CacheCreate: 500},
		ToolUse:     "F10BTool",
		IsSidechain: true,
	}
	subStats := parser.SubagentStats{
		AgentID:         "agent-f10b",
		AgentType:       "agent",
		ActivationStart: base.Add(time.Second),
		LastTimestamp:   base.Add(time.Second),
		CurrentTurnNum:  12, // → subscript "₁₂"
		TurnCount:       12,
		Turns:           []parser.Turn{subTurn},
	}

	a := makeStdAssembler(5, 80)
	d := probes.Data{
		Session: &parser.SessionStats{
			TurnCount:     1,
			Turns:         []parser.Turn{orchT},
			LastTimestamp: base,
		},
		Subagents:    []parser.SubagentStats{subStats},
		Now:          base.Add(2 * time.Minute),
		TerminalCols: 80,
	}
	out := a.Render(d)

	// Locate subagent row.
	subRowLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "↳") {
			subRowLine = l
			break
		}
	}
	if subRowLine == "" {
		t.Fatalf("F10(b): subagent row (with '↳') not found\noutput:\n%s", out)
	}

	// Contract: # cell must contain "↳₁₂" (U+2081 + U+2082), not "↳12".
	wantSubscript := "↳₁₂" // ↳ + U+2081 + U+2082
	if !strings.Contains(subRowLine, wantSubscript) {
		t.Errorf("F10(b): subagent # cell must contain %q (subscript '₁₂', U+2081..U+2082);\n"+
			"got row: %s\nNote: '↳12' (ASCII) means subscript mapping not yet implemented.",
			wantSubscript, subRowLine)
	}

	// Guard against ASCII multi-digit form.
	if strings.Contains(subRowLine, "↳12") {
		t.Errorf("F10(b): subagent # cell must NOT contain ASCII '↳12'; must use '↳₁₂';\n"+
			"row: %s", subRowLine)
	}
}
