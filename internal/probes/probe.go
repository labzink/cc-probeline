package probes

import (
	"time"

	"github.com/labzink/cc-probeline/internal/parser"
	"github.com/labzink/cc-probeline/internal/renderer"
	"github.com/labzink/cc-probeline/internal/state"
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
	TerminalCols int    // 0 = detect failed; probes should fall back to 80
	SessionID    string // CC session id; "" disables hint state persistence (Phase 4.4)

	// ExtraCacheEvents lets main inject synthetic alerts (e.g. config load
	// errors) that are not derived from session/subagent JSONL data. Phase 6.
	ExtraCacheEvents []parser.CacheEvent

	// Phase 6.8.a: delta-based cost fields.
	// SessionTotal is the cost incurred in this session (ccTotal − BaselineCost).
	// Populated by main after cost.Reconcile; zero when state not yet loaded.
	SessionTotal float64

	// Phase 6.9.a: session duration delta.
	// SessionDurMS is TotalAPIDurationMS − BaselineDurMS (resets on /clear).
	// Populated by main via cost.SessionDuration; zero when state not yet loaded.
	SessionDurMS int64

	// LastRequestCost is the cost of the most recent prompt group
	// (ccTotal − PromptCost[curGroupID]). Zero when not yet computed.
	LastRequestCost float64

	// PerTurnCostFn returns the finalized per-turn USD cost for the given UUID.
	// Returns (0, false) when the turn has no recorded cost yet.
	// Set by main from state.Session.PerTurnCost via cost.PerTurn.
	// nil when state not available (graceful degradation: render "—").
	PerTurnCostFn func(uuid string) (float64, bool)

	// State is the reconciled per-session state. Used by perTurnTable to pass
	// to renderer.RenderUnified for per-turn cost column (C1). nil means state
	// not available; renderer degrades gracefully (shows "—" for all turns).
	State *state.Session

	// CommitBadgeCount is the "✓ N committed" badge count to render this refresh
	// (Phase 6.95.a). Zero means no badge. Set by main via state.CommitBadgeTick;
	// GitProbe renders it (green) in Full/Compact, never in Minimal.
	CommitBadgeCount int
}

// Config carries per-invocation configuration flags. It is a lightweight struct
// (not the full internal/config.Config) scoped to what probes actually need.
// All XEnabled fields default to false; callers must populate from config.ToProbesConfig.
type Config struct {
	// Per-widget toggles (Phase 6 — from config.Widgets). Default false; set
	// via ToProbesConfig to mirror the user's config. Phase 4-5 callers that
	// do not populate these fields should use Default()-sourced Config to get
	// all-true behaviour.
	ModelEnabled    bool
	EffortEnabled   bool
	CostEnabled     bool
	ProjectEnabled  bool
	EmailEnabled    bool
	TimeEnabled     bool
	CtxEnabled      bool
	CacheEnabled    bool
	QuotaEnabled    bool
	GitEnabled      bool
	SubagentEnabled bool

	// TableRows is the maximum number of per-turn rows shown in the subagent
	// table. Set from config.General.TableRows via ToProbesConfig. When 0 the
	// assembler applies its own default (10). Capped at 40 by SetTableRows.
	TableRows int

	// Per-probe values (Phase 6 — from config.Probes).
	// Email is the override address for the Email probe. When empty the probe
	// reads the address from the CC session JSONL.
	Email string

	// Threshold values (Phase 6 — from config.Thresholds).
	CostBudgetUSD      float64
	CtxWarnRatio       float64
	CtxCriticalRatio   float64
	OrchTTLMinutes     int
	SubagentGapMinutes int

	// IsSubagentContext indicates that the probe is being rendered in a subagent
	// display context (not the main orchestrator line). When true, TTL is
	// suppressed (T-23). This is a runtime flag, not a config threshold —
	// SubagentGapMinutes is the detection threshold (C3 fix).
	IsSubagentContext bool
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
