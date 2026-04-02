package claude

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jamesreagan/autobacklog/internal/config"
)

func newTestClient(skipPerms bool) *Client {
	cfg := config.ClaudeConfig{
		Binary:                     "claude",
		Model:                      "sonnet",
		MaxBudgetPerCall:           5.0,
		MaxBudgetTotal:             50.0,
		Timeout:                    10 * time.Minute,
		DangerouslySkipPermissions: skipPerms,
	}
	return NewClient(cfg, slog.Default())
}

func TestBuildArgs_SkipPermissionsDisabled(t *testing.T) {
	c := newTestClient(false)
	args := c.buildArgs("do something", true)
	for _, arg := range args {
		if arg == "--dangerously-skip-permissions" {
			t.Error("--dangerously-skip-permissions should not be present when DangerouslySkipPermissions=false")
		}
	}
}

func TestBuildArgs_SkipPermissionsEnabled(t *testing.T) {
	c := newTestClient(true)
	args := c.buildArgs("do something", true)
	found := false
	for _, arg := range args {
		if arg == "--dangerously-skip-permissions" {
			found = true
			break
		}
	}
	if !found {
		t.Error("--dangerously-skip-permissions should be present when DangerouslySkipPermissions=true")
	}
}

func TestBuildArgs_JSONOutputIncluded(t *testing.T) {
	c := newTestClient(false)
	args := c.buildArgs("prompt", true)
	for i, arg := range args {
		if arg == "--output-format" && i+1 < len(args) && args[i+1] == "json" {
			return
		}
	}
	t.Error("--output-format json should be present when jsonOutput=true")
}

func TestBuildArgs_JSONOutputExcluded(t *testing.T) {
	c := newTestClient(false)
	args := c.buildArgs("prompt", false)
	for _, arg := range args {
		if arg == "--output-format" {
			t.Error("--output-format should not be present when jsonOutput=false")
		}
	}
}

func TestBuildArgs_PromptIsLast(t *testing.T) {
	c := newTestClient(false)
	prompt := "my test prompt"
	args := c.buildArgs(prompt, true)
	if len(args) == 0 || args[len(args)-1] != prompt {
		t.Errorf("prompt should be the last argument, got %v", args)
	}
}
