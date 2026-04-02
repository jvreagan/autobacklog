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

// FormatPRBody creates a structured PR body from the given fields.
func FormatPRBody(title, description, category, testResults string) string {
	var b strings.Builder
	b.WriteString("## Summary\n\n")
	b.WriteString(description)
	b.WriteString("\n\n")
	b.WriteString("## Category\n\n")
	b.WriteString(category)
	b.WriteString("\n\n")
	if testResults != "" {
		b.WriteString("## Test Results\n\n")
		b.WriteString("```\n")
		b.WriteString(testResults)
		b.WriteString("\n```\n\n")
	}
	b.WriteString("---\n")
	b.WriteString("*Created automatically by [autobacklog](https://github.com/jamesreagan/autobacklog)*\n")
	return b.String()
}
