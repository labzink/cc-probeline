package probes

import (
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/stdin"
)

// Level controls how aggressively a probe compresses its output to fit
// narrow terminals. Probes must handle all three levels independently.
type Level int

const (
	LevelFull    Level = iota // full form: labels + values + units
	LevelCompact              // compact: drop labels, keep values
	LevelMinimal              // minimal: drop non-critical blocks or abbreviate to bare minimum
)

// String returns the human-readable name used in slog messages and debug output.
func (l Level) String() string {
	switch l {
	case LevelFull:
		return "full"
	case LevelCompact:
		return "compact"
	case LevelMinimal:
		return "minimal"
	default:
		return "unknown"
	}
}

// Data is the common render snapshot assembled once per invocation and passed
// to every probe. Populated by the parser (Phase 3) and stdin decoder (Phase 4)
// before any probe runs.
type Data struct {
	Stdin        stdin.Payload        // parsed stdin from CC hook
	Session      *parser.SessionStats // JSONL aggregates; nil = empty session
	Subagents    []parser.SubagentStats
	Git          *parser.GitStatus // nil = not in a git repo or detection failed
	Now          time.Time
	TerminalCols int // 0 = detect failed; probes should fall back to 80
}

// Config carries per-invocation configuration flags. It is a lightweight struct
// (not the full internal/config.Config) scoped to what probes actually need.
// Additional fields will be added as Phase 4 subtasks require them.
type Config struct {
	Email        string
	EmailEnabled bool
	QuotaEnabled bool
}

// Probe is the single interface every status-line block must implement.
// Probes are stateless: all state comes from Data and Config.
//
// Render parameter order: (d Data, c Config, t renderer.Theme, level Level)
// — amendment 2026-05-18, from 3-param to 4-param to pass Config explicitly.
type Probe interface {
	// Name returns a unique identifier used in logs and registry comments.
	Name() string

	// Priority returns the display priority: 0 (always visible) to 4 (drop first).
	// Matches §A4 priority table in the concept doc.
	Priority() int

	// Visible reports whether this probe has meaningful data to show.
	// Returning false causes the renderer to skip the probe entirely.
	Visible(d Data, c Config) bool

	// Render produces the display string for the given level.
	// Output must be plain text with no embedded ANSI codes; the renderer
	// wraps it with Theme colours after calling Render.
	Render(d Data, c Config, t renderer.Theme, level Level) string

	// MinWidth returns the minimum character width of the Minimal-level output.
	// Used by the renderer to decide which probes to drop when space is tight.
	MinWidth() int
}

// Compile-time interface conformance checks: every probe type must satisfy
// Probe. A missing 4-param Render or wrong Visible signature breaks the build.
var (
	_ Probe = (*ModelProbe)(nil)
	_ Probe = (*EffortProbe)(nil)
	_ Probe = (*CostProbe)(nil)
	_ Probe = (*ProjectProbe)(nil)
	_ Probe = (*EmailProbe)(nil)
	_ Probe = (*TimeProbe)(nil)
	_ Probe = (*CtxProbe)(nil)
	_ Probe = (*CacheProbe)(nil)
	_ Probe = (*QuotaProbe)(nil)
	_ Probe = (*GitProbe)(nil)
	_ Probe = (*SubagentProbe)(nil)
)
