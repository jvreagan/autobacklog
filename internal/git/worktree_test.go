//go:build !windows

package git

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesreagan/autobacklog/internal/testutil"
)

func TestAddWorktree(t *testing.T) {
	bare := testutil.InitBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone")
	log := slog.Default()

	repo := NewRepo(bare, "main", workDir, "", log)
	ctx := context.Background()

	if err := repo.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	wtPath := filepath.Join(t.TempDir(), "worktree1")
	if err := repo.AddWorktree(ctx, wtPath); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	// Verify the worktree directory exists and has files
	if _, err := os.Stat(filepath.Join(wtPath, "README.md")); err != nil {
		t.Errorf("worktree should contain README.md: %v", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	bare := testutil.InitBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone")
	log := slog.Default()

	repo := NewRepo(bare, "main", workDir, "", log)
	ctx := context.Background()

	if err := repo.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	wtPath := filepath.Join(t.TempDir(), "worktree2")
	if err := repo.AddWorktree(ctx, wtPath); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	if err := repo.RemoveWorktree(ctx, wtPath); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	// Verify the directory is removed
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory should be removed after RemoveWorktree")
	}
}

func TestNewWorktreeRepo(t *testing.T) {
	log := slog.Default()
	repo := NewRepo("https://example.com/repo.git", "main", "/tmp/main", "secret", log)

	wtRepo := repo.NewWorktreeRepo("/tmp/worktree")
	if wtRepo.WorkDir() != "/tmp/worktree" {
		t.Errorf("WorkDir() = %q, want /tmp/worktree", wtRepo.WorkDir())
	}
	if wtRepo.url != repo.url {
		t.Errorf("url = %q, want %q", wtRepo.url, repo.url)
	}
	if wtRepo.branch != repo.branch {
		t.Errorf("branch = %q, want %q", wtRepo.branch, repo.branch)
	}
	if wtRepo.pat != repo.pat {
		t.Errorf("pat should be inherited from parent repo")
	}
}
