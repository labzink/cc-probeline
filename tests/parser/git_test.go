// Package parser_test contains RED tests for Phase 6.5.b1 — git status detection.
// Contract: plans/phase-6.5/spec-common.md §2.1
// API (not yet implemented):
//
//	parser.DetectGit(ctx context.Context, dir string) (*GitStatus, error)
//	parser.GitStatus{Branch string, ModifiedCount int, Worktree string}
package parser_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/labzink/cc-probeline/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// gitEnv returns the minimal env vars required so git commands succeed in
// a sandboxed temp directory (no global gitconfig needed).
func gitEnv() []string {
	return []string{
		"HOME=" + os.Getenv("HOME"), // needed for system git binary lookup
		"PATH=" + os.Getenv("PATH"),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	}
}

// initRepo initialises a bare-minimum git repo in dir and creates an initial
// empty commit so HEAD resolves to a branch.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	env := gitEnv()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("commit", "--allow-empty", "-m", "init")
}

// ---------------------------------------------------------------------------
// T-1: DetectGit returns a non-empty Branch in a valid git repository.
// ---------------------------------------------------------------------------

func TestDetectGit_RealRepo(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	status, err := parser.DetectGit(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil *GitStatus, got nil")
	}
	if status.Branch == "" {
		t.Error("expected non-empty Branch, got empty string")
	}
}

// ---------------------------------------------------------------------------
// T-2: DetectGit returns (nil, non-nil error) for a directory that is not
// inside any git repository.
// ---------------------------------------------------------------------------

func TestDetectGit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	// dir is freshly created — not a git repo.

	status, err := parser.DetectGit(context.Background(), dir)
	if err == nil {
		t.Fatal("expected non-nil error for non-repo dir, got nil")
	}
	if status != nil {
		t.Errorf("expected nil *GitStatus for non-repo dir, got %+v", status)
	}
}

// ---------------------------------------------------------------------------
// T-3: DetectGit reports a non-empty Worktree field when called from a
// linked worktree (created via `git worktree add`).
// ---------------------------------------------------------------------------

func TestDetectGit_Worktree(t *testing.T) {
	mainDir := t.TempDir()
	initRepo(t, mainDir)

	wtDir := filepath.Join(t.TempDir(), "linked-wt")

	cmd := exec.Command("git", "worktree", "add", wtDir)
	cmd.Dir = mainDir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	status, err := parser.DetectGit(context.Background(), wtDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil *GitStatus, got nil")
	}
	if status.Worktree == "" {
		t.Error("expected non-empty Worktree for linked worktree, got empty string")
	}
}

// ---------------------------------------------------------------------------
// T-4: DetectGit respects context cancellation and returns quickly.
// ---------------------------------------------------------------------------

func TestDetectGit_CtxCancelled(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	_, err := parser.DetectGit(ctx, dir)
	if err == nil {
		t.Error("expected non-nil error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// T-5: DetectGit reports ModifiedCount >= 1 when there are untracked files.
// ---------------------------------------------------------------------------

func TestDetectGit_ModifiedCount(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	// Create an untracked file in the repo working tree.
	untrackedPath := filepath.Join(dir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	status, err := parser.DetectGit(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil *GitStatus, got nil")
	}
	if status.ModifiedCount < 1 {
		t.Errorf("expected ModifiedCount >= 1 with untracked file, got %d", status.ModifiedCount)
	}
}
