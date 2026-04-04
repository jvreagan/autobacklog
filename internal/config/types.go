package config

import "time"

// Config is the top-level configuration for autobacklog.
type Config struct {
	Repo          RepoConfig          `yaml:"repo"`
	GitHub        GitHubConfig        `yaml:"github"`
	Claude        ClaudeConfig        `yaml:"claude"`
	Backlog       BacklogConfig       `yaml:"backlog"`
	Testing       TestingConfig       `yaml:"testing"`
	Mode          string              `yaml:"mode"` // "oneshot" or "daemon"
	Daemon        DaemonConfig        `yaml:"daemon"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Logging       LoggingConfig       `yaml:"logging"`
}

type RepoConfig struct {
	URL            string `yaml:"url"`
	Branch         string `yaml:"branch"`
	WorkDir        string `yaml:"work_dir"`
	PRBranchPrefix string `yaml:"pr_branch_prefix"`
}

type GitHubConfig struct {
	PAT          string `yaml:"pat"`
	PATFile      string `yaml:"pat_file"`
	AutoMerge    bool   `yaml:"auto_merge"`
	CreateIssues bool   `yaml:"create_issues"`
	IssueLabel   string `yaml:"issue_label"`
}

type ClaudeConfig struct {
	Binary                    string        `yaml:"binary"`
	Model                     string        `yaml:"model"`
	MaxBudgetPerCall          float64       `yaml:"max_budget_per_call"`
	MaxBudgetTotal            float64       `yaml:"max_budget_total"`
	Timeout                   time.Duration `yaml:"timeout"`
	DangerouslySkipPermissions bool         `yaml:"dangerously_skip_permissions"`
}

type BacklogConfig struct {
	HighThreshold   int `yaml:"high_threshold"`
	MediumThreshold int `yaml:"medium_threshold"`
	LowThreshold    int `yaml:"low_threshold"`
	MaxPerCycle     int `yaml:"max_per_cycle"`
	StaleDays       int `yaml:"stale_days"`
}

type TestingConfig struct {
	AutoDetect      bool          `yaml:"auto_detect"`
	OverrideCommand string        `yaml:"override_command"`
	Timeout         time.Duration `yaml:"timeout"`
	MaxRetries      int           `yaml:"max_retries"`
}

type DaemonConfig struct {
	Interval   time.Duration `yaml:"interval"`
	QuietStart string        `yaml:"quiet_start"` // "HH:MM" format
	QuietEnd   string        `yaml:"quiet_end"`
}

type NotificationsConfig struct {
	Enabled    bool           `yaml:"enabled"`
	SMTP       SMTPConfig     `yaml:"smtp"`
	Recipients []string       `yaml:"recipients"`
	Events     EventsConfig   `yaml:"events"`
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
}

type EventsConfig struct {
	OnCycleComplete bool `yaml:"on_cycle_complete"`
	OnStuck         bool `yaml:"on_stuck"`
	OnOutOfTokens   bool `yaml:"on_out_of_tokens"`
	OnPRCreated     bool `yaml:"on_pr_created"`
	OnError         bool `yaml:"on_error"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`  // "debug", "info", "warn", "error"
	File   string `yaml:"file"`
	Format string `yaml:"format"` // "text" or "json"
}
