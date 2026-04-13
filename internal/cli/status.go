package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
)

func newStatusCmd() *cobra.Command {
	var days int

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current backlog state",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			ctx := context.Background()

			// Scope by repo URL when a config file is available.
			var repoURL string
			filter := backlog.ListFilter{}
			if cfgFile != "" {
				cfg, cfgErr := config.Load(cfgFile)
				if cfgErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to parse config %s: %v\n", cfgFile, cfgErr)
				} else if cfg.Repo.URL != "" {
					repoURL = cfg.Repo.URL
					filter.RepoURL = &repoURL
				}
			}

			items, err := store.List(ctx, filter)
			if err != nil {
				return fmt.Errorf("listing items: %w", err)
			}

			if len(items) == 0 {
				cmd.Println("Backlog is empty.")
			} else {
				// Count by status
				counts := map[backlog.Status]int{}
				for _, item := range items {
					counts[item.Status]++
				}

				cmd.Printf("Backlog: %d items total\n", len(items))
				cmd.Printf("  Pending: %d  In Progress: %d  Done: %d  Failed: %d  Skipped: %d\n\n",
					counts[backlog.StatusPending],
					counts[backlog.StatusInProgress],
					counts[backlog.StatusDone],
					counts[backlog.StatusFailed],
					counts[backlog.StatusSkipped],
				)

				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "PRIORITY\tCATEGORY\tSTATUS\tTITLE\tFILE")
				for _, item := range items {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						item.Priority, item.Category, item.Status, item.Title, item.FilePath)
				}
				w.Flush()
				cmd.Println()
			}

			// Cycle history
			if repoURL == "" {
				return nil
			}

			since := time.Now().UTC().AddDate(0, 0, -days)
			cycles, err := store.ListCycles(ctx, repoURL, since)
			if err != nil {
				return fmt.Errorf("listing cycles: %w", err)
			}

			if len(cycles) == 0 {
				cmd.Println("No cycle history found.")
				return nil
			}

			cmd.Printf("Recent Cycles (last %d days):\n\n", days)
			cmd.Printf("%-20s %6s %4s %5s %4s %7s %8s\n",
				"TIMESTAMP", "FOUND", "NEW", "IMPL", "PRs", "ERRORS", "COST")
			var totFound, totNew, totImpl, totPRs, totErrors int
			var totCost float64
			for _, c := range cycles {
				cmd.Printf("%-20s %6d %4d %5d %4d %7d %8s\n",
					c.Timestamp.Format("2006-01-02 15:04"),
					c.ItemsFound, c.ItemsInserted, c.ItemsImplemented,
					c.PRsCreated, c.ErrorCount,
					fmt.Sprintf("$%.2f", c.TotalCost))
				totFound += c.ItemsFound
				totNew += c.ItemsInserted
				totImpl += c.ItemsImplemented
				totPRs += c.PRsCreated
				totErrors += c.ErrorCount
				totCost += c.TotalCost
			}
			cmd.Printf("%-20s %6s %4s %5s %4s %7s %8s\n",
				"", "-----", "---", "----", "---", "------", "------")
			cmd.Printf("%-20s %6d %4d %5d %4d %7d %8s\n",
				fmt.Sprintf("Total (%d cycles)", len(cycles)),
				totFound, totNew, totImpl, totPRs, totErrors,
				fmt.Sprintf("$%.2f", totCost))

			return nil
		},
	}

	cmd.Flags().IntVar(&days, "days", 30, "number of days of cycle history to show")
	return cmd
}
