//go:build !windows

package github

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jamesreagan/autobacklog/internal/testutil"
)

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"rate limit", "API rate limit exceeded", true},
		{"403 error", "HTTP 403: forbidden", true},
		{"secondary rate", "You have exceeded a secondary rate limit", true},
		{"normal error", "HTTP 404: Not Found", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRateLimited(tt.stderr)
			if got != tt.want {
				t.Errorf("isRateLimited(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

// resetStats resets the package-level Stats singleton for test isolation.
func resetStats(t *testing.T) {
	t.Helper()
	Stats.Reset()
	t.Cleanup(func() { Stats.Reset() })
}

func TestRunGH_Success(t *testing.T) {
	resetStats(t)
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "hello from gh"`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	out, err := runGH(ctx, workDir, log, "version")
	if err != nil {
		t.Fatalf("runGH: %v", err)
	}
	if out != "hello from gh" {
		t.Errorf("output = %q, want %q", out, "hello from gh")
	}
	if Stats.Calls() != 1 {
		t.Errorf("Stats.Calls() = %d, want 1", Stats.Calls())
	}
	if Stats.Failures() != 0 {
		t.Errorf("Stats.Failures() = %d, want 0", Stats.Failures())
	}
}

func TestRunGH_RateLimitRetry(t *testing.T) {
	resetStats(t)
	binDir := testutil.StubBinDir(t)
	// Fail with 403 twice, then succeed on the third call.
	// Use a counter file in the working directory to track attempts.
	testutil.WriteStubScript(t, binDir, "gh", `
COUNTER_FILE="$(pwd)/.gh_counter"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 0 > "$COUNTER_FILE"
fi
COUNT=$(cat "$COUNTER_FILE")
COUNT=$((COUNT + 1))
echo $COUNT > "$COUNTER_FILE"
if [ "$COUNT" -le 2 ]; then
  echo "HTTP 403: rate limit exceeded" >&2
  exit 1
fi
rm -f "$COUNTER_FILE"
echo "success"
`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	out, err := runGH(ctx, workDir, log, "api", "/test")
	if err != nil {
		t.Fatalf("runGH should have succeeded after retries: %v", err)
	}
	if out != "success" {
		t.Errorf("output = %q, want %q", out, "success")
	}
	if Stats.Calls() != 1 {
		t.Errorf("Stats.Calls() = %d, want 1", Stats.Calls())
	}
	if Stats.Retries() != 2 {
		t.Errorf("Stats.Retries() = %d, want 2", Stats.Retries())
	}
	if Stats.Failures() != 0 {
		t.Errorf("Stats.Failures() = %d, want 0 (succeeded after retries)", Stats.Failures())
	}
}

func TestRunGH_NonRateLimitError(t *testing.T) {
	resetStats(t)
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "HTTP 404: Not Found" >&2; exit 1`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	_, err := runGH(ctx, workDir, log, "api", "/missing")
	if err == nil {
		t.Fatal("expected error for non-rate-limit failure")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want to contain '404'", err)
	}
	if Stats.Calls() != 1 {
		t.Errorf("Stats.Calls() = %d, want 1", Stats.Calls())
	}
	if Stats.Failures() != 1 {
		t.Errorf("Stats.Failures() = %d, want 1", Stats.Failures())
	}
}

func TestRunGH_RetriesExhausted(t *testing.T) {
	resetStats(t)
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "HTTP 403: rate limit exceeded" >&2; exit 1`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	_, err := runGH(ctx, workDir, log, "api", "/limited")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %v, want to contain 'rate limit'", err)
	}
	if Stats.Calls() != 1 {
		t.Errorf("Stats.Calls() = %d, want 1", Stats.Calls())
	}
	// 3 retries (backoffs[0], backoffs[1], backoffs[2]) then failure on 4th attempt
	if Stats.Retries() != 3 {
		t.Errorf("Stats.Retries() = %d, want 3", Stats.Retries())
	}
	if Stats.Failures() != 1 {
		t.Errorf("Stats.Failures() = %d, want 1", Stats.Failures())
	}
}

func TestRunGH_ContextCanceled(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "HTTP 403: rate limit exceeded" >&2; exit 1`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	workDir := t.TempDir()
	log := slog.Default()

	_, err := runGH(ctx, workDir, log, "api", "/test")
	if err == nil {
		t.Fatal("expected error when context is canceled")
	}
}
