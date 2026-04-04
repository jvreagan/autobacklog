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

// --- Integration tests using a local bare repo ---

// initBareRepo creates a bare git repository with a single committed file on
// the "main" branch. It returns the path to the bare repo directory.
func initBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "upstream.git")

	// Create the bare repository.
	runCmd(t, "", "git", "init", "--bare", bare)

	// Clone into a temporary working copy so we can create an initial commit.
	staging := filepath.Join(t.TempDir(), "staging")
	runCmd(t, "", "git", "clone", bare, staging)
	runCmd(t, staging, "git", "config", "user.email", "test@test.com")
	runCmd(t, staging, "git", "config", "user.name", "Test")

	// git clone of an empty bare repo may default to "master"; make sure we
	// are on "main" before pushing.
	runCmd(t, staging, "git", "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(staging, "README.md"), []byte("# test repo\n"), 0644); err != nil {
		t.Fatalf("writing seed file: %v", err)
	}
	runCmd(t, staging, "git", "add", "-A")
	runCmd(t, staging, "git", "commit", "-m", "initial commit")
	runCmd(t, staging, "git", "push", "-u", "origin", "main")

	return bare
}

// runCmd is a small helper that runs a command and fails the test on error.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func TestCloneOrPull_Clone(t *testing.T) {
	bare := initBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone-target")
	r := NewRepo(bare, "main", workDir, "", slog.Default())

	ctx := context.Background()
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull (clone): %v", err)
	}

	// The working directory should now exist and contain the seed file.
	readme := filepath.Join(workDir, "README.md")
	data, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("reading cloned README: %v", err)
	}
	if string(data) != "# test repo\n" {
		t.Errorf("unexpected README content: %q", data)
	}
}

func TestCloneOrPull_Pull(t *testing.T) {
	bare := initBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone-target")
	r := NewRepo(bare, "main", workDir, "", slog.Default())

	ctx := context.Background()

	// First call: clone.
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull (clone): %v", err)
	}

	// Push a new commit to the bare repo via a separate working copy so the
	// main clone can pull it.
	staging := filepath.Join(t.TempDir(), "staging2")
	runCmd(t, "", "git", "clone", bare, staging)
	runCmd(t, staging, "git", "config", "user.email", "test@test.com")
	runCmd(t, staging, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(staging, "extra.txt"), []byte("extra\n"), 0644); err != nil {
		t.Fatalf("writing extra file: %v", err)
	}
	runCmd(t, staging, "git", "add", "-A")
	runCmd(t, staging, "git", "commit", "-m", "add extra file")
	runCmd(t, staging, "git", "push", "origin", "main")

	// Second call: should pull the new commit.
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull (pull): %v", err)
	}

	extra := filepath.Join(workDir, "extra.txt")
	data, err := os.ReadFile(extra)
	if err != nil {
		t.Fatalf("reading pulled file: %v", err)
	}
	if string(data) != "extra\n" {
		t.Errorf("unexpected extra.txt content: %q", data)
	}
}

func TestCreateBranch(t *testing.T) {
	bare := initBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone-target")
	r := NewRepo(bare, "main", workDir, "", slog.Default())

	ctx := context.Background()
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	branchName, err := r.CreateBranch(ctx, "autobacklog", "bug", "Fix Null Pointer")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if branchName != "autobacklog/bug/fix-null-pointer" {
		t.Errorf("branch name = %q, want autobacklog/bug/fix-null-pointer", branchName)
	}

	// Verify git is on the new branch.
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	actual := strings.TrimSpace(string(out))
	if actual != branchName {
		t.Errorf("HEAD branch = %q, want %q", actual, branchName)
	}
}

func TestHasChanges_Integration(t *testing.T) {
	bare := initBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone-target")
	r := NewRepo(bare, "main", workDir, "", slog.Default())

	ctx := context.Background()
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	// Clean clone should have no changes.
	has, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if has {
		t.Error("expected no changes in a freshly cloned repo")
	}

	// Write a new file; HasChanges should now return true.
	if err := os.WriteFile(filepath.Join(workDir, "dirty.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	has, err = r.HasChanges(ctx)
	if err != nil {
		t.Fatalf("HasChanges after modification: %v", err)
	}
	if !has {
		t.Error("expected changes after writing a new file")
	}
}

func TestCheckoutBranch(t *testing.T) {
	bare := initBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone-target")
	r := NewRepo(bare, "main", workDir, "", slog.Default())

	ctx := context.Background()
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	// Create a feature branch, then switch back to main.
	if _, err := r.CreateBranch(ctx, "autobacklog", "feat", "New Thing"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if err := r.CheckoutBranch(ctx, "main"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	actual := strings.TrimSpace(string(out))
	if actual != "main" {
		t.Errorf("HEAD branch = %q, want main", actual)
	}
}

func TestStageAllAndCommit(t *testing.T) {
	bare := initBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "clone-target")
	r := NewRepo(bare, "main", workDir, "", slog.Default())

	ctx := context.Background()
	if err := r.CloneOrPull(ctx); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	// Configure committer identity for the cloned repo.
	runCmd(t, workDir, "git", "config", "user.email", "test@test.com")
	runCmd(t, workDir, "git", "config", "user.name", "Test")

	// Create a new file and stage it.
	newFile := filepath.Join(workDir, "feature.txt")
	if err := os.WriteFile(newFile, []byte("feature\n"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	if err := r.StageAll(ctx); err != nil {
		t.Fatalf("StageAll: %v", err)
	}

	// Commit the staged changes.
	if err := r.Commit(ctx, "add feature file"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// After commit, the working directory should be clean.
	has, err := r.HasChanges(ctx)
	if err != nil {
		t.Fatalf("HasChanges after commit: %v", err)
	}
	if has {
		t.Error("expected no changes after committing")
	}

	// Verify the commit message landed in the log.
	cmd := exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	msg := strings.TrimSpace(string(out))
	if msg != "add feature file" {
		t.Errorf("commit message = %q, want %q", msg, "add feature file")
	}
}
