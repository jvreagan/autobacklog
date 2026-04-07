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
			want:  "",
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

// --- Edge-case tests for issue #61 ---

func TestExtractJSON_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace only",
			input: "   \n\t  \n  ",
			want:  "",
		},
		{
			name:  "plain text no JSON",
			input: "This is just plain text with no JSON at all.",
			want:  "",
		},
		{
			name:  "plain text with brackets in prose",
			input: "See section [A] for details on the {config} option.",
			want:  "[A]",
		},
		{
			name:  "nested arrays inside JSON array",
			input: `[{"tags":["a","b"],"matrix":[[1,2],[3,4]]}]`,
			want:  `[{"tags":["a","b"],"matrix":[[1,2],[3,4]]}]`,
		},
		{
			name:  "nested objects inside JSON array",
			input: `[{"meta":{"nested":{"deep":"value"}},"title":"bug"}]`,
			want:  `[{"meta":{"nested":{"deep":"value"}},"title":"bug"}]`,
		},
		{
			name:  "nested with surrounding text",
			input: "Findings:\n[{\"items\":[{\"sub\":1},{\"sub\":2}]}]\nDone.",
			want:  `[{"items":[{"sub":1},{"sub":2}]}]`,
		},
		{
			name:  "multiple JSON arrays picks first balanced one",
			input: "First: [{\"a\":1}] and second: [{\"b\":2},{\"c\":3}] end.",
			want:  `[{"a":1}]`,
		},
		{
			name:  "multiple arrays first is empty",
			input: "Start: [] then [{\"real\":true}] end.",
			want:  "[]",
		},
		{
			name:  "brackets inside quoted strings are ignored",
			input: `[{"title":"fix [bug] in {config}","desc":"handle ]]]"}]`,
			want:  `[{"title":"fix [bug] in {config}","desc":"handle ]]]"}]`,
		},
		{
			name:  "escaped quotes inside strings",
			input: `[{"title":"a \"quoted\" value"}]`,
			want:  `[{"title":"a \"quoted\" value"}]`,
		},
		{
			name:  "unbalanced open bracket returns empty",
			input: `Some text [{"a":1 more text`,
			want:  "",
		},
		{
			name:  "object preferred when no array present",
			input: `Commentary: {"key":"val"} done`,
			want:  `{"key":"val"}`,
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

func TestParseReviewOutput_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantItems int
		wantErr   bool
		check     func(t *testing.T, items []*backlog.Item, cost float64)
	}{
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only output",
			input:   "   \n\t  ",
			wantErr: true,
		},
		{
			name:    "plain text no JSON",
			input:   "I found no issues in the codebase. Everything looks great!",
			wantErr: true,
		},
		{
			name:      "findings with nested arrays in values",
			input:     `[{"title":"Complex finding","description":"Found in [multiple] areas","file_path":"main.go","line_number":10,"priority":"high","category":"bug"}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Title != "Complex finding" {
					t.Errorf("Title = %q, want %q", items[0].Title, "Complex finding")
				}
				if items[0].Description != "Found in [multiple] areas" {
					t.Errorf("Description = %q", items[0].Description)
				}
				if items[0].Priority != backlog.PriorityHigh {
					t.Errorf("Priority = %q, want %q", items[0].Priority, backlog.PriorityHigh)
				}
			},
		},
		{
			name: "findings with nested objects",
			input: `[{"title":"Nested","description":"desc","file_path":"a.go","line_number":1,"priority":"low","category":"bug"},` +
				`{"title":"Also nested","description":"desc2","file_path":"b.go","line_number":2,"priority":"medium","category":"security"}]`,
			wantItems: 2,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Title != "Nested" {
					t.Errorf("items[0].Title = %q", items[0].Title)
				}
				if items[1].Title != "Also nested" {
					t.Errorf("items[1].Title = %q", items[1].Title)
				}
				if items[1].Category != backlog.CategorySecurity {
					t.Errorf("items[1].Category = %q", items[1].Category)
				}
			},
		},
		{
			name:    "malformed JSON inside valid ClaudeResponse envelope",
			input:   `{"result":"[{\"title\":\"bug\", invalid json here}]","cost_usd":{"input":0.01,"output":0.02,"total":0.03}}`,
			wantErr: true,
		},
		{
			name:    "ClaudeResponse with empty result field",
			input:   `{"result":"","cost_usd":{"input":0.01,"output":0.02,"total":0.03}}`,
			wantErr: true,
		},
		{
			name:      "unknown priority normalizes to low",
			input:     `[{"title":"Unknown pri","description":"desc","file_path":"x.go","line_number":1,"priority":"critical","category":"bug"}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Priority != backlog.PriorityLow {
					t.Errorf("Priority = %q, want %q (default for unknown)", items[0].Priority, backlog.PriorityLow)
				}
			},
		},
		{
			name:      "unknown category normalizes to refactor",
			input:     `[{"title":"Unknown cat","description":"desc","file_path":"x.go","line_number":1,"priority":"high","category":"feature"}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Category != backlog.CategoryRefactor {
					t.Errorf("Category = %q, want %q (default for unknown)", items[0].Category, backlog.CategoryRefactor)
				}
			},
		},
		{
			name:      "empty priority and category normalize to defaults",
			input:     `[{"title":"Empty fields","description":"desc","file_path":"x.go","line_number":1,"priority":"","category":""}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Priority != backlog.PriorityLow {
					t.Errorf("Priority = %q, want %q", items[0].Priority, backlog.PriorityLow)
				}
				if items[0].Category != backlog.CategoryRefactor {
					t.Errorf("Category = %q, want %q", items[0].Category, backlog.CategoryRefactor)
				}
			},
		},
		{
			name:      "missing priority and category fields normalize to defaults",
			input:     `[{"title":"No pri/cat","description":"desc","file_path":"x.go","line_number":1}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Priority != backlog.PriorityLow {
					t.Errorf("Priority = %q, want %q", items[0].Priority, backlog.PriorityLow)
				}
				if items[0].Category != backlog.CategoryRefactor {
					t.Errorf("Category = %q, want %q", items[0].Category, backlog.CategoryRefactor)
				}
			},
		},
		{
			name: "multiple JSON arrays in text picks first",
			input: `Here are the findings:
[{"title":"First","description":"d","file_path":"a.go","line_number":1,"priority":"high","category":"bug"}]
And also:
[{"title":"Second","description":"d","file_path":"b.go","line_number":2,"priority":"low","category":"docs"}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Title != "First" {
					t.Errorf("Title = %q, want %q (should pick first array)", items[0].Title, "First")
				}
			},
		},
		{
			name: "multiple arrays first is empty second has items",
			input: `No findings: []
Actually: [{"title":"Oops","description":"d","file_path":"a.go","line_number":1,"priority":"high","category":"bug"}]`,
			wantItems: 0,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				// First balanced array is [], so we get zero items
			},
		},
		{
			name: "ClaudeResponse with valid result containing multiple arrays picks first",
			input: `{"result":"[{\"title\":\"First\",\"description\":\"d\",\"file_path\":\"a.go\",\"line_number\":1,\"priority\":\"high\",\"category\":\"bug\"}]\nAlso: [{\"title\":\"Second\",\"description\":\"d\",\"file_path\":\"b.go\",\"line_number\":2,\"priority\":\"low\",\"category\":\"docs\"}]","cost_usd":{"input":0.01,"output":0.02,"total":0.03}}`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Title != "First" {
					t.Errorf("Title = %q, want %q", items[0].Title, "First")
				}
				if cost != 0.03 {
					t.Errorf("cost = %f, want 0.03", cost)
				}
			},
		},
		{
			name:      "priority with unusual casing and whitespace",
			input:     `[{"title":"Weird pri","description":"d","file_path":"x.go","line_number":1,"priority":"  HIGH  ","category":"  BUG  "}]`,
			wantItems: 1,
			check: func(t *testing.T, items []*backlog.Item, cost float64) {
				if items[0].Priority != backlog.PriorityHigh {
					t.Errorf("Priority = %q, want %q", items[0].Priority, backlog.PriorityHigh)
				}
				if items[0].Category != backlog.CategoryBug {
					t.Errorf("Category = %q, want %q", items[0].Category, backlog.CategoryBug)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, cost, err := ParseReviewOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (items=%d)", len(items))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tt.wantItems {
				t.Fatalf("len(items) = %d, want %d", len(items), tt.wantItems)
			}
			if tt.check != nil {
				tt.check(t, items, cost)
			}
		})
	}
}
