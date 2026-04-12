package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
	"github.com/spf13/cobra"
)

func newAPIStatsCmd() *cobra.Command {
	var days int

	cmd := &cobra.Command{
		Use:   "api-stats",
		Short: "Show GitHub API usage statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("determining home directory: %w", err)
			}
			dbPath := filepath.Join(home, ".autobacklog", "backlog.db")
			store, err := backlog.NewSQLiteStore(dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer store.Close()

			since := time.Now().UTC().AddDate(0, 0, -days)
			records, err := store.ListAPIStats(cmd.Context(), cfg.Repo.URL, since)
			if err != nil {
				return fmt.Errorf("listing api stats: %w", err)
			}

			if len(records) == 0 {
				cmd.Println("No API stats records found.")
				return nil
			}

			var totalCalls, totalRetries, totalRateLimits, totalFailures int
			for _, r := range records {
				totalCalls += r.Calls
				totalRetries += r.Retries
				totalRateLimits += r.RateLimits
				totalFailures += r.Failures
			}

			cmd.Printf("GitHub API stats for %s (last %d days, %d cycles)\n\n", cfg.Repo.URL, days, len(records))
			cmd.Printf("%-15s %8s\n", "Metric", "Total")
			cmd.Printf("%-15s %8s\n", "-----------", "-----")
			cmd.Printf("%-15s %8d\n", "Calls", totalCalls)
			cmd.Printf("%-15s %8d\n", "Retries", totalRetries)
			cmd.Printf("%-15s %8d\n", "Rate Limits", totalRateLimits)
			cmd.Printf("%-15s %8d\n", "Failures", totalFailures)

			return nil
		},
	}

	cmd.Flags().IntVar(&days, "days", 30, "number of days to look back")
	return cmd
}
