package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/jamesreagan/autobacklog/internal/claude"
)

// allowedTestCommands is the set of commands that auto-detection may produce.
// Any detected command not in this list is rejected (#122).
var allowedTestCommands = map[string]bool{
	"go":      true,
	"npm":     true,
	"npx":     true,
	"yarn":    true,
	"pnpm":    true,
	"pytest":  true,
	"python":  true,
	"python3": true,
	"mvn":     true,
	"gradle":  true,
	"gradlew": true,
	"cargo":   true,
	"make":    true,
	"dotnet":  true,
	"mix":     true,
	"bundle":  true,
	"rake":    true,
	"bun":     true,
}

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

// ValidateCommand checks that a test command is in the allowlist (#122).
func ValidateCommand(command string) error {
	base := command
	if i := strings.LastIndex(command, "/"); i >= 0 {
		base = command[i+1:]
	}
	if !allowedTestCommands[base] {
		return fmt.Errorf("test command %q is not in the allowed list", command)
	}
	return nil
}

// Run executes the given test command and returns the result.
// Returns a non-nil error for infrastructure failures (e.g. missing binary)
// as distinct from test failures (#196).
func (r *Runner) Run(ctx context.Context, workDir, command string, args []string) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	r.log.Info("running tests", "command", command, "args", args, "dir", workDir)
	start := time.Now()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir

	// Use shared LimitedWriter (#197). Cap combined output at 50 MB.
	const maxTestOutput = 50 * 1024 * 1024
	var buf bytes.Buffer
	combined := &claude.LimitedWriter{W: &buf, Limit: maxTestOutput}
	cmd.Stdout = combined
	cmd.Stderr = combined

	err := cmd.Run()
	elapsed := time.Since(start)
	output := buf.String()

	if ctx.Err() != nil {
		return &Result{
			Passed:  false,
			Output:  fmt.Sprintf("test timed out after %s\n%s", r.timeout, output),
			Elapsed: elapsed,
		}, nil
	}

	if err != nil {
		// #196: distinguish missing binary from test failure
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return nil, fmt.Errorf("test binary not found: %w", err)
		}
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
