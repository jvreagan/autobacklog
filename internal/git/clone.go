package git

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
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
// Uses `git rev-parse --git-dir` instead of os.Stat(".git") for robustness
// with worktrees and partial clones (#170).
func (r *Repo) CloneOrPull(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = r.workDir
	if err := cmd.Run(); err == nil {
		return r.pull(ctx)
	}
	return r.clone(ctx)
}

func (r *Repo) clone(ctx context.Context) error {
	r.log.Info("cloning repository", "url", r.url, "branch", r.branch, "dir", r.workDir)

	if err := os.MkdirAll(r.workDir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	if err := r.runGit(ctx, "", "clone", "--branch", r.branch, "--single-branch", r.url, r.workDir); err != nil {
		return fmt.Errorf("cloning repo: %w", err)
	}

	return nil
}

func (r *Repo) pull(ctx context.Context) error {
	r.log.Info("pulling latest changes", "branch", r.branch)

	// Discard any leftover changes from a previous run — this is autobacklog's
	// scratch directory, not the user's working copy.
	if err := r.runGit(ctx, r.workDir, "reset", "--hard"); err != nil {
		return fmt.Errorf("resetting work dir: %w", err)
	}
	if err := r.runGit(ctx, r.workDir, "clean", "-fd"); err != nil {
		return fmt.Errorf("cleaning work dir: %w", err)
	}

	if err := r.runGit(ctx, r.workDir, "checkout", r.branch); err != nil {
		return fmt.Errorf("checking out %s: %w", r.branch, err)
	}
	if err := r.runGit(ctx, r.workDir, "pull", "origin", r.branch); err != nil {
		return fmt.Errorf("pulling: %w", err)
	}

	return nil
}

// runGit runs a git subcommand. When a PAT is configured, credentials are
// supplied via a transient credential helper that reads GIT_PAT from the
// process environment, keeping the secret out of the URL, git config, and
// command-line argument lists.
func (r *Repo) runGit(ctx context.Context, dir string, args ...string) error {
	if r.pat != "" {
		// First entry clears any pre-existing helpers; second installs ours.
		// The PAT is referenced via $GIT_PAT (set on the child process env),
		// so it never appears in command arguments or git remote configuration.
		credHelper := `!f(){ echo "username=x-access-token"; echo "password=$GIT_PAT"; };f`
		args = append([]string{
			"-c", "credential.helper=",
			"-c", "credential.helper=" + credHelper,
		}, args...)
	}
	return r.run(ctx, dir, "git", args...)
}

func (r *Repo) run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	// #218: filter out existing GIT_PAT and GIT_TERMINAL_PROMPT to avoid duplicates
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

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		argStr := strings.Join(args, " ")
		// #165: trim trailing whitespace/newlines from stderr
		errStr := strings.TrimSpace(stderr.String())
		if r.pat != "" {
			// Redact raw, slash-encoded, and fully URL-encoded forms of the PAT
			// to prevent credential leakage in error messages and logs.
			redactAll := func(s string) string {
				s = strings.ReplaceAll(s, r.pat, "[REDACTED]")
				s = strings.ReplaceAll(s, strings.ReplaceAll(r.pat, "/", "%2F"), "[REDACTED]")
				s = strings.ReplaceAll(s, url.PathEscape(r.pat), "[REDACTED]")
				s = strings.ReplaceAll(s, url.QueryEscape(r.pat), "[REDACTED]")
				return s
			}
			argStr = redactAll(argStr)
			errStr = redactAll(errStr)
		}
		if errStr != "" {
			return fmt.Errorf("%s %s: %w\n%s", name, argStr, err, errStr)
		}
		return fmt.Errorf("%s %s: %w", name, argStr, err)
	}
	return nil
}
