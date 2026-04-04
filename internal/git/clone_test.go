package git

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// TestRunRedactsPAT verifies the safety-net redaction in run(): even if a PAT
// somehow appears in command arguments, it is scrubbed from error messages.
func TestRunRedactsPAT(t *testing.T) {
	const pat = "ghp_supersecrettoken"
	r := NewRepo("https://github.com/user/repo.git", "main", t.TempDir(), pat, slog.Default())

	// Run a command that fails and includes the PAT in its arguments.
	err := r.run(context.Background(), "", "git", "clone", "https://"+pat+"@github.com/user/repo.git", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
	if strings.Contains(err.Error(), pat) {
		t.Errorf("error message contains PAT: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("error message does not contain [REDACTED]: %v", err)
	}
}

func TestRunNoPATNoRedaction(t *testing.T) {
	r := NewRepo("https://github.com/user/repo.git", "main", t.TempDir(), "", slog.Default())

	err := r.run(context.Background(), "", "git", "clone", "https://github.com/user/nonexistent.git", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
	if strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("error message unexpectedly contains [REDACTED]: %v", err)
	}
}

// TestRunGitPATNotInArgs verifies that runGit never embeds the PAT in command
// arguments. The PAT travels only via the GIT_PAT environment variable.
func TestRunGitPATNotInArgs(t *testing.T) {
	const pat = "ghp_supersecrettoken"
	r := NewRepo("https://github.com/user/repo.git", "main", t.TempDir(), pat, slog.Default())

	err := r.runGit(context.Background(), "", "clone", "https://github.com/user/repo.git", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
	if strings.Contains(err.Error(), pat) {
		t.Errorf("PAT leaked into error message: %v", err)
	}
	// The credential helper arg references $GIT_PAT, not the literal secret.
	if strings.Contains(err.Error(), "GIT_PAT="+pat) {
		t.Errorf("PAT leaked via env var assignment in error message: %v", err)
	}
}
