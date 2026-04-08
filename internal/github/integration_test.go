//go:build !windows

package github

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jamesreagan/autobacklog/internal/testutil"
)

func TestCreatePR_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "https://github.com/owner/repo/pull/42"`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	url, err := CreatePR(ctx, workDir, PRRequest{
		Title:      "Test PR",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feature",
	}, log)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if url != "https://github.com/owner/repo/pull/42" {
		t.Errorf("url = %q, want https://github.com/owner/repo/pull/42", url)
	}
}

func TestEnableAutoMerge_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `exit 0`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	err := EnableAutoMerge(ctx, workDir, "https://github.com/owner/repo/pull/42", log)
	if err != nil {
		t.Fatalf("EnableAutoMerge: %v", err)
	}
}

func TestListIssues_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	// gh is called twice: first for repo view, then for api --paginate
	testutil.WriteStubScript(t, binDir, "gh", `
case "$1" in
  repo)
    echo "owner/repo"
    ;;
  api)
    echo '[{"number":1,"title":"bug report","body":"fix it","state":"open","labels":[{"name":"autobacklog"}]},{"number":2,"title":"pull req","body":"","state":"open","labels":[],"pull_request":{}}]'
    ;;
  *)
    exit 1
    ;;
esac
`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	issues, err := ListIssues(ctx, workDir, "autobacklog", log)
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	// Should filter out the PR
	if len(issues) != 1 {
		t.Fatalf("len = %d, want 1 (PR should be filtered)", len(issues))
	}
	if issues[0].Number != 1 {
		t.Errorf("Number = %d, want 1", issues[0].Number)
	}
	if issues[0].Title != "bug report" {
		t.Errorf("Title = %q, want 'bug report'", issues[0].Title)
	}
}

func TestEnsureLabel_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `exit 0`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	err := EnsureLabel(ctx, workDir, "autobacklog", log)
	if err != nil {
		t.Fatalf("EnsureLabel: %v", err)
	}
}

func TestCreateIssue_Integration(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "https://github.com/owner/repo/issues/99"`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	num, err := CreateIssue(ctx, workDir, "Test Issue", "body text", []string{"bug"}, log)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if num != 99 {
		t.Errorf("num = %d, want 99", num)
	}
}

func TestCreatePR_Error(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "something went wrong" >&2; exit 1`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	_, err := CreatePR(ctx, workDir, PRRequest{
		Title:      "Fail",
		Body:       "body",
		BaseBranch: "main",
		HeadBranch: "feature",
	}, log)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error = %v, want to contain 'something went wrong'", err)
	}
}

func TestListIssues_RepoViewError(t *testing.T) {
	binDir := testutil.StubBinDir(t)
	testutil.WriteStubScript(t, binDir, "gh", `echo "not a repo" >&2; exit 1`)

	ctx := context.Background()
	workDir := t.TempDir()
	log := slog.Default()

	_, err := ListIssues(ctx, workDir, "autobacklog", log)
	if err == nil {
		t.Fatal("expected error when repo view fails")
	}
}
