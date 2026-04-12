package github

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// runGH executes a gh CLI command with retry on rate-limit errors.
// Returns stdout output. Retries up to 3 times with 1s/2s/4s backoff.
func runGH(ctx context.Context, workDir string, log *slog.Logger, args ...string) (string, error) {
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	Stats.RecordCall()

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
			Stats.RecordFailure()
			return "", fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, errMsg)
		}

		Stats.RecordRetry()

		backoff := backoffs[attempt]
		if hint := parseRetryAfter(errMsg); hint > 0 && hint > backoff {
			backoff = hint
		}

		log.Warn("rate limited by GitHub, retrying",
			"attempt", attempt+1, "backoff", backoff, "args", args)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
		}
	}
}

var retryAfterRe = regexp.MustCompile(`(?i)retry after (\d+)`)

// parseRetryAfter extracts a "retry after N" hint (in seconds) from stderr.
// Returns 0 if no hint is found.
func parseRetryAfter(stderr string) time.Duration {
	m := retryAfterRe.FindStringSubmatch(stderr)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// isRateLimited returns true if the error output indicates a GitHub API rate limit.
func isRateLimited(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "403") ||
		strings.Contains(lower, "secondary rate")
}
