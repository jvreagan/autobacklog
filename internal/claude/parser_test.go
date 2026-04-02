package claude

import (
	"testing"

	"github.com/jamesreagan/autobacklog/internal/backlog"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean array",
			input: `[{"title":"bug"}]`,
			want:  `[{"title":"bug"}]`,
		},
		{
			name:  "array with surrounding text",
			input: "Here are the findings:\n[{\"title\":\"bug\"}]\nThat's all.",
			want:  `[{"title":"bug"}]`,
		},
		{
			name:  "object",
			input: `{"command":"go test"}`,
			want:  `{"command":"go test"}`,
		},
		{
			name:  "object with commentary",
			input: "Based on my analysis:\n{\"command\":\"go test\"}\nLet me know if you need more.",
			want:  `{"command":"go test"}`,
		},
		{
			name:  "no JSON",
			input: "no json here",
			want:  "no json here",
		},
		{
			name:  "whitespace",
			input: "  \n  [{\"a\":1}]  \n  ",
			want:  `[{"a":1}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePriority(t *testing.T) {
	tests := []struct {
		input string
		want  backlog.Priority
	}{
		{"high", backlog.PriorityHigh},
		{"HIGH", backlog.PriorityHigh},
		{"  High  ", backlog.PriorityHigh},
		{"medium", backlog.PriorityMedium},
		{"Medium", backlog.PriorityMedium},
		{"low", backlog.PriorityLow},
		{"unknown", backlog.PriorityLow},
		{"", backlog.PriorityLow},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizePriority(tt.input)
			if got != tt.want {
				t.Errorf("normalizePriority(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		input string
		want  backlog.Category
	}{
		{"bug", backlog.CategoryBug},
		{"Bug", backlog.CategoryBug},
		{"security", backlog.CategorySecurity},
		{"performance", backlog.CategoryPerformance},
		{"refactor", backlog.CategoryRefactor},
		{"test", backlog.CategoryTest},
		{"docs", backlog.CategoryDocs},
		{"style", backlog.CategoryStyle},
		{"unknown", backlog.CategoryRefactor},
		{"", backlog.CategoryRefactor},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeCategory(tt.input)
			if got != tt.want {
				t.Errorf("normalizeCategory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseReviewOutput_RawJSON(t *testing.T) {
	input := `[
		{
			"title": "SQL injection vulnerability",
			"description": "Use parameterized queries",
			"file_path": "db/queries.go",
			"line_number": 42,
			"priority": "high",
			"category": "security"
		},
		{
			"title": "Missing error check",
			"description": "Handle error from Close()",
			"file_path": "server.go",
			"line_number": 15,
			"priority": "medium",
			"category": "bug"
		}
	]`

	items, _, err := ParseReviewOutput(input)
	if err != nil {
		t.Fatalf("ParseReviewOutput: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}

	if items[0].Title != "SQL injection vulnerability" {
		t.Errorf("items[0].Title = %q", items[0].Title)
	}
	if items[0].FilePath != "db/queries.go" {
		t.Errorf("items[0].FilePath = %q", items[0].FilePath)
	}
	if items[0].LineNumber != 42 {
		t.Errorf("items[0].LineNumber = %d", items[0].LineNumber)
	}
	if items[0].Priority != backlog.PriorityHigh {
		t.Errorf("items[0].Priority = %q", items[0].Priority)
	}
	if items[0].Category != backlog.CategorySecurity {
		t.Errorf("items[0].Category = %q", items[0].Category)
	}

	if items[1].Title != "Missing error check" {
		t.Errorf("items[1].Title = %q", items[1].Title)
	}
	if items[1].Priority != backlog.PriorityMedium {
		t.Errorf("items[1].Priority = %q", items[1].Priority)
	}
}

func TestParseReviewOutput_ClaudeJSONWrapper(t *testing.T) {
	input := `{"result":"[{\"title\":\"Fix bug\",\"description\":\"desc\",\"file_path\":\"a.go\",\"line_number\":1,\"priority\":\"low\",\"category\":\"bug\"}]","cost_usd":{"input":0.01,"output":0.02,"total":0.03}}`

	items, cost, err := ParseReviewOutput(input)
	if err != nil {
		t.Fatalf("ParseReviewOutput: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Title != "Fix bug" {
		t.Errorf("Title = %q", items[0].Title)
	}
	if cost != 0.03 {
		t.Errorf("cost = %f, want 0.03", cost)
	}
}

func TestParseReviewOutput_EmptyArray(t *testing.T) {
	input := `[]`

	items, _, err := ParseReviewOutput(input)
	if err != nil {
		t.Fatalf("ParseReviewOutput: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("len = %d, want 0", len(items))
	}
}

func TestParseReviewOutput_Invalid(t *testing.T) {
	_, _, err := ParseReviewOutput("not json at all")
	if err == nil {
		t.Error("should error on invalid input")
	}
}

func TestParseTestDetection(t *testing.T) {
	input := `{"command": "npm test", "framework": "jest"}`

	det, err := ParseTestDetection(input)
	if err != nil {
		t.Fatalf("ParseTestDetection: %v", err)
	}

	if det.Command != "npm test" {
		t.Errorf("Command = %q", det.Command)
	}
	if det.Framework != "jest" {
		t.Errorf("Framework = %q", det.Framework)
	}
}

func TestParseTestDetection_WithCommentary(t *testing.T) {
	input := "Based on the project structure:\n{\"command\": \"pytest\", \"framework\": \"pytest\"}\nThis project uses pytest."

	det, err := ParseTestDetection(input)
	if err != nil {
		t.Fatalf("ParseTestDetection: %v", err)
	}

	if det.Command != "pytest" {
		t.Errorf("Command = %q", det.Command)
	}
}
