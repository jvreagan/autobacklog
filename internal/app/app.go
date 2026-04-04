package app

import (
	"context"
	"fmt"
	"log/slog"

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
	notifier      notify.Notifier
	log           *slog.Logger
	dryRun        bool
	reviewItems   []*backlog.Item // transient: review → ingest
	selectedItems []*backlog.Item // transient: threshold → implement
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

// New creates a new App orchestrator with production dependencies.
func New(cfg *config.Config, store backlog.Store, notifier notify.Notifier, log *slog.Logger, dryRun bool) (*App, error) {
	pat, err := cfg.ResolveGitHubPAT()
	if err != nil && !dryRun {
		log.Warn("no GitHub PAT configured", "error", err)
	}

	repo := git.NewRepo(cfg.Repo.URL, cfg.Repo.Branch, cfg.Repo.WorkDir, pat, log)
	claudeClient := claude.NewClient(cfg.Claude, log)
	testRunner := runner.NewRunner(log, cfg.Testing.Timeout)

	return NewWithDeps(cfg, repo, claudeClient, testRunner, &defaultPRCreator{log: log}, store, notifier, log, dryRun), nil
}

// NewWithDeps creates an App with explicitly provided dependencies (for testing).
func NewWithDeps(
	cfg *config.Config,
	repo Repository,
	aiClient AIClient,
	testRunner TestRunner,
	prCreator PRCreator,
	store backlog.Store,
	notifier notify.Notifier,
	log *slog.Logger,
	dryRun bool,
) *App {
	mgr := backlog.NewManager(store, log)
	return &App{
		cfg:       cfg,
		repo:      repo,
		claude:    aiClient,
		store:     store,
		manager:   mgr,
		runner:    testRunner,
		prCreator: prCreator,
		notifier:  notifier,
		log:       log,
		dryRun:    dryRun,
	}
}

// RunCycle executes one full cycle of the state machine.
func (a *App) RunCycle(ctx context.Context) (*CycleStats, error) {
	stats := &CycleStats{}
	state := StateClone

	a.log.Info("starting cycle", "dry_run", a.dryRun, "repo", a.cfg.Repo.URL)

	for state != StateDone {
		a.log.Info("entering state", "state", state.String(), "action", state.Description())

		var err error
		switch state {
		case StateClone:
			err = a.doClone(ctx)
		case StateReview:
			err = a.doReview(ctx, stats)
		case StateIngest:
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
			a.notifier.Send(notify.ErrorNotification(state.String(), err))
			return stats, err
		}

		a.log.Info("completed state", "state", state.String())
		state = state.Next()
	}

	// Clean stale items
	a.log.Info("cleaning stale backlog items", "stale_days", a.cfg.Backlog.StaleDays)
	a.manager.CleanStale(ctx, a.cfg.Backlog.StaleDays)

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

	a.notifier.Send(notify.CycleCompleteNotification(
		stats.ItemsFound, stats.ItemsImplemented, stats.PRsCreated,
		stats.BudgetSummary,
	))

	return stats, nil
}

func (a *App) doClone(ctx context.Context) error {
	if a.dryRun {
		a.log.Info("[dry-run] would clone/pull repo", "url", a.cfg.Repo.URL, "branch", a.cfg.Repo.Branch, "work_dir", a.cfg.Repo.WorkDir)
		return nil
	}
	a.log.Info("cloning or pulling repository", "url", a.cfg.Repo.URL, "branch", a.cfg.Repo.Branch, "work_dir", a.cfg.Repo.WorkDir)
	return a.repo.CloneOrPull(ctx)
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
	inserted, err := a.manager.Ingest(ctx, a.reviewItems)
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	a.log.Info("ingestion complete", "new_items_inserted", inserted, "duplicates_skipped", len(a.reviewItems)-inserted)
	stats.ItemsInserted = inserted
	a.reviewItems = nil
	return nil
}

func (a *App) doEvaluateThreshold(ctx context.Context, stats *CycleStats) error {
	a.log.Info("evaluating backlog thresholds",
		"high_threshold", a.cfg.Backlog.HighThreshold,
		"medium_threshold", a.cfg.Backlog.MediumThreshold,
		"low_threshold", a.cfg.Backlog.LowThreshold,
		"max_per_cycle", a.cfg.Backlog.MaxPerCycle,
	)
	result, err := backlog.EvaluateThreshold(ctx, a.store,
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
		a.log.Info("implementing item", "index", i+1, "of", len(a.selectedItems), "title", item.Title)
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
		a.notifier.Send(notify.OutOfTokensNotification(
			a.claude.Budget().Spent(), a.cfg.Claude.MaxBudgetTotal,
		))
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
		a.repo.RevertToClean(ctx)
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
		a.notifier.Send(notify.StuckNotification(item.Title, item.FilePath, item.Attempts, err.Error()))
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
	prBody := gh.FormatPRBody(item.Title, item.Description, string(item.Category), testResult)
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

	a.notifier.Send(notify.PRCreatedNotification(item.Title, prURL, item.Description))

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
		a.log.Info("auto-detecting test framework", "work_dir", workDir)
		detected := runner.Detect(workDir, a.log)
		if detected == nil {
			a.log.Warn("no test framework detected, skipping tests")
			return "no test framework detected", nil
		}
		command = detected.Command
		args = detected.Args
		a.log.Info("detected test framework", "command", command, "args", args)
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
