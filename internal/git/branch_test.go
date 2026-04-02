package git

import "testing"

func TestFormatBranchName(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		category string
		title    string
		want     string
	}{
		{
			name:     "basic",
			prefix:   "autobacklog",
			category: "bug",
			title:    "Fix null pointer",
			want:     "autobacklog/bug/fix-null-pointer",
		},
		{
			name:     "special chars removed",
			prefix:   "autobacklog",
			category: "security",
			title:    "SQL injection in user_query()",
			want:     "autobacklog/security/sql-injection-in-user-query",
		},
		{
			name:     "uppercase normalized",
			prefix:   "autobacklog",
			category: "Refactor",
			title:    "Extract HTTP Handler",
			want:     "autobacklog/refactor/extract-http-handler",
		},
		{
			name:     "long title truncated",
			prefix:   "autobacklog",
			category: "perf",
			title:    "This is a very long title that should definitely be truncated because it exceeds the fifty character limit for branch names",
			want:     "autobacklog/perf/this-is-a-very-long-title-that-should-definitely-b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBranchName(tt.prefix, tt.category, tt.title)
			if got != tt.want {
				t.Errorf("formatBranchName(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.category, tt.title, got, tt.want)
			}
		})
	}
}

func TestFormatBranchName_NoTrailingHyphen(t *testing.T) {
	// A title that, when truncated at 50 chars, might leave a trailing hyphen
	got := formatBranchName("ab", "x", "abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcde-ghij")
	if got[len(got)-1] == '-' {
		t.Errorf("branch name should not end with hyphen: %q", got)
	}
}
