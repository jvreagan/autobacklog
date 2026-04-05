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
	cfg    config.ClaudeConfig
	budget *Budget
	log    *slog.Logger
}

// NewClient creates a new Claude CLI client.
func NewClient(cfg config.ClaudeConfig, log *slog.Logger) *Client {
	return &Client{
		cfg:    cfg,
		budget: NewBudget(cfg.MaxBudgetTotal),
		log:    log,
	}
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
	if !c.budget.CanSpend(c.cfg.MaxBudgetPerCall) {
		return "", fmt.Errorf("budget exceeded: %s", c.budget.String())
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	args := c.buildArgs(prompt, true)

	c.log.Info("invoking claude", "model", c.cfg.Model, "work_dir", workDir, "budget_remaining", fmt.Sprintf("$%.2f", c.budget.Remaining()))

	cmd := exec.CommandContext(ctx, c.cfg.Binary, args...)
	cmd.Dir = workDir
	cmd.Env = filteredEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: maxOutputBytes}
	// Stream stderr to terminal so the user sees progress during JSON-mode calls
	cmd.Stderr = io.MultiWriter(&limitedWriter{w: &stderr, limit: maxOutputBytes}, os.Stderr)

	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("claude timed out after %s: %w", c.cfg.Timeout, ctx.Err())
		}
		c.log.Error("claude CLI failed",
			"exit_error", err,
			"stderr", stderr.String(),
			"stdout_len", stdout.Len(),
			"args", args[:len(args)-1], // log args without the prompt
		)
		return "", fmt.Errorf("claude failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	output := stdout.String()

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
	if !c.budget.CanSpend(c.cfg.MaxBudgetPerCall) {
		return "", fmt.Errorf("budget exceeded: %s", c.budget.String())
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	args := c.buildArgs(prompt, false)

	c.log.Info("invoking claude (print mode)", "model", c.cfg.Model, "work_dir", workDir)

	cmd := exec.CommandContext(ctx, c.cfg.Binary, args...)
	cmd.Dir = workDir
	cmd.Env = filteredEnv()

	var stdout, stderr bytes.Buffer
	// Stream both stdout and stderr to the terminal so the user sees Claude's progress
	cmd.Stdout = io.MultiWriter(&limitedWriter{w: &stdout, limit: maxOutputBytes}, os.Stdout)
	cmd.Stderr = io.MultiWriter(&limitedWriter{w: &stderr, limit: maxOutputBytes}, os.Stderr)

	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("claude timed out: %w", ctx.Err())
		}
		c.log.Error("claude CLI failed (print mode)",
			"exit_error", err,
			"stderr", stderr.String(),
			"stdout_len", stdout.Len(),
			"args", args[:len(args)-1],
		)
		return "", fmt.Errorf("claude failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	// Conservative budget recording for non-JSON mode
	c.budget.Record(c.cfg.MaxBudgetPerCall)

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
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := lw.w.Write(p)
	lw.written += n
	return n, err
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
