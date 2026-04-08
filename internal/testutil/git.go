//go:build !windows

package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// InitBareRepo creates a bare git repository with a single committed file on
// the "main" branch. Returns the path to the bare repo directory.
func InitBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "upstream.git")

	RunCmd(t, "", "git", "init", "--bare", bare)

	staging := filepath.Join(t.TempDir(), "staging")
	RunCmd(t, "", "git", "clone", bare, staging)
	RunCmd(t, staging, "git", "config", "user.email", "test@test.com")
	RunCmd(t, staging, "git", "config", "user.name", "Test")
	RunCmd(t, staging, "git", "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(staging, "README.md"), []byte("# test repo\n"), 0644); err != nil {
		t.Fatalf("writing seed file: %v", err)
	}
	RunCmd(t, staging, "git", "add", "-A")
	RunCmd(t, staging, "git", "commit", "-m", "initial commit")
	RunCmd(t, staging, "git", "push", "-u", "origin", "main")

	return bare
}

// RunCmd runs a command and fails the test on error.
func RunCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
