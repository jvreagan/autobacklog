package github

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
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

	prURL, err := runGH(ctx, workDir, log,
		"pr", "create",
		"--title", req.Title,
		"--body", req.Body,
		"--base", req.BaseBranch,
		"--head", req.HeadBranch,
	)
	if err != nil {
		return "", err
	}

	log.Info("pull request created", "url", prURL)
	return prURL, nil
}

// EnableAutoMerge enables GitHub auto-merge on a PR using `gh pr merge --squash --auto`.
// This tells GitHub to merge the PR automatically once all required checks pass.
func EnableAutoMerge(ctx context.Context, workDir string, prURL string, log *slog.Logger) error {
	log.Info("enabling auto-merge", "pr", prURL)

	_, err := runGH(ctx, workDir, log, "pr", "merge", prURL, "--squash", "--auto", "--delete-branch")
	if err != nil {
		return err
	}

	log.Info("auto-merge enabled", "pr", prURL)
	return nil
}

// PRState represents the status of a GitHub pull request.
type PRState string

const (
	PRStateOpen   PRState = "OPEN"
	PRStateMerged PRState = "MERGED"
	PRStateClosed PRState = "CLOSED"
)

// PRStatusResult holds the result of checking a PR's status.
type PRStatusResult struct {
	State PRState
}

// PRStatus checks the state of a PR using `gh pr view`.
func PRStatus(ctx context.Context, workDir, prURL string, log *slog.Logger) (*PRStatusResult, error) {
	out, err := runGH(ctx, workDir, log, "pr", "view", prURL, "--json", "state")
	if err != nil {
		return nil, err
	}

	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, fmt.Errorf("parsing pr view output: %w", err)
	}

	state := PRState(strings.ToUpper(result.State))
	log.Debug("checked PR status", "url", prURL, "state", state)
	return &PRStatusResult{State: state}, nil
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
	b.WriteString("*Created automatically by [autobacklog](https://github.com/jvreagan/autobacklog)*\n")
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

// PRReviewComment represents a single review comment on a PR.
type PRReviewComment struct {
	Body   string `json:"body"`
	Author string `json:"author"`
	State  string `json:"state"`
}

// PRReviewsResult holds the parsed reviews and metadata from a PR.
type PRReviewsResult struct {
	Reviews    []PRReviewComment
	HeadBranch string
}

// FetchPRReviews fetches review comments for the given PR URL using `gh pr view`.
func FetchPRReviews(ctx context.Context, workDir, prURL string, log *slog.Logger) (*PRReviewsResult, error) {
	out, err := runGH(ctx, workDir, log, "pr", "view", prURL, "--json", "reviews,comments,headRefName")
	if err != nil {
		return nil, err
	}

	var raw struct {
		Reviews []struct {
			Body   string `json:"body"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
		} `json:"reviews"`
		Comments []struct {
			Body   string `json:"body"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"comments"`
		HeadRefName string `json:"headRefName"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parsing pr reviews output: %w", err)
	}

	result := &PRReviewsResult{HeadBranch: raw.HeadRefName}
	for _, r := range raw.Reviews {
		if r.Body == "" {
			continue
		}
		result.Reviews = append(result.Reviews, PRReviewComment{
			Body:   r.Body,
			Author: r.Author.Login,
			State:  r.State,
		})
	}
	for _, c := range raw.Comments {
		if c.Body == "" {
			continue
		}
		result.Reviews = append(result.Reviews, PRReviewComment{
			Body:   c.Body,
			Author: c.Author.Login,
			State:  "COMMENTED",
		})
	}

	log.Debug("fetched PR reviews", "url", prURL, "review_count", len(result.Reviews), "head", result.HeadBranch)
	return result, nil
}

// ReviewsHash computes a SHA-256 hash of the review comment bodies for change detection.
func ReviewsHash(reviews []PRReviewComment) string {
	if len(reviews) == 0 {
		return ""
	}
	h := sha256.New()
	for _, r := range reviews {
		h.Write([]byte(r.Body))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// BatchPRItem holds the details for a single item in a batch PR body.
type BatchPRItem struct {
	Title       string
	Description string
	Category    string
	Priority    string
	IssueNumber int
}

// FormatBatchPRBody creates a structured PR body listing all batch items.
func FormatBatchPRBody(items []BatchPRItem, testResults string) string {
	var b strings.Builder
	b.WriteString("## Summary\n\n")
	b.WriteString("This PR implements multiple backlog items in a single batch:\n\n")

	for i, item := range items {
		fmt.Fprintf(&b, "### %d. %s\n\n", i+1, item.Title)
		b.WriteString(item.Description)
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "**Category:** %s | **Priority:** %s\n", item.Category, item.Priority)
		if item.IssueNumber > 0 {
			fmt.Fprintf(&b, "\nFixes #%d\n", item.IssueNumber)
		}
		b.WriteString("\n")
	}

	if testResults != "" {
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
	b.WriteString("*Created automatically by [autobacklog](https://github.com/jvreagan/autobacklog)*\n")
	return b.String()
}
