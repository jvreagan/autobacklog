//go:build !windows

package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jvreagan/autobacklog/internal/backlog"
	"github.com/jvreagan/autobacklog/internal/config"
	"github.com/jvreagan/autobacklog/internal/notify"
	"github.com/jvreagan/autobacklog/internal/testutil"
)

func TestEndToEnd_RunCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}

	// ---- set up local git repo ----
	bare := testutil.InitBareRepo(t)
	workDir := filepath.Join(t.TempDir(), "work")
	testutil.RunCmd(t, "", "git", "clone", "--branch", "main", bare, workDir)
	testutil.RunCmd(t, workDir, "git", "config", "user.email", "test@test.com")
	testutil.RunCmd(t, workDir, "git", "config", "user.name", "Test")

	// Write a Go file so Claude's review has something to find.
	if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}
	testutil.RunCmd(t, workDir, "git", "add", "-A")
	testutil.RunCmd(t, workDir, "git", "commit", "-m", "add main.go")
	testutil.RunCmd(t, workDir, "git", "push", "origin", "main")

	// ---- set up stub binaries ----
	binDir := testutil.StubBinDir(t)

	// Claude stub: when --output-format json is in args, return review findings.
	// Otherwise (print mode for implementation), create a file change.
	testutil.WriteStubScript(t, binDir, "claude", `
for arg in "$@"; do
  case "$arg" in
    --output-format)
      # JSON mode = review
      cat <<'ENDJSON'
{"result":"[{\"title\":\"Add error handling to main\",\"description\":\"The main function should handle errors.\",\"file_path\":\"main.go\",\"line_number\":3,\"priority\":\"high\",\"category\":\"bug\"}]","cost_usd":{"input":0.01,"output":0.02,"total":0.03}}
ENDJSON
      exit 0
      ;;
  esac
done
# Print mode = implementation: create a file change
echo '// improved by autobacklog' >> main.go
`)

	// gh stub: handles pr create, issue list, label create, repo view, api, auth
	testutil.WriteStubScript(t, binDir, "gh", `
case "$1" in
  pr)
    case "$2" in
      create)
        echo "https://github.com/test/repo/pull/1"
        ;;
      merge)
        exit 0
        ;;
    esac
    ;;
  issue)
    case "$2" in
      list)
        echo "[]"
        ;;
      create)
        echo "https://github.com/test/repo/issues/1"
        ;;
    esac
    ;;
  label)
    exit 0
    ;;
  repo)
    echo "test/repo"
    ;;
  api)
    echo "[]"
    ;;
  auth)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`)

	// ---- set up config ----
	cfg := &config.Config{
		Repo: config.RepoConfig{
			URL:            bare,
			Branch:         "main",
			WorkDir:        workDir,
			PRBranchPrefix: "autobacklog",
		},
		GitHub: config.GitHubConfig{
			IssueLabel: "autobacklog",
		},
		Claude: config.ClaudeConfig{
			Binary:           "claude",
			Model:            "sonnet",
			MaxBudgetPerCall: 5.0,
			MaxBudgetTotal:   50.0,
			Timeout:          30 * time.Second,
		},
		Backlog: config.BacklogConfig{
			HighThreshold:   1,
			MediumThreshold: 3,
			LowThreshold:    5,
			MaxPerCycle:     5,
			StaleDays:       30,
		},
		Testing: config.TestingConfig{
			OverrideCommand: "true",
			Timeout:         30 * time.Second,
			MaxRetries:      1,
		},
		Mode:       "oneshot",
		HelperMode: "buildbacklog",
		Logging:    config.LoggingConfig{Level: "info", Format: "text"},
	}

	// ---- set up store ----
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	// ---- create app using production constructor ----
	log := slog.Default()
	app, err := New(cfg, store, notify.NoopNotifier{}, log, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// ---- run a full cycle ----
	ctx := context.Background()
	stats, err := app.RunCycle(ctx)
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}

	// ---- verify results ----
	if stats.ItemsFound == 0 {
		t.Error("expected items found > 0")
	}
	if stats.ItemsInserted == 0 {
		t.Error("expected items inserted > 0")
	}
	if stats.PRsCreated == 0 {
		t.Error("expected PRs created > 0")
	}

	// Check that at least one item reached "done" status in the database.
	doneStatus := backlog.StatusDone
	doneItems, err := store.List(ctx, backlog.ListFilter{Status: &doneStatus})
	if err != nil {
		t.Fatalf("listing done items: %v", err)
	}
	if len(doneItems) == 0 {
		t.Error("expected at least one item with status 'done'")
	}
}
