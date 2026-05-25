package config

// Pin pelletier/go-toml/v2 as a direct dependency so `go mod tidy` keeps it
// in go.mod. The real import (with parser code) lands in 6.b loader.
import _ "github.com/pelletier/go-toml/v2"

// Default returns the built-in default Config. Equivalent to the state when
// no config file exists at any location in the cascade. Always non-nil.
// Each call returns an independent copy; mutating the result does not affect
// subsequent calls.
func Default() *Config {
	return &Config{
		Version: 1,
		General: General{
			TutorialHints:       true,
			NoColor:             false,
			NerdFont:            false,
			RefreshIntervalHint: 5,
		},
		Theme: Theme{
			Name:   "default",
			Colors: ThemeColors{}, // all empty strings == "use palette default"
		},
		Widgets: Widgets{
			Model:    true,
			Effort:   true,
			Cost:     true,
			Project:  true,
			Email:    true,
			Time:     true,
			Ctx:      true,
			Cache:    true,
			Quota:    true,
			Git:      true,
			Subagent: true,
		},
		Thresholds: Thresholds{
			CostBudgetUSD:      0, // 0 = warning disabled
			CtxWarnRatio:       0.70,
			CtxCriticalRatio:   0.90,
			OrchTTLMinutes:     60,
			SubagentGapMinutes: 5,
		},
		Probes: Probes{Email: EmailOpts{Address: ""}},
	}
}

func Load(path string) (*Config, []Error) {
	return &Config{}, nil
}

func LoadCascade(cwd string) (*Config, Source, []Error) {
	return &Config{}, SourceDefaults, nil
}
