package parser

// GitStatus holds a lightweight snapshot of the working-tree git state.
// The detector that populates this struct is implemented in Phase 4.1.c.
//
// TODO(4.1.c): add DetectGit(ctx context.Context, dir string) (*GitStatus, error)
type GitStatus struct {
	Branch        string
	ModifiedCount int
	Worktree      string
}
