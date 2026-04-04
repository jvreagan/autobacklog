package app

import (
	"fmt"
	"strings"
)

// State represents a step in the orchestrator state machine.
type State int

const (
	StateClone State = iota
	StateImportIssues
	StateReview
	StateIngest
	StateEvaluateThreshold
	StateImplement
	StateTest  // pass-through: tests run inline during StateImplement
	StatePR    // pass-through: PRs created inline during StateImplement
	StateDocument
	StateDone
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClone:
		return "CLONE"
	case StateImportIssues:
		return "IMPORT_ISSUES"
	case StateReview:
		return "REVIEW"
	case StateIngest:
		return "INGEST"
	case StateEvaluateThreshold:
		return "EVALUATE_THRESHOLD"
	case StateImplement:
		return "IMPLEMENT"
	case StateTest:
		return "TEST"
	case StatePR:
		return "PR"
	case StateDocument:
		return "DOCUMENT"
	case StateDone:
		return "DONE"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}

// Description returns a human-readable description of what this state does.
func (s State) Description() string {
	switch s {
	case StateClone:
		return "cloning or pulling the target repository"
	case StateImportIssues:
		return "importing labeled GitHub issues"
	case StateReview:
		return "reviewing codebase with Claude for improvement opportunities"
	case StateIngest:
		return "ingesting review items into the backlog database"
	case StateEvaluateThreshold:
		return "evaluating backlog thresholds to decide what to implement"
	case StateImplement:
		return "implementing selected backlog items"
	case StateTest:
		return "running tests against implemented changes"
	case StatePR:
		return "creating pull requests for implemented changes"
	case StateDocument:
		return "updating documentation for implemented changes"
	case StateDone:
		return "cycle complete"
	default:
		return "unknown"
	}
}

// Next returns the next state in the sequence.
func (s State) Next() State {
	if s >= StateDone {
		return StateDone
	}
	return s + 1
}

// ItemResult records the outcome of a single backlog item.
type ItemResult struct {
	Title    string
	Category string
	Status   string // "done", "failed", "skipped"
	PRLink   string
}

// CycleStats tracks statistics for a single cycle.
type CycleStats struct {
	ItemsFound       int
	ItemsInserted    int
	ItemsImplemented int
	IssuesImported   int
	IssuesCreated    int
	PRsCreated       int
	PRsAutoMerged    int
	TestFailures     int
	Errors           []error
	Items            []ItemResult
	BudgetSummary    string
}

// Summary returns a human-readable summary of the cycle.
func (s *CycleStats) Summary() string {
	if len(s.Items) == 0 {
		if s.ItemsFound == 0 {
			return "Cycle complete: no items found."
		}
		return fmt.Sprintf("Cycle complete: %d items found, none implemented.", s.ItemsFound)
	}

	var done, failed, skipped int
	for _, item := range s.Items {
		switch item.Status {
		case "done":
			done++
		case "failed":
			failed++
		case "skipped":
			skipped++
		}
	}

	// Header line
	var parts []string
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d implemented", done))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if s.PRsCreated > 0 {
		parts = append(parts, fmt.Sprintf("%d PR created", s.PRsCreated))
	}
	if s.IssuesImported > 0 {
		parts = append(parts, fmt.Sprintf("%d issues imported", s.IssuesImported))
	}
	if s.IssuesCreated > 0 {
		parts = append(parts, fmt.Sprintf("%d issues created", s.IssuesCreated))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Cycle complete: %s\n", strings.Join(parts, ", "))

	// Per-item lines
	for _, item := range s.Items {
		cat := ""
		if item.Category != "" {
			cat = fmt.Sprintf(" [%s]", item.Category)
		}
		switch item.Status {
		case "done":
			suffix := ""
			if item.PRLink != "" {
				suffix = fmt.Sprintf(" → %s", item.PRLink)
			}
			fmt.Fprintf(&b, "\n  ✓ %s%s%s", item.Title, cat, suffix)
		case "failed":
			fmt.Fprintf(&b, "\n  ✗ %s%s — failed", item.Title, cat)
		case "skipped":
			fmt.Fprintf(&b, "\n  - %s%s — skipped", item.Title, cat)
		}
	}

	if s.BudgetSummary != "" {
		fmt.Fprintf(&b, "\n\nBudget: %s", s.BudgetSummary)
	}

	return b.String()
}
