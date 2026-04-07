package cli

import (
	"testing"
	"time"
)

// #156: isQuietHoursAt is now defined in daemon.go, so tests use the real function.

func TestIsQuietHours_Deterministic(t *testing.T) {
	tests := []struct {
		name  string
		start string
		end   string
		now   time.Time
		want  bool
	}{
		// Empty inputs
		{"empty start", "", "06:00", time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC), false},
		{"empty end", "22:00", "", time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC), false},
		{"both empty", "", "", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), false},

		// Same-day range (09:00–17:00)
		{"same-day inside", "09:00", "17:00", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), true},
		{"same-day at start", "09:00", "17:00", time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC), true},
		{"same-day before start", "09:00", "17:00", time.Date(2025, 1, 1, 8, 59, 0, 0, time.UTC), false},
		{"same-day at end", "09:00", "17:00", time.Date(2025, 1, 1, 17, 0, 0, 0, time.UTC), false},
		{"same-day after end", "09:00", "17:00", time.Date(2025, 1, 1, 18, 0, 0, 0, time.UTC), false},

		// Midnight-spanning range (22:00–06:00)
		{"midnight-span before midnight", "22:00", "06:00", time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC), true},
		{"midnight-span at start", "22:00", "06:00", time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC), true},
		{"midnight-span after midnight", "22:00", "06:00", time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC), true},
		{"midnight-span at end", "22:00", "06:00", time.Date(2025, 1, 2, 6, 0, 0, 0, time.UTC), false},
		{"midnight-span outside daytime", "22:00", "06:00", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), false},
		{"midnight-span just before start", "22:00", "06:00", time.Date(2025, 1, 1, 21, 59, 0, 0, time.UTC), false},

		// Edge: same start and end
		{"same start and end", "12:00", "12:00", time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), false},

		// Invalid format
		{"invalid start", "bad", "06:00", time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC), false},
		{"invalid end", "22:00", "bad", time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isQuietHoursAt(tt.start, tt.end, tt.now)
			if got != tt.want {
				t.Errorf("isQuietHoursAt(%q, %q, %v) = %v, want %v",
					tt.start, tt.end, tt.now.Format("15:04"), got, tt.want)
			}
		})
	}
}

// Existing non-deterministic tests kept for coverage of the real isQuietHours function.

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
	if isQuietHours("12:00", "12:00") {
		t.Error("same start/end should not be quiet hours")
	}
}
