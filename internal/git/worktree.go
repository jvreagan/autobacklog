package git

import (
	"context"
	"fmt"
)

// AddWorktree creates a new detached git worktree at the given path based on
// the repo's configured branch. The worktree is detached (no branch checked
// out) so that implementItem can create its own feature branch.
func (r *Repo) AddWorktree(ctx context.Context, path string) error {
	r.log.Info("adding worktree", "path", path)
	return r.runGit(ctx, r.workDir, "worktree", "add", "--detach", path, r.branch)
}

// RemoveWorktree removes a git worktree.
func (r *Repo) RemoveWorktree(ctx context.Context, path string) error {
	r.log.Info("removing worktree", "path", path)
	if err := r.runGit(ctx, r.workDir, "worktree", "remove", path, "--force"); err != nil {
		return fmt.Errorf("removing worktree %s: %w", path, err)
	}
	return nil
}

// NewWorktreeRepo creates a new Repo instance that operates in the given
// worktree directory. It shares the same URL, branch, and PAT but uses a
// different working directory.
func (r *Repo) NewWorktreeRepo(path string) *Repo {
	return &Repo{
		url:     r.url,
		branch:  r.branch,
		workDir: path,
		pat:     r.pat,
		log:     r.log,
	}
}
