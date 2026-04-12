package config

import "time"

// Config is the top-level configuration for autobacklog.
type Config struct {
	Repo          RepoConfig          `yaml:"repo"`
	GitHub        GitHubConfig        `yaml:"github"`
	Claude        ClaudeConfig        `yaml:"claude"`
	Backlog       BacklogConfig       `yaml:"backlog"`
	Testing       TestingConfig       `yaml:"testing"`
	Mode          string              `yaml:"mode"`        // "oneshot" or "daemon"
	HelperMode    string              `yaml:"helper_mode"` // "buildbacklog" or "burndown"
	Daemon        DaemonConfig        `yaml:"daemon"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Logging       LoggingConfig       `yaml:"logging"`
	WebUI         WebUIConfig         `yaml:"webui"`
}

// RepoConfig defines the target repository settings.
type RepoConfig struct {
	URL            string `yaml:"url"`              // Git clone URL (HTTPS), required
	Branch         string `yaml:"branch"`           // Target branch, default "main"
	WorkDir        string `yaml:"work_dir"`         // Local clone directory, default "/tmp/autobacklog"
	PRBranchPrefix string `yaml:"pr_branch_prefix"` // Prefix for PR branches, default "autobacklog"
}

// GitHubConfig holds GitHub authentication and integration settings.
type GitHubConfig struct {
	PAT          string `yaml:"pat"`            // GitHub PAT (inline)
	PATFile      string `yaml:"pat_file"`       // Path to file containing PAT
	AutoMerge    bool   `yaml:"auto_merge"`     // Enable auto-merge after CI passes
	CreateIssues bool   `yaml:"create_issues"`  // Create GitHub issues for new backlog items
	IssueLabel   string `yaml:"issue_label"`    // Label for importing/creating issues, default "autobacklog"
	PRFollowUp   bool   `yaml:"pr_follow_up"`   // Auto-address PR review comments
	MaxFollowUps int    `yaml:"max_follow_ups"` // Max PR follow-up iterations per item (0 = unlimited)
}

// ClaudeConfig configures the Claude Code CLI integration.
type ClaudeConfig struct {
	Binary                    string        `yaml:"binary"`                      // Path to claude CLI binary, default "claude"
	Model                     string        `yaml:"model"`                       // Model to use (sonnet, opus, haiku), default "sonnet"
	MaxBudgetPerCall          float64       `yaml:"max_budget_per_call"`         // USD budget cap per CLI invocation, default 10.00
	MaxBudgetTotal            float64       `yaml:"max_budget_total"`            // USD total budget across all invocations, default 100.00
	Timeout                   time.Duration `yaml:"timeout"`                     // Timeout per invocation, default 10m
	DangerouslySkipPermissions bool         `yaml:"dangerously_skip_permissions"` // Pass --dangerously-skip-permissions to Claude CLI
}

// BacklogConfig controls backlog prioritization and lifecycle.
type BacklogConfig struct {
	HighThreshold   int  `yaml:"high_threshold"`   // Min high items to trigger implementation, default 1
	MediumThreshold int  `yaml:"medium_threshold"` // Min medium items to trigger batch, default 3
	LowThreshold    int  `yaml:"low_threshold"`    // Min low items to trigger batch, default 5
	MaxPerCycle     int  `yaml:"max_per_cycle"`     // Max items to implement per cycle, default 5
	MaxConcurrent   int  `yaml:"max_concurrent"`    // Max concurrent implementations (requires worktrees), default 1
	StaleDays       int  `yaml:"stale_days"`        // Days before cleaning terminal items, default 30
	BatchImplement  bool `yaml:"batch_implement"`   // Implement all selected items in one Claude session
}

// TestingConfig controls test detection and execution.
type TestingConfig struct {
	AutoDetect      bool          `yaml:"auto_detect"`      // Auto-detect test framework, default true
	OverrideCommand string        `yaml:"override_command"` // Override test command (bypasses auto-detection)
	Timeout         time.Duration `yaml:"timeout"`          // Test execution timeout, default 5m
	MaxRetries      int           `yaml:"max_retries"`      // Max fix attempts when tests fail, default 3
}

// DaemonConfig controls the continuous daemon loop.
type DaemonConfig struct {
	Interval   time.Duration `yaml:"interval"`    // Time between cycles, default 1h
	QuietStart string        `yaml:"quiet_start"` // Start of quiet hours, "HH:MM" format
	QuietEnd   string        `yaml:"quiet_end"`   // End of quiet hours, "HH:MM" format
}

// NotificationsConfig controls email notification delivery.
type NotificationsConfig struct {
	Enabled    bool           `yaml:"enabled"`    // Enable email notifications
	SMTP       SMTPConfig     `yaml:"smtp"`       // SMTP server settings
	Recipients []string       `yaml:"recipients"` // Recipient email addresses
	Events     EventsConfig   `yaml:"events"`     // Per-event toggles
}

// SMTPConfig holds SMTP connection settings for notifications.
type SMTPConfig struct {
	Host     string `yaml:"host"`     // SMTP server hostname
	Port     int    `yaml:"port"`     // SMTP port, default 587
	Username string `yaml:"username"` // SMTP username
	Password string `yaml:"password"` // SMTP password
	From     string `yaml:"from"`     // Sender email address
}

// EventsConfig toggles individual notification events.
type EventsConfig struct {
	OnCycleComplete bool `yaml:"on_cycle_complete"` // Notify on cycle completion
	OnStuck         bool `yaml:"on_stuck"`          // Notify when item is stuck
	OnOutOfTokens   bool `yaml:"on_out_of_tokens"`  // Notify when budget exceeded
	OnPRCreated     bool `yaml:"on_pr_created"`     // Notify when PR is created
	OnError         bool `yaml:"on_error"`          // Notify on unexpected errors
}

// LoggingConfig controls structured logging output.
type LoggingConfig struct {
	Level  string `yaml:"level"`  // Log level: "debug", "info", "warn", "error"; default "info"
	File   string `yaml:"file"`   // Optional log file path (also logs to stderr)
	Format string `yaml:"format"` // Output format: "text" or "json"; default "text"
}

// WebUIConfig controls the optional real-time web status UI.
type WebUIConfig struct {
	Port int `yaml:"port"` // HTTP port for web UI; 0 = disabled (default)
}
