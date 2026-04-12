package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jamesreagan/autobacklog/internal/app"
	"github.com/jamesreagan/autobacklog/internal/logging"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run a single improvement cycle",
		RunE:  runOnce,
	}
}

func runOnce(cmd *cobra.Command, args []string) error {
	s, err := setup()
	if err != nil {
		return err
	}
	defer logging.Cleanup()
	defer s.store.Close()
	defer s.cancel()
	if s.uiServer != nil {
		defer s.uiServer.Shutdown(context.Background())
	}

	// #158: if mode is "daemon" in config, warn and run one cycle instead of
	// silently entering daemon loop. Use the `daemon` subcommand for continuous mode.
	if s.cfg.Mode == "daemon" {
		s.log.Warn("config has mode=daemon but 'run' executes a single cycle; use 'autobacklog daemon' for continuous mode")
	}

	// Run one cycle (or loop in burndown mode)
	var stats *app.CycleStats
	if s.cfg.HelperMode == "burndown" {
		stats, err = s.orchestrator.RunBurndown(s.ctx)
	} else {
		stats, err = s.orchestrator.RunCycle(s.ctx)
	}
	if err != nil {
		return fmt.Errorf("cycle failed: %w", err)
	}

	broadcastStats(s.hub, stats)
	s.log.Info(stats.Summary())

	return nil
}
