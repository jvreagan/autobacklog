package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current backlog state",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	dbPath := filepath.Join(os.Getenv("HOME"), ".autobacklog", "backlog.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Scope by repo URL when a config file is available.
	filter := backlog.ListFilter{}
	if cfg, err := config.Load(cfgFile); err == nil && cfg.Repo.URL != "" {
		filter.RepoURL = &cfg.Repo.URL
	}

	items, err := store.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing items: %w", err)
	}

	if len(items) == 0 {
		cmd.Println("Backlog is empty.")
		return nil
	}

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

	return nil
}
