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

	// Cap combined output at 50 MB to prevent OOM on verbose test suites.
	const maxTestOutput = 50 * 1024 * 1024
	combined := &limitedBuffer{limit: maxTestOutput}
	cmd.Stdout = combined
	cmd.Stderr = combined

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

// RunOverride runs a test override command string via sh -c.
// The command is trusted (sourced from config file); no shell escaping is applied.
func (r *Runner) RunOverride(ctx context.Context, workDir, command string) (*Result, error) {
	return r.Run(ctx, workDir, "sh", []string{"-c", command})
}

// limitedBuffer is a bytes.Buffer that silently discards writes beyond the limit.
type limitedBuffer struct {
	buf     bytes.Buffer
	limit   int
	written int
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	remaining := lb.limit - lb.written
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := lb.buf.Write(p)
	lb.written += n
	return n, err
}

func (lb *limitedBuffer) String() string {
	return lb.buf.String()
}
