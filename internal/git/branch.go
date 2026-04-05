package git

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// CreateBranch creates and checks out a branch. If the branch already exists
// locally (e.g. from a previous cycle that crashed), it is checked out and
// reset to HEAD so implementation starts from a clean state.
func (r *Repo) CreateBranch(ctx context.Context, prefix, category, title string) (string, error) {
	branchName := formatBranchName(prefix, category, title)

	exists, err := r.branchExists(ctx, branchName)
	if err != nil {
		return "", fmt.Errorf("checking branch %s: %w", branchName, err)
	}

	if exists {
		r.log.Info("branch already exists, reusing", "name", branchName)
		if err := r.run(ctx, r.workDir, "git", "checkout", branchName); err != nil {
			return "", fmt.Errorf("checking out existing branch %s: %w", branchName, err)
		}
		if err := r.run(ctx, r.workDir, "git", "reset", "--hard", "HEAD"); err != nil {
			return "", fmt.Errorf("resetting branch %s: %w", branchName, err)
		}
		return branchName, nil
	}

	r.log.Info("creating branch", "name", branchName)
	if err := r.run(ctx, r.workDir, "git", "checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("creating branch %s: %w", branchName, err)
	}
	return branchName, nil
}

// branchExists reports whether the named branch exists locally.
func (r *Repo) branchExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--list", name)
	cmd.Dir = r.workDir
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// CheckoutBranch checks out an existing branch.
func (r *Repo) CheckoutBranch(ctx context.Context, branch string) error {
	return r.run(ctx, r.workDir, "git", "checkout", branch)
}

// Push pushes a branch to origin.
func (r *Repo) Push(ctx context.Context, branch string) error {
	r.log.Info("pushing branch", "name", branch)
	return r.run(ctx, r.workDir, "git", "push", "origin", branch)
}

// DeleteBranch deletes a local branch.
func (r *Repo) DeleteBranch(ctx context.Context, branch string) error {
	return r.run(ctx, r.workDir, "git", "branch", "-D", branch)
}

// formatBranchName creates a clean branch name from components.
func formatBranchName(prefix, category, title string) string {
	slug := strings.ToLower(title)
	slug = nonAlphaNum.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	// Truncate to keep branch name reasonable
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}
	return fmt.Sprintf("%s/%s/%s", prefix, strings.ToLower(category), slug)
}
