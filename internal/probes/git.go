package probes

import (
	"github.com/labzink/cc-probeline/internal/renderer"
)

// GitProbe renders the current git branch and modified-file count.
// Visible only when d.Git is non-nil (i.e., we are inside a git repository).
//
// Display:
//
//	Full/Compact: "⎇ <branch>" (or "⎇ -:<branch>" if Worktree != "") + " ⚠N" if N > 0
//	Minimal:      middle-truncated branch with "…", at least 8 runes in the result
type GitProbe struct{}

func (p *GitProbe) Name() string  { return "git" }
func (p *GitProbe) Priority() int { return 1 }
func (p *GitProbe) MinWidth() int { return 8 }

// Visible returns false when d.Git is nil.
func (p *GitProbe) Visible(d Data, c Config) bool {
	return d.Git != nil
}

// Render formats the git branch status.
func (p *GitProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	if d.Git == nil {
		return ""
	}

	branch := d.Git.Branch
	if d.Git.Worktree != "" {
		branch = "-:" + branch
	}

	if level == LevelMinimal {
		branch = gitMiddleTruncate(branch, 8)
		return "⎇ " + branch
	}

	// Full or Compact: same format.
	result := "⎇ " + branch
	if d.Git.ModifiedCount > 0 {
		result += " ⚠" + itoa(d.Git.ModifiedCount)
	}
	return result
}

// itoa converts a non-negative int to its decimal string representation.
// Used to avoid importing strconv in a small helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// gitMiddleTruncate middle-truncates s to produce a result of exactly minWidth runes
// when len(runes) > minWidth. Uses head=ceil((minWidth-1)/2), tail=floor((minWidth-1)/2).
func gitMiddleTruncate(s string, minWidth int) string {
	runes := []rune(s)
	if len(runes) <= minWidth {
		return s
	}
	tail := (minWidth - 1) / 2
	head := minWidth - 1 - tail
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}
