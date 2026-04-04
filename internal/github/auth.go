package github

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

var (
	storedPAT string
	patMu     sync.Mutex
)

// SetupAuth configures gh CLI authentication using a PAT.
// The PAT is stored in-process and injected into child process environments
// via ghEnv() rather than mutating the global os environment.
func SetupAuth(ctx context.Context, pat string, log *slog.Logger) error {
	if pat == "" {
		// Check if gh is already authenticated
		cmd := exec.CommandContext(ctx, "gh", "auth", "status")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gh not authenticated and no PAT provided")
		}
		log.Info("gh CLI already authenticated")
		return nil
	}

	log.Info("setting up gh CLI authentication")

	patMu.Lock()
	storedPAT = pat
	patMu.Unlock()

	// Verify auth works by passing PAT via subprocess env only
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	cmd.Env = ghEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh auth failed: %w", err)
	}

	return nil
}

// ghEnv returns the current process environment with GITHUB_TOKEN set to the
// stored PAT. This avoids mutating the global process environment.
func ghEnv() []string {
	patMu.Lock()
	pat := storedPAT
	patMu.Unlock()

	env := os.Environ()
	if pat != "" {
		env = append(env, "GITHUB_TOKEN="+pat)
	}
	return env
}
