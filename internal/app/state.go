package app

import "fmt"

// State represents a step in the orchestrator state machine.
type State int

const (
	StateClone State = iota
	StateReview
	StateIngest
	StateEvaluateThreshold
	StateImplement
	StateTest
	StatePR
	StateDocument
	StateDone
)

func (s State) String() string {
	switch s {
	case StateClone:
		return "CLONE"
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

// CycleStats tracks statistics for a single cycle.
type CycleStats struct {
	ItemsFound       int
	ItemsInserted    int
	ItemsImplemented int
	PRsCreated       int
	PRsAutoMerged    int
	TestFailures     int
	Errors           []error
}
