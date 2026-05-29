package parser

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// GitStatus holds a lightweight snapshot of the working-tree git state.
type GitStatus struct {
	// Branch is the abbreviated HEAD ref (e.g. "main", "feature/foo").
	Branch string
	// ModifiedCount is the number of changed/untracked lines in `git status --porcelain`.
	ModifiedCount int
	// Worktree is non-empty when the directory is a linked git worktree.
	// Value is the path reported by --git-common-dir.
	Worktree string
}

// DetectGit inspects dir and returns a GitStatus describing the current
// branch, modified-file count, and whether it is a linked worktree.
//
// The caller is expected to set a context timeout (~150ms) before calling.
// All git subprocesses run with GIT_OPTIONAL_LOCKS=0 to avoid acquiring
// lock files during the read-only status queries.
//
// Returns (nil, err) when:
//   - ctx is already cancelled or times out during execution.
//   - dir is not inside a git repository.
//   - any required git invocation fails.
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

	// runGit executes a git subcommand in dir with the shared env and ctx.
	// Returns trimmed stdout, or an error on non-zero exit or ctx cancellation.
	runGit := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = gitEnv
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(out), "\r\n"), nil
	}

	// Resolve HEAD branch name.
	branch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		slog.Debug("parser.DetectGit rev-parse branch failed", "dir", dir, "err", err)
		return nil, fmt.Errorf("parser: DetectGit rev-parse branch: %w", err)
	}

	// Count modified/untracked files from porcelain output.
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	cmd.Env = gitEnv
	porcelainOut, err := cmd.Output()
	if err != nil {
		slog.Debug("parser.DetectGit status --porcelain failed", "dir", dir, "err", err)
		return nil, fmt.Errorf("parser: DetectGit status porcelain: %w", err)
	}
	modifiedCount := countNonEmptyLines(porcelainOut)

	// Detect linked worktree: --git-common-dir differs from --git-dir.
	commonDir, err := runGit("rev-parse", "--git-common-dir")
	if err != nil {
		slog.Debug("parser.DetectGit rev-parse --git-common-dir failed", "dir", dir, "err", err)
		return nil, fmt.Errorf("parser: DetectGit rev-parse common-dir: %w", err)
	}
	gitDir, err := runGit("rev-parse", "--git-dir")
	if err != nil {
		slog.Debug("parser.DetectGit rev-parse --git-dir failed", "dir", dir, "err", err)
		return nil, fmt.Errorf("parser: DetectGit rev-parse git-dir: %w", err)
	}

	var worktree string
	if commonDir != gitDir {
		// Linked worktree: --git-common-dir points to the main repo's .git,
		// while --git-dir points to the per-worktree .git directory.
		worktree = commonDir
	}

	gs := &GitStatus{
		Branch:        branch,
		ModifiedCount: modifiedCount,
		Worktree:      worktree,
	}
	slog.Debug("parser.DetectGit done", "branch", gs.Branch, "modified", gs.ModifiedCount, "worktree", gs.Worktree)
	return gs, nil
}

// countNonEmptyLines counts the number of non-empty lines in b.
func countNonEmptyLines(b []byte) int {
	count := 0
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			count++
		}
	}
	return count
}
