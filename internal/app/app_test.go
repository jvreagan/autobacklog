package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/claude"
	"github.com/jamesreagan/autobacklog/internal/config"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/notify"
	"github.com/jamesreagan/autobacklog/internal/runner"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockRepo struct {
	workDir          string
	cloneOrPullErr   error
	createBranchName string
	createBranchErr  error
	checkoutErr      error
	pushErr          error
	stageAllErr      error
	commitErr        error
	hasChangesVal    bool
	hasChangesErr    error
	revertErr        error
	deleteBranchErr  error

	cloneOrPullCalls  int
	createBranchCalls int
	checkoutCalls     int
	pushCalls         int
	stageAllCalls     int
	commitCalls       int
	hasChangesCalls   int
	deleteBranchCalls int
	revertCalls       int
}

func (m *mockRepo) WorkDir() string                          { return m.workDir }
func (m *mockRepo) CloneOrPull(_ context.Context) error      { m.cloneOrPullCalls++; return m.cloneOrPullErr }
func (m *mockRepo) CreateBranch(_ context.Context, prefix, category, title string) (string, error) {
	m.createBranchCalls++
	if m.createBranchErr != nil {
		return "", m.createBranchErr
	}
	name := m.createBranchName
	if name == "" {
		name = fmt.Sprintf("%s/%s/%s", prefix, category, title)
	}
	return name, nil
}
func (m *mockRepo) CheckoutBranch(_ context.Context, _ string) error {
	m.checkoutCalls++
	return m.checkoutErr
}
func (m *mockRepo) Push(_ context.Context, _ string) error {
	m.pushCalls++
	return m.pushErr
}
func (m *mockRepo) StageAll(_ context.Context) error  { m.stageAllCalls++; return m.stageAllErr }
func (m *mockRepo) Commit(_ context.Context, _ string) error { m.commitCalls++; return m.commitErr }
func (m *mockRepo) HasChanges(_ context.Context) (bool, error) {
	m.hasChangesCalls++
	return m.hasChangesVal, m.hasChangesErr
}
func (m *mockRepo) RevertToClean(_ context.Context) error { m.revertCalls++; return m.revertErr }
func (m *mockRepo) DeleteBranch(_ context.Context, _ string) error {
	m.deleteBranchCalls++
	return m.deleteBranchErr
}

type mockAIClient struct {
	runOutputs     []string
	runErrors      []error
	runPrintOutputs []string
	runPrintErrors  []error
	budget         *claude.Budget

	runIdx      int
	runPrintIdx int
	runCalls      int
	runPrintCalls int
}

func newMockAIClient(maxBudget float64) *mockAIClient {
	return &mockAIClient{budget: claude.NewBudget(maxBudget)}
}

func (m *mockAIClient) Run(_ context.Context, _, _ string) (string, error) {
	m.runCalls++
	idx := m.runIdx
	m.runIdx++
	var out string
	var err error
	if idx < len(m.runOutputs) {
		out = m.runOutputs[idx]
	}
	if idx < len(m.runErrors) {
		err = m.runErrors[idx]
	}
	return out, err
}

func (m *mockAIClient) RunPrint(_ context.Context, _, _ string) (string, error) {
	m.runPrintCalls++
	idx := m.runPrintIdx
	m.runPrintIdx++
	var out string
	var err error
	if idx < len(m.runPrintOutputs) {
		out = m.runPrintOutputs[idx]
	}
	if idx < len(m.runPrintErrors) {
		err = m.runPrintErrors[idx]
	}
	return out, err
}

func (m *mockAIClient) Budget() *claude.Budget { return m.budget }

type mockTestRunner struct {
	results []*runner.Result
	errors  []error
	idx     int
	calls   int
}

func (m *mockTestRunner) Run(_ context.Context, _, _ string, _ []string) (*runner.Result, error) {
	m.calls++
	idx := m.idx
	m.idx++
	var r *runner.Result
	var err error
	if idx < len(m.results) {
		r = m.results[idx]
	}
	if idx < len(m.errors) {
		err = m.errors[idx]
	}
	return r, err
}

type mockPRCreator struct {
	prURL          string
	createPRErr    error
	autoMergeErr   error
	createPRCalls  int
	autoMergeCalls int
}

func (m *mockPRCreator) CreatePR(_ context.Context, _ string, _ gh.PRRequest) (string, error) {
	m.createPRCalls++
	return m.prURL, m.createPRErr
}

func (m *mockPRCreator) EnableAutoMerge(_ context.Context, _, _ string) error {
	m.autoMergeCalls++
	return m.autoMergeErr
}

type mockIssueManager struct {
	createIssueNum   int
	createIssueErr   error
	ensureLabelErr   error
	listIssues       []gh.Issue
	listIssuesErr    error
	createIssueCalls int
	ensureLabelCalls int
	listIssuesCalls  int
}

func (m *mockIssueManager) EnsureLabel(_ context.Context, _, _ string) error {
	m.ensureLabelCalls++
	return m.ensureLabelErr
}

func (m *mockIssueManager) CreateIssue(_ context.Context, _, _, _ string, _ []string) (int, error) {
	m.createIssueCalls++
	return m.createIssueNum, m.createIssueErr
}

func (m *mockIssueManager) ListIssues(_ context.Context, _, _ string) ([]gh.Issue, error) {
	m.listIssuesCalls++
	return m.listIssues, m.listIssuesErr
}

type mockNotifier struct {
	notifications []notify.Notification
}

func (m *mockNotifier) Send(n notify.Notification) error {
	m.notifications = append(m.notifications, n)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testConfig() *config.Config {
	return &config.Config{
		Repo: config.RepoConfig{
			URL:            "https://github.com/test/repo.git",
			Branch:         "main",
			WorkDir:        "/tmp/test-repo",
			PRBranchPrefix: "autobacklog",
		},
		GitHub: config.GitHubConfig{AutoMerge: false},
		Claude: config.ClaudeConfig{
			Model:            "sonnet",
			MaxBudgetPerCall: 5.0,
			MaxBudgetTotal:   100.0,
			Timeout:          10 * time.Minute,
		},
		Backlog: config.BacklogConfig{
			HighThreshold:   1,
			MediumThreshold: 3,
			LowThreshold:    5,
			MaxPerCycle:     5,
			StaleDays:       30,
		},
		Testing: config.TestingConfig{
			AutoDetect: false,
			MaxRetries: 2,
			Timeout:    5 * time.Minute,
		},
		Logging: config.LoggingConfig{Level: "info"},
	}
}

func newTestStore(t *testing.T) backlog.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func newTestApp(t *testing.T, opts ...func(*App)) *App {
	t.Helper()
	cfg := testConfig()
	store := newTestStore(t)
	repo := &mockRepo{workDir: "/tmp/test-repo", hasChangesVal: true}
	ai := newMockAIClient(100.0)
	tr := &mockTestRunner{}
	pr := &mockPRCreator{prURL: "https://github.com/test/repo/pull/1"}
	im := &mockIssueManager{createIssueNum: 1}
	notif := &mockNotifier{}
	log := slog.Default()

	a := NewWithDeps(cfg, repo, ai, tr, pr, im, store, notif, log, false)
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func reviewJSON(items ...[2]string) string {
	// items are [title, priority] pairs
	s := `[`
	for i, pair := range items {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf(`{"title":"%s","description":"desc","file_path":"f.go","priority":"%s","category":"bug"}`, pair[0], pair[1])
	}
	s += `]`
	return s
}

// ---------------------------------------------------------------------------
// RunCycle tests
// ---------------------------------------------------------------------------

func TestRunCycle_DryRun(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	repo := a.repo.(*mockRepo)
	ai := a.claude.(*mockAIClient)

	stats, err := a.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if stats.ItemsFound != 0 {
		t.Errorf("ItemsFound = %d, want 0", stats.ItemsFound)
	}
	if repo.cloneOrPullCalls != 0 {
		t.Errorf("cloneOrPull called %d times, want 0 in dry-run", repo.cloneOrPullCalls)
	}
	if ai.runCalls != 0 {
		t.Errorf("AI Run called %d times, want 0 in dry-run", ai.runCalls)
	}
}

func TestRunCycle_CloneError_Stops(t *testing.T) {
	a := newTestApp(t)
	repo := a.repo.(*mockRepo)
	repo.cloneOrPullErr = errors.New("clone failed")

	_, err := a.RunCycle(context.Background())
	if err == nil {
		t.Fatal("expected error from RunCycle")
	}
	ai := a.claude.(*mockAIClient)
	if ai.runCalls != 0 {
		t.Error("AI should not be called after clone error")
	}
}

func TestRunCycle_ReviewError_Stops(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	ai.runErrors = []error{errors.New("claude down")}

	_, err := a.RunCycle(context.Background())
	if err == nil {
		t.Fatal("expected error from RunCycle")
	}
}

func TestRunCycle_FullCycle(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	// Review output with one high-priority item
	ai.runOutputs = []string{reviewJSON([2]string{"Fix null pointer", "high"})}
	// RunPrint: implement, then document
	ai.runPrintOutputs = []string{"implemented", "documented"}

	repo := a.repo.(*mockRepo)
	repo.hasChangesVal = true

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	// Need testing enabled to exercise implementItem
	a.cfg.Testing.OverrideCommand = "go test ./..."

	stats, err := a.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if stats.ItemsFound != 1 {
		t.Errorf("ItemsFound = %d, want 1", stats.ItemsFound)
	}
	if stats.ItemsInserted != 1 {
		t.Errorf("ItemsInserted = %d, want 1", stats.ItemsInserted)
	}
	if stats.ItemsImplemented != 1 {
		t.Errorf("ItemsImplemented = %d, want 1", stats.ItemsImplemented)
	}
	if stats.PRsCreated != 1 {
		t.Errorf("PRsCreated = %d, want 1", stats.PRsCreated)
	}
}

func TestRunCycle_NoItemsSelectedAfterThreshold(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	// Review returns items below threshold
	ai.runOutputs = []string{reviewJSON([2]string{"Low priority thing", "low"})}

	// Low threshold is 5, so 1 low item won't trigger
	a.cfg.Backlog.LowThreshold = 5

	stats, err := a.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if stats.ItemsImplemented != 0 {
		t.Errorf("ItemsImplemented = %d, want 0", stats.ItemsImplemented)
	}
}

func TestRunCycle_CleanStaleCalled(t *testing.T) {
	a := newTestApp(t)
	a.dryRun = true // simplest path to completion
	_, err := a.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	// CleanStale runs after the loop completes — we just verify no panic
}

func TestRunCycle_Burndown_SkipsReviewAndIngest(t *testing.T) {
	a := newTestApp(t)
	a.cfg.HelperMode = "burndown"
	ai := a.claude.(*mockAIClient)

	// In burndown mode, Claude.Run (used by review) should never be called.
	// Only RunPrint would be called if there were items to implement.
	// With no items in the backlog, the cycle should complete with zero AI calls.

	stats, err := a.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	if ai.runCalls != 0 {
		t.Errorf("AI Run called %d times, want 0 (review should be skipped)", ai.runCalls)
	}
	if stats.ItemsFound != 0 {
		t.Errorf("ItemsFound = %d, want 0 (no review in burndown)", stats.ItemsFound)
	}
	if stats.ItemsInserted != 0 {
		t.Errorf("ItemsInserted = %d, want 0 (no ingest in burndown)", stats.ItemsInserted)
	}
}

func TestRunBurndown_LoopsUntilDrained(t *testing.T) {
	a := newTestApp(t)
	a.cfg.HelperMode = "burndown"
	store := a.store

	// Insert two pending items so the first cycle finds work.
	item1 := backlog.NewItem("fix alpha", "desc", "a.go", backlog.PriorityHigh, backlog.CategoryBug)
	item1.RepoURL = a.cfg.Repo.URL
	item2 := backlog.NewItem("fix beta", "desc", "b.go", backlog.PriorityHigh, backlog.CategoryBug)
	item2.RepoURL = a.cfg.Repo.URL
	if err := store.Insert(context.Background(), item1); err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(context.Background(), item2); err != nil {
		t.Fatal(err)
	}

	// Mock AI: RunPrint called once per item (implement) + optionally doc.
	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"ok", "ok", "ok", "ok", "ok", "ok"}

	// Mock test runner: always pass (one result per item).
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{
		{Passed: true, Output: "all pass"},
		{Passed: true, Output: "all pass"},
	}

	stats, err := a.RunBurndown(context.Background())
	if err != nil {
		t.Fatalf("RunBurndown: %v", err)
	}

	// Should have implemented exactly 2 items across cycle(s), then a final
	// cycle with 0 implementations causing the loop to exit.
	if stats.ItemsImplemented != 2 {
		t.Errorf("ItemsImplemented = %d, want 2", stats.ItemsImplemented)
	}
	if stats.PRsCreated != 2 {
		t.Errorf("PRsCreated = %d, want 2", stats.PRsCreated)
	}
}

func TestRunBurndown_SelectsBelowThreshold(t *testing.T) {
	a := newTestApp(t)
	a.cfg.HelperMode = "burndown"
	// Default medium threshold is 3, but we only have 1 medium item.
	// Burndown should still select it.
	store := a.store

	item := backlog.NewItem("refactor widget", "desc", "w.go", backlog.PriorityMedium, backlog.CategoryRefactor)
	item.RepoURL = a.cfg.Repo.URL
	if err := store.Insert(context.Background(), item); err != nil {
		t.Fatal(err)
	}

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"ok", "ok", "ok"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "pass"}}

	stats, err := a.RunBurndown(context.Background())
	if err != nil {
		t.Fatalf("RunBurndown: %v", err)
	}
	if stats.ItemsImplemented != 1 {
		t.Errorf("ItemsImplemented = %d, want 1 (burndown should bypass thresholds)", stats.ItemsImplemented)
	}
}

func TestRunBurndown_EmptyBacklog(t *testing.T) {
	a := newTestApp(t)
	a.cfg.HelperMode = "burndown"

	// No items in backlog — should exit after one cycle.
	stats, err := a.RunBurndown(context.Background())
	if err != nil {
		t.Fatalf("RunBurndown: %v", err)
	}
	if stats.ItemsImplemented != 0 {
		t.Errorf("ItemsImplemented = %d, want 0", stats.ItemsImplemented)
	}
}

func TestCycleStats_Merge(t *testing.T) {
	a := &CycleStats{ItemsFound: 3, ItemsImplemented: 2, PRsCreated: 2}
	b := &CycleStats{ItemsFound: 1, ItemsImplemented: 0, Errors: []error{errors.New("x")}}
	a.Merge(b)
	if a.ItemsFound != 4 {
		t.Errorf("ItemsFound = %d, want 4", a.ItemsFound)
	}
	if a.ItemsImplemented != 2 {
		t.Errorf("ItemsImplemented = %d, want 2", a.ItemsImplemented)
	}
	if len(a.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(a.Errors))
	}
}

func TestCycleStats_Merge_Nil(t *testing.T) {
	a := &CycleStats{ItemsFound: 3}
	a.Merge(nil)
	if a.ItemsFound != 3 {
		t.Errorf("ItemsFound = %d, want 3", a.ItemsFound)
	}
}

// ---------------------------------------------------------------------------
// doClone tests
// ---------------------------------------------------------------------------

func TestDoClone_DryRun(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	if err := a.doClone(context.Background()); err != nil {
		t.Fatalf("doClone: %v", err)
	}
	repo := a.repo.(*mockRepo)
	if repo.cloneOrPullCalls != 0 {
		t.Error("should not call CloneOrPull in dry-run")
	}
}

func TestDoClone_Error(t *testing.T) {
	a := newTestApp(t)
	a.repo.(*mockRepo).cloneOrPullErr = errors.New("network error")
	err := a.doClone(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// doReview tests
// ---------------------------------------------------------------------------

func TestDoReview_DryRun(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	stats := &CycleStats{}
	if err := a.doReview(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if a.reviewItems != nil {
		t.Error("reviewItems should be nil in dry-run")
	}
}

func TestDoReview_ParsesItems(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	ai.runOutputs = []string{reviewJSON([2]string{"Bug A", "high"}, [2]string{"Bug B", "medium"})}

	stats := &CycleStats{}
	if err := a.doReview(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if stats.ItemsFound != 2 {
		t.Errorf("ItemsFound = %d, want 2", stats.ItemsFound)
	}
	if len(a.reviewItems) != 2 {
		t.Errorf("reviewItems len = %d, want 2", len(a.reviewItems))
	}
}

func TestDoReview_ClaudeError(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	ai.runErrors = []error{errors.New("claude error")}

	stats := &CycleStats{}
	err := a.doReview(context.Background(), stats)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDoReview_ParseError(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	ai.runOutputs = []string{"not valid json at all"}

	stats := &CycleStats{}
	err := a.doReview(context.Background(), stats)
	if err == nil {
		t.Fatal("expected error from malformed output")
	}
}

// ---------------------------------------------------------------------------
// doIngest tests
// ---------------------------------------------------------------------------

func TestDoIngest_DryRun(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	stats := &CycleStats{}
	if err := a.doIngest(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
}

func TestDoIngest_NilItems(t *testing.T) {
	a := newTestApp(t)
	a.reviewItems = nil
	stats := &CycleStats{}
	if err := a.doIngest(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if stats.ItemsInserted != 0 {
		t.Errorf("ItemsInserted = %d, want 0", stats.ItemsInserted)
	}
}

func TestDoIngest_Success(t *testing.T) {
	a := newTestApp(t)
	a.reviewItems = []*backlog.Item{
		backlog.NewItem("Bug 1", "desc", "a.go", backlog.PriorityHigh, backlog.CategoryBug),
	}
	stats := &CycleStats{}
	if err := a.doIngest(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if stats.ItemsInserted != 1 {
		t.Errorf("ItemsInserted = %d, want 1", stats.ItemsInserted)
	}
	if a.reviewItems != nil {
		t.Error("reviewItems should be cleared after ingest")
	}
}

// ---------------------------------------------------------------------------
// doEvaluateThreshold tests
// ---------------------------------------------------------------------------

func TestDoEvaluateThreshold_NoItems(t *testing.T) {
	a := newTestApp(t)
	stats := &CycleStats{}
	if err := a.doEvaluateThreshold(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if a.selectedItems != nil {
		t.Error("selectedItems should be nil when no items in store")
	}
}

func TestDoEvaluateThreshold_SelectsItems(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	// Insert a high-priority item (threshold = 1)
	item := backlog.NewItem("Critical bug", "fix it", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	item.RepoURL = a.cfg.Repo.URL
	if err := a.store.Insert(ctx, item); err != nil {
		t.Fatal(err)
	}

	stats := &CycleStats{}
	if err := a.doEvaluateThreshold(ctx, stats); err != nil {
		t.Fatal(err)
	}
	if len(a.selectedItems) != 1 {
		t.Errorf("selectedItems len = %d, want 1", len(a.selectedItems))
	}
}

// ---------------------------------------------------------------------------
// doDocument tests
// ---------------------------------------------------------------------------

func TestDoDocument_DryRun(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	stats := &CycleStats{ItemsImplemented: 1}
	if err := a.doDocument(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
}

func TestDoDocument_NoItems(t *testing.T) {
	a := newTestApp(t)
	stats := &CycleStats{ItemsImplemented: 0}
	if err := a.doDocument(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	ai := a.claude.(*mockAIClient)
	if ai.runPrintCalls != 0 {
		t.Error("should not invoke Claude when no items implemented")
	}
}

func TestDoDocument_Success(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"docs updated"}

	doneItem := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	doneItem.Status = backlog.StatusDone
	a.selectedItems = []*backlog.Item{doneItem}

	stats := &CycleStats{ItemsImplemented: 1}
	if err := a.doDocument(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if ai.runPrintCalls != 1 {
		t.Errorf("runPrintCalls = %d, want 1", ai.runPrintCalls)
	}
}

// ---------------------------------------------------------------------------
// implementItem tests
// ---------------------------------------------------------------------------

func TestImplementItem_FullFlow(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	stats := &CycleStats{}
	if err := a.implementItem(ctx, item, stats); err != nil {
		t.Fatalf("implementItem: %v", err)
	}

	if item.Status != backlog.StatusDone {
		t.Errorf("status = %q, want done", item.Status)
	}
	if stats.ItemsImplemented != 1 {
		t.Errorf("ItemsImplemented = %d, want 1", stats.ItemsImplemented)
	}
	if stats.PRsCreated != 1 {
		t.Errorf("PRsCreated = %d, want 1", stats.PRsCreated)
	}
}

func TestImplementItem_BudgetExceeded(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	// Exhaust the budget
	a.claude.(*mockAIClient).budget.Record(99.0)

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}

	notif := a.notifier.(*mockNotifier)
	if len(notif.notifications) == 0 {
		t.Error("expected out-of-tokens notification")
	}
}

func TestImplementItem_ClaudeError(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintErrors = []error{errors.New("claude failed")}

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected error from claude failure")
	}
}

func TestImplementItem_NoChanges_MarksSkipped(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.hasChangesVal = false

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"no changes needed"}

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Status != backlog.StatusSkipped {
		t.Errorf("status = %q, want skipped", item.Status)
	}
	if stats.ItemsImplemented != 0 {
		t.Error("should not increment implemented for skipped item")
	}
}

func TestImplementItem_TestsFail_AllRetries_MarksFailed(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 1
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented", "fix attempt"}

	tr := a.runner.(*mockTestRunner)
	// Fail twice (initial + 1 retry)
	tr.results = []*runner.Result{
		{Passed: false, Output: "FAIL"},
		{Passed: false, Output: "FAIL again"},
	}

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected error from test failures")
	}
	if item.Status != backlog.StatusFailed {
		t.Errorf("status = %q, want failed", item.Status)
	}
	repo := a.repo.(*mockRepo)
	if repo.revertCalls != 1 {
		t.Errorf("revertCalls = %d, want 1", repo.revertCalls)
	}
}

func TestImplementItem_TestsFail_ThenPass(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 2
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	// implement + fix
	ai.runPrintOutputs = []string{"implemented", "fix attempt"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{
		{Passed: false, Output: "FAIL"},
		{Passed: true, Output: "ok"},
	}

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Status != backlog.StatusDone {
		t.Errorf("status = %q, want done", item.Status)
	}
}

func TestImplementItem_CommitError(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.commitErr = errors.New("commit failed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected commit error")
	}
}

func TestImplementItem_PushError(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.pushErr = errors.New("push rejected")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected push error")
	}
}

func TestImplementItem_CreatePRError(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	pr := a.prCreator.(*mockPRCreator)
	pr.createPRErr = errors.New("PR creation failed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected PR creation error")
	}
}

func TestImplementItem_AutoMerge_Success(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.GitHub.AutoMerge = true
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	if err := a.implementItem(ctx, item, stats); err != nil {
		t.Fatal(err)
	}
	if stats.PRsAutoMerged != 1 {
		t.Errorf("PRsAutoMerged = %d, want 1", stats.PRsAutoMerged)
	}
	pr := a.prCreator.(*mockPRCreator)
	if pr.autoMergeCalls != 1 {
		t.Errorf("autoMergeCalls = %d, want 1", pr.autoMergeCalls)
	}
}

func TestImplementItem_AutoMerge_FailureNonFatal(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.GitHub.AutoMerge = true
	ctx := context.Background()

	pr := a.prCreator.(*mockPRCreator)
	pr.prURL = "https://github.com/test/repo/pull/1"
	pr.autoMergeErr = errors.New("auto-merge not allowed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err != nil {
		t.Fatalf("auto-merge failure should be non-fatal, got: %v", err)
	}
	if stats.PRsAutoMerged != 0 {
		t.Errorf("PRsAutoMerged = %d, want 0", stats.PRsAutoMerged)
	}
}

func TestImplementItem_AutoMerge_Disabled(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.GitHub.AutoMerge = false
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	a.implementItem(ctx, item, stats)

	pr := a.prCreator.(*mockPRCreator)
	if pr.autoMergeCalls != 0 {
		t.Error("auto-merge should not be called when disabled")
	}
}

// ---------------------------------------------------------------------------
// runTestsWithRetry tests
// ---------------------------------------------------------------------------

func TestRunTestsWithRetry_OverrideCommand(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "npm test"

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	out, err := a.runTestsWithRetry(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want 'ok'", out)
	}
	if tr.calls != 1 {
		t.Errorf("runner calls = %d, want 1", tr.calls)
	}
}

func TestRunTestsWithRetry_AutoDetect(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.AutoDetect = true
	a.cfg.Testing.OverrideCommand = ""
	// WorkDir has no detectable framework
	repo := a.repo.(*mockRepo)
	repo.workDir = t.TempDir()

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	out, err := a.runTestsWithRetry(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if out != "no test framework detected" {
		t.Errorf("output = %q, want 'no test framework detected'", out)
	}
}

func TestRunTestsWithRetry_NoFrameworkDetected(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.AutoDetect = true
	a.cfg.Testing.OverrideCommand = ""
	a.repo.(*mockRepo).workDir = t.TempDir()

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	out, err := a.runTestsWithRetry(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if out != "no test framework detected" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestRunTestsWithRetry_TestingDisabled(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.AutoDetect = false
	a.cfg.Testing.OverrideCommand = ""

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	out, err := a.runTestsWithRetry(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if out != "tests disabled" {
		t.Errorf("output = %q, want 'tests disabled'", out)
	}
}

func TestRunTestsWithRetry_PassesFirst(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "all pass"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	out, err := a.runTestsWithRetry(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if out != "all pass" {
		t.Errorf("output = %q", out)
	}
}

func TestRunTestsWithRetry_PassesAfterRetry(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 2

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"fix1"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{
		{Passed: false, Output: "FAIL"},
		{Passed: true, Output: "ok"},
	}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	out, err := a.runTestsWithRetry(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("output = %q", out)
	}
}

func TestRunTestsWithRetry_ExhaustsRetries(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 1

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"fix1"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{
		{Passed: false, Output: "FAIL1"},
		{Passed: false, Output: "FAIL2"},
	}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	_, err := a.runTestsWithRetry(context.Background(), item)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestRunTestsWithRetry_RunnerError(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."

	tr := a.runner.(*mockTestRunner)
	tr.errors = []error{errors.New("runner crashed")}
	tr.results = []*runner.Result{nil}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	_, err := a.runTestsWithRetry(context.Background(), item)
	if err == nil {
		t.Fatal("expected runner error")
	}
}

// ---------------------------------------------------------------------------
// Negative regression tests
// ---------------------------------------------------------------------------

func TestRunCycle_NeverExecutesStatesAfterError(t *testing.T) {
	a := newTestApp(t)
	a.repo.(*mockRepo).cloneOrPullErr = errors.New("clone failed")

	a.RunCycle(context.Background())

	ai := a.claude.(*mockAIClient)
	if ai.runCalls != 0 {
		t.Errorf("AI Run should not be called after clone failure, got %d calls", ai.runCalls)
	}
	if ai.runPrintCalls != 0 {
		t.Errorf("AI RunPrint should not be called after clone failure, got %d calls", ai.runPrintCalls)
	}
}

func TestRunCycle_DryRun_NeverCallsExternalServices(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })

	a.RunCycle(context.Background())

	repo := a.repo.(*mockRepo)
	ai := a.claude.(*mockAIClient)
	tr := a.runner.(*mockTestRunner)
	pr := a.prCreator.(*mockPRCreator)

	if repo.cloneOrPullCalls != 0 {
		t.Error("clone should not be called in dry-run")
	}
	if ai.runCalls != 0 {
		t.Error("AI run should not be called in dry-run")
	}
	if ai.runPrintCalls != 0 {
		t.Error("AI runPrint should not be called in dry-run")
	}
	if tr.calls != 0 {
		t.Error("test runner should not be called in dry-run")
	}
	if pr.createPRCalls != 0 {
		t.Error("PR creator should not be called in dry-run")
	}
}

func TestImplementItem_NeverSetsDone_OnFailure(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 0
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: false, Output: "FAIL"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	a.implementItem(ctx, item, stats)

	if item.Status == backlog.StatusDone {
		t.Error("item should NOT be marked done after test failure")
	}
}

func TestImplementItem_NeverCreatesPR_WithoutPassingTests(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 0
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: false, Output: "FAIL"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	a.implementItem(ctx, item, stats)

	pr := a.prCreator.(*mockPRCreator)
	if pr.createPRCalls != 0 {
		t.Error("PR should NOT be created when tests fail")
	}
}

func TestImplementItem_NeverPushes_WithoutCommit(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.stageAllErr = errors.New("stage failed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	a.implementItem(ctx, item, stats)

	if repo.pushCalls != 0 {
		t.Error("push should NOT be called when staging fails")
	}
}

func TestImplementItem_RevertsToClean_OnTestFailure(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 0
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: false, Output: "FAIL"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	a.implementItem(ctx, item, stats)

	repo := a.repo.(*mockRepo)
	if repo.revertCalls != 1 {
		t.Errorf("revertCalls = %d, want 1", repo.revertCalls)
	}
}

func TestRunTestsWithRetry_NeverExceedsMaxRetries(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 2

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"fix1", "fix2"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{
		{Passed: false, Output: "FAIL1"},
		{Passed: false, Output: "FAIL2"},
		{Passed: false, Output: "FAIL3"},
	}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.runTestsWithRetry(context.Background(), item)

	// maxRetries=2 means 3 total runs (1 initial + 2 retries)
	if tr.calls != 3 {
		t.Errorf("runner calls = %d, want 3 (initial + 2 retries)", tr.calls)
	}
	// Claude fix is only called on retries (not the last failed attempt)
	if ai.runPrintCalls != 2 {
		t.Errorf("fix calls = %d, want 2", ai.runPrintCalls)
	}
}

// ---------------------------------------------------------------------------
// State tests (extending existing)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// implementItem — additional error path tests
// ---------------------------------------------------------------------------

func TestImplementItem_CreateBranchError(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.createBranchErr = errors.New("branch already exists")

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected branch creation error")
	}
	// Claude should not be invoked if branch creation fails
	ai := a.claude.(*mockAIClient)
	if ai.runPrintCalls != 0 {
		t.Error("Claude should not be called when branch creation fails")
	}
}

func TestImplementItem_HasChangesError(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.hasChangesErr = errors.New("git status failed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected HasChanges error")
	}
	// Tests should not be run
	tr := a.runner.(*mockTestRunner)
	if tr.calls != 0 {
		t.Error("tests should not run when HasChanges errors")
	}
}

func TestImplementItem_CheckoutAfterPR_Error(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.checkoutErr = errors.New("checkout failed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	// The final checkout error IS returned as an error
	if err == nil {
		t.Fatal("expected checkout error after PR")
	}
	// PR should still have been created
	if stats.PRsCreated != 1 {
		t.Errorf("PRsCreated = %d, want 1 (PR was created before checkout failed)", stats.PRsCreated)
	}
}

func TestImplementItem_UpdateToDone_Error(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	// Close the store after insert to cause Update to fail
	a.store.Close()

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	// The first store.Update (to in_progress) will fail
	if err == nil {
		t.Fatal("expected store error")
	}
}

// ---------------------------------------------------------------------------
// runTestsWithRetry — Claude fix error during retry
// ---------------------------------------------------------------------------

func TestRunTestsWithRetry_ClaudeFixError(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 2

	ai := a.claude.(*mockAIClient)
	// Claude fix attempt fails
	ai.runPrintErrors = []error{errors.New("claude fix failed")}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: false, Output: "FAIL"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	_, err := a.runTestsWithRetry(context.Background(), item)
	if err == nil {
		t.Fatal("expected error from Claude fix failure")
	}
	// Only 1 test run (before the fix attempt)
	if tr.calls != 1 {
		t.Errorf("runner calls = %d, want 1", tr.calls)
	}
}

// ---------------------------------------------------------------------------
// doDocument — additional path tests
// ---------------------------------------------------------------------------

func TestDoDocument_ClaudeError_NonFatal(t *testing.T) {
	a := newTestApp(t)
	ai := a.claude.(*mockAIClient)
	ai.runPrintErrors = []error{errors.New("doc update failed")}

	doneItem := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	doneItem.Status = backlog.StatusDone
	a.selectedItems = []*backlog.Item{doneItem}

	stats := &CycleStats{ItemsImplemented: 1}
	err := a.doDocument(context.Background(), stats)
	if err != nil {
		t.Fatalf("doDocument should not return error (non-fatal), got: %v", err)
	}
}

func TestDoDocument_NoDoneItems(t *testing.T) {
	a := newTestApp(t)

	// selectedItems exist but none are done
	failedItem := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	failedItem.Status = backlog.StatusFailed
	a.selectedItems = []*backlog.Item{failedItem}

	stats := &CycleStats{ItemsImplemented: 1}
	err := a.doDocument(context.Background(), stats)
	if err != nil {
		t.Fatal(err)
	}
	ai := a.claude.(*mockAIClient)
	if ai.runPrintCalls != 0 {
		t.Error("should not invoke Claude when no items are done")
	}
}

// ---------------------------------------------------------------------------
// doImplement — dry run with selected items
// ---------------------------------------------------------------------------

func TestDoImplement_DryRun_WithSelectedItems(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	ctx := context.Background()

	a.selectedItems = []*backlog.Item{
		backlog.NewItem("Fix A", "desc", "a.go", backlog.PriorityHigh, backlog.CategoryBug),
		backlog.NewItem("Fix B", "desc", "b.go", backlog.PriorityMedium, backlog.CategoryRefactor),
	}

	stats := &CycleStats{}
	err := a.doImplement(ctx, stats)
	if err != nil {
		t.Fatal(err)
	}

	ai := a.claude.(*mockAIClient)
	if ai.runPrintCalls != 0 {
		t.Error("Claude should not be called in dry-run")
	}
	repo := a.repo.(*mockRepo)
	if repo.createBranchCalls != 0 {
		t.Error("branches should not be created in dry-run")
	}
}

// ---------------------------------------------------------------------------
// Multi-item mixed success/failure
// ---------------------------------------------------------------------------

func TestDoImplement_MultipleItems_MixedResults(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 0
	ctx := context.Background()

	item1 := backlog.NewItem("Fix A", "desc", "a.go", backlog.PriorityHigh, backlog.CategoryBug)
	item2 := backlog.NewItem("Fix B", "desc", "b.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item1)
	a.store.Insert(ctx, item2)
	a.selectedItems = []*backlog.Item{item1, item2}

	ai := a.claude.(*mockAIClient)
	// First item: implement succeeds; Second item: implement succeeds
	ai.runPrintOutputs = []string{"implemented1", "implemented2"}

	tr := a.runner.(*mockTestRunner)
	// First item: tests fail; Second item: tests pass
	tr.results = []*runner.Result{
		{Passed: false, Output: "FAIL"},
		{Passed: true, Output: "ok"},
	}

	stats := &CycleStats{}
	err := a.doImplement(ctx, stats)
	// doImplement itself should not error (errors go to stats.Errors)
	if err != nil {
		t.Fatalf("doImplement should not return error: %v", err)
	}
	if len(stats.Errors) != 1 {
		t.Errorf("stats.Errors len = %d, want 1 (one item failed)", len(stats.Errors))
	}
	if stats.ItemsImplemented != 1 {
		t.Errorf("ItemsImplemented = %d, want 1", stats.ItemsImplemented)
	}
}

// ---------------------------------------------------------------------------
// RunCycle — notification verification
// ---------------------------------------------------------------------------

func TestRunCycle_ErrorNotificationSent(t *testing.T) {
	a := newTestApp(t)
	a.repo.(*mockRepo).cloneOrPullErr = errors.New("clone failed")

	a.RunCycle(context.Background())

	notif := a.notifier.(*mockNotifier)
	found := false
	for _, n := range notif.notifications {
		if n.Event == notify.EventError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error notification on state failure")
	}
}

func TestRunCycle_CycleCompleteNotificationSent(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })

	a.RunCycle(context.Background())

	notif := a.notifier.(*mockNotifier)
	found := false
	for _, n := range notif.notifications {
		if n.Event == notify.EventCycleComplete {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cycle_complete notification after successful cycle")
	}
}

func TestRunCycle_StuckNotificationOnTestFailure(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 0

	ai := a.claude.(*mockAIClient)
	ai.runOutputs = []string{reviewJSON([2]string{"Fix bug", "high"})}
	ai.runPrintOutputs = []string{"implemented"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: false, Output: "FAIL"}}

	stats, err := a.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle should not error (item failure is non-fatal): %v", err)
	}

	notif := a.notifier.(*mockNotifier)
	found := false
	for _, n := range notif.notifications {
		if n.Event == notify.EventStuck {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected stuck notification when item tests fail")
	}
	_ = stats
}

// ---------------------------------------------------------------------------
// Strengthened negative regression tests
// ---------------------------------------------------------------------------

func TestImplementItem_FailureStatus_IsFailed(t *testing.T) {
	// Stronger version: positively assert StatusFailed, not just != Done
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	a.cfg.Testing.MaxRetries = 0
	ctx := context.Background()

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}

	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: false, Output: "FAIL"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	a.implementItem(ctx, item, stats)

	if item.Status != backlog.StatusFailed {
		t.Errorf("status = %q, want 'failed'", item.Status)
	}
	if item.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", item.Attempts)
	}
}

func TestRunCycle_NeverExecutesStatesAfterError_VerifiesNotification(t *testing.T) {
	a := newTestApp(t)
	a.repo.(*mockRepo).cloneOrPullErr = errors.New("clone failed")

	stats, err := a.RunCycle(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify stats recorded the error
	if len(stats.Errors) != 1 {
		t.Errorf("stats.Errors len = %d, want 1", len(stats.Errors))
	}

	// Verify error notification was sent
	notif := a.notifier.(*mockNotifier)
	if len(notif.notifications) == 0 {
		t.Error("expected error notification")
	}

	// Verify no downstream calls
	ai := a.claude.(*mockAIClient)
	if ai.runCalls+ai.runPrintCalls != 0 {
		t.Error("no AI calls should happen after clone failure")
	}
	tr := a.runner.(*mockTestRunner)
	if tr.calls != 0 {
		t.Error("no test runner calls should happen after clone failure")
	}
}

func TestImplementItem_NeverPushes_WithoutCommit_VerifiesError(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	repo := a.repo.(*mockRepo)
	repo.stageAllErr = errors.New("stage failed")

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	item := backlog.NewItem("Fix", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	a.store.Insert(ctx, item)

	stats := &CycleStats{}
	err := a.implementItem(ctx, item, stats)
	if err == nil {
		t.Fatal("expected staging error")
	}

	if repo.pushCalls != 0 {
		t.Error("push should NOT be called when staging fails")
	}
	if repo.commitCalls != 0 {
		t.Error("commit should NOT be called when staging fails")
	}
	pr := a.prCreator.(*mockPRCreator)
	if pr.createPRCalls != 0 {
		t.Error("PR should NOT be created when staging fails")
	}
}

// ---------------------------------------------------------------------------
// State tests (extending existing)
// ---------------------------------------------------------------------------

func TestState_Description_AllStates(t *testing.T) {
	states := []State{
		StateClone, StateImportIssues, StateReview, StateIngest, StateEvaluateThreshold,
		StateImplement, StateTest, StatePR, StateDocument, StateDone,
	}
	for _, s := range states {
		desc := s.Description()
		if desc == "" {
			t.Errorf("State(%d).Description() is empty", s)
		}
	}
}

func TestState_Description_Unknown(t *testing.T) {
	s := State(99)
	if s.Description() != "unknown" {
		t.Errorf("unknown state description = %q, want 'unknown'", s.Description())
	}
}

// ---------------------------------------------------------------------------
// doImportIssues tests
// ---------------------------------------------------------------------------

func TestDoImportIssues_DryRun(t *testing.T) {
	a := newTestApp(t, func(a *App) { a.dryRun = true })
	stats := &CycleStats{}
	if err := a.doImportIssues(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	im := a.issueManager.(*mockIssueManager)
	if im.listIssuesCalls != 0 {
		t.Error("should not list issues in dry-run")
	}
}

func TestDoImportIssues_NoLabel(t *testing.T) {
	a := newTestApp(t)
	a.cfg.GitHub.IssueLabel = ""
	stats := &CycleStats{}
	if err := a.doImportIssues(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	im := a.issueManager.(*mockIssueManager)
	if im.listIssuesCalls != 0 {
		t.Error("should not list issues when label is empty")
	}
}

func TestDoImportIssues_ListError_NonFatal(t *testing.T) {
	a := newTestApp(t)
	a.cfg.GitHub.IssueLabel = "autobacklog"
	im := a.issueManager.(*mockIssueManager)
	im.listIssuesErr = errors.New("gh failed")

	stats := &CycleStats{}
	err := a.doImportIssues(context.Background(), stats)
	if err != nil {
		t.Fatalf("list error should be non-fatal, got: %v", err)
	}
}

func TestDoImportIssues_ImportsNewIssues(t *testing.T) {
	a := newTestApp(t)
	a.cfg.GitHub.IssueLabel = "autobacklog"
	im := a.issueManager.(*mockIssueManager)
	im.listIssues = []gh.Issue{
		{Number: 10, Title: "Issue from GH", Body: "body text", State: "open"},
	}

	stats := &CycleStats{}
	if err := a.doImportIssues(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if stats.IssuesImported != 1 {
		t.Errorf("IssuesImported = %d, want 1", stats.IssuesImported)
	}

	// Verify item was inserted
	issueNum := 10
	items, _ := a.store.List(context.Background(), backlog.ListFilter{IssueNumber: &issueNum})
	if len(items) != 1 {
		t.Fatalf("expected 1 item with issue_number=10, got %d", len(items))
	}
	if items[0].Title != "Issue from GH" {
		t.Errorf("Title = %q, want 'Issue from GH'", items[0].Title)
	}
}

func TestDoImportIssues_SkipsAlreadyImported(t *testing.T) {
	a := newTestApp(t)
	a.cfg.GitHub.IssueLabel = "autobacklog"
	ctx := context.Background()

	// Pre-insert an item with issue_number=10
	existing := backlog.NewItem("Already here", "desc", "", backlog.PriorityMedium, backlog.CategoryRefactor)
	existing.RepoURL = a.cfg.Repo.URL
	existing.IssueNumber = 10
	a.store.Insert(ctx, existing)

	im := a.issueManager.(*mockIssueManager)
	im.listIssues = []gh.Issue{
		{Number: 10, Title: "Issue from GH", Body: "body", State: "open"},
	}

	stats := &CycleStats{}
	if err := a.doImportIssues(ctx, stats); err != nil {
		t.Fatal(err)
	}
	if stats.IssuesImported != 0 {
		t.Errorf("IssuesImported = %d, want 0 (already exists)", stats.IssuesImported)
	}
}

// ---------------------------------------------------------------------------
// Outbound issue creation tests
// ---------------------------------------------------------------------------

func TestDoIngest_CreatesIssues_WhenConfigured(t *testing.T) {
	a := newTestApp(t)
	a.cfg.GitHub.CreateIssues = true
	a.cfg.GitHub.IssueLabel = "autobacklog"

	im := a.issueManager.(*mockIssueManager)
	im.createIssueNum = 55

	a.reviewItems = []*backlog.Item{
		backlog.NewItem("New bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug),
	}

	stats := &CycleStats{}
	if err := a.doIngest(context.Background(), stats); err != nil {
		t.Fatal(err)
	}
	if stats.IssuesCreated != 1 {
		t.Errorf("IssuesCreated = %d, want 1", stats.IssuesCreated)
	}
	if im.createIssueCalls != 1 {
		t.Errorf("createIssueCalls = %d, want 1", im.createIssueCalls)
	}
}

func TestDoIngest_SkipsIssueCreation_WhenDisabled(t *testing.T) {
	a := newTestApp(t)
	a.cfg.GitHub.CreateIssues = false

	a.reviewItems = []*backlog.Item{
		backlog.NewItem("New bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug),
	}

	stats := &CycleStats{}
	if err := a.doIngest(context.Background(), stats); err != nil {
		t.Fatal(err)
	}

	im := a.issueManager.(*mockIssueManager)
	if im.createIssueCalls != 0 {
		t.Error("should not create issues when CreateIssues is false")
	}
}

// ---------------------------------------------------------------------------
// implementItem — Fixes #N in PR body
// ---------------------------------------------------------------------------

func TestImplementItem_IncludesFixesInPRBody(t *testing.T) {
	a := newTestApp(t)
	a.cfg.Testing.OverrideCommand = "go test ./..."
	ctx := context.Background()

	item := backlog.NewItem("Fix bug", "desc", "f.go", backlog.PriorityHigh, backlog.CategoryBug)
	item.IssueNumber = 42
	a.store.Insert(ctx, item)

	ai := a.claude.(*mockAIClient)
	ai.runPrintOutputs = []string{"implemented"}
	tr := a.runner.(*mockTestRunner)
	tr.results = []*runner.Result{{Passed: true, Output: "ok"}}

	pr := a.prCreator.(*mockPRCreator)
	pr.prURL = "https://github.com/test/repo/pull/1"

	// Capture the PR body by wrapping the mock
	var capturedBody string
	origPR := a.prCreator
	a.prCreator = &prBodyCapture{inner: origPR, body: &capturedBody}

	stats := &CycleStats{}
	if err := a.implementItem(ctx, item, stats); err != nil {
		t.Fatalf("implementItem: %v", err)
	}

	if capturedBody == "" {
		t.Fatal("PR body was not captured")
	}
	if !strings.Contains(capturedBody, "Fixes #42") {
		t.Errorf("PR body should contain 'Fixes #42', got:\n%s", capturedBody)
	}
}

// prBodyCapture wraps a PRCreator to capture the PR body
type prBodyCapture struct {
	inner PRCreator
	body  *string
}

func (p *prBodyCapture) CreatePR(ctx context.Context, workDir string, req gh.PRRequest) (string, error) {
	*p.body = req.Body
	return p.inner.CreatePR(ctx, workDir, req)
}

func (p *prBodyCapture) EnableAutoMerge(ctx context.Context, workDir string, prURL string) error {
	return p.inner.EnableAutoMerge(ctx, workDir, prURL)
}
