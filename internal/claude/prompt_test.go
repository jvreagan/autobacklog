package claude

import (
	"strings"
	"testing"

	"github.com/jvreagan/autobacklog/internal/backlog"
)

func TestReviewPrompt(t *testing.T) {
	p := ReviewPrompt()

	if p == "" {
		t.Fatal("ReviewPrompt should not be empty")
	}
	if !strings.Contains(p, "JSON") {
		t.Error("should mention JSON output format")
	}
	if !strings.Contains(p, "priority") {
		t.Error("should mention priority")
	}
	if !strings.Contains(p, "category") {
		t.Error("should mention category")
	}
}

func TestImplementPrompt(t *testing.T) {
	item := &backlog.Item{
		Title:       "Fix null pointer",
		Description: "Handle nil case in handler",
		FilePath:    "handlers/user.go",
		Category:    backlog.CategoryBug,
		Priority:    backlog.PriorityHigh,
	}

	p := ImplementPrompt(item)

	if !strings.Contains(p, "Fix null pointer") {
		t.Error("should contain title")
	}
	if !strings.Contains(p, "Handle nil case") {
		t.Error("should contain description")
	}
	if !strings.Contains(p, "handlers/user.go") {
		t.Error("should contain file path")
	}
	if !strings.Contains(p, "bug") {
		t.Error("should contain category")
	}
	if !strings.Contains(p, "high") {
		t.Error("should contain priority")
	}
}

func TestFixTestPrompt(t *testing.T) {
	output := "FAIL TestFoo: expected 42, got 0"
	p := FixTestPrompt(output)

	if !strings.Contains(p, output) {
		t.Error("should contain test output")
	}
	if !strings.Contains(p, "Do not disable") {
		t.Error("should instruct not to disable tests")
	}
}

func TestDocumentPrompt(t *testing.T) {
	changes := []string{"Fixed auth bug", "Added rate limiting"}
	p := DocumentPrompt(changes)

	if !strings.Contains(p, "Fixed auth bug") {
		t.Error("should contain first change")
	}
	if !strings.Contains(p, "Added rate limiting") {
		t.Error("should contain second change")
	}
}

func TestDetectTestPrompt(t *testing.T) {
	p := DetectTestPrompt()

	if p == "" {
		t.Fatal("DetectTestPrompt should not be empty")
	}
	if !strings.Contains(p, "JSON") {
		t.Error("should request JSON output")
	}
	if !strings.Contains(p, "command") {
		t.Error("should mention command field")
	}
}

func TestAddressReviewPrompt(t *testing.T) {
	p := AddressReviewPrompt("Fix null pointer", "Please add a nil check before dereferencing")

	if !strings.Contains(p, "Fix null pointer") {
		t.Error("should contain item title")
	}
	if !strings.Contains(p, "Please add a nil check") {
		t.Error("should contain review feedback")
	}
	if !strings.Contains(p, "<review-feedback>") {
		t.Error("should wrap feedback in XML tags")
	}
	if !strings.Contains(p, "docs/") {
		t.Error("should contain docs directive")
	}
}

func TestAddressReviewPrompt_Truncation(t *testing.T) {
	longFeedback := strings.Repeat("x", maxPromptTestOutput+1000)
	p := AddressReviewPrompt("title", longFeedback)

	if !strings.Contains(p, "... (truncated) ...") {
		t.Error("should contain truncation marker")
	}
	// Prompt should be shorter than if all feedback were included
	pFull := AddressReviewPrompt("title", "short")
	overhead := len(pFull) - len("short")
	if len(p) > overhead+maxPromptTestOutput+100 {
		t.Errorf("prompt too long: %d chars, expected around %d", len(p), overhead+maxPromptTestOutput)
	}
}

func TestBatchImplementPrompt(t *testing.T) {
	items := []*backlog.Item{
		{Title: "Fix null pointer", Description: "Handle nil case", FilePath: "handlers/user.go", Category: backlog.CategoryBug, Priority: backlog.PriorityHigh},
		{Title: "Add caching", Description: "Cache API responses", FilePath: "api/client.go", Category: backlog.CategoryPerformance, Priority: backlog.PriorityMedium},
		{Title: "Update docs", Description: "Fix typos in README", FilePath: "README.md", Category: backlog.CategoryDocs, Priority: backlog.PriorityLow},
	}

	p := BatchImplementPrompt(items)

	// All items should appear with XML tags
	for i, item := range items {
		if !strings.Contains(p, item.Title) {
			t.Errorf("should contain title of item %d", i)
		}
		if !strings.Contains(p, item.Description) {
			t.Errorf("should contain description of item %d", i)
		}
		if !strings.Contains(p, item.FilePath) {
			t.Errorf("should contain file path of item %d", i)
		}
	}

	if !strings.Contains(p, `<backlog-item index="1">`) {
		t.Error("should contain indexed XML tags")
	}
	if !strings.Contains(p, `<backlog-item index="3">`) {
		t.Error("should contain XML tag for third item")
	}
	if !strings.Contains(p, "docs/") {
		t.Error("should contain docs directive")
	}
}
