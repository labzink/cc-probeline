// Package probes_test — black-box tests for SubagentProbe (Phase 4.1.c RED).
//
// Sources:
//   - plans/concepts/phase-4-architecture.md §4.1.c (lines 542-611)
//   - plans/tasks/phase-4-step1-plan.md §Subtask 4.1.c (lines 442-448, 464-485)
//   - plans/concepts/phase-4-architecture.md §4.0 (lines 228-285) — resolver design
//
// SubagentProbe contract:
//
//	Visible(d, cfg) == false  when len(d.Stdin.Tasks) == 0
//	Render returns one line per task joined with "\n"
//	resolver: findByAgentID(d.Subagents, task.ID)
//	  match:   "<name> · <model> · <ctx>/<max> · $<cost> · ⏱<time> · <last_tool>"
//	  no match: slog.Warn("probes.subagent: task.ID not matched", ...) + stdin-only fallback
//	  Level=Compact: drop last_tool (5 fields)
//	  Level=Minimal: 3 fields "<name> · <model> · <ctx>/<max>"
//
// All five tests compile-fail in RED because probes.SubagentProbe does not
// exist yet — that is the intended RED state.
package probes_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// captureHandler is a minimal slog.Handler that records log records so that
// tests can assert on warning messages without I/O side-effects.
// Copied verbatim from tests/stdin/payload_test.go lines 18-41.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// hasWarnContaining reports whether any captured record at Warn level
// has a message containing substr.
func (h *captureHandler) hasWarnContaining(substr string) bool {
	for _, r := range h.records {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, substr) {
			return true
		}
	}
	return false
}

// TestSubagent_NoTasks verifies that SubagentProbe.Visible returns false when
// Stdin.Tasks is nil — no subagent widget should be rendered.
//
// PLAN line 443: Stdin.Tasks=nil → Visible()=false.
func TestSubagent_NoTasks(t *testing.T) {
	p := &probes.SubagentProbe{}
	cfg := probes.Config{}
	d := probes.Data{
		Stdin: stdin.Payload{Tasks: nil},
	}

	got := p.Visible(d, cfg)
	if got != false {
		t.Errorf("Visible(Tasks=nil): want false, got true")
	}
}

// TestSubagent_OneTask_Match verifies that a single task matched to a subagent
// produces one enriched output line (no newline) containing the task name,
// model, last tool, and the " · " separator.
//
// PLAN line 444: one matched task → single line with enriched fields.
// PLAN line 480: "<task.Name> · <model> · <ctx>/<max> · $<cost> · ⏱<time> · <last_tool>"
func TestSubagent_OneTask_Match(t *testing.T) {
	p := &probes.SubagentProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := probes.Data{
		Stdin: stdin.Payload{
			Tasks: []stdin.Task{
				{ID: "a1", Name: "reviewer"},
			},
		},
		Subagents: []parser.SubagentStats{
			{
				AgentID:  "a1",
				Model:    "sonnet-4-6",
				Tokens:   parser.TokenCounts{Input: 50000},
				LastTool: "Read",
			},
		},
	}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// Must be a single line — no newlines.
	if strings.Contains(got, "\n") {
		t.Errorf("Render(Full, 1 task): want single line, got newline in %q", got)
	}
	// Must contain task name, model, last tool, and separator.
	for _, substr := range []string{"reviewer", "sonnet-4-6", "Read", " · "} {
		if !strings.Contains(got, substr) {
			t.Errorf("Render(Full, 1 task): want %q in output, got %q", substr, got)
		}
	}
}

// TestSubagent_TwoTasks_BothMatch verifies that two matched tasks produce
// exactly two output lines in the same order as the input tasks slice.
//
// PLAN line 445: 2 tasks → 2 lines separated by "\n"; order preserved.
func TestSubagent_TwoTasks_BothMatch(t *testing.T) {
	p := &probes.SubagentProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}
	d := probes.Data{
		Stdin: stdin.Payload{
			Tasks: []stdin.Task{
				{ID: "a1", Name: "t1"},
				{ID: "a2", Name: "t2"},
			},
		},
		Subagents: []parser.SubagentStats{
			{AgentID: "a1", Model: "sonnet-4-6", LastTool: "Bash"},
			{AgentID: "a2", Model: "haiku-4-5", LastTool: "Read"},
		},
	}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// Exactly one "\n" → two lines.
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Errorf("Render(Full, 2 tasks): want exactly 2 lines, got %d in %q", len(lines), got)
	}
	// Order must match task slice order.
	if !strings.Contains(lines[0], "t1") {
		t.Errorf("line[0]: want %q, got %q", "t1", lines[0])
	}
	if !strings.Contains(lines[1], "t2") {
		t.Errorf("line[1]: want %q, got %q", "t2", lines[1])
	}
}

// TestSubagent_NoMatch_Fallback verifies that when a task's ID does not match
// any AgentID in d.Subagents the probe emits a slog.Warn with the exact message
// "probes.subagent: task.ID not matched" and renders a fallback line that
// contains the task name and "?" placeholders.
//
// PLAN line 446: Tasks=[{ID:"unknown"}], Subagents=[] →
//
//	slog.Warn("probes.subagent: task.ID not matched", ...)
//	render: "<name> · ? · ? · $? · ⏱<elapsed> · ?"
func TestSubagent_NoMatch_Fallback(t *testing.T) {
	p := &probes.SubagentProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	// Install capture handler before Render to intercept slog output.
	h := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	d := probes.Data{
		Stdin: stdin.Payload{
			Tasks: []stdin.Task{
				{ID: "unknown", Name: "orphan"},
			},
		},
		Subagents: []parser.SubagentStats{},
	}

	got := p.Render(d, cfg, th, probes.LevelFull)

	// (a) slog.Warn must have been emitted with the exact match-failure message.
	const wantWarn = "probes.subagent: task.ID not matched"
	if !h.hasWarnContaining(wantWarn) {
		t.Errorf("expected slog.Warn containing %q, got records: %v", wantWarn, h.records)
	}

	// (b) Fallback render must contain task name and "?" placeholder.
	if !strings.Contains(got, "orphan") {
		t.Errorf("fallback render: want task name %q in output, got %q", "orphan", got)
	}
	if !strings.Contains(got, "?") {
		t.Errorf("fallback render: want %q placeholder in output, got %q", "?", got)
	}
}

// TestSubagent_Levels verifies that the three display levels drop fields as
// specified in the PLAN:
//
//	Full (LevelFull):    6 fields — includes last_tool.
//	Compact (LevelCompact): 5 fields — last_tool dropped.
//	Minimal (LevelMinimal): 3 fields — "<name> · <model> · <ctx>/<max>" only.
//
// PLAN line 447: Full=6 fields, Compact=5 (drop last_tool), Minimal=3.
// PLAN lines 483-485: explicit field sets per level.
func TestSubagent_Levels(t *testing.T) {
	p := &probes.SubagentProbe{}
	cfg := probes.Config{}
	th := renderer.Theme{}

	const lastTool = "Read"
	d := probes.Data{
		Stdin: stdin.Payload{
			Tasks: []stdin.Task{
				{ID: "a1", Name: "devagent"},
			},
		},
		Subagents: []parser.SubagentStats{
			{
				AgentID:  "a1",
				Model:    "sonnet-4-6",
				Tokens:   parser.TokenCounts{Input: 80000},
				LastTool: lastTool,
			},
		},
	}

	// LevelFull must include the last_tool value.
	full := p.Render(d, cfg, th, probes.LevelFull)
	if !strings.Contains(full, lastTool) {
		t.Errorf("Render(Full): want last_tool %q in output, got %q", lastTool, full)
	}

	// LevelCompact must NOT include the last_tool value (it is dropped).
	compact := p.Render(d, cfg, th, probes.LevelCompact)
	if strings.Contains(compact, lastTool) {
		t.Errorf("Render(Compact): want last_tool %q dropped, still present in %q",
			lastTool, compact)
	}

	// LevelMinimal must have exactly 2 " · " separators (3 fields).
	minimal := p.Render(d, cfg, th, probes.LevelMinimal)
	separatorCount := strings.Count(minimal, " · ")
	if separatorCount != 2 {
		t.Errorf("Render(Minimal): want exactly 2 \" · \" separators (3 fields), got %d in %q",
			separatorCount, minimal)
	}
	// Model and task name must be present in Minimal output.
	if !strings.Contains(minimal, "devagent") {
		t.Errorf("Render(Minimal): want task name %q in output, got %q", "devagent", minimal)
	}
	if !strings.Contains(minimal, "sonnet-4-6") {
		t.Errorf("Render(Minimal): want model %q in output, got %q", "sonnet-4-6", minimal)
	}
}
