package app

import (
	"strings"
	"testing"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClone, "CLONE"},
		{StateImportIssues, "IMPORT_ISSUES"},
		{StateReview, "REVIEW"},
		{StateIngest, "INGEST"},
		{StateEvaluateThreshold, "EVALUATE_THRESHOLD"},
		{StateImplement, "IMPLEMENT"},
		{StateTest, "TEST"},
		{StatePR, "PR"},
		{StateDocument, "DOCUMENT"},
		{StateDone, "DONE"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestState_Next(t *testing.T) {
	// Verify the state machine transitions in order
	expected := []State{
		StateClone, StateImportIssues, StateReview, StateIngest, StateEvaluateThreshold,
		StateImplement, StateTest, StatePR, StateDocument, StateDone,
	}

	for i := 0; i < len(expected)-1; i++ {
		got := expected[i].Next()
		want := expected[i+1]
		if got != want {
			t.Errorf("%s.Next() = %s, want %s", expected[i], got, want)
		}
	}

	// Done stays at Done
	if StateDone.Next() != StateDone {
		t.Errorf("StateDone.Next() = %s, want DONE", StateDone.Next())
	}
}

func TestState_UnknownString(t *testing.T) {
	s := State(99)
	got := s.String()
	if got != "UNKNOWN(99)" {
		t.Errorf("State(99).String() = %q, want UNKNOWN(99)", got)
	}
}

func TestCycleStats_Summary_WithItems(t *testing.T) {
	stats := &CycleStats{
		PRsCreated: 2,
		Items: []ItemResult{
			{Title: "Add auth tests", Category: "test", Status: "done", PRLink: "https://github.com/org/repo/pull/42"},
			{Title: "Expose user API", Category: "refactor", Status: "done", PRLink: "https://github.com/org/repo/pull/43"},
			{Title: "Fix race condition", Category: "bug", Status: "failed"},
			{Title: "Fix typo", Category: "style", Status: "skipped"},
		},
	}

	got := stats.Summary()

	for _, want := range []string{
		"2 implemented",
		"1 failed",
		"1 skipped",
		"2 PRs created",
		"✓ Add auth tests [test] → https://github.com/org/repo/pull/42",
		"✓ Expose user API [refactor] → https://github.com/org/repo/pull/43",
		"✗ Fix race condition [bug] — failed",
		"- Fix typo [style] — skipped",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Summary() missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestCycleStats_Summary_NoItems(t *testing.T) {
	tests := []struct {
		name  string
		stats CycleStats
		want  string
	}{
		{"no items found", CycleStats{}, "no items found"},
		{"none implemented", CycleStats{ItemsFound: 5}, "none implemented"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stats.Summary()
			if !strings.Contains(got, tt.want) {
				t.Errorf("Summary() = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestCycleStats_Summary_AllFailed(t *testing.T) {
	stats := &CycleStats{
		Items: []ItemResult{
			{Title: "Task A", Category: "bug", Status: "failed"},
			{Title: "Task B", Category: "bug", Status: "failed"},
		},
	}

	got := stats.Summary()
	if !strings.Contains(got, "2 failed") {
		t.Errorf("Summary() missing '2 failed'\ngot:\n%s", got)
	}
	if strings.Contains(got, "implemented") {
		t.Errorf("Summary() should not contain 'implemented' when all failed\ngot:\n%s", got)
	}
}

func TestCycleStats_Summary_BudgetIncluded(t *testing.T) {
	stats := &CycleStats{
		Items: []ItemResult{
			{Title: "Task A", Status: "done"},
		},
		PRsCreated:    1,
		BudgetSummary: "$4.20 / $100.00 spent (3 invocations)",
	}

	got := stats.Summary()
	if !strings.Contains(got, "Budget: $4.20 / $100.00 spent (3 invocations)") {
		t.Errorf("Summary() missing budget line\ngot:\n%s", got)
	}
}
