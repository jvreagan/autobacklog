package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate an example configuration file",
		RunE:  runInit,
	}
}

const exampleConfig = `# Autobacklog Configuration
# See https://github.com/jamesreagan/autobacklog for full documentation.

repo:
  url: "https://github.com/your-org/your-repo.git"
  branch: "main"
  work_dir: "/tmp/autobacklog/your-repo"
  pr_branch_prefix: "autobacklog"

github:
  # Use one of: pat, pat_file, or GITHUB_TOKEN env var
  pat: "${GITHUB_TOKEN}"
  # pat_file: "/path/to/pat-file"
  auto_merge: false                  # enable to auto-merge PRs after CI passes

claude:
  binary: "claude"
  model: "sonnet"
  max_budget_per_call: 10.00
  max_budget_total: 100.00
  timeout: "10m"
  # dangerously_skip_permissions: false  # pass --dangerously-skip-permissions to Claude CLI

backlog:
  high_threshold: 1     # implement immediately when any high-priority item exists
  medium_threshold: 3   # batch when 3+ medium items accumulate
  low_threshold: 5      # batch when 5+ low items accumulate
  max_per_cycle: 5      # max items to implement per cycle
  stale_days: 30        # auto-clean completed/failed items after 30 days

testing:
  auto_detect: true
  # override_command: "npm test"    # uncomment to override auto-detection
  timeout: "5m"
  max_retries: 3

mode: "oneshot"                    # "oneshot" or "daemon"

daemon:
  interval: "1h"
  # quiet_start: "22:00"           # optional quiet hours
  # quiet_end: "06:00"

notifications:
  enabled: false
  smtp:
    host: "smtp.example.com"
    port: 587
    username: "${SMTP_USERNAME}"
    password: "${SMTP_PASSWORD}"
    from: "autobacklog@example.com"
  recipients:
    - "dev-team@example.com"
  events:
    on_cycle_complete: true
    on_stuck: true
    on_out_of_tokens: true
    on_pr_created: true
    on_error: true

logging:
  level: "info"                   # debug, info, warn, error
  # file: "/var/log/autobacklog.log"
  format: "text"                  # text or json
`

func runInit(cmd *cobra.Command, args []string) error {
	filename := "autobacklog.yaml"
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("%s already exists; remove it first or edit it directly", filename)
	}

	if err := os.WriteFile(filename, []byte(exampleConfig), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("Created %s — edit it with your repo and credentials, then run:\n", filename)
	fmt.Println("  autobacklog run --config autobacklog.yaml")
	return nil
}
