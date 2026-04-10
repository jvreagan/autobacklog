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

func newCostsCmd() *cobra.Command {
	var days int

	cmd := &cobra.Command{
		Use:   "costs",
		Short: "Show cost analytics for Claude invocations",
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
			records, err := store.ListCosts(cmd.Context(), cfg.Repo.URL, since)
			if err != nil {
				return fmt.Errorf("listing costs: %w", err)
			}

			if len(records) == 0 {
				cmd.Println("No cost records found.")
				return nil
			}

			// Summary by prompt type
			typeTotals := map[string]float64{}
			typeCount := map[string]int{}
			var total float64
			for _, r := range records {
				typeTotals[r.PromptType] += r.CostTotal
				typeCount[r.PromptType]++
				total += r.CostTotal
			}

			cmd.Printf("Cost summary for %s (last %d days)\n\n", cfg.Repo.URL, days)
			cmd.Printf("%-15s %8s %8s\n", "Prompt Type", "Count", "Cost")
			cmd.Printf("%-15s %8s %8s\n", "-----------", "-----", "----")
			for _, pt := range []string{"review", "implement", "fix_test", "document"} {
				if typeCount[pt] > 0 {
					cmd.Printf("%-15s %8d %8s\n", pt, typeCount[pt], fmt.Sprintf("$%.2f", typeTotals[pt]))
				}
			}
			cmd.Printf("%-15s %8s %8s\n", "", "", "------")
			cmd.Printf("%-15s %8d %8s\n", "Total", len(records), fmt.Sprintf("$%.2f", total))

			return nil
		},
	}

	cmd.Flags().IntVar(&days, "days", 30, "number of days to look back")
	return cmd
}
