//go:build !windows

package git

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvreagan/autobacklog/internal/testutil"
)

// TestRunGitRetry_TransientRetry verifies that runGitRetry retries on transient
// errors and succeeds once the command passes.
func TestRunGitRetry_TransientRetry(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	workDir := t.TempDir()
	counterFile := filepath.Join(workDir, ".git_counter")

	// Stub git: fails twice with "connection reset" (exit 128), succeeds on third.
	testutil.WriteStubScript(t, binDir, "git", `
COUNTER_FILE="`+counterFile+`"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 0 > "$COUNTER_FILE"
fi
COUNT=$(cat "$COUNTER_FILE")
COUNT=$((COUNT + 1))
echo $COUNT > "$COUNTER_FILE"
if [ "$COUNT" -le 2 ]; then
  echo "fatal: read: connection reset by peer" >&2
  exit 128
fi
echo "ok"
`)

	r := NewRepo("https://example.com/repo.git", "main", workDir, "", 3, slog.Default())
	err := r.runGitRetry(context.Background(), workDir, "clone", "https://example.com/repo.git")
	if err != nil {
		t.Fatalf("runGitRetry should have succeeded after transient retries: %v", err)
	}
}

// TestRunGitRetry_NonTransientNoRetry verifies that runGitRetry returns
// immediately (no retries) for non-transient errors.
func TestRunGitRetry_NonTransientNoRetry(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	workDir := t.TempDir()
	counterFile := filepath.Join(workDir, ".git_counter")

	// Stub git: always fails with "not a git repository" (non-transient).
	testutil.WriteStubScript(t, binDir, "git", `
COUNTER_FILE="`+counterFile+`"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 0 > "$COUNTER_FILE"
fi
COUNT=$(cat "$COUNTER_FILE")
COUNT=$((COUNT + 1))
echo $COUNT > "$COUNTER_FILE"
echo "fatal: not a git repository" >&2
exit 128
`)

	r := NewRepo("https://example.com/repo.git", "main", workDir, "", 3, slog.Default())
	err := r.runGitRetry(context.Background(), workDir, "status")
	if err == nil {
		t.Fatal("runGitRetry should have returned error for non-transient failure")
	}

	// Verify only 1 attempt was made (no retries)
	data, readErr := os.ReadFile(counterFile)
	if readErr != nil {
		t.Fatalf("reading counter file: %v", readErr)
	}
	count := strings.TrimSpace(string(data))
	if count != "1" {
		t.Errorf("expected 1 attempt (no retries), got %s attempts", count)
	}
}
