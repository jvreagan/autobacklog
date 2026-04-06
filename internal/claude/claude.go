package claude

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/config"
)

// Client wraps the Claude Code CLI for subprocess invocation.
type Client struct {
	cfg        config.ClaudeConfig
	budget     *Budget
	log        *slog.Logger
	stdoutSink io.Writer
	stderrSink io.Writer
}

// NewClient creates a new Claude CLI client.
func NewClient(cfg config.ClaudeConfig, log *slog.Logger) *Client {
	return &Client{
		cfg:        cfg,
		budget:     NewBudget(cfg.MaxBudgetTotal),
		log:        log,
		stdoutSink: os.Stdout,
		stderrSink: os.Stderr,
	}
}

// SetOutputWriters overrides the default stdout/stderr sinks used when
// streaming Claude CLI output to the terminal.
func (c *Client) SetOutputWriters(stdout, stderr io.Writer) {
	c.stdoutSink = stdout
	c.stderrSink = stderr
}

// Budget returns the budget tracker.
func (c *Client) Budget() *Budget {
	return c.budget
}

// buildArgs constructs the CLI arguments for a Claude invocation.
// If jsonOutput is true, --output-format json is included for structured responses.
func (c *Client) buildArgs(prompt string, jsonOutput bool) []string {
	args := []string{"--print"}
	if jsonOutput {
		args = append(args, "--output-format", "json")
	}
	args = append(args, "--model", c.cfg.Model, "--max-budget-usd", fmt.Sprintf("%.2f", c.cfg.MaxBudgetPerCall))
	if c.cfg.DangerouslySkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	return append(args, prompt)
}

// Run invokes the Claude CLI with the given prompt in the given working directory.
// Returns the raw output string and any error.
func (c *Client) Run(ctx context.Context, workDir, prompt string) (string, error) {
	output, err := c.execute(ctx, workDir, prompt, true)
	if err != nil {
		return "", err
	}

	// Try to extract cost from output and record it
	resp, parseErr := parseClaudeResponse(output)
	if parseErr == nil && resp.Cost.Total > 0 {
		c.budget.Record(resp.Cost.Total)
		c.log.Info("claude invocation complete", "cost", fmt.Sprintf("$%.4f", resp.Cost.Total), "budget_status", c.budget.String())
	} else {
		// Record the max per-call budget as a conservative estimate
		c.budget.Record(c.cfg.MaxBudgetPerCall)
		c.log.Warn("could not parse cost from output, recording max budget per call")
	}

	return output, nil
}

// RunPrint invokes Claude in print-only mode (no JSON output) for implementation tasks.
func (c *Client) RunPrint(ctx context.Context, workDir, prompt string) (string, error) {
	output, err := c.execute(ctx, workDir, prompt, false)
	if err != nil {
		return "", err
	}

	// Conservative budget recording for non-JSON mode
	c.budget.Record(c.cfg.MaxBudgetPerCall)

	return output, nil
}

// execute is the shared implementation for Run and RunPrint. When jsonOutput is
// true, stdout is captured only (for JSON parsing); when false, stdout is also
// streamed to the terminal so the user sees Claude's progress.
func (c *Client) execute(ctx context.Context, workDir, prompt string, jsonOutput bool) (string, error) {
	if !c.budget.CanSpend(c.cfg.MaxBudgetPerCall) {
		return "", fmt.Errorf("budget exceeded: %s", c.budget.String())
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	args := c.buildArgs(prompt, jsonOutput)

	mode := "print mode"
	if jsonOutput {
		mode = "json mode"
	}
	c.log.Info("invoking claude", "mode", mode, "model", c.cfg.Model, "work_dir", workDir, "budget_remaining", fmt.Sprintf("$%.2f", c.budget.Remaining()))

	cmd := exec.CommandContext(ctx, c.cfg.Binary, args...)
	cmd.Dir = workDir
	cmd.Env = filteredEnv()

	var stdout, stderr bytes.Buffer
	if jsonOutput {
		cmd.Stdout = &limitedWriter{w: &stdout, limit: maxOutputBytes}
	} else {
		cmd.Stdout = io.MultiWriter(&limitedWriter{w: &stdout, limit: maxOutputBytes}, c.stdoutSink)
	}
	cmd.Stderr = io.MultiWriter(&limitedWriter{w: &stderr, limit: maxOutputBytes}, c.stderrSink)

	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("claude timed out after %s: %w", c.cfg.Timeout, ctx.Err())
		}
		c.log.Error("claude CLI failed",
			"mode", mode,
			"exit_error", err,
			"stderr", stderr.String(),
			"stdout_len", stdout.Len(),
			"args", args[:len(args)-1], // log args without the prompt
		)
		return "", fmt.Errorf("claude failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	return stdout.String(), nil
}

// maxOutputBytes is the maximum size of captured stdout/stderr from the Claude CLI (100 MB).
const maxOutputBytes = 100 * 1024 * 1024

// limitedWriter wraps an io.Writer and stops writing after limit bytes.
type limitedWriter struct {
	w       *bytes.Buffer
	limit   int
	written int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	remaining := lw.limit - lw.written
	if remaining <= 0 {
		return len(p), nil // discard silently
	}
	originalLen := len(p)
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := lw.w.Write(p)
	lw.written += n
	if err != nil {
		return n, err
	}
	// Return the original length to satisfy the io.Writer contract:
	// n == len(p) when err == nil. Excess bytes are intentionally discarded.
	return originalLen, nil
}

// filteredEnv returns the current environment with the CLAUDECODE variable
// removed, which prevents the "nested session" check from blocking invocation.
func filteredEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			env = append(env, e)
		}
	}
	return env
}
