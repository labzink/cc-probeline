package probes

import (
	"strconv"

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
func (p *GitProbe) Priority() int { return 2 }
func (p *GitProbe) MinWidth() int { return 8 }

// Visible returns false when GitEnabled is false or d.Git is nil.
func (p *GitProbe) Visible(d Data, c Config) bool {
	if !c.GitEnabled {
		return false
	}
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
		branch = middleTruncate(branch, 8)
		return "⎇ " + branch
	}

	// Full or Compact: same format.
	result := "⎇ " + branch
	if d.Git.ModifiedCount > 0 {
		result += " ⚠" + strconv.Itoa(d.Git.ModifiedCount)
	}
	return result
}
