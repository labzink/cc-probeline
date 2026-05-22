package main

import (
	"fmt"
	"runtime/debug"
)

// Build metadata. Primary source: -ldflags at build time, e.g.
//
//	go build -ldflags "-X main.version=v0.1.0 \
//	                   -X main.commit=$(git rev-parse --short HEAD) \
//	                   -X main.buildDate=$(date -u +%Y-%m-%dT%H:%MZ)" \
//	         ./cmd/cc-probeline/
//
// Fallback via debug.ReadBuildInfo (for `go install`) is wired in 5.a.
var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

// versionString returns the formatted version string. When build metadata was
// not injected via -ldflags, it attempts to read VCS information from the
// embedded debug.BuildInfo (available when built via `go install` or
// `go build` with module support).
func versionString() string {
	v, c, d := version, commit, buildDate
	if v == "dev" || c == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					if c == "" && len(s.Value) > 0 {
						end := 7
						if len(s.Value) < end {
							end = len(s.Value)
						}
						c = s.Value[:end]
					}
				case "vcs.time":
					if d == "" {
						d = s.Value
					}
				}
			}
		}
	}
	return fmt.Sprintf("cc-probeline %s (commit %s, built %s)", v, c, d)
}
