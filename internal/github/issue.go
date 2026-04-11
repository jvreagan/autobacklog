package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Issue represents a GitHub issue.
type Issue struct {
	Number      int          `json:"number"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	State       string       `json:"state"`
	Labels      []IssueLabel `json:"labels"`
	PullRequest *struct{}    `json:"pull_request,omitempty"` // non-nil when the issue is actually a PR
}

// IssueLabel represents a GitHub issue label.
type IssueLabel struct {
	Name string `json:"name"`
}

// EnsureLabel creates a GitHub label if it doesn't already exist.
func EnsureLabel(ctx context.Context, workDir, label string, log *slog.Logger) error {
	log.Info("ensuring GitHub label exists", "label", label)

	_, err := runGH(ctx, workDir, log, "label", "create", label,
		"--description", "Managed by autobacklog",
		"--color", "0E8A16",
		"--force")
	if err != nil {
		return err
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

	issueURL, err := runGH(ctx, workDir, log, args...)
	if err != nil {
		return 0, err
	}

	num, err := parseIssueNumber(issueURL)
	if err != nil {
		return 0, fmt.Errorf("parsing issue URL %q: %w", issueURL, err)
	}

	log.Info("GitHub issue created", "number", num, "url", issueURL)
	return num, nil
}

// repoNWO returns the "owner/repo" name-with-owner for the repository in workDir.
func repoNWO(ctx context.Context, workDir string, log *slog.Logger) (string, error) {
	return runGH(ctx, workDir, log, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
}

// ListIssues lists open GitHub issues with the given label using the GitHub REST API
// with automatic pagination via gh api --paginate.
func ListIssues(ctx context.Context, workDir, label string, log *slog.Logger) ([]Issue, error) {
	log.Info("listing GitHub issues", "label", label)

	nwo, err := repoNWO(ctx, workDir, log)
	if err != nil {
		return nil, fmt.Errorf("determining repo: %w", err)
	}

	output, err := ghAPIPaginate(ctx, workDir, fmt.Sprintf(
		"/repos/%s/issues?state=open&labels=%s&per_page=100", nwo, label,
	), log)
	if err != nil {
		return nil, err
	}

	issues, err := parsePagedJSON(output)
	if err != nil {
		return nil, fmt.Errorf("parsing paginated issue list: %w", err)
	}

	// Filter out pull requests (the REST API includes PRs in the issues endpoint).
	filtered := issues[:0]
	for _, issue := range issues {
		if issue.PullRequest != nil {
			continue
		}
		filtered = append(filtered, issue)
	}

	log.Info("listed GitHub issues", "count", len(filtered), "label", label)
	return filtered, nil
}

// ghAPIPaginate calls gh api --paginate. Rate-limit retry is handled by runGH.
func ghAPIPaginate(ctx context.Context, workDir, endpoint string, log *slog.Logger) (string, error) {
	return runGH(ctx, workDir, log, "api", "--paginate", endpoint)
}

// parsePagedJSON merges the concatenated JSON arrays produced by gh api --paginate.
// When --paginate returns multiple pages, it outputs multiple JSON arrays
// concatenated together (e.g., "[...][...]"). This function merges them into one.
func parsePagedJSON(output string) ([]Issue, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var all []Issue
	decoder := json.NewDecoder(strings.NewReader(output))
	for decoder.More() {
		var page []Issue
		if err := decoder.Decode(&page); err != nil {
			return nil, fmt.Errorf("decoding paged JSON: %w", err)
		}
		all = append(all, page...)
	}
	return all, nil
}

// LabelNames returns a slice of label name strings.
func (i Issue) LabelNames() []string {
	names := make([]string, len(i.Labels))
	for j, l := range i.Labels {
		names[j] = l.Name
	}
	return names
}

// parseIssueNumber extracts the issue number from a GitHub issue URL.
// Expected format: https://github.com/owner/repo/issues/123
func parseIssueNumber(url string) (int, error) {
	url = strings.TrimRight(url, "/")
	if url == "" {
		return 0, fmt.Errorf("empty URL")
	}
	parts := strings.Split(url, "/")
	last := parts[len(parts)-1]
	num, err := strconv.Atoi(last)
	if err != nil {
		return 0, fmt.Errorf("last URL segment %q is not a number: %w", last, err)
	}
	// #164: reject negative and zero issue numbers
	if num <= 0 {
		return 0, fmt.Errorf("invalid issue number: %d", num)
	}
	return num, nil
}
