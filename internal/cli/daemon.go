package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jamesreagan/autobacklog/internal/app"
	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/logging"
	"github.com/jamesreagan/autobacklog/internal/notify"
)

func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run as a continuous daemon",
		RunE:  runDaemon,
	}
}

func runDaemon(cmd *cobra.Command, args []string) error {
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

	dbPath := filepath.Join(os.Getenv("HOME"), ".autobacklog", "backlog.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	if !dryRun {
		pat, _ := cfg.ResolveGitHubPAT()
		ctx := context.Background()
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

	orchestrator, err := app.New(cfg, store, notifier, log, dryRun)
	if err != nil {
		return fmt.Errorf("creating orchestrator: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("received shutdown signal, finishing current cycle...")
		cancel()
	}()

	log.Info("daemon started", "interval", cfg.Daemon.Interval)

	for {
		if isQuietHours(cfg.Daemon.QuietStart, cfg.Daemon.QuietEnd) {
			log.Info("quiet hours, sleeping", "until", cfg.Daemon.QuietEnd)
			select {
			case <-ctx.Done():
				log.Info("daemon stopped")
				return nil
			case <-time.After(10 * time.Minute):
				continue
			}
		}

		stats, err := orchestrator.RunCycle(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Info("daemon stopped by signal")
				return nil
			}
			log.Error("cycle failed", "error", err)
		} else {
			log.Info("cycle complete",
				"items_found", stats.ItemsFound,
				"items_implemented", stats.ItemsImplemented,
				"prs_created", stats.PRsCreated,
			)
		}

		log.Info("sleeping until next cycle", "duration", cfg.Daemon.Interval)
		select {
		case <-ctx.Done():
			log.Info("daemon stopped")
			return nil
		case <-time.After(cfg.Daemon.Interval):
		}
	}
}

func isQuietHours(start, end string) bool {
	if start == "" || end == "" {
		return false
	}

	now := time.Now()
	startTime, err := time.Parse("15:04", start)
	if err != nil {
		return false
	}
	endTime, err := time.Parse("15:04", end)
	if err != nil {
		return false
	}

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startTime.Hour()*60 + startTime.Minute()
	endMinutes := endTime.Hour()*60 + endTime.Minute()

	if startMinutes <= endMinutes {
		return currentMinutes >= startMinutes && currentMinutes < endMinutes
	}
	// Spans midnight
	return currentMinutes >= startMinutes || currentMinutes < endMinutes
}
