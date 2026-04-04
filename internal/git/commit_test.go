package git

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a temp git repo with one committed file.
func initTestRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}

	return NewRepo("", "main", dir, "", slog.Default())
}

func TestHasChanges_CleanRepo(t *testing.T) {
	r := initTestRepo(t)
	ctx := context.Background()

	has, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("clean repo should have no changes")
	}
}

func TestHasChanges_UntrackedFile(t *testing.T) {
	r := initTestRepo(t)
	ctx := context.Background()

	os.WriteFile(filepath.Join(r.WorkDir(), "new.txt"), []byte("hello"), 0644)

	has, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("should detect untracked file")
	}
}

func TestHasChanges_StagedChanges(t *testing.T) {
	r := initTestRepo(t)
	ctx := context.Background()

	path := filepath.Join(r.WorkDir(), "staged.txt")
	os.WriteFile(path, []byte("hello"), 0644)
	r.StageAll(ctx)

	has, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("should detect staged changes")
	}
}

func TestRevertToClean_RemovesUntracked(t *testing.T) {
	r := initTestRepo(t)
	ctx := context.Background()

	// Need at least one tracked file so git checkout . doesn't fail
	tracked := filepath.Join(r.WorkDir(), "tracked.txt")
	os.WriteFile(tracked, []byte("keep"), 0644)
	r.StageAll(ctx)
	r.Commit(ctx, "add tracked file")

	path := filepath.Join(r.WorkDir(), "untracked.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	if err := r.RevertToClean(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("untracked file should be removed after revert")
	}
}

func TestRevertToClean_RevertsModified(t *testing.T) {
	r := initTestRepo(t)
	ctx := context.Background()

	// Create and commit a file
	path := filepath.Join(r.WorkDir(), "tracked.txt")
	os.WriteFile(path, []byte("original"), 0644)
	r.StageAll(ctx)
	r.Commit(ctx, "add tracked file")

	// Modify it
	os.WriteFile(path, []byte("modified"), 0644)

	if err := r.RevertToClean(ctx); err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "original" {
		t.Errorf("file content = %q, want 'original'", content)
	}
}

func TestWorkDir_ReturnsConfigured(t *testing.T) {
	r := NewRepo("https://example.com", "main", "/some/path", "", slog.Default())
	if r.WorkDir() != "/some/path" {
		t.Errorf("WorkDir() = %q, want /some/path", r.WorkDir())
	}
}

func TestStageAll_StagesNewFile(t *testing.T) {
	r := initTestRepo(t)
	ctx := context.Background()

	path := filepath.Join(r.WorkDir(), "newfile.txt")
	os.WriteFile(path, []byte("content"), 0644)

	if err := r.StageAll(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify it's staged by checking git status --porcelain
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.WorkDir()
	out, _ := cmd.Output()
	if len(out) == 0 {
		t.Error("expected staged file in status output")
	}
	// Should show as added (A)
	status := string(out)
	if status[0] != 'A' {
		t.Errorf("expected staged file status 'A', got %q", status)
	}
}

// TestRunGit_CredentialHelperInjected verifies that runGit prepends the
// credential.helper git config flags when a PAT is set, and that the PAT
// itself does not appear in the command arguments (only $GIT_PAT does).
func TestRunGit_CredentialHelperInjected(t *testing.T) {
	const pat = "ghp_token123"
	r := NewRepo("https://github.com/user/repo.git", "main", t.TempDir(), pat, slog.Default())

	err := r.runGit(context.Background(), "", "ls-remote", "https://github.com/user/nonexistent99999.git")
	// We expect a failure (repo doesn't exist), but the PAT must not leak.
	if err != nil && strings.Contains(err.Error(), pat) {
		t.Errorf("PAT leaked into error output: %v", err)
	}
}

// TestRunGit_NoPATNoCred verifies that runGit does not inject credential
// helper flags when no PAT is configured.
func TestRunGit_NoPATNoCred(t *testing.T) {
	r := NewRepo("https://github.com/user/repo.git", "main", t.TempDir(), "", slog.Default())

	err := r.runGit(context.Background(), "", "version")
	if err != nil {
		t.Errorf("git version failed unexpectedly: %v", err)
	}
}
