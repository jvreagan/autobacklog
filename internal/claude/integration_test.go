//go:build !windows

package claude

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jamesreagan/autobacklog/internal/config"
	"github.com/jamesreagan/autobacklog/internal/testutil"
)

func newStubClient(t *testing.T, binary string, timeout time.Duration) *Client {
	t.Helper()
	cfg := config.ClaudeConfig{
		Binary:           binary,
		Model:            "sonnet",
		MaxBudgetPerCall: 5.0,
		MaxBudgetTotal:   50.0,
		Timeout:          timeout,
	}
	c := NewClient(cfg, slog.Default())
	// Silence output during tests
	c.SetOutputWriters(&bytes.Buffer{}, &bytes.Buffer{})
	return c
}

func TestClient_Run_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	// Stub claude binary that returns valid JSON output
	testutil.WriteStubScript(t, binDir, "claude", `
echo '{"result":"[{\"title\":\"Fix bug\",\"description\":\"desc\",\"file_path\":\"a.go\",\"priority\":\"high\",\"category\":\"bug\"}]","cost_usd":{"input":0.01,"output":0.02,"total":0.03}}'
`)

	c := newStubClient(t, "claude", 30*time.Second)
	ctx := context.Background()

	output, err := c.Run(ctx, t.TempDir(), "review the codebase")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(output, "Fix bug") {
		t.Errorf("output should contain 'Fix bug', got: %s", output)
	}

	// Budget should have recorded the cost
	if c.budget.Spent() < 0.03 {
		t.Errorf("budget spent = %.4f, want >= 0.03", c.budget.Spent())
	}
}

func TestClient_RunPrint_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "claude", `echo "Implementation complete"`)

	c := newStubClient(t, "claude", 30*time.Second)
	ctx := context.Background()

	output, err := c.RunPrint(ctx, t.TempDir(), "implement changes")
	if err != nil {
		t.Fatalf("RunPrint: %v", err)
	}

	if !strings.Contains(output, "Implementation complete") {
		t.Errorf("output = %q, want to contain 'Implementation complete'", output)
	}
}

func TestClient_Run_BinaryFailure(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "claude", `echo "crash" >&2; exit 1`)

	c := newStubClient(t, "claude", 30*time.Second)
	ctx := context.Background()

	_, err := c.Run(ctx, t.TempDir(), "do something")
	if err == nil {
		t.Fatal("expected error from failing binary")
	}
	if !strings.Contains(err.Error(), "claude failed") {
		t.Errorf("error = %v, want to contain 'claude failed'", err)
	}
}

func TestClient_Run_Timeout(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "claude", `sleep 10`)

	c := newStubClient(t, "claude", 500*time.Millisecond)
	ctx := context.Background()

	_, err := c.Run(ctx, t.TempDir(), "slow prompt")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want to contain 'timed out'", err)
	}
}

func TestClient_RunPrint_StreamsOutput(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "claude", `
echo "line 1"
echo "line 2"
echo "line 3"
`)

	var stdoutBuf bytes.Buffer
	c := newStubClient(t, "claude", 30*time.Second)
	c.SetOutputWriters(&stdoutBuf, &bytes.Buffer{})
	ctx := context.Background()

	_, err := c.RunPrint(ctx, t.TempDir(), "stream test")
	if err != nil {
		t.Fatalf("RunPrint: %v", err)
	}

	// In print mode, stdout should be streamed to the sink
	if !strings.Contains(stdoutBuf.String(), "line 1") {
		t.Errorf("stdout sink should contain 'line 1', got: %q", stdoutBuf.String())
	}
}
