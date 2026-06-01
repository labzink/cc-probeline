// Package parser_test contains RED tests for Phase 6.5.b1 — git status detection.
// Contract: plans/phase-6.5/spec-common.md §2.1
// API (not yet implemented):
//
//	parser.DetectGit(ctx context.Context, dir string) (*GitStatus, error)
//	parser.GitStatus{Branch string, ModifiedCount int, Worktree string}
//
// Phase 6.8.c additions — T-G1: TestDetectGit_PorcelainV2
// Contract: plans/phase-6.8/spec-common.md §2.2, §2.3, §4 Insurance #4
// New API (not yet implemented):
//
//	parser.ParsePorcelainV2(output string) (*GitStatus, error)
package parser_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// ---------------------------------------------------------------------------
// T-G1: TestDetectGit_PorcelainV2 — Phase 6.8.c
// Contract: spec-common.md §2.2 parser.DetectGit, §2.3 behaviour, T-13.
// Tests that ParsePorcelainV2 correctly extracts Branch + ModifiedCount from
// fixture output of `git status --porcelain=v2 --branch`.
// Fixture sanity: clean.txt=0 changes, main branch;
//                 modified.txt=3 changes (2 tracked + 1 untracked), feature branch;
//                 detached.txt=1 change, (detached) HEAD;
//                 worktree.txt=2 changes, worktree-branch.
// ---------------------------------------------------------------------------

// fixturePorcelain reads a file from tests/fixtures/git/ and returns its content.
// The path is resolved relative to the repository root via the go test working dir.
func fixturePorcelain(t *testing.T, name string) string {
	t.Helper()
	// go test sets cwd to the package directory (tests/parser/).
	// Walk up two levels to reach repo root.
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "git", name))
	if err != nil {
		t.Fatalf("fixturePorcelain: read %q: %v", name, err)
	}
	return string(data)
}

func TestDetectGit_PorcelainV2(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		wantBranch    string
		wantModified  int
		wantDetached  bool // true if branch == "(detached)"
	}{
		{
			// clean.txt: 4 header lines, 0 non-# lines.
			// Expected: branch="main", modified=0.
			name:         "clean repo on main",
			fixture:      "clean.txt",
			wantBranch:   "main",
			wantModified: 0,
		},
		{
			// modified.txt: 4 header lines, 3 non-# lines (2 tracked changes + 1 untracked).
			// Expected: branch="feature/my-feature", modified=3.
			name:         "feature branch with 3 changes",
			fixture:      "modified.txt",
			wantBranch:   "feature/my-feature",
			wantModified: 3,
		},
		{
			// detached.txt: 2 header lines, 1 non-# line (tracked change).
			// Expected: branch="(detached)", modified=1.
			name:         "detached HEAD with 1 change",
			fixture:      "detached.txt",
			wantBranch:   "(detached)",
			wantModified: 1,
			wantDetached: true,
		},
		{
			// worktree.txt: 4 header lines, 2 non-# lines (1 tracked + 1 untracked).
			// Expected: branch="worktree-branch", modified=2.
			name:         "worktree branch with 2 changes",
			fixture:      "worktree.txt",
			wantBranch:   "worktree-branch",
			wantModified: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := fixturePorcelain(t, tc.fixture)

			// Given: fixture output of `git status --porcelain=v2 --branch`.
			// When: ParsePorcelainV2 processes it.
			gs, err := parser.ParsePorcelainV2(output)

			// Then: no error, valid GitStatus with correct branch and count.
			if err != nil {
				t.Fatalf("ParsePorcelainV2: unexpected error: %v", err)
			}
			if gs == nil {
				t.Fatal("ParsePorcelainV2: expected non-nil *GitStatus, got nil")
			}
			if gs.Branch != tc.wantBranch {
				t.Errorf("Branch: got %q, want %q", gs.Branch, tc.wantBranch)
			}
			if gs.ModifiedCount != tc.wantModified {
				t.Errorf("ModifiedCount: got %d, want %d", gs.ModifiedCount, tc.wantModified)
			}
			// Verify detached HEAD is preserved literally (not normalised away).
			if tc.wantDetached && !strings.HasPrefix(gs.Branch, "(detached)") {
				t.Errorf("expected detached HEAD branch, got %q", gs.Branch)
			}
		})
	}
}

// TestDetectGit_PorcelainV2_EmptyOutput verifies that an empty output string
// returns an error (not a nil GitStatus silently).
// T-13 insurance: format must be parseable; empty = unrecognised input.
func TestDetectGit_PorcelainV2_EmptyOutput(t *testing.T) {
	gs, err := parser.ParsePorcelainV2("")
	if err == nil {
		t.Errorf("expected error for empty porcelain output, got nil (gs=%+v)", gs)
	}
}
