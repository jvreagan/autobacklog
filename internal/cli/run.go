package cli

import (
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

	// If mode is "daemon", loop with the configured interval
	if s.cfg.Mode == "daemon" {
		return runDaemonLoop(s.ctx, s.cfg, s.orchestrator, s.log)
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

	s.log.Info(stats.Summary())

	return nil
}
