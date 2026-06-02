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
//
// Colour markers applied per B3 §5:
//
//   - branch segment "⎇ <name>" → {{color:cyan}}…{{reset}}
//
//   - warning segment "⚠N" (when ModifiedCount > 0) → {{color:yellow}}…{{reset}}
//
//     Full:    "⎇ <branch>" (+ " ⚠N" if N>0), no truncation.
//     Compact: "⎇ <branch12>" (+ " ⚠N" if N>0), branch middle-truncated to 12.
//     Minimal: "⎇ <branch8>", no ⚠N.
func (p *GitProbe) Render(d Data, c Config, t renderer.Theme, level Level) string {
	if d.Git == nil {
		return ""
	}

	branch := d.Git.Branch
	if d.Git.Worktree != "" {
		branch = "-:" + branch
	}

	// warnSegment returns the warn segment (with colour marker when AnsiEnabled) or "".
	// AnsiEnabled=true: "⚠ N" with space (spec §2.3 T-29); plain path keeps no space
	// for backward compatibility with AnsiEnabled=false callers.
	warnSegment := func(n int) string {
		if n <= 0 {
			return ""
		}
		if t.AnsiEnabled {
			return " {{color:yellow}}⚠ " + strconv.Itoa(n) + "{{reset}}"
		}
		return " ⚠" + strconv.Itoa(n)
	}

	switch level {
	case LevelMinimal:
		branchText := "⎇ " + middleTruncate(branch, 8)
		if t.AnsiEnabled {
			return "{{color:cyan}}" + branchText + "{{reset}}"
		}
		return branchText
	case LevelCompact:
		branchText := "⎇ " + middleTruncate(branch, 12)
		if t.AnsiEnabled {
			return "{{color:cyan}}" + branchText + "{{reset}}" + warnSegment(d.Git.ModifiedCount)
		}
		return branchText + warnSegment(d.Git.ModifiedCount)
	default: // LevelFull
		branchText := "⎇ " + branch
		if t.AnsiEnabled {
			return "{{color:cyan}}" + branchText + "{{reset}}" + warnSegment(d.Git.ModifiedCount)
		}
		return branchText + warnSegment(d.Git.ModifiedCount)
	}
}
