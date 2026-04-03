package cli

import "testing"

func TestIsQuietHours_NotInPeriod(t *testing.T) {
	// Use a time range we know we're not in (testing at any time of day):
	// We test the pure logic by checking specific cases
	tests := []struct {
		name  string
		start string
		end   string
		want  bool
	}{
		// These test the logic — not tied to "now"
		{"empty start", "", "06:00", false},
		{"empty end", "22:00", "", false},
		{"both empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isQuietHours(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("isQuietHours(%q, %q) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestIsQuietHours_SpansMidnight(t *testing.T) {
	// 22:00-06:00 spans midnight — verify it doesn't panic
	got := isQuietHours("22:00", "06:00")
	// Result depends on current time, just verify no panic
	_ = got
}

func TestIsQuietHours_EmptyStrings(t *testing.T) {
	if isQuietHours("", "") {
		t.Error("empty strings should return false")
	}
}

func TestIsQuietHours_InvalidFormat(t *testing.T) {
	if isQuietHours("not-a-time", "06:00") {
		t.Error("invalid format should return false")
	}
	if isQuietHours("22:00", "bad") {
		t.Error("invalid format should return false")
	}
}

func TestIsQuietHours_SameStartEnd(t *testing.T) {
	// Same start and end should be a zero-length window = not quiet
	if isQuietHours("12:00", "12:00") {
		t.Error("same start/end should not be quiet hours")
	}
}
