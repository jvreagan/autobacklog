package notify

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// CycleCompleteNotification creates a notification for a completed cycle.
func CycleCompleteNotification(found, implemented, prsCreated int, budgetStatus string) Notification {
	return Notification{
		Event:   EventCycleComplete,
		Subject: fmt.Sprintf("Cycle complete: %d found, %d implemented, %d PRs", found, implemented, prsCreated),
		Body: fmt.Sprintf(`Autobacklog cycle completed.

Items found:       %d
Items implemented: %d
PRs created:       %d
Budget status:     %s
`, found, implemented, prsCreated, budgetStatus),
	}
}

// StuckNotification creates a notification when an item is stuck after max retries.
func StuckNotification(title, filePath string, attempts int, lastError string) Notification {
	return Notification{
		Event:   EventStuck,
		Subject: fmt.Sprintf("Stuck: %s", title),
		Body: fmt.Sprintf(`An item is stuck after %d attempts.

Title: %s
File:  %s

Last error:
%s

This item has been marked as failed and requires manual attention.
`, attempts, title, filePath, lastError),
	}
}

// OutOfTokensNotification creates a notification when the budget is exceeded.
func OutOfTokensNotification(spent, limit float64) Notification {
	return Notification{
		Event:   EventOutOfTokens,
		Subject: "Budget exceeded — daemon paused",
		Body: fmt.Sprintf(`The autobacklog daemon has been paused because the budget was exceeded.

Spent: $%.2f
Limit: $%.2f

Increase the budget in your config file and restart the daemon.
`, spent, limit),
	}
}

// PRCreatedNotification creates a notification when a new PR is created.
func PRCreatedNotification(title, prURL, description string) Notification {
	return Notification{
		Event:   EventPRCreated,
		Subject: fmt.Sprintf("PR created: %s", title),
		Body: fmt.Sprintf(`A new pull request has been created.

Title: %s
URL:   %s

%s
`, title, prURL, description),
	}
}

// ErrorNotification creates a notification for unexpected errors.
func ErrorNotification(context string, err error) Notification {
	return Notification{
		Event:   EventError,
		Subject: fmt.Sprintf("Error: %s", truncate(context, 50)),
		Body: fmt.Sprintf(`An unexpected error occurred in autobacklog.

Context: %s
Error:   %s
`, context, err),
	}
}

// truncate shortens s to at most maxLen runes, appending "..." if truncated.
// Uses rune-aware slicing to avoid splitting multi-byte UTF-8 characters (#214).
func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-3]) + "..."
}
