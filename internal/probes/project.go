package probes

import (
	"path/filepath"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// ProjectProbe renders the basename of the current working directory.
// Falls back to "?" when Cwd is empty or the basename is empty (e.g. root "/").
// Always visible. Priority P2.
type ProjectProbe struct{}

func (p *ProjectProbe) Name() string  { return "project" }
func (p *ProjectProbe) Priority() int { return 2 }
func (p *ProjectProbe) MinWidth() int { return len("?") }

// Visible returns false when ProjectEnabled is false; otherwise always true (falls back to "?").
func (p *ProjectProbe) Visible(d Data, c Config) bool {
	if !c.ProjectEnabled {
		return false
	}
	return true
}

// Render returns the project name derived from basename(Cwd):
//
//	Full:    full basename (no truncation).
//	Compact: middle-truncate to 12 runes.
//	Minimal: middle-truncate to 8 runes.
func (p *ProjectProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	name := filepath.Base(d.Stdin.Cwd)
	if name == "" || name == "." || name == "/" {
		return "?"
	}

	switch level {
	case LevelCompact:
		return middleTruncate(name, 12)
	case LevelMinimal:
		return middleTruncate(name, 8)
	default:
		return name
	}
}
