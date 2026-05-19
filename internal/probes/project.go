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

// Visible always returns true: the probe falls back to "?" for empty Cwd.
func (p *ProjectProbe) Visible(d Data, c Config) bool { return true }

// Render returns the project name derived from basename(Cwd):
//
//	Full/Compact: full basename (no truncation).
//	Minimal:      middle-truncate to ≥ 8 chars if longer (using "…" as separator).
func (p *ProjectProbe) Render(d Data, _ Config, t renderer.Theme, level Level) string {
	name := filepath.Base(d.Stdin.Cwd)
	if name == "" || name == "." || name == "/" {
		return "?"
	}

	if level == LevelMinimal {
		return middleTruncate(name, 8)
	}
	return name
}
