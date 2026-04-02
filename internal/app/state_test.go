package app

import "testing"

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClone, "CLONE"},
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
		StateClone, StateReview, StateIngest, StateEvaluateThreshold,
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
