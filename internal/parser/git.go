package parser

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitStatus holds a lightweight snapshot of the working-tree git state.
type GitStatus struct {
	// Branch is the abbreviated HEAD ref (e.g. "main", "feature/foo").
	// When HEAD is detached, value is "(detached)".
	Branch string
	// ModifiedCount is the number of changed/untracked entries reported by
	// `git status --porcelain=v2 --branch` (non-header lines).
	ModifiedCount int
	// Worktree is non-empty when the directory is a linked git worktree.
	// Value is the path reported by the git-common-dir inference from v2 output.
	Worktree string
}

// DetectGit inspects dir and returns a GitStatus describing the current
// branch, modified-file count, and whether it is a linked worktree.
// It issues exactly one subprocess: `git status --porcelain=v2 --branch`.
//
// The caller is expected to set a context timeout (~150ms) before calling.
// GIT_OPTIONAL_LOCKS=0 prevents lock-file creation during the read-only call.
//
// Returns (nil, err) when:
//   - ctx is already cancelled or times out during execution.
//   - dir is not inside a git repository.
//   - the git invocation fails.
func DetectGit(ctx context.Context, dir string) (*GitStatus, error) {
	slog.Debug("parser.DetectGit start", "dir", dir)

	// Fast-path: return immediately if the context is already done.
	select {
	case <-ctx.Done():
		slog.Debug("parser.DetectGit context already cancelled", "dir", dir)
		return nil, fmt.Errorf("parser: DetectGit context cancelled: %w", ctx.Err())
	default:
	}

	gitEnv := append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v2", "--branch")
	cmd.Dir = dir
	cmd.Env = gitEnv
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("parser.DetectGit subprocess failed", "dir", dir, "err", err)
		return nil, fmt.Errorf("parser: DetectGit subprocess: %w", err)
	}

	gs, parseErr := ParsePorcelainV2(string(out))
	if parseErr != nil {
		slog.Warn("parser.DetectGit parse failed", "dir", dir, "err", parseErr)
		return nil, fmt.Errorf("parser: DetectGit parse: %w", parseErr)
	}

	// Detect linked worktree via filesystem: in a linked worktree, .git is a file
	// (not a directory) containing a "gitdir: <path>" pointer. No subprocess needed.
	gs.Worktree = detectWorktree(dir)

	slog.Debug("parser.DetectGit done", "branch", gs.Branch, "modified", gs.ModifiedCount, "worktree", gs.Worktree)
	return gs, nil
}

// detectWorktree returns a non-empty string when dir is a linked git worktree.
// A linked worktree has a .git file (not directory) with a "gitdir:" line.
// The main worktree has a .git directory, so it returns "".
func detectWorktree(dir string) string {
	gitPath := filepath.Join(dir, ".git")
	fi, err := os.Lstat(gitPath)
	if err != nil {
		return ""
	}
	// Main worktree: .git is a directory.
	if fi.IsDir() {
		return ""
	}
	// Linked worktree: .git is a file with "gitdir: <path>".
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if strings.HasPrefix(line, prefix) {
		return strings.TrimPrefix(line, prefix)
	}
	return ""
}

// ParsePorcelainV2 parses the output of `git status --porcelain=v2 --branch`
// and returns a GitStatus. Returns an error if the output is empty or contains
// no recognisable header lines.
//
// Format reference (git-status(1)):
//
//	# branch.oid <sha>
//	# branch.head <name>      — branch name; "(detached)" when HEAD is detached
//	# branch.upstream <name>  — optional
//	# branch.ab +A -B         — optional ahead/behind
//	<entry lines>             — one per changed/untracked file (no leading '#')
func ParsePorcelainV2(output string) (*GitStatus, error) {
	if strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("parser: ParsePorcelainV2: empty output")
	}

	var gs GitStatus
	headerSeen := false

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			headerSeen = true
			// Parse header lines we care about.
			switch {
			case strings.HasPrefix(line, "# branch.head "):
				raw := strings.TrimPrefix(line, "# branch.head ")
				if raw == "(detached)" {
					gs.Branch = "(detached)"
				} else {
					gs.Branch = strings.TrimSpace(raw)
				}
			}
			// Other headers (# branch.oid, # branch.upstream, # branch.ab,
			// # branch.worktree) are ignored; not needed for our use case.
		} else {
			// Non-header lines: each represents one changed or untracked entry.
			gs.ModifiedCount++
		}
	}

	if !headerSeen {
		return nil, fmt.Errorf("parser: ParsePorcelainV2: no header lines found")
	}

	return &gs, nil
}

// ResolveGitStatus applies the anti-flicker logic for the git probe.
// It is a pure function — it does not mutate any arguments.
//
// Logic:
//   - err == nil  → return fresh (the new successful result).
//   - err != nil && last != nil → return last (previous good state; anti-flicker).
//   - err != nil && last == nil → return nil (hide git segment on first failure).
func ResolveGitStatus(fresh *GitStatus, err error, last *GitStatus) *GitStatus {
	if err == nil {
		return fresh
	}
	// err != nil: fall back to last known good state (may be nil).
	return last
}
