package config

type Config struct {
	Version    int        `toml:"version"`
	General    General    `toml:"general"`
	Theme      Theme      `toml:"theme"`
	Widgets    Widgets    `toml:"widgets"`
	Thresholds Thresholds `toml:"thresholds"`
	Probes     Probes     `toml:"probes"`
}

type General struct {
	TutorialHints       bool `toml:"tutorial_hints"`
	NoColor             bool `toml:"no_color"`
	NerdFont            bool `toml:"nerd_font"`
	RefreshIntervalHint int  `toml:"refresh_interval_hint"`
}

type Theme struct {
	Name   string      `toml:"name"`
	Colors ThemeColors `toml:"colors"`
}

type ThemeColors struct {
	Cyan, Yellow, Red, Green, Orange, Magenta, Dim string
}

type Widgets struct {
	Model, Effort, Cost, Project, Email, Time, Ctx, Cache, Quota, Git, Subagent bool
}

type Thresholds struct {
	CostBudgetUSD      float64
	CtxWarnRatio       float64
	CtxCriticalRatio   float64
	OrchTTLMinutes     int
	SubagentGapMinutes int
}

type Probes struct {
	Email EmailOpts `toml:"email"`
}

type EmailOpts struct {
	Address string `toml:"address"`
}
