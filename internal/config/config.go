package config

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Default configuration values.
const (
	DefaultBranch          = "main"
	DefaultWorkDir         = "/tmp/autobacklog"
	DefaultPRBranchPrefix  = "autobacklog"
	DefaultBinary          = "claude"
	DefaultModel           = "sonnet"
	DefaultBudgetPerCall   = 10.0
	DefaultBudgetTotal     = 100.0
	DefaultHighThreshold   = 1
	DefaultMediumThreshold = 3
	DefaultLowThreshold    = 5
	DefaultMaxPerCycle     = 5
	DefaultStaleDays       = 30
	DefaultMaxRetries      = 3
	DefaultMode            = "oneshot"
	DefaultHelperMode      = "buildbacklog"
	DefaultLogLevel        = "info"
	DefaultLogFormat       = "text"
	DefaultSMTPPort        = 587
	DefaultIssueLabel      = "autobacklog"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads a YAML config file, interpolates environment variables, and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	warnLiteralSecrets(string(data))

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

// envVarRef matches a value that is entirely a single ${VAR} reference.
var envVarRef = regexp.MustCompile(`^\$\{[^}]+\}$`)

// warnLiteralSecrets logs a warning if any sensitive config fields are set as
// plain-text literals rather than ${VAR} env var references in the raw YAML.
func warnLiteralSecrets(rawYAML string) {
	raw := &Config{}
	if err := yaml.Unmarshal([]byte(rawYAML), raw); err != nil {
		return // YAML errors are reported later during the real parse
	}
	if raw.Notifications.SMTP.Password != "" && !envVarRef.MatchString(raw.Notifications.SMTP.Password) {
		slog.Warn("notifications.smtp.password is stored as a plain-text literal in the config file; use ${SMTP_PASSWORD} to reference an environment variable instead")
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Repo.Branch == "" {
		cfg.Repo.Branch = DefaultBranch
	}
	if cfg.Repo.WorkDir == "" {
		cfg.Repo.WorkDir = DefaultWorkDir
	}
	if cfg.Repo.PRBranchPrefix == "" {
		cfg.Repo.PRBranchPrefix = DefaultPRBranchPrefix
	}
	if cfg.Claude.Binary == "" {
		cfg.Claude.Binary = DefaultBinary
	}
	if cfg.Claude.Model == "" {
		cfg.Claude.Model = DefaultModel
	}
	if cfg.Claude.MaxBudgetPerCall == 0 {
		cfg.Claude.MaxBudgetPerCall = DefaultBudgetPerCall
	}
	if cfg.Claude.MaxBudgetTotal == 0 {
		cfg.Claude.MaxBudgetTotal = DefaultBudgetTotal
	}
	if cfg.Claude.Timeout == 0 {
		cfg.Claude.Timeout = 10 * time.Minute
	}
	if cfg.Backlog.HighThreshold == 0 {
		cfg.Backlog.HighThreshold = DefaultHighThreshold
	}
	if cfg.Backlog.MediumThreshold == 0 {
		cfg.Backlog.MediumThreshold = DefaultMediumThreshold
	}
	if cfg.Backlog.LowThreshold == 0 {
		cfg.Backlog.LowThreshold = DefaultLowThreshold
	}
	if cfg.Backlog.MaxPerCycle == 0 {
		cfg.Backlog.MaxPerCycle = DefaultMaxPerCycle
	}
	if cfg.Backlog.StaleDays == 0 {
		cfg.Backlog.StaleDays = DefaultStaleDays
	}
	if cfg.Testing.Timeout == 0 {
		cfg.Testing.Timeout = 5 * time.Minute
	}
	if cfg.Testing.MaxRetries == 0 {
		cfg.Testing.MaxRetries = DefaultMaxRetries
	}
	if !cfg.Testing.AutoDetect && cfg.Testing.OverrideCommand == "" {
		cfg.Testing.AutoDetect = true
	}
	if cfg.Mode == "" {
		cfg.Mode = DefaultMode
	}
	if cfg.HelperMode == "" {
		cfg.HelperMode = DefaultHelperMode
	}
	if cfg.Daemon.Interval == 0 {
		cfg.Daemon.Interval = 1 * time.Hour
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = DefaultLogLevel
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = DefaultLogFormat
	}
	if cfg.Notifications.SMTP.Port == 0 {
		cfg.Notifications.SMTP.Port = DefaultSMTPPort
	}
	if cfg.GitHub.IssueLabel == "" {
		cfg.GitHub.IssueLabel = DefaultIssueLabel
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
	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[strings.ToLower(cfg.Logging.Format)] {
		return fmt.Errorf("logging.format must be 'text' or 'json', got %q", cfg.Logging.Format)
	}
	if strings.Contains(cfg.Claude.Binary, "..") {
		return fmt.Errorf("claude.binary must not contain path traversal (..), got %q", cfg.Claude.Binary)
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
