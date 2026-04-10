package config

import (
	"fmt"
	"log/slog"
	"net/url"
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

	// #123: interpolate after YAML parsing to prevent YAML injection via env vars.
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	interpolateConfigEnvVars(cfg)

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

// interpolateConfigEnvVars applies env var interpolation to all string fields
// in the parsed config, avoiding YAML injection (#123).
func interpolateConfigEnvVars(cfg *Config) {
	cfg.Repo.URL = interpolateEnvVars(cfg.Repo.URL)
	cfg.Repo.Branch = interpolateEnvVars(cfg.Repo.Branch)
	cfg.Repo.WorkDir = interpolateEnvVars(cfg.Repo.WorkDir)
	cfg.Repo.PRBranchPrefix = interpolateEnvVars(cfg.Repo.PRBranchPrefix)
	cfg.GitHub.PAT = interpolateEnvVars(cfg.GitHub.PAT)
	cfg.GitHub.PATFile = interpolateEnvVars(cfg.GitHub.PATFile)
	cfg.GitHub.IssueLabel = interpolateEnvVars(cfg.GitHub.IssueLabel)
	cfg.Claude.Binary = interpolateEnvVars(cfg.Claude.Binary)
	cfg.Claude.Model = interpolateEnvVars(cfg.Claude.Model)
	cfg.Notifications.SMTP.Host = interpolateEnvVars(cfg.Notifications.SMTP.Host)
	cfg.Notifications.SMTP.Username = interpolateEnvVars(cfg.Notifications.SMTP.Username)
	cfg.Notifications.SMTP.Password = interpolateEnvVars(cfg.Notifications.SMTP.Password)
	cfg.Notifications.SMTP.From = interpolateEnvVars(cfg.Notifications.SMTP.From)
	for i, r := range cfg.Notifications.Recipients {
		cfg.Notifications.Recipients[i] = interpolateEnvVars(r)
	}
	cfg.Logging.File = interpolateEnvVars(cfg.Logging.File)
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
	// #140: also check GitHub PAT
	if raw.GitHub.PAT != "" && !envVarRef.MatchString(raw.GitHub.PAT) {
		slog.Warn("github.pat is stored as a plain-text literal in the config file; use ${GITHUB_TOKEN} to reference an environment variable instead")
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
	if cfg.Backlog.MaxConcurrent == 0 {
		cfg.Backlog.MaxConcurrent = 1
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

// validQuietTime validates a "HH:MM" time string (#139).
func validQuietTime(s string) bool {
	if s == "" {
		return true
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return false
	}
	// Verify round-trip to catch formats like "25:99"
	return t.Format("15:04") == s
}

func validate(cfg *Config) error {
	if cfg.Repo.URL == "" {
		return fmt.Errorf("repo.url is required")
	}
	// #204: validate repo URL format
	if !strings.HasPrefix(cfg.Repo.URL, "https://") && !strings.HasPrefix(cfg.Repo.URL, "git@") && !strings.HasPrefix(cfg.Repo.URL, "http://") {
		return fmt.Errorf("repo.url must start with https://, http://, or git@, got %q", cfg.Repo.URL)
	}
	if strings.HasPrefix(cfg.Repo.URL, "http://") || strings.HasPrefix(cfg.Repo.URL, "https://") {
		if _, err := url.Parse(cfg.Repo.URL); err != nil {
			return fmt.Errorf("repo.url is not a valid URL: %w", err)
		}
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
	if strings.HasPrefix(cfg.Claude.Binary, "/") || strings.HasPrefix(cfg.Claude.Binary, "~") {
		return fmt.Errorf("claude.binary must be a bare command name (not an absolute path), got %q", cfg.Claude.Binary)
	}
	if cfg.Claude.MaxBudgetPerCall > cfg.Claude.MaxBudgetTotal {
		return fmt.Errorf("claude.max_budget_per_call ($%.2f) must not exceed claude.max_budget_total ($%.2f)", cfg.Claude.MaxBudgetPerCall, cfg.Claude.MaxBudgetTotal)
	}
	if cfg.WebUI.Port < 0 || cfg.WebUI.Port > 65535 {
		return fmt.Errorf("webui.port must be 0-65535, got %d", cfg.WebUI.Port)
	}
	// #138: validate non-negative numeric fields
	if cfg.Claude.MaxBudgetPerCall < 0 {
		return fmt.Errorf("claude.max_budget_per_call must be non-negative, got %.2f", cfg.Claude.MaxBudgetPerCall)
	}
	if cfg.Claude.MaxBudgetTotal < 0 {
		return fmt.Errorf("claude.max_budget_total must be non-negative, got %.2f", cfg.Claude.MaxBudgetTotal)
	}
	if cfg.Backlog.StaleDays < 0 {
		return fmt.Errorf("backlog.stale_days must be non-negative, got %d", cfg.Backlog.StaleDays)
	}
	if cfg.Backlog.MaxPerCycle < 0 {
		return fmt.Errorf("backlog.max_per_cycle must be non-negative, got %d", cfg.Backlog.MaxPerCycle)
	}
	if cfg.Backlog.MaxConcurrent < 0 {
		return fmt.Errorf("backlog.max_concurrent must be non-negative, got %d", cfg.Backlog.MaxConcurrent)
	}
	if cfg.Testing.MaxRetries < 0 {
		return fmt.Errorf("testing.max_retries must be non-negative, got %d", cfg.Testing.MaxRetries)
	}
	if cfg.Notifications.SMTP.Port < 0 {
		return fmt.Errorf("notifications.smtp.port must be non-negative, got %d", cfg.Notifications.SMTP.Port)
	}
	// #139: validate quiet hours format
	if !validQuietTime(cfg.Daemon.QuietStart) {
		return fmt.Errorf("daemon.quiet_start must be in HH:MM format, got %q", cfg.Daemon.QuietStart)
	}
	if !validQuietTime(cfg.Daemon.QuietEnd) {
		return fmt.Errorf("daemon.quiet_end must be in HH:MM format, got %q", cfg.Daemon.QuietEnd)
	}
	if cfg.Notifications.Enabled {
		if cfg.Notifications.SMTP.Host == "" {
			return fmt.Errorf("notifications.smtp.host is required when notifications are enabled")
		}
		if len(cfg.Notifications.Recipients) == 0 {
			return fmt.Errorf("notifications.recipients is required when notifications are enabled")
		}
	}
	// #206: warn about unresolved env var placeholders
	warnUnresolvedVars(cfg)
	return nil
}

// warnUnresolvedVars logs warnings for any ${VAR} patterns still present in
// critical config fields after interpolation (#206).
func warnUnresolvedVars(cfg *Config) {
	check := func(field, value string) {
		if envVarPattern.MatchString(value) {
			slog.Warn("unresolved environment variable in config", "field", field, "value", value)
		}
	}
	check("github.pat", cfg.GitHub.PAT)
	check("notifications.smtp.password", cfg.Notifications.SMTP.Password)
	check("notifications.smtp.username", cfg.Notifications.SMTP.Username)
}

// ResolveGitHubPAT returns the PAT from config, reading from file if pat_file is set.
func (cfg *Config) ResolveGitHubPAT() (string, error) {
	if cfg.GitHub.PAT != "" {
		return cfg.GitHub.PAT, nil
	}
	if cfg.GitHub.PATFile != "" {
		// #141: check file permissions
		info, err := os.Stat(cfg.GitHub.PATFile)
		if err != nil {
			return "", fmt.Errorf("reading PAT file: %w", err)
		}
		if perm := info.Mode().Perm(); perm&0077 != 0 {
			slog.Warn("PAT file has overly permissive permissions", "path", cfg.GitHub.PATFile, "mode", fmt.Sprintf("%04o", perm))
		}
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
