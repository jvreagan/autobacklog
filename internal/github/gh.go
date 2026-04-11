package github

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// runGH executes a gh CLI command with retry on rate-limit errors.
// Returns stdout output. Retries up to 3 times with 1s/2s/4s backoff.
func runGH(ctx context.Context, workDir string, log *slog.Logger, args ...string) (string, error) {
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; ; attempt++ {
		cmd := exec.CommandContext(ctx, "gh", args...)
		cmd.Dir = workDir
		cmd.Env = ghEnv()

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			return strings.TrimSpace(stdout.String()), nil
		}

		errMsg := stderr.String()
		if !isRateLimited(errMsg) || attempt >= len(backoffs) {
			return "", fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, errMsg)
		}

		log.Warn("rate limited by GitHub, retrying",
			"attempt", attempt+1, "backoff", backoffs[attempt], "args", args)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoffs[attempt]):
		}
	}
}

// isRateLimited returns true if the error output indicates a GitHub API rate limit.
func isRateLimited(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "403") ||
		strings.Contains(lower, "secondary rate")
}
