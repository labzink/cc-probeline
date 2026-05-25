package config

// Config is the top-level configuration structure for cc-probeline.
// It is unmarshalled from a TOML file found via the cascade (see LoadCascade).
// Use Default() to obtain the built-in defaults when no file exists.
type Config struct {
	// Version is the config file format version. Currently always 1.
	// Future breaking changes will increment this number so the loader can
	// apply migration logic before unmarshalling into this struct.
	Version int `toml:"version"`

	// General groups settings that affect the overall behaviour of the tool:
	// colours, font glyphs, and hints. Change these to tune the visual style
	// without touching individual widget settings.
	General General `toml:"general"`

	// Theme selects a named colour palette and allows per-colour overrides.
	// Set Name to "default", "high-contrast", or "minimal"; leave individual
	// Colors fields empty to use the palette default.
	Theme Theme `toml:"theme"`

	// Widgets controls which probes are rendered. Set a field to false to
	// permanently hide the corresponding status-line widget. All widgets are
	// visible by default to preserve Phase 4-5 behaviour.
	Widgets Widgets `toml:"widgets"`

	// Thresholds defines numeric cutoffs used by probes to emit warnings.
	// CostBudgetUSD=0 disables budget warnings. Ratios are in the [0,1] range.
	Thresholds Thresholds `toml:"thresholds"`

	// Probes groups per-probe configuration that is not covered by the widget
	// toggles above. Currently only the Email probe requires extra settings.
	Probes Probes `toml:"probes"`
}

// General groups top-level display settings that are not widget-specific.
type General struct {
	// TutorialHints enables inline hints shown in the status line when the
	// session is fresh or when notable events occur. Set to false to suppress.
	TutorialHints bool `toml:"tutorial_hints"`

	// NoColor forces plain-text output with no ANSI colour codes. Equivalent
	// to setting NO_COLOR=1 in the environment. The environment variable takes
	// precedence over this field.
	NoColor bool `toml:"no_color"`

	// NerdFont enables Nerd Font glyph icons in widget output. Set to true
	// when your terminal uses a patched Nerd Font. The terminal auto-detection
	// at startup may also set this automatically.
	NerdFont bool `toml:"nerd_font"`

	// RefreshIntervalHint is the suggested refresh cadence in seconds, passed
	// to Claude Code via the hook handshake. Does not affect the actual
	// rendering interval — CC controls that. Range: 1-60.
	RefreshIntervalHint int `toml:"refresh_interval_hint"`
}

// Theme selects a named palette and allows per-field hex overrides.
type Theme struct {
	// Name is the built-in palette name. Recognised values: "default",
	// "high-contrast", "minimal". Unknown names fall back to "default" (the
	// validator emits a warning separately so the adapter stays pure).
	Name string `toml:"name"`

	// Colors holds optional hex overrides for individual semantic colours.
	// Any non-empty string replaces the palette value for that colour role.
	// Empty strings (the default) keep the palette colour unchanged.
	Colors ThemeColors `toml:"colors"`
}

// ThemeColors holds per-field hex colour overrides for the active palette.
// All fields are optional. An empty string means "use the palette default".
type ThemeColors struct {
	// Cyan overrides the cyan semantic colour (git branch, orch label).
	Cyan string `toml:"cyan"`

	// Yellow overrides the yellow semantic colour (warnings, agent IDs).
	Yellow string `toml:"yellow"`

	// Red overrides the red semantic colour (critical state, cache miss).
	Red string `toml:"red"`

	// Green overrides the green semantic colour (healthy state, low cost).
	Green string `toml:"green"`

	// Orange overrides the orange semantic colour (progress 70-90%).
	Orange string `toml:"orange"`

	// Magenta overrides the magenta semantic colour ([high] effort indicator).
	Magenta string `toml:"magenta"`

	// Dim overrides the dim/muted colour used for secondary text separators.
	Dim string `toml:"dim"`
}

// Widgets controls visibility for each status-line probe widget.
// All fields default to true (all widgets visible).
type Widgets struct {
	// Model shows the active Claude model name (e.g. "claude-sonnet-4-5").
	Model bool `toml:"model"`

	// Effort shows the effort level indicator ([high], [normal], etc.).
	Effort bool `toml:"effort"`

	// Cost shows the running session cost estimate in USD.
	Cost bool `toml:"cost"`

	// Project shows the project/working-directory name.
	Project bool `toml:"project"`

	// Email shows the user email address from the CC session.
	Email bool `toml:"email"`

	// Time shows the elapsed session time.
	Time bool `toml:"time"`

	// Ctx shows the context window usage as a progress bar.
	Ctx bool `toml:"ctx"`

	// Cache shows cache-read vs cache-create token counts.
	Cache bool `toml:"cache"`

	// Quota shows the daily/monthly quota usage if available.
	Quota bool `toml:"quota"`

	// Git shows the current git branch and dirty-state indicator.
	Git bool `toml:"git"`

	// Subagent shows active subagent count and orchestrator TTL.
	Subagent bool `toml:"subagent"`
}

// Thresholds defines numeric cutoffs used by probes to decide when to emit
// warnings or change colour. All values are optional overrides.
type Thresholds struct {
	// CostBudgetUSD is the per-session cost budget in USD. When the running
	// cost exceeds this value the cost probe turns red. 0 disables the check.
	CostBudgetUSD float64 `toml:"cost_budget_usd"`

	// CtxWarnRatio is the context-window fill ratio at which the Ctx probe
	// switches to warning colour. Range: (0, 1). Default: 0.70.
	CtxWarnRatio float64 `toml:"ctx_warn_ratio"`

	// CtxCriticalRatio is the fill ratio at which the Ctx probe turns red.
	// Must be > CtxWarnRatio. Range: (0, 1). Default: 0.90.
	CtxCriticalRatio float64 `toml:"ctx_critical_ratio"`

	// OrchTTLMinutes is the orchestrator idle timeout in minutes. The
	// subagent probe emits a warning when the orchestrator has been idle
	// longer than this value. Default: 60.
	OrchTTLMinutes int `toml:"orch_ttl_minutes"`

	// SubagentGapMinutes is the expected maximum gap between subagent
	// heartbeats in minutes. A larger gap triggers a stale-agent warning.
	// Default: 5.
	SubagentGapMinutes int `toml:"subagent_gap_minutes"`
}

// Probes groups per-probe configuration values that are not widget toggles.
type Probes struct {
	// Email holds configuration specific to the Email probe.
	Email EmailOpts `toml:"email"`
}

// EmailOpts holds configuration for the Email probe.
type EmailOpts struct {
	// Address overrides the email address shown by the Email probe.
	// When empty the probe reads the address from the CC session JSONL.
	Address string `toml:"address"`
}
