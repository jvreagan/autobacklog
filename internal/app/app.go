package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/claude"
	"github.com/jamesreagan/autobacklog/internal/config"
	"github.com/jamesreagan/autobacklog/internal/git"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/notify"
	"github.com/jamesreagan/autobacklog/internal/runner"
)

// App is the main orchestrator that drives the state machine.
type App struct {
	cfg           *config.Config
	repo          Repository
	claude        AIClient
	store         backlog.Store
	manager       *backlog.Manager
	runner        TestRunner
	prCreator     PRCreator
	issueManager  IssueManager
	notifier      notify.Notifier
	log           *slog.Logger
	dryRun        bool
	reviewItems      []*backlog.Item      // transient: review → ingest
	selectedItems    []*backlog.Item      // transient: threshold → implement
	cachedDetect     *runner.DetectResult // cached test framework per cycle
	burndownTotal    int                  // total pending items at burndown start
	burndownDone     int                  // items addressed so far in burndown
}

// defaultPRCreator wraps the free functions in the github package.
type defaultPRCreator struct {
	log *slog.Logger
}

func (d *defaultPRCreator) CreatePR(ctx context.Context, workDir string, req gh.PRRequest) (string, error) {
	return gh.CreatePR(ctx, workDir, req, d.log)
}

func (d *defaultPRCreator) EnableAutoMerge(ctx context.Context, workDir string, prURL string) error {
	return gh.EnableAutoMerge(ctx, workDir, prURL, d.log)
}

// defaultIssueManager wraps the free functions in the github package.
type defaultIssueManager struct {
	log *slog.Logger
}

func (d *defaultIssueManager) EnsureLabel(ctx context.Context, workDir, label string) error {
	return gh.EnsureLabel(ctx, workDir, label, d.log)
}

func (d *defaultIssueManager) CreateIssue(ctx context.Context, workDir, title, body string, labels []string) (int, error) {
	return gh.CreateIssue(ctx, workDir, title, body, labels, d.log)
}

func (d *defaultIssueManager) ListIssues(ctx context.Context, workDir, label string) ([]gh.Issue, error) {
	return gh.ListIssues(ctx, workDir, label, d.log)
}

// New creates a new App orchestrator with production dependencies.
func New(cfg *config.Config, store backlog.Store, notifier notify.Notifier, log *slog.Logger, dryRun bool) (*App, error) {
	pat, err := cfg.ResolveGitHubPAT()
	if err != nil && !dryRun {
		log.Warn("no GitHub PAT configured", "error", err)
	}

	repo := git.NewRepo(cfg.Repo.URL, cfg.Repo.Branch, cfg.Repo.WorkDir, pat, log)
	claudeClient := claude.NewClient(cfg.Claude, log)
	testRunner := runner.NewRunner(log, cfg.Testing.Timeout)

	return NewWithDeps(cfg, repo, claudeClient, testRunner, &defaultPRCreator{log: log}, &defaultIssueManager{log: log}, store, notifier, log, dryRun), nil
}

// NewWithDeps creates an App with explicitly provided dependencies (for testing).
func NewWithDeps(
	cfg *config.Config,
	repo Repository,
	aiClient AIClient,
	testRunner TestRunner,
	prCreator PRCreator,
	issueManager IssueManager,
	store backlog.Store,
	notifier notify.Notifier,
	log *slog.Logger,
	dryRun bool,
) *App {
	mgr := backlog.NewManager(store, log)
	return &App{
		cfg:          cfg,
		repo:         repo,
		claude:       aiClient,
		store:        store,
		manager:      mgr,
		runner:       testRunner,
		prCreator:    prCreator,
		issueManager: issueManager,
		notifier:     notifier,
		log:          log,
		dryRun:       dryRun,
	}
}

// RunCycle executes one full cycle of the state machine.
func (a *App) RunCycle(ctx context.Context) (*CycleStats, error) {
	stats := &CycleStats{}
	state := StateClone

	a.log.Info("starting cycle", "dry_run", a.dryRun, "helper_mode", a.cfg.HelperMode, "repo", a.cfg.Repo.URL)

	for state != StateDone {
		a.log.Info("entering state", "state", state.String(), "action", state.Description())

		var err error
		switch state {
		case StateClone:
			err = a.doClone(ctx)
		case StateImportIssues:
			err = a.doImportIssues(ctx, stats)
		case StateReview:
			if a.cfg.HelperMode == "burndown" {
				a.log.Info("[burndown] skipping review — implementing existing backlog items")
				state = state.Next()
				continue
			}
			err = a.doReview(ctx, stats)
		case StateIngest:
			if a.cfg.HelperMode == "burndown" {
				a.log.Info("[burndown] skipping ingest — implementing existing backlog items")
				state = state.Next()
				continue
			}
			err = a.doIngest(ctx, stats)
		case StateEvaluateThreshold:
			err = a.doEvaluateThreshold(ctx, stats)
		case StateImplement:
			err = a.doImplement(ctx, stats)
		case StateTest:
			a.log.Info("skipping state (tests run during implementation)", "state", state.String())
			state = state.Next()
			continue
		case StatePR:
			a.log.Info("skipping state (PRs created during implementation)", "state", state.String())
			state = state.Next()
			continue
		case StateDocument:
			err = a.doDocument(ctx, stats)
		}

		if err != nil {
			stats.Errors = append(stats.Errors, err)
			a.log.Error("state failed", "state", state.String(), "error", err)
			if nErr := a.notifier.Send(notify.ErrorNotification(state.String(), err)); nErr != nil {
				a.log.Warn("failed to send error notification", "error", nErr)
			}
			return stats, err
		}

		a.log.Info("completed state", "state", state.String())
		state = state.Next()
	}

	// Clean stale items
	a.log.Info("cleaning stale backlog items", "stale_days", a.cfg.Backlog.StaleDays)
	if _, err := a.manager.CleanStale(ctx, a.cfg.Repo.URL, a.cfg.Backlog.StaleDays); err != nil {
		a.log.Warn("clean stale failed", "error", err)
	}

	// Send cycle summary
	a.log.Info("cycle complete",
		"items_found", stats.ItemsFound,
		"items_inserted", stats.ItemsInserted,
		"items_implemented", stats.ItemsImplemented,
		"prs_created", stats.PRsCreated,
		"prs_auto_merged", stats.PRsAutoMerged,
		"errors", len(stats.Errors),
	)
	stats.BudgetSummary = a.claude.Budget().String()

	if nErr := a.notifier.Send(notify.CycleCompleteNotification(
		stats.ItemsFound, stats.ItemsImplemented, stats.PRsCreated,
		stats.BudgetSummary,
	)); nErr != nil {
		a.log.Warn("failed to send cycle notification", "error", nErr)
	}

	return stats, nil
}

// RunBurndown loops RunCycle until the backlog is drained (no items implemented
// in a cycle). Returns cumulative stats across all cycles.
func (a *App) RunBurndown(ctx context.Context) (*CycleStats, error) {
	// Count total pending items for progress logging.
	pendingStatus := backlog.StatusPending
	pending, err := a.store.List(ctx, backlog.ListFilter{Status: &pendingStatus, RepoURL: &a.cfg.Repo.URL})
	if err != nil {
		return nil, fmt.Errorf("listing pending items: %w", err)
	}
	a.burndownTotal = len(pending)
	a.burndownDone = 0
	a.log.Info("[burndown] starting", "pending_items", a.burndownTotal)

	var cumulative CycleStats
	for cycle := 1; ; cycle++ {
		a.log.Info("[burndown] starting cycle", "cycle", cycle, "remaining", a.burndownTotal-a.burndownDone)
		stats, err := a.RunCycle(ctx)
		if err != nil {
			cumulative.Merge(stats)
			return &cumulative, err
		}
		cumulative.Merge(stats)
		if stats.ItemsImplemented == 0 {
			a.log.Info("[burndown] backlog drained", "total_cycles", cycle, "items_addressed", a.burndownDone, "total", a.burndownTotal)
			break
		}
	}
	a.burndownTotal = 0
	a.burndownDone = 0
	return &cumulative, nil
}

func (a *App) doClone(ctx context.Context) error {
	if a.dryRun {
		a.log.Info("[dry-run] would clone/pull repo", "url", a.cfg.Repo.URL, "branch", a.cfg.Repo.Branch, "work_dir", a.cfg.Repo.WorkDir)
		return nil
	}
	a.log.Info("cloning or pulling repository", "url", a.cfg.Repo.URL, "branch", a.cfg.Repo.Branch, "work_dir", a.cfg.Repo.WorkDir)
	return a.repo.CloneOrPull(ctx)
}

func (a *App) doImportIssues(ctx context.Context, stats *CycleStats) error {
	label := a.cfg.GitHub.IssueLabel
	if label == "" {
		a.log.Info("no issue label configured, skipping import")
		return nil
	}
	if a.dryRun {
		a.log.Info("[dry-run] would import GitHub issues", "label", label)
		return nil
	}

	a.log.Info("importing labeled GitHub issues", "label", label)
	issues, err := a.issueManager.ListIssues(ctx, a.repo.WorkDir(), label)
	if err != nil {
		a.log.Warn("failed to list GitHub issues, continuing", "error", err)
		return nil
	}

	// Pre-fetch existing issue numbers to avoid N+1 queries.
	existingItems, err := a.store.List(ctx, backlog.ListFilter{RepoURL: &a.cfg.Repo.URL})
	if err != nil {
		a.log.Warn("failed to list existing items, continuing", "error", err)
		return nil
	}
	importedNums := make(map[int]bool, len(existingItems))
	for _, item := range existingItems {
		if item.IssueNumber > 0 {
			importedNums[item.IssueNumber] = true
		}
	}

	var importFailures int
	for _, issue := range issues {
		if importedNums[issue.Number] {
			a.log.Info("issue already imported, skipping", "issue_number", issue.Number, "title", issue.Title)
			continue
		}

		priority, category := inferFromLabels(issue.LabelNames())
		item := backlog.NewItem(issue.Title, issue.Body, "", priority, category)
		item.RepoURL = a.cfg.Repo.URL
		item.IssueNumber = issue.Number

		if err := a.store.Insert(ctx, item); err != nil {
			a.log.Warn("failed to import issue", "issue_number", issue.Number, "error", err)
			importFailures++
			continue
		}

		importedNums[issue.Number] = true
		stats.IssuesImported++
		a.log.Info("imported issue", "issue_number", issue.Number, "title", issue.Title)
	}

	if importFailures > 0 {
		a.log.Warn("some issue imports failed", "failures", importFailures, "imported", stats.IssuesImported)
	}

	return nil
}

// inferFromLabels derives priority and category from GitHub issue labels.
// Falls back to PriorityMedium and CategoryRefactor when no matching labels are found.
func inferFromLabels(labels []string) (backlog.Priority, backlog.Category) {
	priority := backlog.PriorityMedium
	category := backlog.CategoryRefactor

	for _, l := range labels {
		l = strings.ToLower(l)
		switch {
		// Priority labels (e.g., "priority:high", "P1", "critical")
		case strings.Contains(l, "critical") || l == "p0" || l == "p1" || strings.HasSuffix(l, ":high") || l == "high":
			priority = backlog.PriorityHigh
		case l == "p3" || l == "p4" || strings.HasSuffix(l, ":low") || l == "low":
			priority = backlog.PriorityLow

		// Category labels
		case l == "bug" || strings.Contains(l, "bugfix"):
			category = backlog.CategoryBug
		case l == "security":
			category = backlog.CategorySecurity
		case l == "performance" || l == "perf":
			category = backlog.CategoryPerformance
		case l == "test" || l == "testing" || l == "tests":
			category = backlog.CategoryTest
		case l == "documentation" || l == "docs":
			category = backlog.CategoryDocs
		case l == "style" || l == "linting":
			category = backlog.CategoryStyle
		}
	}

	return priority, category
}

func (a *App) doReview(ctx context.Context, stats *CycleStats) error {
	if a.dryRun {
		a.log.Info("[dry-run] would review codebase with Claude", "model", a.cfg.Claude.Model, "work_dir", a.cfg.Repo.WorkDir)
		return nil
	}

	a.log.Info("invoking Claude to review codebase", "model", a.cfg.Claude.Model, "budget_per_call", a.cfg.Claude.MaxBudgetPerCall)
	output, err := a.claude.Run(ctx, a.repo.WorkDir(), claude.ReviewPrompt())
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}

	a.log.Info("parsing Claude review output")
	items, _, err := claude.ParseReviewOutput(output)
	if err != nil {
		return fmt.Errorf("parsing review: %w", err)
	}

	stats.ItemsFound = len(items)
	a.log.Info("review complete", "items_found", len(items))
	for i, item := range items {
		a.log.Info("review item", "index", i+1, "title", item.Title, "priority", item.Priority, "category", item.Category)
	}
	a.reviewItems = items
	return nil
}

func (a *App) doIngest(ctx context.Context, stats *CycleStats) error {
	if a.dryRun {
		a.log.Info("[dry-run] would ingest items into backlog", "items_to_ingest", len(a.reviewItems))
		return nil
	}

	if a.reviewItems == nil {
		a.log.Info("no review items to ingest")
		return nil
	}

	a.log.Info("ingesting review items into backlog", "items_to_ingest", len(a.reviewItems))
	inserted, err := a.manager.Ingest(ctx, a.cfg.Repo.URL, a.reviewItems)
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	a.log.Info("ingestion complete", "new_items_inserted", inserted, "duplicates_skipped", len(a.reviewItems)-inserted)
	stats.ItemsInserted = inserted
	a.reviewItems = nil

	// Create GitHub issues for new items when configured
	if a.cfg.GitHub.CreateIssues && !a.dryRun && inserted > 0 {
		a.createIssuesForNewItems(ctx, stats)
	}

	return nil
}

// createIssuesForNewItems creates GitHub issues for all pending items that don't
// yet have an associated issue number. This intentionally includes items from
// previous cycles whose issue creation failed, acting as an automatic retry.
func (a *App) createIssuesForNewItems(ctx context.Context, stats *CycleStats) {
	label := a.cfg.GitHub.IssueLabel
	if err := a.issueManager.EnsureLabel(ctx, a.repo.WorkDir(), label); err != nil {
		a.log.Warn("failed to ensure GitHub label exists, skipping issue creation", "label", label, "error", err)
		return
	}

	status := backlog.StatusPending
	zeroIssue := 0
	items, err := a.store.List(ctx, backlog.ListFilter{
		Status:      &status,
		RepoURL:     &a.cfg.Repo.URL,
		IssueNumber: &zeroIssue,
	})
	if err != nil {
		a.log.Warn("failed to list items for issue creation", "error", err)
		return
	}

	for _, item := range items {
		body := fmt.Sprintf("**%s**\n\n%s\n\n**File:** `%s`\n**Priority:** %s\n**Category:** %s\n\n---\n*Created by [autobacklog](https://github.com/jamesreagan/autobacklog)*",
			item.Title, item.Description, item.FilePath, item.Priority, item.Category)

		issueNum, err := a.issueManager.CreateIssue(ctx, a.repo.WorkDir(), item.Title, body, []string{label})
		if err != nil {
			a.log.Warn("failed to create GitHub issue", "title", item.Title, "error", err)
			continue
		}

		item.IssueNumber = issueNum
		if err := a.store.Update(ctx, item); err != nil {
			a.log.Warn("failed to update item with issue number", "title", item.Title, "error", err)
			continue
		}

		stats.IssuesCreated++
		a.log.Info("created GitHub issue", "title", item.Title, "issue_number", issueNum)
	}
}

func (a *App) doEvaluateThreshold(ctx context.Context, stats *CycleStats) error {
	a.log.Info("evaluating backlog thresholds",
		"high_threshold", a.cfg.Backlog.HighThreshold,
		"medium_threshold", a.cfg.Backlog.MediumThreshold,
		"low_threshold", a.cfg.Backlog.LowThreshold,
		"max_per_cycle", a.cfg.Backlog.MaxPerCycle,
	)
	result, err := backlog.EvaluateThreshold(ctx, a.store, a.cfg.Repo.URL,
		a.cfg.Backlog.HighThreshold,
		a.cfg.Backlog.MediumThreshold,
		a.cfg.Backlog.LowThreshold,
		a.cfg.Backlog.MaxPerCycle,
	)
	if err != nil {
		return fmt.Errorf("threshold evaluation: %w", err)
	}

	a.log.Info("threshold evaluation result",
		"should_implement", result.ShouldImplement,
		"reason", result.Reason,
		"selected_items", len(result.SelectedItems),
	)

	if !result.ShouldImplement {
		a.selectedItems = nil
		return nil
	}

	for i, item := range result.SelectedItems {
		a.log.Info("selected for implementation", "index", i+1, "title", item.Title, "priority", item.Priority)
	}
	a.selectedItems = result.SelectedItems
	return nil
}

func (a *App) doImplement(ctx context.Context, stats *CycleStats) error {
	if a.selectedItems == nil {
		a.log.Info("no items selected for implementation, nothing to do")
		return nil
	}

	if a.dryRun {
		for i, item := range a.selectedItems {
			a.log.Info("[dry-run] would implement item", "index", i+1, "title", item.Title, "priority", item.Priority, "category", item.Category)
		}
		return nil
	}

	a.log.Info("beginning implementation", "items_to_implement", len(a.selectedItems))
	for i, item := range a.selectedItems {
		if a.burndownTotal > 0 {
			a.burndownDone++
			a.log.Info(fmt.Sprintf("[burndown] addressing item %d of %d: %s", a.burndownDone, a.burndownTotal, item.Title))
		} else {
			a.log.Info("implementing item", "index", i+1, "of", len(a.selectedItems), "title", item.Title)
		}
		if err := a.implementItem(ctx, item, stats); err != nil {
			a.log.Error("failed to implement item", "title", item.Title, "error", err)
			stats.Errors = append(stats.Errors, err)
		}
	}

	return nil
}

func (a *App) implementItem(ctx context.Context, item *backlog.Item, stats *CycleStats) error {
	a.log.Info("starting item implementation", "title", item.Title, "priority", item.Priority, "category", item.Category, "attempt", item.Attempts+1)

	// Mark as in progress
	item.Status = backlog.StatusInProgress
	item.Attempts++
	if err := a.store.Update(ctx, item); err != nil {
		return fmt.Errorf("updating item status to in_progress: %w", err)
	}

	// Check budget
	a.log.Info("checking budget", "spent", a.claude.Budget().Spent(), "max_per_call", a.cfg.Claude.MaxBudgetPerCall, "max_total", a.cfg.Claude.MaxBudgetTotal)
	if !a.claude.Budget().CanSpend(a.cfg.Claude.MaxBudgetPerCall) {
		if nErr := a.notifier.Send(notify.OutOfTokensNotification(
			a.claude.Budget().Spent(), a.cfg.Claude.MaxBudgetTotal,
		)); nErr != nil {
			a.log.Warn("failed to send out-of-tokens notification", "error", nErr)
		}
		return fmt.Errorf("budget exceeded")
	}

	// Create branch
	a.log.Info("creating feature branch", "prefix", a.cfg.Repo.PRBranchPrefix, "category", item.Category)
	branchName, err := a.repo.CreateBranch(ctx, a.cfg.Repo.PRBranchPrefix, string(item.Category), item.Title)
	if err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}
	a.log.Info("created branch", "branch", branchName)

	// Invoke Claude to implement
	a.log.Info("invoking Claude to implement changes", "title", item.Title)
	prompt := claude.ImplementPrompt(item)
	_, err = a.claude.RunPrint(ctx, a.repo.WorkDir(), prompt)
	if err != nil {
		if coErr := a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch); coErr != nil {
			a.log.Error("failed to checkout main branch after implement error", "branch", a.cfg.Repo.Branch, "error", coErr)
		}
		return fmt.Errorf("claude implement: %w", err)
	}

	// Check if there are any changes
	a.log.Info("checking for code changes")
	hasChanges, err := a.repo.HasChanges(ctx)
	if err != nil {
		return fmt.Errorf("checking changes: %w", err)
	}
	if !hasChanges {
		a.log.Warn("claude made no changes", "title", item.Title)
		if err := a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch); err != nil {
			return fmt.Errorf("checkout main branch after no changes: %w", err)
		}
		item.Status = backlog.StatusSkipped
		if err := a.store.Update(ctx, item); err != nil {
			return fmt.Errorf("updating item status to skipped: %w", err)
		}
		stats.Items = append(stats.Items, ItemResult{
			Title:    item.Title,
			Category: string(item.Category),
			Status:   "skipped",
		})
		return nil
	}

	// Run tests with retry loop
	a.log.Info("running tests", "max_retries", a.cfg.Testing.MaxRetries)
	testResult, err := a.runTestsWithRetry(ctx, item)
	if err != nil {
		if revertErr := a.repo.RevertToClean(ctx); revertErr != nil {
			a.log.Error("failed to revert working directory", "error", revertErr)
		}
		if coErr := a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch); coErr != nil {
			a.log.Error("failed to checkout main branch after test failure", "branch", a.cfg.Repo.Branch, "error", coErr)
		}
		item.Status = backlog.StatusFailed
		if updateErr := a.store.Update(ctx, item); updateErr != nil {
			a.log.Error("failed to update item status to failed", "title", item.Title, "error", updateErr)
		}
		stats.Items = append(stats.Items, ItemResult{
			Title:    item.Title,
			Category: string(item.Category),
			Status:   "failed",
		})
		if nErr := a.notifier.Send(notify.StuckNotification(item.Title, item.FilePath, item.Attempts, err.Error())); nErr != nil {
			a.log.Warn("failed to send stuck notification", "error", nErr)
		}
		return err
	}

	// Stage and commit
	a.log.Info("tests passed, staging and committing changes")
	if err := a.repo.StageAll(ctx); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	commitMsg := fmt.Sprintf("autobacklog: %s\n\n%s", item.Title, item.Description)
	if err := a.repo.Commit(ctx, commitMsg); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	// Push and create PR
	a.log.Info("pushing branch to remote", "branch", branchName)
	if err := a.repo.Push(ctx, branchName); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	a.log.Info("creating pull request", "title", item.Title, "base", a.cfg.Repo.Branch, "head", branchName)
	prBody := gh.FormatPRBody(item.Title, item.Description, string(item.Category), testResult, item.IssueNumber)
	prURL, err := a.prCreator.CreatePR(ctx, a.repo.WorkDir(), gh.PRRequest{
		Title:      fmt.Sprintf("[autobacklog] %s", item.Title),
		Body:       prBody,
		BaseBranch: a.cfg.Repo.Branch,
		HeadBranch: branchName,
	})
	if err != nil {
		return fmt.Errorf("creating PR: %w", err)
	}

	a.log.Info("pull request created", "pr_url", prURL, "title", item.Title)

	item.Status = backlog.StatusDone
	item.PRLink = prURL
	if err := a.store.Update(ctx, item); err != nil {
		return fmt.Errorf("updating item status to done: %w", err)
	}
	stats.ItemsImplemented++
	stats.PRsCreated++
	stats.Items = append(stats.Items, ItemResult{
		Title:    item.Title,
		Category: string(item.Category),
		Status:   "done",
		PRLink:   prURL,
	})

	if nErr := a.notifier.Send(notify.PRCreatedNotification(item.Title, prURL, item.Description)); nErr != nil {
		a.log.Warn("failed to send PR notification", "error", nErr)
	}

	// Enable auto-merge if configured
	if a.cfg.GitHub.AutoMerge {
		if err := a.prCreator.EnableAutoMerge(ctx, a.repo.WorkDir(), prURL); err != nil {
			a.log.Warn("auto-merge failed, PR still open for manual merge", "pr", prURL, "error", err)
		} else {
			stats.PRsAutoMerged++
		}
	}

	// Return to main branch for next item
	a.log.Info("returning to base branch", "branch", a.cfg.Repo.Branch)
	if err := a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch); err != nil {
		return fmt.Errorf("checkout main branch after PR: %w", err)
	}

	return nil
}

func (a *App) runTestsWithRetry(ctx context.Context, item *backlog.Item) (string, error) {
	workDir := a.repo.WorkDir()
	maxRetries := a.cfg.Testing.MaxRetries

	// Detect or use override test command
	var command string
	var args []string

	if a.cfg.Testing.OverrideCommand != "" {
		command = "sh"
		args = []string{"-c", a.cfg.Testing.OverrideCommand}
		a.log.Info("using override test command", "command", a.cfg.Testing.OverrideCommand)
	} else if a.cfg.Testing.AutoDetect {
		// Cache detection result to avoid redundant filesystem checks per item.
		if a.cachedDetect == nil {
			a.log.Info("auto-detecting test framework", "work_dir", workDir)
			a.cachedDetect = runner.Detect(workDir, a.log)
		}
		if a.cachedDetect == nil {
			a.log.Warn("no test framework detected, skipping tests")
			return "no test framework detected", nil
		}
		command = a.cachedDetect.Command
		args = a.cachedDetect.Args
		a.log.Info("using detected test framework", "command", command, "args", args)
	} else {
		a.log.Info("testing disabled, skipping")
		return "tests disabled", nil
	}

	var lastOutput string
	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		a.log.Info("running test suite", "attempt", attempt, "max_attempts", maxRetries+1)
		result, err := a.runner.Run(ctx, workDir, command, args)
		if err != nil {
			return "", fmt.Errorf("running tests: %w", err)
		}

		if result.Passed {
			a.log.Info("tests passed", "attempt", attempt)
			return result.Output, nil
		}

		lastOutput = result.Output
		if attempt <= maxRetries {
			a.log.Warn("tests failed, invoking Claude to fix", "attempt", attempt, "max_retries", maxRetries)

			// Ask Claude to fix the tests
			fixPrompt := claude.FixTestPrompt(result.Output)
			_, err = a.claude.RunPrint(ctx, workDir, fixPrompt)
			if err != nil {
				return "", fmt.Errorf("claude fix attempt %d: %w", attempt, err)
			}
		}
	}

	return "", fmt.Errorf("tests still failing after %d retries:\n%s", maxRetries, lastOutput)
}

func (a *App) doDocument(ctx context.Context, stats *CycleStats) error {
	if a.dryRun {
		a.log.Info("[dry-run] would update documentation", "items_implemented", stats.ItemsImplemented)
		return nil
	}
	if stats.ItemsImplemented == 0 {
		a.log.Info("no items implemented, skipping documentation update")
		return nil
	}

	// Documentation updates are optional — don't fail the cycle if they fail
	var changes []string
	for _, item := range a.selectedItems {
		if item.Status == backlog.StatusDone {
			changes = append(changes, item.Title)
		}
	}

	if len(changes) == 0 {
		a.log.Info("no successful changes to document")
		return nil
	}

	a.log.Info("invoking Claude to update documentation", "changes", len(changes))
	prompt := claude.DocumentPrompt(changes)
	_, err := a.claude.RunPrint(ctx, a.repo.WorkDir(), prompt)
	if err != nil {
		a.log.Warn("documentation update failed", "error", err)
		// Non-fatal
	}

	return nil
}
