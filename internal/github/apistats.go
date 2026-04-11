package github

import (
	"fmt"
	"sync"
)

// APIStats tracks GitHub API usage statistics in a thread-safe manner.
type APIStats struct {
	mu         sync.Mutex
	calls      int
	retries    int
	rateLimits int
	failures   int
}

// Stats is the package-level singleton for tracking GitHub API usage.
var Stats = &APIStats{}

// RecordCall increments the call counter.
func (s *APIStats) RecordCall() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
}

// RecordRetry increments both retry and rate-limit counters.
func (s *APIStats) RecordRetry() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retries++
	s.rateLimits++
}

// RecordFailure increments the failure counter.
func (s *APIStats) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
}

// Calls returns the total number of API calls made.
func (s *APIStats) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Retries returns the total number of retries.
func (s *APIStats) Retries() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retries
}

// RateLimits returns the total number of rate-limit hits.
func (s *APIStats) RateLimits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rateLimits
}

// Failures returns the total number of final failures.
func (s *APIStats) Failures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures
}

// Snapshot returns the current counters as a plain struct (no mutex).
type APIStatsSnapshot struct {
	Calls      int
	Retries    int
	RateLimits int
	Failures   int
}

// Snapshot returns a point-in-time copy of the counters.
func (s *APIStats) Snapshot() APIStatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return APIStatsSnapshot{
		Calls:      s.calls,
		Retries:    s.retries,
		RateLimits: s.rateLimits,
		Failures:   s.failures,
	}
}

// Reset zeroes all counters.
func (s *APIStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = 0
	s.retries = 0
	s.rateLimits = 0
	s.failures = 0
}

// String returns a human-readable summary of API usage.
func (s *APIStats) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	callNoun := "calls"
	if s.calls == 1 {
		callNoun = "call"
	}
	retryNoun := "retries"
	if s.retries == 1 {
		retryNoun = "retry"
	}
	failNoun := "failures"
	if s.failures == 1 {
		failNoun = "failure"
	}

	return fmt.Sprintf("%d %s, %d %s, %d %s",
		s.calls, callNoun, s.retries, retryNoun, s.failures, failNoun)
}
