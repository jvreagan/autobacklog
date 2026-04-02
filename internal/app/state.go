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
	TestFailures     int
	Errors           []error
}
