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
	"sync"

	"github.com/jvreagan/autobacklog/internal/config"
)

// Client wraps the Claude Code CLI for subprocess invocation.
type Client struct {
	cfg        config.ClaudeConfig
	budget     *Budget
	log        *slog.Logger

	sinkMu     sync.Mutex // #151: protects stdoutSink/stderrSink
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
// Must be called during initialization, before any Run/RunPrint calls (#151).
func (c *Client) SetOutputWriters(stdout, stderr io.Writer) {
	c.sinkMu.Lock()
	defer c.sinkMu.Unlock()
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
		// #153: record a smaller fallback instead of MaxBudgetPerCall
		fallback := c.cfg.MaxBudgetPerCall * 0.1
		c.budget.Record(fallback)
		c.log.Warn("could not parse cost from output, recording conservative estimate", "fallback", fmt.Sprintf("$%.4f", fallback))
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

	c.sinkMu.Lock()
	stdoutSink := c.stdoutSink
	stderrSink := c.stderrSink
	c.sinkMu.Unlock()

	var stdout, stderr bytes.Buffer
	if jsonOutput {
		cmd.Stdout = &LimitedWriter{W: &stdout, Limit: maxOutputBytes}
	} else {
		cmd.Stdout = io.MultiWriter(&LimitedWriter{W: &stdout, Limit: maxOutputBytes}, stdoutSink)
	}
	cmd.Stderr = io.MultiWriter(&LimitedWriter{W: &stderr, Limit: maxOutputBytes}, stderrSink)

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

// maxOutputBytes is the maximum size of captured stdout/stderr from the Claude CLI (10 MB).
// Reduced from 100 MB to avoid OOM on constrained systems (#199).
const maxOutputBytes = 10 * 1024 * 1024

// LimitedWriter wraps an io.Writer and stops writing after Limit bytes.
// Exported so it can be shared with the runner package (#197).
type LimitedWriter struct {
	W       io.Writer
	Limit   int
	Written int
}

func (lw *LimitedWriter) Write(p []byte) (int, error) {
	remaining := lw.Limit - lw.Written
	if remaining <= 0 {
		return len(p), nil // discard silently
	}
	originalLen := len(p)
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := lw.W.Write(p)
	lw.Written += n
	if err != nil {
		return n, err
	}
	// Return the original length to satisfy the io.Writer contract:
	// n == len(p) when err == nil. Excess bytes are intentionally discarded.
	return originalLen, nil
}

// filteredEnv returns the current environment with the CLAUDECODE variable
// removed, which prevents the "nested session" check from blocking invocation.
// Also filters sensitive environment variables (#200).
func filteredEnv() []string {
	// Prefixes of sensitive env vars to exclude from Claude subprocess.
	sensitivePrefix := []string{
		"AWS_SECRET", "AWS_SESSION_TOKEN",
		"DATABASE_", "DB_PASSWORD", "DB_",
		"PRIVATE_KEY", "SECRET_",
	}
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		key := e[:strings.IndexByte(e, '=')]
		skip := false
		for _, prefix := range sensitivePrefix {
			if strings.HasPrefix(strings.ToUpper(key), prefix) {
				skip = true
				break
			}
		}
		if !skip {
			env = append(env, e)
		}
	}
	return env
}
