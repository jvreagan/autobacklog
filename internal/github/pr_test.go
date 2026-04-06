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

	// Triple backticks inside test results should be escaped to prevent
	// breaking the outer code fence.
	if strings.Contains(body, "```\ncode block\n```") {
		t.Error("backtick sequences in test output should be escaped")
	}
	if !strings.Contains(body, "` ` `") {
		t.Error("expected escaped backticks in body")
	}
}
