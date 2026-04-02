package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jamesreagan/autobacklog/internal/app"
	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/logging"
	"github.com/jamesreagan/autobacklog/internal/notify"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run a single improvement cycle",
		RunE:  runOnce,
	}
}

func runOnce(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if verbose {
		cfg.Logging.Level = "debug"
	}

	log, err := logging.Setup(cfg.Logging)
	if err != nil {
		return fmt.Errorf("setting up logging: %w", err)
	}

	// Setup DB
	dbPath := filepath.Join(os.Getenv("HOME"), ".autobacklog", "backlog.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Setup GitHub auth
	if !dryRun {
		pat, _ := cfg.ResolveGitHubPAT()
		ctx := context.Background()
		if err := gh.SetupAuth(ctx, pat, log); err != nil {
			log.Warn("GitHub auth setup failed", "error", err)
		}
	}

	// Setup notifier
	var notifier notify.Notifier
	if cfg.Notifications.Enabled {
		notifier = notify.NewEmailNotifier(cfg.Notifications, log)
	} else {
		notifier = notify.NoopNotifier{}
	}

	// Create orchestrator
	orchestrator, err := app.New(cfg, store, notifier, log, dryRun)
	if err != nil {
		return fmt.Errorf("creating orchestrator: %w", err)
	}

	// Handle signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("received shutdown signal")
		cancel()
	}()

	// Run one cycle
	stats, err := orchestrator.RunCycle(ctx)
	if err != nil {
		return fmt.Errorf("cycle failed: %w", err)
	}

	log.Info("cycle complete",
		"items_found", stats.ItemsFound,
		"items_inserted", stats.ItemsInserted,
		"items_implemented", stats.ItemsImplemented,
		"prs_created", stats.PRsCreated,
	)

	return nil
}
