package probes

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// SubagentProbe renders one line per active subagent task.
// It resolves each stdin Task to a SubagentStats entry by matching task.ID to
// SubagentStats.AgentID. Unmatched tasks produce a fallback line with "?"
// placeholders and emit slog.Warn.
//
// Visible only when len(d.Stdin.Tasks) > 0.
//
// Display (fields separated by " · "):
//
//	Full:    "<name> · <model> · <ctx>/<max> · $<cost> · ⏱<time> · <last_tool>"  (6 fields)
//	Compact: "<name> · <model> · <ctx>/<max> · $<cost> · ⏱<time>"               (5 fields, last_tool dropped)
//	Minimal: "<name> · <model> · <ctx>/<max>"                                     (3 fields)
type SubagentProbe struct{}

func (p *SubagentProbe) Name() string  { return "subagent" }
func (p *SubagentProbe) Priority() int { return 1 }
func (p *SubagentProbe) MinWidth() int { return len("? · ? · ?") }

// Visible returns false when Stdin.Tasks is empty or nil.
func (p *SubagentProbe) Visible(d Data, c Config) bool {
	return len(d.Stdin.Tasks) > 0
}

// Render produces one output line per task, joined with "\n".
func (p *SubagentProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	lines := make([]string, 0, len(d.Stdin.Tasks))
	for _, task := range d.Stdin.Tasks {
		stats, ok := findSubagentByID(d.Subagents, task.ID)
		var line string
		if ok {
			line = renderMatchedLine(task, stats, level)
		} else {
			slog.Warn("probes.subagent: task.ID not matched",
				"taskID", task.ID,
				"taskName", task.Name,
			)
			line = renderFallbackLine(task.Name, level)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// findSubagentByID returns the SubagentStats whose AgentID matches id.
func findSubagentByID(subagents []parser.SubagentStats, id string) (parser.SubagentStats, bool) {
	for _, s := range subagents {
		if s.AgentID == id {
			return s, true
		}
	}
	return parser.SubagentStats{}, false
}

// renderMatchedLine renders the enriched display line for a matched task+stats pair.
func renderMatchedLine(task stdin.Task, stats parser.SubagentStats, level Level) string {
	// Phase 4.1: context window size is unknown for subagents — use "?" as max.
	ctxK := formatK(stats.Tokens.Input)
	ctxField := ctxK + "/?"

	// Phase 4.1: no per-subagent cost or duration; use zero placeholders.
	costField := fmt.Sprintf("$%.2f", 0.0)
	timeField := "⏱00:00"

	lastTool := stats.LastTool
	if lastTool == "" {
		lastTool = "?"
	}

	name := task.Name

	switch level {
	case LevelMinimal:
		return name + " · " + stats.Model + " · " + ctxField
	case LevelCompact:
		return name + " · " + stats.Model + " · " + ctxField + " · " + costField + " · " + timeField
	default: // LevelFull
		return name + " · " + stats.Model + " · " + ctxField + " · " + costField + " · " + timeField + " · " + lastTool
	}
}

// renderFallbackLine renders a fallback line when no SubagentStats matched the task.
func renderFallbackLine(name string, level Level) string {
	switch level {
	case LevelMinimal:
		return name + " · ? · ?"
	case LevelCompact:
		return name + " · ? · ? · $? · ⏱?"
	default: // LevelFull
		return name + " · ? · ? · $? · ⏱? · ?"
	}
}
