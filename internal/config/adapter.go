package config

import (
	"github.com/labzink/cc-probeline/internal/probes"
	"github.com/labzink/cc-probeline/internal/renderer"
)

func ToProbesConfig(cfg Config) probes.Config {
	return probes.Config{}
}

func ToTheme(cfg Config, base renderer.Theme) renderer.Theme {
	return base
}
