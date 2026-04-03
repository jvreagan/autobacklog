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
	repo          *git.Repo
	claude        *claude.Client
	store         backlog.Store
	manager       *backlog.Manager
	runner        *runner.Runner
	notifier      notify.Notifier
	log           *slog.Logger
	dryRun        bool
	reviewItems   []*backlog.Item // transient: review → ingest
	selectedItems []*backlog.Item // transient: threshold → implement
}

// New creates a new App orchestrator.
func New(cfg *config.Config, store backlog.Store, notifier notify.Notifier, log *slog.Logger, dryRun bool) (*App, error) {
	pat, err := cfg.ResolveGitHubPAT()
	if err != nil && !dryRun {
		log.Warn("no GitHub PAT configured", "error", err)
	}

	repo := git.NewRepo(cfg.Repo.URL, cfg.Repo.Branch, cfg.Repo.WorkDir, pat, log)
	claudeClient := claude.NewClient(cfg.Claude, log)
	mgr := backlog.NewManager(store, log)
	testRunner := runner.NewRunner(log, cfg.Testing.Timeout)

	return &App{
		cfg:      cfg,
		repo:     repo,
		claude:   claudeClient,
		store:    store,
		manager:  mgr,
		runner:   testRunner,
		notifier: notifier,
		log:      log,
		dryRun:   dryRun,
	}, nil
}

// RunCycle executes one full cycle of the state machine.
func (a *App) RunCycle(ctx context.Context) (*CycleStats, error) {
	stats := &CycleStats{}
	state := StateClone

	for state != StateDone {
		a.log.Info("state transition", "state", state.String())

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
			// Tests are run per-item inside doImplement
			state = state.Next()
			continue
		case StatePR:
			// PRs are created per-item inside doImplement
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

		state = state.Next()
	}

	// Clean stale items
	a.manager.CleanStale(ctx, a.cfg.Backlog.StaleDays)

	// Send cycle summary
	a.notifier.Send(notify.CycleCompleteNotification(
		stats.ItemsFound, stats.ItemsImplemented, stats.PRsCreated,
		a.claude.Budget().String(),
	))

	return stats, nil
}

func (a *App) doClone(ctx context.Context) error {
	if a.dryRun {
		a.log.Info("[dry-run] would clone/pull repo", "url", a.cfg.Repo.URL)
		return nil
	}
	return a.repo.CloneOrPull(ctx)
}

func (a *App) doReview(ctx context.Context, stats *CycleStats) error {
	if a.dryRun {
		a.log.Info("[dry-run] would review codebase with Claude")
		return nil
	}

	output, err := a.claude.Run(ctx, a.repo.WorkDir(), claude.ReviewPrompt())
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}

	items, _, err := claude.ParseReviewOutput(output)
	if err != nil {
		return fmt.Errorf("parsing review: %w", err)
	}

	stats.ItemsFound = len(items)
	// Store items temporarily for ingest phase
	a.reviewItems = items
	return nil
}

func (a *App) doIngest(ctx context.Context, stats *CycleStats) error {
	if a.dryRun {
		a.log.Info("[dry-run] would ingest items into backlog")
		return nil
	}

	if a.reviewItems == nil {
		return nil
	}

	inserted, err := a.manager.Ingest(ctx, a.reviewItems)
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	stats.ItemsInserted = inserted
	a.reviewItems = nil
	return nil
}

func (a *App) doEvaluateThreshold(ctx context.Context, stats *CycleStats) error {
	result, err := backlog.EvaluateThreshold(ctx, a.store,
		a.cfg.Backlog.HighThreshold,
		a.cfg.Backlog.MediumThreshold,
		a.cfg.Backlog.LowThreshold,
		a.cfg.Backlog.MaxPerCycle,
	)
	if err != nil {
		return fmt.Errorf("threshold evaluation: %w", err)
	}

	a.log.Info("threshold evaluation", "should_implement", result.ShouldImplement, "reason", result.Reason, "items", len(result.SelectedItems))

	if !result.ShouldImplement {
		a.selectedItems = nil
		return nil
	}

	a.selectedItems = result.SelectedItems
	return nil
}

func (a *App) doImplement(ctx context.Context, stats *CycleStats) error {
	if a.selectedItems == nil {
		a.log.Info("no items to implement")
		return nil
	}

	if a.dryRun {
		a.log.Info("[dry-run] would implement items", "count", len(a.selectedItems))
		return nil
	}

	for _, item := range a.selectedItems {
		if err := a.implementItem(ctx, item, stats); err != nil {
			a.log.Error("failed to implement item", "title", item.Title, "error", err)
			stats.Errors = append(stats.Errors, err)
			// Continue with other items
		}
	}

	return nil
}

func (a *App) implementItem(ctx context.Context, item *backlog.Item, stats *CycleStats) error {
	a.log.Info("implementing item", "title", item.Title, "priority", item.Priority)

	// Mark as in progress
	item.Status = backlog.StatusInProgress
	item.Attempts++
	if err := a.store.Update(ctx, item); err != nil {
		return fmt.Errorf("updating item status to in_progress: %w", err)
	}

	// Check budget
	if !a.claude.Budget().CanSpend(a.cfg.Claude.MaxBudgetPerCall) {
		a.notifier.Send(notify.OutOfTokensNotification(
			a.claude.Budget().Spent(), a.cfg.Claude.MaxBudgetTotal,
		))
		return fmt.Errorf("budget exceeded")
	}

	// Create branch
	branchName, err := a.repo.CreateBranch(ctx, a.cfg.Repo.PRBranchPrefix, string(item.Category), item.Title)
	if err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Invoke Claude to implement
	prompt := claude.ImplementPrompt(item)
	_, err = a.claude.RunPrint(ctx, a.repo.WorkDir(), prompt)
	if err != nil {
		a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch)
		return fmt.Errorf("claude implement: %w", err)
	}

	// Check if there are any changes
	hasChanges, err := a.repo.HasChanges(ctx)
	if err != nil {
		return fmt.Errorf("checking changes: %w", err)
	}
	if !hasChanges {
		a.log.Warn("claude made no changes", "title", item.Title)
		a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch)
		item.Status = backlog.StatusSkipped
		if err := a.store.Update(ctx, item); err != nil {
			return fmt.Errorf("updating item status to skipped: %w", err)
		}
		return nil
	}

	// Run tests with retry loop
	testResult, err := a.runTestsWithRetry(ctx, item)
	if err != nil {
		a.repo.RevertToClean(ctx)
		a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch)
		item.Status = backlog.StatusFailed
		if updateErr := a.store.Update(ctx, item); updateErr != nil {
			a.log.Error("failed to update item status to failed", "title", item.Title, "error", updateErr)
		}
		a.notifier.Send(notify.StuckNotification(item.Title, item.FilePath, item.Attempts, err.Error()))
		return err
	}

	// Stage and commit
	a.repo.StageAll(ctx)
	commitMsg := fmt.Sprintf("autobacklog: %s\n\n%s", item.Title, item.Description)
	if err := a.repo.Commit(ctx, commitMsg); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	// Push and create PR
	if err := a.repo.Push(ctx, branchName); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	prBody := gh.FormatPRBody(item.Title, item.Description, string(item.Category), testResult)
	prURL, err := gh.CreatePR(ctx, a.repo.WorkDir(), gh.PRRequest{
		Title:      fmt.Sprintf("[autobacklog] %s", item.Title),
		Body:       prBody,
		BaseBranch: a.cfg.Repo.Branch,
		HeadBranch: branchName,
	}, a.log)
	if err != nil {
		return fmt.Errorf("creating PR: %w", err)
	}

	item.Status = backlog.StatusDone
	item.PRLink = prURL
	if err := a.store.Update(ctx, item); err != nil {
		return fmt.Errorf("updating item status to done: %w", err)
	}
	stats.ItemsImplemented++
	stats.PRsCreated++

	a.notifier.Send(notify.PRCreatedNotification(item.Title, prURL, item.Description))

	// Return to main branch for next item
	a.repo.CheckoutBranch(ctx, a.cfg.Repo.Branch)

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
	} else if a.cfg.Testing.AutoDetect {
		detected := runner.Detect(workDir, a.log)
		if detected == nil {
			a.log.Warn("no test framework detected, skipping tests")
			return "no test framework detected", nil
		}
		command = detected.Command
		args = detected.Args
	} else {
		return "tests disabled", nil
	}

	var lastOutput string
	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		result, err := a.runner.Run(ctx, workDir, command, args)
		if err != nil {
			return "", fmt.Errorf("running tests: %w", err)
		}

		if result.Passed {
			return result.Output, nil
		}

		lastOutput = result.Output
		if attempt <= maxRetries {
			a.log.Warn("tests failed, attempting fix", "attempt", attempt, "max_retries", maxRetries)

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
	if a.dryRun || stats.ItemsImplemented == 0 {
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
		return nil
	}

	prompt := claude.DocumentPrompt(changes)
	_, err := a.claude.RunPrint(ctx, a.repo.WorkDir(), prompt)
	if err != nil {
		a.log.Warn("documentation update failed", "error", err)
		// Non-fatal
	}

	return nil
}

