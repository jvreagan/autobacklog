package claude

import (
	"strings"
	"testing"

	"github.com/jamesreagan/autobacklog/internal/backlog"
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
