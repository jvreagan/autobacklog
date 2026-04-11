package github

import (
	"sync"
	"testing"
)

func TestAPIStats_InitialState(t *testing.T) {
	s := &APIStats{}
	if s.Calls() != 0 || s.Retries() != 0 || s.RateLimits() != 0 || s.Failures() != 0 {
		t.Errorf("new APIStats should have all zero counters")
	}
}

func TestAPIStats_RecordCall(t *testing.T) {
	s := &APIStats{}
	s.RecordCall()
	s.RecordCall()
	if s.Calls() != 2 {
		t.Errorf("Calls() = %d, want 2", s.Calls())
	}
}

func TestAPIStats_RecordRetry(t *testing.T) {
	s := &APIStats{}
	s.RecordRetry()
	if s.Retries() != 1 {
		t.Errorf("Retries() = %d, want 1", s.Retries())
	}
	if s.RateLimits() != 1 {
		t.Errorf("RateLimits() = %d, want 1", s.RateLimits())
	}
}

func TestAPIStats_RecordFailure(t *testing.T) {
	s := &APIStats{}
	s.RecordFailure()
	s.RecordFailure()
	s.RecordFailure()
	if s.Failures() != 3 {
		t.Errorf("Failures() = %d, want 3", s.Failures())
	}
}

func TestAPIStats_Snapshot(t *testing.T) {
	s := &APIStats{}
	s.RecordCall()
	s.RecordCall()
	s.RecordRetry()
	s.RecordFailure()

	snap := s.Snapshot()
	if snap.Calls != 2 {
		t.Errorf("Snapshot.Calls = %d, want 2", snap.Calls)
	}
	if snap.Retries != 1 {
		t.Errorf("Snapshot.Retries = %d, want 1", snap.Retries)
	}
	if snap.RateLimits != 1 {
		t.Errorf("Snapshot.RateLimits = %d, want 1", snap.RateLimits)
	}
	if snap.Failures != 1 {
		t.Errorf("Snapshot.Failures = %d, want 1", snap.Failures)
	}
}

func TestAPIStats_Reset(t *testing.T) {
	s := &APIStats{}
	s.RecordCall()
	s.RecordRetry()
	s.RecordFailure()
	s.Reset()

	if s.Calls() != 0 || s.Retries() != 0 || s.RateLimits() != 0 || s.Failures() != 0 {
		t.Errorf("after Reset() all counters should be zero")
	}
}

func TestAPIStats_String_Plural(t *testing.T) {
	s := &APIStats{}
	s.RecordCall()
	s.RecordCall()
	s.RecordRetry()
	s.RecordRetry()
	s.RecordFailure()
	s.RecordFailure()

	got := s.String()
	want := "2 calls, 2 retries, 2 failures"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestAPIStats_String_Singular(t *testing.T) {
	s := &APIStats{}
	s.RecordCall()
	s.RecordRetry()
	s.RecordFailure()

	got := s.String()
	want := "1 call, 1 retry, 1 failure"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestAPIStats_String_Zero(t *testing.T) {
	s := &APIStats{}
	got := s.String()
	want := "0 calls, 0 retries, 0 failures"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestAPIStats_ConcurrentAccess(t *testing.T) {
	s := &APIStats{}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			s.RecordCall()
		}()
		go func() {
			defer wg.Done()
			s.RecordRetry()
		}()
		go func() {
			defer wg.Done()
			s.RecordFailure()
		}()
	}

	wg.Wait()

	if s.Calls() != 100 {
		t.Errorf("Calls() = %d, want 100", s.Calls())
	}
	if s.Retries() != 100 {
		t.Errorf("Retries() = %d, want 100", s.Retries())
	}
	if s.Failures() != 100 {
		t.Errorf("Failures() = %d, want 100", s.Failures())
	}
}
