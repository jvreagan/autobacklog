package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
	defer s.store.Close()
	defer s.cancel()

	// If mode is "daemon", loop with the configured interval
	if s.cfg.Mode == "daemon" {
		return runDaemonLoop(s.ctx, s.cfg, s.orchestrator, s.log)
	}

	// Run one cycle
	stats, err := s.orchestrator.RunCycle(s.ctx)
	if err != nil {
		return fmt.Errorf("cycle failed: %w", err)
	}

	s.log.Info(stats.Summary())

	return nil
}
