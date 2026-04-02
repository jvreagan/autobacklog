package runner

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// Result holds the outcome of a test run.
type Result struct {
	Passed  bool
	Output  string
	Elapsed time.Duration
}

// Runner executes test suites.
type Runner struct {
	log     *slog.Logger
	timeout time.Duration
}

// NewRunner creates a new test runner.
func NewRunner(log *slog.Logger, timeout time.Duration) *Runner {
	return &Runner{log: log, timeout: timeout}
}

// Run executes the given test command and returns the result.
func (r *Runner) Run(ctx context.Context, workDir, command string, args []string) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	r.log.Info("running tests", "command", command, "args", args, "dir", workDir)
	start := time.Now()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err := cmd.Run()
	elapsed := time.Since(start)
	output := combined.String()

	if ctx.Err() != nil {
		return &Result{
			Passed:  false,
			Output:  fmt.Sprintf("test timed out after %s\n%s", r.timeout, output),
			Elapsed: elapsed,
		}, nil
	}

	if err != nil {
		r.log.Warn("tests failed", "elapsed", elapsed, "error", err)
		return &Result{
			Passed:  false,
			Output:  output,
			Elapsed: elapsed,
		}, nil
	}

	r.log.Info("tests passed", "elapsed", elapsed)
	return &Result{
		Passed:  true,
		Output:  output,
		Elapsed: elapsed,
	}, nil
}

// RunOverride runs a test override command string (parsed as shell command).
func (r *Runner) RunOverride(ctx context.Context, workDir, command string) (*Result, error) {
	return r.Run(ctx, workDir, "sh", []string{"-c", command})
}
