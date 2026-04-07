package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// StageAll stages all changes in the working directory.
func (r *Repo) StageAll(ctx context.Context) error {
	return r.run(ctx, r.workDir, "git", "add", "-A")
}

// Commit creates a commit with the given message.
// Sets git user identity via -c flags so commits succeed even without global git config.
func (r *Repo) Commit(ctx context.Context, message string) error {
	r.log.Info("committing changes", "message", message)
	return r.run(ctx, r.workDir, "git",
		"-c", "user.name=autobacklog",
		"-c", "user.email=autobacklog@noreply",
		"commit", "-m", message)
}

// HasChanges checks if there are any staged or unstaged changes.
// #144: reuses the same environment filtering as run() to avoid divergence.
func (r *Repo) HasChanges(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = r.workDir
	// Build env the same way run() does (#144)
	env := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_PAT=") && !strings.HasPrefix(e, "GIT_TERMINAL_PROMPT=") {
			env = append(env, e)
		}
	}
	if r.pat != "" {
		env = append(env, "GIT_PAT="+r.pat, "GIT_TERMINAL_PROMPT=0")
	}
	cmd.Env = env
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}

	return strings.TrimSpace(stdout.String()) != "", nil
}

// RevertToClean resets the working directory to the last commit.
// #172: uses -fdx to also remove gitignored files created during implementation.
func (r *Repo) RevertToClean(ctx context.Context) error {
	r.log.Warn("reverting working directory to clean state")
	if err := r.run(ctx, r.workDir, "git", "checkout", "."); err != nil {
		return err
	}
	return r.run(ctx, r.workDir, "git", "clean", "-fdx")
}
