package git

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// Repo manages git operations on a working directory.
type Repo struct {
	url     string
	branch  string
	workDir string
	pat     string
	log     *slog.Logger
}

// NewRepo creates a new Repo instance.
func NewRepo(url, branch, workDir, pat string, log *slog.Logger) *Repo {
	return &Repo{
		url:     url,
		branch:  branch,
		workDir: workDir,
		pat:     pat,
		log:     log,
	}
}

// WorkDir returns the working directory path.
func (r *Repo) WorkDir() string {
	return r.workDir
}

// CloneOrPull clones the repo if not present, or pulls latest changes.
func (r *Repo) CloneOrPull(ctx context.Context) error {
	if _, err := os.Stat(r.workDir + "/.git"); err == nil {
		return r.pull(ctx)
	}
	return r.clone(ctx)
}

func (r *Repo) clone(ctx context.Context) error {
	r.log.Info("cloning repository", "url", r.url, "branch", r.branch, "dir", r.workDir)

	if err := os.MkdirAll(r.workDir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	cloneURL := r.authenticatedURL()
	err := r.run(ctx, "", "git", "clone", "--branch", r.branch, "--single-branch", cloneURL, r.workDir)
	if err != nil {
		return fmt.Errorf("cloning repo: %w", err)
	}

	return nil
}

func (r *Repo) pull(ctx context.Context) error {
	r.log.Info("pulling latest changes", "branch", r.branch)

	// Reset to clean state and pull
	if err := r.run(ctx, r.workDir, "git", "checkout", r.branch); err != nil {
		return fmt.Errorf("checking out %s: %w", r.branch, err)
	}
	if err := r.run(ctx, r.workDir, "git", "pull", "origin", r.branch); err != nil {
		return fmt.Errorf("pulling: %w", err)
	}

	return nil
}

// authenticatedURL inserts the PAT into the clone URL for HTTPS auth.
func (r *Repo) authenticatedURL() string {
	if r.pat == "" {
		return r.url
	}
	// https://github.com/user/repo.git → https://<pat>@github.com/user/repo.git
	if strings.HasPrefix(r.url, "https://") {
		return strings.Replace(r.url, "https://", "https://"+r.pat+"@", 1)
	}
	return r.url
}

func (r *Repo) run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		argStr := strings.Join(args, " ")
		if r.pat != "" {
			argStr = strings.ReplaceAll(argStr, r.pat, "[REDACTED]")
		}
		return fmt.Errorf("%s %s: %w\n%s", name, argStr, err, stderr.String())
	}
	return nil
}
