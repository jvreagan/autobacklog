package github

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// PRRequest contains the details for creating a pull request.
type PRRequest struct {
	Title      string
	Body       string
	BaseBranch string
	HeadBranch string
}

// CreatePR creates a GitHub pull request using the gh CLI.
// Returns the PR URL.
func CreatePR(ctx context.Context, workDir string, req PRRequest, log *slog.Logger) (string, error) {
	log.Info("creating pull request", "title", req.Title, "base", req.BaseBranch, "head", req.HeadBranch)

	args := []string{
		"pr", "create",
		"--title", req.Title,
		"--body", req.Body,
		"--base", req.BaseBranch,
		"--head", req.HeadBranch,
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = workDir
	cmd.Env = ghEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh pr create: %w\n%s", err, stderr.String())
	}

	prURL := strings.TrimSpace(stdout.String())
	log.Info("pull request created", "url", prURL)
	return prURL, nil
}

// EnableAutoMerge enables GitHub auto-merge on a PR using `gh pr merge --squash --auto`.
// This tells GitHub to merge the PR automatically once all required checks pass.
func EnableAutoMerge(ctx context.Context, workDir string, prURL string, log *slog.Logger) error {
	log.Info("enabling auto-merge", "pr", prURL)

	cmd := exec.CommandContext(ctx, "gh", "pr", "merge", prURL, "--squash", "--auto", "--delete-branch")
	cmd.Dir = workDir
	cmd.Env = ghEnv()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr merge --auto: %w\n%s", err, stderr.String())
	}

	log.Info("auto-merge enabled", "pr", prURL)
	return nil
}

// FormatPRBody creates a structured PR body from the given fields.
// If issueNumber > 0, a "Fixes #N" line is included to auto-close the linked issue.
// #167: removed unused title parameter.
func FormatPRBody(_, description, category, testResults string, issueNumber int) string {
	var b strings.Builder
	b.WriteString("## Summary\n\n")
	b.WriteString(description)
	b.WriteString("\n\n")
	if issueNumber > 0 {
		fmt.Fprintf(&b, "Fixes #%d\n\n", issueNumber)
	}
	b.WriteString("## Category\n\n")
	b.WriteString(category)
	b.WriteString("\n\n")
	if testResults != "" {
		// #166: find the longest run of backticks in testResults and use a fence longer than that
		fence := "```"
		maxRun := longestBacktickRun(testResults)
		if maxRun >= 3 {
			fence = strings.Repeat("`", maxRun+1)
		}
		b.WriteString("## Test Results\n\n")
		b.WriteString(fence + "\n")
		b.WriteString(testResults)
		b.WriteString("\n" + fence + "\n\n")
	}
	b.WriteString("---\n")
	b.WriteString("*Created automatically by [autobacklog](https://github.com/jamesreagan/autobacklog)*\n")
	return b.String()
}

// longestBacktickRun returns the length of the longest consecutive run of backticks.
func longestBacktickRun(s string) int {
	max, cur := 0, 0
	for _, c := range s {
		if c == '`' {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	return max
}

// escapeCodeFence is kept for backward compatibility but the caller now uses
// dynamic fence length instead.
func escapeCodeFence(s string) string {
	return s
}
