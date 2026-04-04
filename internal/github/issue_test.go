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
