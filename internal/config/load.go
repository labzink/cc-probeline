package config

import (
	"bytes"
	"errors"
	"log/slog"
	"os"

	"github.com/pelletier/go-toml/v2"
)

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
			TableRows:           10,
			Mode:                "standard",
		},
		Theme: Theme{
			Name:   "default",
			Colors: ThemeColors{},
		},
		Widgets: Widgets{
			Model:   true,
			Effort:  true,
			Cost:    true,
			Project: true,
			Email:   true,
			Time:    true,
			Ctx:     true,
			Quota:   true,
			Git:     true,
		},
		Thresholds: Thresholds{
			CostBudgetUSD:      0,
			CtxWarnRatio:       0.70,
			CtxCriticalRatio:   0.90,
			OrchTTLMinutes:     60,
			SubagentGapMinutes: 5,
		},
		Probes: Probes{Email: EmailOpts{Address: ""}},
	}
}

// Load reads and parses the TOML file at path. Returns (cfg, errs):
//   - cfg is always non-nil (Default()-filled on parse failure).
//   - errs contains SeverityError entries for parse failures plus any
//     Validate(cfg) entries for semantic issues.
//
// Empty path is equivalent to Default() with no errors.
// path must be an absolute path (caller responsibility).
func Load(path string) (*Config, []Error) {
	if path == "" {
		return Default(), nil
	}
	cfg, errs := parseFile(path)
	errs = append(errs, Validate(cfg)...)
	return cfg, errs
}

// LoadCascade resolves the config cascade for the given working directory and
// returns the best available Config together with the cascade Source that
// produced it.
//
// Precedence (highest to lowest):
//  1. CC_PROBELINE_CONFIG env var → load that file; no fall-through on error.
//  2. Project-local .cc-probeline.toml found by walking up from cwd.
//  3. Global config ($XDG_CONFIG_HOME/cc-probeline/config.toml or equivalent).
//  4. Built-in defaults.
//
// A broken file at the chosen level is returned together with errors; the
// loader never silently falls through to a lower-priority source so that the
// user is aware their explicit config is broken.
func LoadCascade(cwd string) (*Config, Source, []Error) {
	// Step 1: explicit env override.
	if envPath := os.Getenv("CC_PROBELINE_CONFIG"); envPath != "" {
		cfg, errs := Load(envPath)
		if len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("config load error from CC_PROBELINE_CONFIG",
					"path", envPath, "msg", e.Message)
			}
		}
		return cfg, SourceEnv, errs
	}

	// Step 2: project-local config.
	if projPath := findProjectConfig(cwd); projPath != "" {
		cfg, errs := Load(projPath)
		if len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("config load error from project config",
					"path", projPath, "msg", e.Message)
			}
		}
		return cfg, SourceProject, errs
	}

	// Step 3: global config.
	globalPath := globalConfigPath()
	if globalPath != "" {
		if fileExists(globalPath) {
			cfg, errs := Load(globalPath)
			if len(errs) > 0 {
				for _, e := range errs {
					slog.Warn("config load error from global config",
						"path", globalPath, "msg", e.Message)
				}
			}
			return cfg, SourceGlobal, errs
		}
	}

	// Step 4: built-in defaults.
	return Default(), SourceDefaults, nil
}

// parseFile reads path and parses it as TOML using a two-pass strategy:
//  1. Strict mode (DisallowUnknownFields) to detect unknown keys → SeverityWarning.
//  2. Normal mode to materialise Config, starting from Default() values so that
//     a partial file keeps default values for omitted fields.
//
// On a fatal parse error, returns Default() + a SeverityError with position info.
func parseFile(path string) (*Config, []Error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), []Error{{
				Severity: SeverityError,
				File:     path,
				Message:  "config file not found",
			}}
		}
		return Default(), []Error{{
			Severity: SeverityError,
			File:     path,
			Message:  "cannot read config file: " + err.Error(),
		}}
	}

	var errs []Error

	// Pass 1: strict decode to collect unknown-field warnings.
	var strictCfg Config
	dec1 := toml.NewDecoder(bytes.NewReader(data))
	dec1.DisallowUnknownFields()
	if err := dec1.Decode(&strictCfg); err != nil {
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			errs = append(errs, newStrictMissingErrors(path, strictErr)...)
		}
		// Other errors from strict pass are ignored here; pass 2 will catch them.
	}

	// Pass 2: lenient decode — unknown fields are ignored, starts from Default().
	cfg := Default()
	dec2 := toml.NewDecoder(bytes.NewReader(data))
	if err := dec2.Decode(cfg); err != nil {
		var decodeErr *toml.DecodeError
		if errors.As(err, &decodeErr) {
			errs = append(errs, newParseError(path, decodeErr))
		} else {
			errs = append(errs, Error{
				Severity: SeverityError,
				File:     path,
				Message:  err.Error(),
			})
		}
		// Return Default() on fatal parse error.
		return Default(), errs
	}

	return cfg, errs
}
