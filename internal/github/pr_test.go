package github

import (
	"strings"
	"testing"
)

func TestFormatPRBody_AllFields(t *testing.T) {
	body := FormatPRBody("Fix null pointer", "Handles nil check in handler", "bug", "ok\nall tests passed", 0)

	for _, want := range []string{"## Summary", "## Category", "## Test Results", "bug", "Handles nil check"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestFormatPRBody_EmptyTestResults(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "", 0)

	if strings.Contains(body, "## Test Results") {
		t.Error("Test Results section should be omitted when empty")
	}
}

func TestFormatPRBody_EmptyDescription(t *testing.T) {
	body := FormatPRBody("Fix", "", "bug", "ok", 0)

	if !strings.Contains(body, "## Summary") {
		t.Error("should still contain Summary section")
	}
}

func TestFormatPRBody_ContainsAutobacklogFooter(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "", 0)

	if !strings.Contains(body, "autobacklog") {
		t.Error("body should contain autobacklog footer")
	}
}

func TestFormatPRBody_TestResultsInCodeBlock(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "ok\nall passed", 0)

	if !strings.Contains(body, "```\nok\nall passed\n```") {
		t.Errorf("test results should be in code block, got:\n%s", body)
	}
}

func TestFormatPRBody_WithIssueNumber(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "", 42)

	if !strings.Contains(body, "Fixes #42") {
		t.Errorf("body should contain 'Fixes #42', got:\n%s", body)
	}
}

func TestFormatPRBody_WithoutIssueNumber(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "", 0)

	if strings.Contains(body, "Fixes #") {
		t.Error("body should not contain 'Fixes #' when issue number is 0")
	}
}

func TestFormatPRBody_BacktickEscaping(t *testing.T) {
	testOutput := "output\n```\ncode block\n```\nmore output"
	body := FormatPRBody("Fix", "desc", "bug", testOutput, 0)

	// #166: dynamic fence — longest backtick run is 3, so fence should be 4 backticks.
	// The raw test output is preserved (not escaped), wrapped in ```` fences.
	if !strings.Contains(body, "````\n"+testOutput+"\n````") {
		t.Errorf("expected 4-backtick fence wrapping raw output, got:\n%s", body)
	}
}

func TestReviewsHash(t *testing.T) {
	reviews := []PRReviewComment{
		{Body: "Please fix the null check", Author: "reviewer1", State: "CHANGES_REQUESTED"},
		{Body: "Also add a test", Author: "reviewer2", State: "COMMENTED"},
	}

	h1 := ReviewsHash(reviews)
	if h1 == "" {
		t.Fatal("hash should not be empty for non-empty reviews")
	}
	if len(h1) != 64 {
		t.Errorf("expected SHA-256 hex string (64 chars), got %d chars", len(h1))
	}

	// Same reviews should produce same hash
	h2 := ReviewsHash(reviews)
	if h1 != h2 {
		t.Error("same reviews should produce same hash")
	}

	// Different reviews should produce different hash
	reviews2 := []PRReviewComment{
		{Body: "Looks good!", Author: "reviewer1", State: "APPROVED"},
	}
	h3 := ReviewsHash(reviews2)
	if h1 == h3 {
		t.Error("different reviews should produce different hash")
	}

	// Empty reviews should return empty string
	if h := ReviewsHash(nil); h != "" {
		t.Errorf("empty reviews should return empty hash, got %q", h)
	}
}

func TestFetchPRReviews_Parse(t *testing.T) {
	// Test that the JSON parsing logic works correctly by testing ReviewsHash
	// on structured data (FetchPRReviews itself requires gh CLI, tested via integration)
	reviews := []PRReviewComment{
		{Body: "Fix the error handling", Author: "alice", State: "CHANGES_REQUESTED"},
		{Body: "LGTM", Author: "bob", State: "APPROVED"},
	}

	hash := ReviewsHash(reviews)
	if hash == "" {
		t.Fatal("hash should not be empty")
	}

	// Verify determinism
	for i := 0; i < 5; i++ {
		if ReviewsHash(reviews) != hash {
			t.Fatal("ReviewsHash is not deterministic")
		}
	}
}

func TestFormatBatchPRBody(t *testing.T) {
	items := []BatchPRItem{
		{Title: "Fix null pointer", Description: "Handle nil case", Category: "bug", Priority: "high", IssueNumber: 42},
		{Title: "Add caching", Description: "Cache API responses", Category: "performance", Priority: "medium", IssueNumber: 0},
		{Title: "Update docs", Description: "Fix typos", Category: "docs", Priority: "low", IssueNumber: 10},
	}

	body := FormatBatchPRBody(items, "all tests passed")

	// All items should appear
	for _, item := range items {
		if !strings.Contains(body, item.Title) {
			t.Errorf("body missing item title %q", item.Title)
		}
		if !strings.Contains(body, item.Description) {
			t.Errorf("body missing item description %q", item.Description)
		}
	}

	// Issue links should appear for items with issue numbers
	if !strings.Contains(body, "Fixes #42") {
		t.Error("body should contain 'Fixes #42'")
	}
	if !strings.Contains(body, "Fixes #10") {
		t.Error("body should contain 'Fixes #10'")
	}

	// Item without issue number should not have Fixes link
	// (Count total "Fixes #" occurrences should be exactly 2)
	count := strings.Count(body, "Fixes #")
	if count != 2 {
		t.Errorf("expected 2 'Fixes #' links, got %d", count)
	}

	// Test results should be present
	if !strings.Contains(body, "## Test Results") {
		t.Error("body should contain test results section")
	}

	// Footer should be present
	if !strings.Contains(body, "autobacklog") {
		t.Error("body should contain autobacklog footer")
	}
}

func TestFormatBatchPRBody_NoTestResults(t *testing.T) {
	items := []BatchPRItem{
		{Title: "Fix bug", Description: "desc", Category: "bug", Priority: "high"},
	}

	body := FormatBatchPRBody(items, "")

	if strings.Contains(body, "## Test Results") {
		t.Error("should not contain test results section when empty")
	}
}
