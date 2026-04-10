package git

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// CreateBranch creates and checks out a branch. If the branch already exists
// locally (e.g. from a previous cycle that crashed), it is checked out and
// reset to the base branch so implementation starts from a clean state (#127).
func (r *Repo) CreateBranch(ctx context.Context, prefix, category, title string) (string, error) {
	branchName := FormatBranchName(prefix, category, title)

	exists, err := r.branchExists(ctx, branchName)
	if err != nil {
		return "", fmt.Errorf("checking branch %s: %w", branchName, err)
	}

	if exists {
		r.log.Info("branch already exists, reusing", "name", branchName)
		// #143: use runGit for consistency (credentials available if needed)
		if err := r.runGit(ctx, r.workDir, "checkout", branchName); err != nil {
			return "", fmt.Errorf("checking out existing branch %s: %w", branchName, err)
		}
		// #127: reset to base branch, not HEAD (which is a no-op)
		if err := r.runGit(ctx, r.workDir, "reset", "--hard", r.branch); err != nil {
			return "", fmt.Errorf("resetting branch %s to %s: %w", branchName, r.branch, err)
		}
		return branchName, nil
	}

	r.log.Info("creating branch", "name", branchName)
	// #143: use runGit for consistency
	if err := r.runGit(ctx, r.workDir, "checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("creating branch %s: %w", branchName, err)
	}
	return branchName, nil
}

// branchExists reports whether the named branch exists locally.
// #142: use runGit-style execution instead of raw exec.CommandContext.
func (r *Repo) branchExists(ctx context.Context, name string) (bool, error) {
	// Use git rev-parse which is more reliable than git branch --list
	err := r.run(ctx, r.workDir, "git", "rev-parse", "--verify", "refs/heads/"+name)
	if err != nil {
		// Branch doesn't exist — not an error condition
		return false, nil
	}
	return true, nil
}

// CheckoutBranch checks out an existing branch.
// Branch names are sanitized by formatBranchName so they cannot start with "--".
func (r *Repo) CheckoutBranch(ctx context.Context, branch string) error {
	return r.runGit(ctx, r.workDir, "checkout", branch)
}

// Push pushes a branch to origin. Uses runGit so the credential helper is
// available for HTTPS authentication.
func (r *Repo) Push(ctx context.Context, branch string) error {
	r.log.Info("pushing branch", "name", branch)
	return r.runGit(ctx, r.workDir, "push", "origin", branch)
}

// DeleteBranch deletes a local branch. Called after successful PR creation and
// in failure paths (test failure, no changes, Claude error) to prevent branch
// accumulation in long-running daemon mode.
// #162: uses "--" separator to prevent flag injection.
func (r *Repo) DeleteBranch(ctx context.Context, branch string) error {
	return r.runGit(ctx, r.workDir, "branch", "-D", "--", branch)
}

// FormatBranchName creates a clean branch name from components.
// #163: sanitizes prefix and category in addition to title.
func FormatBranchName(prefix, category, title string) string {
	sanitize := func(s string) string {
		s = strings.ToLower(s)
		s = nonAlphaNum.ReplaceAllString(s, "-")
		return strings.Trim(s, "-")
	}

	slug := sanitize(title)
	// Truncate to keep branch name reasonable
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}

	return fmt.Sprintf("%s/%s/%s", sanitize(prefix), sanitize(category), slug)
}
