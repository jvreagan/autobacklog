package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var (
	cfgFile    string
	verbose    bool
	dryRun     bool
	helperMode string
	webuiPort  int
)

// NewRootCmd creates the root command for autobacklog.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "autobacklog",
		Short: "Autonomous code improvement daemon",
		Long:  "Autobacklog continuously reviews code with AI, builds a prioritized backlog, implements improvements, and creates PRs.",
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "autobacklog.yaml", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "run without making changes")
	rootCmd.PersistentFlags().StringVar(&helperMode, "helper-mode", "", "override helper mode (buildbacklog or burndown)")
	rootCmd.PersistentFlags().IntVar(&webuiPort, "webui-port", 0, "enable web UI on this port (0 = disabled)")

	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newCostsCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("autobacklog %s\n", Version)
		},
	}
}
