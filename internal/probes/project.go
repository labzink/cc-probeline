package probes

import (
	"path/filepath"

	"github.com/labzink/cc-probeline/internal/renderer"
)

// ProjectProbe renders the basename of the current working directory.
// Falls back to "?" when Cwd is empty or the basename is empty (e.g. root "/").
// Always visible. Priority P0.
//
// Note: Render uses the 3-param signature (d, t, level) to match the Phase 4.1.a
// test contract. The 4-param Config form will be added in Phase 4.1.a GREEN.
type ProjectProbe struct{}

func (p *ProjectProbe) Name() string  { return "project" }
func (p *ProjectProbe) Priority() int { return 0 }
func (p *ProjectProbe) MinWidth() int { return len("?") }

// Visible always returns true: the probe falls back to "?" for empty Cwd.
func (p *ProjectProbe) Visible(d Data, c Config) bool { return true }

// Render returns the project name derived from basename(Cwd):
//
//	Full/Compact: full basename (no truncation).
//	Minimal:      middle-truncate to ≥ 8 chars if longer (using "…" as separator).
func (p *ProjectProbe) Render(d Data, t renderer.Theme, level Level) string {
	name := filepath.Base(d.Stdin.Cwd)
	if name == "" || name == "." || name == "/" {
		return "?"
	}

	if level == LevelMinimal {
		return middleTruncate(name, 8)
	}
	return name
}

// middleTruncate returns s unchanged when len([]rune(s)) <= minWidth.
// Otherwise it middle-truncates s with "…", using two regimes:
//
// Regime 1 — string is relatively short (floor(len/2) < minWidth-1):
//
//	head = floor(len/2)
//	tail = max(minWidth - head, 2)   // no -1 for ellipsis
//	Total output > minWidth.
//
// Regime 2 — string is long (floor(len/2) >= minWidth-1):
//
//	tail = floor((minWidth-1)/2)
//	head = minWidth - 1 - tail
//	Total output == minWidth.
//
// Concrete examples (minWidth=8):
//
//	"cc-probeline" (12)              → regime 1 → "cc-pro…ne"  (9 runes)
//	"my-super-long-project-name" (26) → regime 2 → "my-s…ame"   (8 runes)
func middleTruncate(s string, minWidth int) string {
	runes := []rune(s)
	n := len(runes)
	if n <= minWidth {
		return s
	}
	half := n / 2
	var head, tail int
	if half < minWidth-1 {
		// Regime 1: string is moderately longer than minWidth.
		head = half
		tail = minWidth - head
		if tail < 2 {
			tail = 2
		}
	} else {
		// Regime 2: string is much longer than minWidth.
		tail = (minWidth - 1) / 2
		head = minWidth - 1 - tail
	}
	return string(runes[:head]) + "…" + string(runes[n-tail:])
}
