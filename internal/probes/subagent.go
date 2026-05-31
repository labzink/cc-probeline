package probes

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

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
func (p *SubagentProbe) Priority() int { return 4 }
func (p *SubagentProbe) MinWidth() int { return 40 } // <name>·<model>·<ctx> minimum

// Visible returns false when SubagentEnabled is false or Stdin.Tasks is empty or nil.
func (p *SubagentProbe) Visible(d Data, c Config) bool {
	if !c.SubagentEnabled {
		return false
	}
	return len(d.Stdin.Tasks) > 0
}

// Render produces one output line per task, joined with "\n".
func (p *SubagentProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	lines := make([]string, 0, len(d.Stdin.Tasks))
	for _, task := range d.Stdin.Tasks {
		stats, ok := findSubagentByID(d.Subagents, task.ID)
		var line string
		if ok {
			line = renderMatchedLine(task, stats, d.Now, level)
		} else {
			slog.Warn("probes.subagent: task.ID not matched",
				"taskID", task.ID,
				"taskName", task.Name,
				"knownAgentIDs", agentIDList(d.Subagents),
			)
			line = renderFallbackLine(task.Name, task.StartTime, d.Now, level)
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

// elapsedColour wraps the ⏱elapsed string with the appropriate colour marker:
//   - elapsed > 300s → red (long-running agent, spec §2.3)
//   - elapsed ≤ 300s → yellow (active agent, spec §2.3)
//
// The ⏱ glyph is included inside the colour span so the escape directly
// precedes the glyph after renderer.Apply (T-12 contract).
func elapsedColour(elapsed time.Duration) string {
	secs := int(elapsed.Seconds())
	formatted := "⏱" + formatElapsed(elapsed)
	if secs > 300 {
		return "{{color:red}}" + formatted + "{{reset}}"
	}
	return "{{color:yellow}}" + formatted + "{{reset}}"
}

// renderMatchedLine renders the enriched display line for a matched task+stats pair.
func renderMatchedLine(task stdin.Task, stats parser.SubagentStats, now time.Time, level Level) string {
	// Phase 4.1: context window size is unknown for subagents — use "?" as max.
	ctxK := formatK(stats.Tokens.Input)
	ctxField := ctxK + "/?"

	// BL-13: subagent per-turn cost not available; hardcoded $0.00
	costField := fmt.Sprintf("$%.2f", 0.0)

	// Elapsed time with colour marker (spec §2.3: >300s → red, ≤300s → yellow).
	elapsed := now.Sub(task.StartTime)
	timeField := elapsedColour(elapsed)

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
// elapsed is computed from task.StartTime; cost remains "?" (no source until Phase 6).
func renderFallbackLine(name string, startTime time.Time, now time.Time, level Level) string {
	timeField := elapsedColour(now.Sub(startTime))
	switch level {
	case LevelMinimal:
		return name + " · ? · ?"
	case LevelCompact:
		return name + " · ? · ? · $? · " + timeField
	default: // LevelFull
		return name + " · ? · ? · $? · " + timeField + " · ?"
	}
}

// formatElapsed formats a duration as "Nm SSs" (< 1h) or "NhMMm" (>= 1h).
// Matches spec §A3 line 146.
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSec := int(d.Seconds())
	if d < time.Hour {
		m := totalSec / 60
		s := totalSec % 60
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	return fmt.Sprintf("%dh %02dm", h, m)
}

// agentIDList returns a slice of AgentID strings from a SubagentStats slice.
// Used for structured slog logging when a task.ID lookup fails.
func agentIDList(subs []parser.SubagentStats) []string {
	ids := make([]string, len(subs))
	for i, s := range subs {
		ids[i] = s.AgentID
	}
	return ids
}
