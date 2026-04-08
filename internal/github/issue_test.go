package github

import "testing"

func TestParseIssueNumber_ValidURL(t *testing.T) {
	num, err := parseIssueNumber("https://github.com/owner/repo/issues/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 42 {
		t.Errorf("got %d, want 42", num)
	}
}

func TestParseIssueNumber_TrailingSlash(t *testing.T) {
	num, err := parseIssueNumber("https://github.com/owner/repo/issues/7/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 7 {
		t.Errorf("got %d, want 7", num)
	}
}

func TestParseIssueNumber_InvalidURL(t *testing.T) {
	_, err := parseIssueNumber("https://github.com/owner/repo/issues/notanumber")
	if err == nil {
		t.Error("expected error for non-numeric issue number")
	}
}

func TestParseIssueNumber_EmptyURL(t *testing.T) {
	_, err := parseIssueNumber("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestParseIssueNumber_JustNumber(t *testing.T) {
	num, err := parseIssueNumber("123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 123 {
		t.Errorf("got %d, want 123", num)
	}
}

func TestParsePagedJSON_SinglePage(t *testing.T) {
	input := `[{"number":1,"title":"bug","body":"fix it","state":"open","labels":[]},{"number":2,"title":"feat","body":"add it","state":"open","labels":[]}]`
	issues, err := parsePagedJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("len = %d, want 2", len(issues))
	}
	if issues[0].Number != 1 {
		t.Errorf("issues[0].Number = %d, want 1", issues[0].Number)
	}
	if issues[1].Title != "feat" {
		t.Errorf("issues[1].Title = %q, want 'feat'", issues[1].Title)
	}
}

func TestParsePagedJSON_MultiPage(t *testing.T) {
	// gh api --paginate concatenates arrays
	input := `[{"number":1,"title":"a","body":"","state":"open","labels":[]}][{"number":2,"title":"b","body":"","state":"open","labels":[]},{"number":3,"title":"c","body":"","state":"open","labels":[]}]`
	issues, err := parsePagedJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("len = %d, want 3", len(issues))
	}
	if issues[2].Number != 3 {
		t.Errorf("issues[2].Number = %d, want 3", issues[2].Number)
	}
}

func TestParsePagedJSON_Empty(t *testing.T) {
	issues, err := parsePagedJSON("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("len = %d, want 0", len(issues))
	}
}

func TestParsePagedJSON_FiltersPullRequests(t *testing.T) {
	// The REST issues endpoint returns PRs too; verify parsePagedJSON decodes them
	// so the caller can filter by PullRequest field.
	input := `[{"number":1,"title":"issue","body":"","state":"open","labels":[]},{"number":2,"title":"pr","body":"","state":"open","labels":[],"pull_request":{}}]`
	issues, err := parsePagedJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("len = %d, want 2", len(issues))
	}
	if issues[0].PullRequest != nil {
		t.Error("issues[0] should not be a PR")
	}
	if issues[1].PullRequest == nil {
		t.Error("issues[1] should be a PR")
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"rate limit", "API rate limit exceeded", true},
		{"403 error", "HTTP 403: forbidden", true},
		{"secondary rate", "You have exceeded a secondary rate limit", true},
		{"normal error", "HTTP 404: Not Found", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRateLimited(tt.stderr)
			if got != tt.want {
				t.Errorf("isRateLimited(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}
