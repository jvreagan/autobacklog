package github

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
)

// SetupAuth configures gh CLI authentication using a PAT.
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

	// Set GITHUB_TOKEN env var for gh CLI
	os.Setenv("GITHUB_TOKEN", pat)

	// Verify auth works
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh auth failed: %w", err)
	}

	return nil
}
