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
	filter := backlog.ListFilter{}
	if cfgFile != "" {
		cfg, cfgErr := config.Load(cfgFile)
		if cfgErr != nil {
			// #159: warn instead of silently ignoring config parse errors
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to parse config %s: %v\n", cfgFile, cfgErr)
		} else if cfg.Repo.URL != "" {
			filter.RepoURL = &cfg.Repo.URL
		}
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
