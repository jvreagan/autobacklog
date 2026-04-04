package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// Issue represents a GitHub issue.
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
}

// EnsureLabel creates a GitHub label if it doesn't already exist.
func EnsureLabel(ctx context.Context, workDir, label string, log *slog.Logger) error {
	log.Info("ensuring GitHub label exists", "label", label)

	cmd := exec.CommandContext(ctx, "gh", "label", "create", label,
		"--description", "Managed by autobacklog",
		"--color", "0E8A16",
		"--force")
	cmd.Dir = workDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh label create: %w\n%s", err, stderr.String())
	}
	return nil
}

// CreateIssue creates a GitHub issue using the gh CLI and returns the issue number.
func CreateIssue(ctx context.Context, workDir, title, body string, labels []string, log *slog.Logger) (int, error) {
	log.Info("creating GitHub issue", "title", title)

	args := []string{"issue", "create", "--title", title, "--body", body}
	for _, label := range labels {
		args = append(args, "--label", label)
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("gh issue create: %w\n%s", err, stderr.String())
	}

	issueURL := strings.TrimSpace(stdout.String())
	num, err := parseIssueNumber(issueURL)
	if err != nil {
		return 0, fmt.Errorf("parsing issue URL %q: %w", issueURL, err)
	}

	log.Info("GitHub issue created", "number", num, "url", issueURL)
	return num, nil
}

// ListIssues lists open GitHub issues with the given label using the gh CLI.
func ListIssues(ctx context.Context, workDir, label string, log *slog.Logger) ([]Issue, error) {
	log.Info("listing GitHub issues", "label", label)

	args := []string{
		"issue", "list",
		"--label", label,
		"--state", "open",
		"--json", "number,title,body,state",
		"--limit", "100",
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh issue list: %w\n%s", err, stderr.String())
	}

	var issues []Issue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing issue list JSON: %w", err)
	}

	log.Info("listed GitHub issues", "count", len(issues), "label", label)
	return issues, nil
}

// parseIssueNumber extracts the issue number from a GitHub issue URL.
// Expected format: https://github.com/owner/repo/issues/123
func parseIssueNumber(url string) (int, error) {
	url = strings.TrimRight(url, "/")
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty URL")
	}
	num, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid issue number in URL: %w", err)
	}
	return num, nil
}
