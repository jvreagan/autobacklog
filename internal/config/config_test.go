package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInterpolateEnvVars(t *testing.T) {
	os.Setenv("TEST_AB_VAR", "hello")
	defer os.Unsetenv("TEST_AB_VAR")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "value=${TEST_AB_VAR}", "value=hello"},
		{"multiple", "${TEST_AB_VAR}/${TEST_AB_VAR}", "hello/hello"},
		{"missing var kept", "${NONEXISTENT_AB_VAR}", "${NONEXISTENT_AB_VAR}"},
		{"no vars", "plain text", "plain text"},
		{"mixed", "a=${TEST_AB_VAR} b=${NONEXISTENT_AB_VAR}", "a=hello b=${NONEXISTENT_AB_VAR}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpolateEnvVars(tt.input)
			if got != tt.want {
				t.Errorf("interpolateEnvVars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Repo.Branch != "main" {
		t.Errorf("default branch = %q, want %q", cfg.Repo.Branch, "main")
	}
	if cfg.Claude.Model != "sonnet" {
		t.Errorf("default model = %q, want %q", cfg.Claude.Model, "sonnet")
	}
	if cfg.Claude.MaxBudgetPerCall != 10.0 {
		t.Errorf("default max_budget_per_call = %f, want 10.0", cfg.Claude.MaxBudgetPerCall)
	}
	if cfg.Backlog.HighThreshold != 1 {
		t.Errorf("default high_threshold = %d, want 1", cfg.Backlog.HighThreshold)
	}
	if cfg.Backlog.MediumThreshold != 3 {
		t.Errorf("default medium_threshold = %d, want 3", cfg.Backlog.MediumThreshold)
	}
	if cfg.Backlog.LowThreshold != 5 {
		t.Errorf("default low_threshold = %d, want 5", cfg.Backlog.LowThreshold)
	}
	if cfg.Mode != "oneshot" {
		t.Errorf("default mode = %q, want %q", cfg.Mode, "oneshot")
	}
	if cfg.HelperMode != "buildbacklog" {
		t.Errorf("default helper_mode = %q, want %q", cfg.HelperMode, "buildbacklog")
	}
	if cfg.Testing.MaxRetries != 3 {
		t.Errorf("default max_retries = %d, want 3", cfg.Testing.MaxRetries)
	}
	if cfg.Daemon.Interval != time.Hour {
		t.Errorf("default interval = %v, want 1h", cfg.Daemon.Interval)
	}
}

func TestApplyDefaultsPreservesExisting(t *testing.T) {
	cfg := &Config{
		Repo: RepoConfig{Branch: "develop"},
		Claude: ClaudeConfig{Model: "opus"},
	}
	applyDefaults(cfg)

	if cfg.Repo.Branch != "develop" {
		t.Errorf("branch overwritten, got %q want %q", cfg.Repo.Branch, "develop")
	}
	if cfg.Claude.Model != "opus" {
		t.Errorf("model overwritten, got %q want %q", cfg.Claude.Model, "opus")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "missing repo url",
			cfg:     Config{Mode: "oneshot", Logging: LoggingConfig{Level: "info", Format: "text"}},
			wantErr: true,
		},
		{
			name:    "invalid mode",
			cfg:     Config{Repo: RepoConfig{URL: "https://example.com"}, Mode: "invalid", Logging: LoggingConfig{Level: "info", Format: "text"}},
			wantErr: true,
		},
		{
			name:    "invalid log level",
			cfg:     Config{Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog", Logging: LoggingConfig{Level: "invalid", Format: "text"}},
			wantErr: true,
		},
		{
			name:    "invalid log format",
			cfg:     Config{Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog", Logging: LoggingConfig{Level: "info", Format: "xml"}},
			wantErr: true,
		},
		{
			name:    "invalid helper_mode",
			cfg:     Config{Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "invalid", Logging: LoggingConfig{Level: "info", Format: "text"}},
			wantErr: true,
		},
		{
			name: "notifications enabled without smtp host",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging:       LoggingConfig{Level: "info", Format: "text"},
				Notifications: NotificationsConfig{Enabled: true, Recipients: []string{"a@b.com"}},
			},
			wantErr: true,
		},
		{
			name: "notifications enabled without recipients",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging:       LoggingConfig{Level: "info", Format: "text"},
				Notifications: NotificationsConfig{Enabled: true, SMTP: SMTPConfig{Host: "smtp.example.com"}},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "daemon", HelperMode: "burndown",
				Logging: LoggingConfig{Level: "debug", Format: "json"},
			},
			wantErr: false,
		},
		{
			name: "valid webui port 0",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging: LoggingConfig{Level: "info", Format: "text"},
				WebUI:   WebUIConfig{Port: 0},
			},
			wantErr: false,
		},
		{
			name: "valid webui port 8080",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging: LoggingConfig{Level: "info", Format: "text"},
				WebUI:   WebUIConfig{Port: 8080},
			},
			wantErr: false,
		},
		{
			name: "valid webui port 65535",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging: LoggingConfig{Level: "info", Format: "text"},
				WebUI:   WebUIConfig{Port: 65535},
			},
			wantErr: false,
		},
		{
			name: "invalid webui port negative",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging: LoggingConfig{Level: "info", Format: "text"},
				WebUI:   WebUIConfig{Port: -1},
			},
			wantErr: true,
		},
		{
			name: "invalid webui port too high",
			cfg: Config{
				Repo: RepoConfig{URL: "https://example.com"}, Mode: "oneshot", HelperMode: "buildbacklog",
				Logging: LoggingConfig{Level: "info", Format: "text"},
				WebUI:   WebUIConfig{Port: 65536},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(&tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	os.Setenv("TEST_AB_TOKEN", "ghp_test123")
	defer os.Unsetenv("TEST_AB_TOKEN")

	yaml := `
repo:
  url: "https://github.com/test/repo.git"
  branch: "develop"
github:
  pat: "${TEST_AB_TOKEN}"
mode: "daemon"
daemon:
  interval: "30m"
logging:
  level: "debug"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Repo.URL != "https://github.com/test/repo.git" {
		t.Errorf("repo.url = %q", cfg.Repo.URL)
	}
	if cfg.Repo.Branch != "develop" {
		t.Errorf("repo.branch = %q, want develop", cfg.Repo.Branch)
	}
	if cfg.GitHub.PAT != "ghp_test123" {
		t.Errorf("github.pat = %q, want ghp_test123", cfg.GitHub.PAT)
	}
	if cfg.Mode != "daemon" {
		t.Errorf("mode = %q, want daemon", cfg.Mode)
	}
	if cfg.Daemon.Interval != 30*time.Minute {
		t.Errorf("daemon.interval = %v, want 30m", cfg.Daemon.Interval)
	}
	// Defaults should be applied
	if cfg.Claude.Model != "sonnet" {
		t.Errorf("claude.model default not applied, got %q", cfg.Claude.Model)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() should error on missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("{{invalid"), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("Load() should error on invalid YAML")
	}
}

// #205: test warnLiteralSecrets by calling the function directly
// instead of re-implementing the detection logic.
func TestWarnLiteralSecrets(t *testing.T) {
	tests := []struct {
		name    string
		rawYAML string
	}{
		{
			name: "literal password does not panic",
			rawYAML: `
notifications:
  smtp:
    password: "hunter2"
`,
		},
		{
			name: "env var reference does not panic",
			rawYAML: `
notifications:
  smtp:
    password: "${SMTP_PASSWORD}"
`,
		},
		{
			name: "empty password does not panic",
			rawYAML: `
notifications:
  smtp:
    host: "smtp.example.com"
`,
		},
		{
			name: "literal GitHub PAT does not panic",
			rawYAML: `
github:
  pat: "ghp_secret123"
`,
		},
		{
			name: "env var GitHub PAT does not panic",
			rawYAML: `
github:
  pat: "${GITHUB_TOKEN}"
`,
		},
		{
			name: "invalid YAML does not panic",
			rawYAML: `{{invalid`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify warnLiteralSecrets doesn't panic for any input.
			// It only emits slog warnings so we call it directly.
			warnLiteralSecrets(tt.rawYAML)
		})
	}
}

// Test envVarRef regex classification directly.
func TestEnvVarRef(t *testing.T) {
	tests := []struct {
		input string
		match bool
	}{
		{"${GITHUB_TOKEN}", true},
		{"${SMTP_PASSWORD}", true},
		{"ghp_secret123", false},
		{"prefix_${VAR}", false}, // envVarRef matches only whole-string refs
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := envVarRef.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("envVarRef.MatchString(%q) = %v, want %v", tt.input, got, tt.match)
			}
		})
	}
}

func TestResolveGitHubPAT(t *testing.T) {
	t.Run("inline PAT", func(t *testing.T) {
		cfg := &Config{GitHub: GitHubConfig{PAT: "ghp_inline"}}
		pat, err := cfg.ResolveGitHubPAT()
		if err != nil {
			t.Fatal(err)
		}
		if pat != "ghp_inline" {
			t.Errorf("got %q, want ghp_inline", pat)
		}
	})

	t.Run("PAT from file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "pat")
		os.WriteFile(f, []byte("ghp_fromfile\n"), 0644)

		cfg := &Config{GitHub: GitHubConfig{PATFile: f}}
		pat, err := cfg.ResolveGitHubPAT()
		if err != nil {
			t.Fatal(err)
		}
		if pat != "ghp_fromfile" {
			t.Errorf("got %q, want ghp_fromfile", pat)
		}
	})

	t.Run("env var fallback", func(t *testing.T) {
		os.Setenv("GITHUB_TOKEN", "ghp_env")
		defer os.Unsetenv("GITHUB_TOKEN")

		cfg := &Config{}
		pat, err := cfg.ResolveGitHubPAT()
		if err != nil {
			t.Fatal(err)
		}
		if pat != "ghp_env" {
			t.Errorf("got %q, want ghp_env", pat)
		}
	})

	t.Run("no PAT configured", func(t *testing.T) {
		os.Unsetenv("GITHUB_TOKEN")
		cfg := &Config{}
		_, err := cfg.ResolveGitHubPAT()
		if err == nil {
			t.Error("should error when no PAT configured")
		}
	})
}

func TestMaxConcurrent_Default(t *testing.T) {
	cfgYAML := `
repo:
  url: https://github.com/test/repo.git
`
	path := filepath.Join(t.TempDir(), "test.yaml")
	os.WriteFile(path, []byte(cfgYAML), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Backlog.MaxConcurrent != 1 {
		t.Errorf("MaxConcurrent = %d, want 1 (default)", cfg.Backlog.MaxConcurrent)
	}
}

func TestMaxConcurrent_Validation(t *testing.T) {
	cfgYAML := `
repo:
  url: https://github.com/test/repo.git
backlog:
  max_concurrent: -1
`
	path := filepath.Join(t.TempDir(), "test.yaml")
	os.WriteFile(path, []byte(cfgYAML), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for negative max_concurrent")
	}
}

func TestMaxConcurrent_Explicit(t *testing.T) {
	cfgYAML := `
repo:
  url: https://github.com/test/repo.git
backlog:
  max_concurrent: 4
`
	path := filepath.Join(t.TempDir(), "test.yaml")
	os.WriteFile(path, []byte(cfgYAML), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Backlog.MaxConcurrent != 4 {
		t.Errorf("MaxConcurrent = %d, want 4", cfg.Backlog.MaxConcurrent)
	}
}

func TestValidate_BatchAndConcurrentMutuallyExclusive(t *testing.T) {
	cfg := Config{
		Repo:       RepoConfig{URL: "https://example.com"},
		Mode:       "oneshot",
		HelperMode: "buildbacklog",
		Logging:    LoggingConfig{Level: "info", Format: "text"},
		Backlog: BacklogConfig{
			BatchImplement: true,
			MaxConcurrent:  2,
		},
	}

	err := validate(&cfg)
	if err == nil {
		t.Fatal("expected error for batch_implement + max_concurrent > 1")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_BatchWithConcurrent1_OK(t *testing.T) {
	cfg := Config{
		Repo:       RepoConfig{URL: "https://example.com"},
		Mode:       "oneshot",
		HelperMode: "buildbacklog",
		Logging:    LoggingConfig{Level: "info", Format: "text"},
		Backlog: BacklogConfig{
			BatchImplement: true,
			MaxConcurrent:  1,
		},
	}

	if err := validate(&cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
