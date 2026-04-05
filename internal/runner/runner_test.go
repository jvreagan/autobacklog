package runner

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestRunner_RunSuccess(t *testing.T) {
	r := NewRunner(slog.Default(), 30*time.Second)
	ctx := context.Background()

	result, err := r.Run(ctx, t.TempDir(), "echo", []string{"tests passed"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !result.Passed {
		t.Error("Passed = false, want true")
	}
	if result.Output == "" {
		t.Error("Output should not be empty")
	}
	if result.Elapsed <= 0 {
		t.Error("Elapsed should be positive")
	}
}

func TestRunner_RunFailure(t *testing.T) {
	r := NewRunner(slog.Default(), 30*time.Second)
	ctx := context.Background()

	result, err := r.Run(ctx, t.TempDir(), "sh", []string{"-c", "echo 'FAIL' && exit 1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Passed {
		t.Error("Passed = true, want false")
	}
	if result.Output == "" {
		t.Error("Output should capture failure message")
	}
}

func TestRunner_RunTimeout(t *testing.T) {
	r := NewRunner(slog.Default(), 100*time.Millisecond)
	ctx := context.Background()

	result, err := r.Run(ctx, t.TempDir(), "sleep", []string{"10"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Passed {
		t.Error("Passed = true, should fail on timeout")
	}
}
