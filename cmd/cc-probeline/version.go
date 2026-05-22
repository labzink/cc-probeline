package main

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
