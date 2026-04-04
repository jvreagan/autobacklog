package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads a YAML config file, interpolates environment variables, and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	expanded := interpolateEnvVars(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// interpolateEnvVars replaces ${VAR} patterns with environment variable values.
func interpolateEnvVars(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

func applyDefaults(cfg *Config) {
	if cfg.Repo.Branch == "" {
		cfg.Repo.Branch = "main"
	}
	if cfg.Repo.WorkDir == "" {
		cfg.Repo.WorkDir = "/tmp/autobacklog"
	}
	if cfg.Repo.PRBranchPrefix == "" {
		cfg.Repo.PRBranchPrefix = "autobacklog"
	}
	if cfg.Claude.Binary == "" {
		cfg.Claude.Binary = "claude"
	}
	if cfg.Claude.Model == "" {
		cfg.Claude.Model = "sonnet"
	}
	if cfg.Claude.MaxBudgetPerCall == 0 {
		cfg.Claude.MaxBudgetPerCall = 10.0
	}
	if cfg.Claude.MaxBudgetTotal == 0 {
		cfg.Claude.MaxBudgetTotal = 100.0
	}
	if cfg.Claude.Timeout == 0 {
		cfg.Claude.Timeout = 10 * time.Minute
	}
	if cfg.Backlog.HighThreshold == 0 {
		cfg.Backlog.HighThreshold = 1
	}
	if cfg.Backlog.MediumThreshold == 0 {
		cfg.Backlog.MediumThreshold = 3
	}
	if cfg.Backlog.LowThreshold == 0 {
		cfg.Backlog.LowThreshold = 5
	}
	if cfg.Backlog.MaxPerCycle == 0 {
		cfg.Backlog.MaxPerCycle = 5
	}
	if cfg.Backlog.StaleDays == 0 {
		cfg.Backlog.StaleDays = 30
	}
	if cfg.Testing.Timeout == 0 {
		cfg.Testing.Timeout = 5 * time.Minute
	}
	if cfg.Testing.MaxRetries == 0 {
		cfg.Testing.MaxRetries = 3
	}
	if cfg.Testing.AutoDetect == false && cfg.Testing.OverrideCommand == "" {
		cfg.Testing.AutoDetect = true
	}
	if cfg.Mode == "" {
		cfg.Mode = "oneshot"
	}
	if cfg.HelperMode == "" {
		cfg.HelperMode = "buildbacklog"
	}
	if cfg.Daemon.Interval == 0 {
		cfg.Daemon.Interval = 1 * time.Hour
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "text"
	}
	if cfg.Notifications.SMTP.Port == 0 {
		cfg.Notifications.SMTP.Port = 587
	}
	if cfg.GitHub.IssueLabel == "" {
		cfg.GitHub.IssueLabel = "autobacklog"
	}
}

func validate(cfg *Config) error {
	if cfg.Repo.URL == "" {
		return fmt.Errorf("repo.url is required")
	}
	if cfg.Mode != "oneshot" && cfg.Mode != "daemon" {
		return fmt.Errorf("mode must be 'oneshot' or 'daemon', got %q", cfg.Mode)
	}
	if cfg.HelperMode != "buildbacklog" && cfg.HelperMode != "burndown" {
		return fmt.Errorf("helper_mode must be 'buildbacklog' or 'burndown', got %q", cfg.HelperMode)
	}
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.Logging.Level)] {
		return fmt.Errorf("logging.level must be debug/info/warn/error, got %q", cfg.Logging.Level)
	}
	if cfg.Notifications.Enabled {
		if cfg.Notifications.SMTP.Host == "" {
			return fmt.Errorf("notifications.smtp.host is required when notifications are enabled")
		}
		if len(cfg.Notifications.Recipients) == 0 {
			return fmt.Errorf("notifications.recipients is required when notifications are enabled")
		}
	}
	return nil
}

// ResolveGitHubPAT returns the PAT from config, reading from file if pat_file is set.
func (cfg *Config) ResolveGitHubPAT() (string, error) {
	if cfg.GitHub.PAT != "" {
		return cfg.GitHub.PAT, nil
	}
	if cfg.GitHub.PATFile != "" {
		data, err := os.ReadFile(cfg.GitHub.PATFile)
		if err != nil {
			return "", fmt.Errorf("reading PAT file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	// Try GITHUB_TOKEN env var as fallback
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("no GitHub PAT configured (set github.pat, github.pat_file, or GITHUB_TOKEN)")
}
