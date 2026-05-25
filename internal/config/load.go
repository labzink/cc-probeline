package config

// Pin pelletier/go-toml/v2 as a direct dependency so `go mod tidy` keeps it
// in go.mod. The real import (with parser code) lands in 6.b loader.
import _ "github.com/pelletier/go-toml/v2"

func Default() *Config {
	return &Config{}
}

func Load(path string) (*Config, []Error) {
	return &Config{}, nil
}

func LoadCascade(cwd string) (*Config, Source, []Error) {
	return &Config{}, SourceDefaults, nil
}
