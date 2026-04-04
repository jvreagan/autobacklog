package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/jamesreagan/autobacklog/internal/app"
	"github.com/jamesreagan/autobacklog/internal/config"
	"github.com/jamesreagan/autobacklog/internal/logging"
)

func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run as a continuous daemon",
		RunE:  runDaemon,
	}
}

func runDaemon(cmd *cobra.Command, args []string) error {
	s, err := setup()
	if err != nil {
		return err
	}
	defer logging.Cleanup()
	defer s.store.Close()
	defer s.cancel()

	return runDaemonLoop(s.ctx, s.cfg, s.orchestrator, s.log)
}

// runDaemonLoop is the shared loop used by both `run` (when mode=daemon) and `daemon`.
func runDaemonLoop(ctx context.Context, cfg *config.Config, orchestrator *app.App, log *slog.Logger) error {
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
				"prs_auto_merged", stats.PRsAutoMerged,
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
