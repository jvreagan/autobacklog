package github

import (
	"strings"
	"testing"
)

func TestFormatPRBody_AllFields(t *testing.T) {
	body := FormatPRBody("Fix null pointer", "Handles nil check in handler", "bug", "ok\nall tests passed")

	for _, want := range []string{"## Summary", "## Category", "## Test Results", "bug", "Handles nil check"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestFormatPRBody_EmptyTestResults(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "")

	if strings.Contains(body, "## Test Results") {
		t.Error("Test Results section should be omitted when empty")
	}
}

func TestFormatPRBody_EmptyDescription(t *testing.T) {
	body := FormatPRBody("Fix", "", "bug", "ok")

	if !strings.Contains(body, "## Summary") {
		t.Error("should still contain Summary section")
	}
}

func TestFormatPRBody_ContainsAutobacklogFooter(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "")

	if !strings.Contains(body, "autobacklog") {
		t.Error("body should contain autobacklog footer")
	}
}

func TestFormatPRBody_TestResultsInCodeBlock(t *testing.T) {
	body := FormatPRBody("Fix", "desc", "bug", "ok\nall passed")

	if !strings.Contains(body, "```\nok\nall passed\n```") {
		t.Errorf("test results should be in code block, got:\n%s", body)
	}
}
