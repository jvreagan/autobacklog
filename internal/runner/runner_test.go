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

// #195: test with a cancelled parent context.
func TestRunner_RunCancelledContext(t *testing.T) {
	r := NewRunner(slog.Default(), 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := r.Run(ctx, t.TempDir(), "sleep", []string{"10"})
	if err != nil {
		// Infrastructure error is acceptable for cancelled context
		return
	}
	if result != nil && result.Passed {
		t.Error("Passed = true, should not pass with cancelled context")
	}
}

// #195: test with a missing binary returns infrastructure error.
func TestRunner_RunMissingBinary(t *testing.T) {
	r := NewRunner(slog.Default(), 30*time.Second)
	ctx := context.Background()

	_, err := r.Run(ctx, t.TempDir(), "nonexistent-test-binary-xyz", nil)
	if err == nil {
		t.Error("expected infrastructure error for missing binary")
	}
}

// ValidateCommand tests (#122)
func TestValidateCommand(t *testing.T) {
	tests := []struct {
		cmd     string
		wantErr bool
	}{
		{"go", false},
		{"npm", false},
		{"pytest", false},
		{"/usr/bin/go", false},
		{"evil-command", true},
		{"rm", true},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			err := ValidateCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand(%q) error = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
		})
	}
}
