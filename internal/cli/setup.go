package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jamesreagan/autobacklog/internal/app"
	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/logging"
	"github.com/jamesreagan/autobacklog/internal/notify"
)

// setupResult bundles the objects created during CLI setup.
type setupResult struct {
	cfg          *config.Config
	store        *backlog.SQLiteStore
	orchestrator *app.App
	log          *slog.Logger
	ctx          context.Context
	cancel       context.CancelFunc
}

// setup loads config, opens the DB, sets up auth, and creates the orchestrator.
// Callers must defer result.store.Close() and result.cancel().
func setup() (*setupResult, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if verbose {
		cfg.Logging.Level = "debug"
	}
	if helperMode != "" {
		cfg.HelperMode = helperMode
	}

	log, err := logging.Setup(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("setting up logging: %w", err)
	}

	dbPath := filepath.Join(os.Getenv("HOME"), ".autobacklog", "backlog.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Create cancellable context before any work so all operations
	// (including auth setup) respect shutdown signals.
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("received shutdown signal")
		cancel()
	}()

	if !dryRun {
		pat, err := cfg.ResolveGitHubPAT()
		if err != nil {
			log.Warn("cannot resolve GitHub PAT", "error", err)
		}
		if err := gh.SetupAuth(ctx, pat, log); err != nil {
			log.Warn("GitHub auth setup failed", "error", err)
		}
	}

	var notifier notify.Notifier
	if cfg.Notifications.Enabled {
		notifier = notify.NewEmailNotifier(cfg.Notifications, log)
	} else {
		notifier = notify.NoopNotifier{}
	}

	// The orchestrator is NOT safe for concurrent use. The daemon loop calls
	// RunCycle sequentially — never call it from multiple goroutines.
	orchestrator, err := app.New(cfg, store, notifier, log, dryRun)
	if err != nil {
		store.Close()
		cancel()
		return nil, fmt.Errorf("creating orchestrator: %w", err)
	}

	return &setupResult{
		cfg:          cfg,
		store:        store,
		orchestrator: orchestrator,
		log:          log,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}
