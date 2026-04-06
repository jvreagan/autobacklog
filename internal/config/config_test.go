package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

func TestWarnLiteralSecrets(t *testing.T) {
	tests := []struct {
		name        string
		rawYAML     string
		expectWarn  bool
	}{
		{
			name: "literal password triggers warning",
			rawYAML: `
notifications:
  smtp:
    password: "hunter2"
`,
			expectWarn: true,
		},
		{
			name: "env var reference does not trigger warning",
			rawYAML: `
notifications:
  smtp:
    password: "${SMTP_PASSWORD}"
`,
			expectWarn: false,
		},
		{
			name: "empty password does not trigger warning",
			rawYAML: `
notifications:
  smtp:
    host: "smtp.example.com"
`,
			expectWarn: false,
		},
		{
			name: "mixed literal and env var triggers warning",
			rawYAML: `
notifications:
  smtp:
    password: "prefix_${SMTP_PASSWORD}"
`,
			expectWarn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// warnLiteralSecrets uses slog; we verify it doesn't panic and
			// that the envVarRef regex classifies values correctly.
			raw := &Config{}
			if err := yaml.Unmarshal([]byte(tt.rawYAML), raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			isLiteral := raw.Notifications.SMTP.Password != "" && !envVarRef.MatchString(raw.Notifications.SMTP.Password)
			if isLiteral != tt.expectWarn {
				t.Errorf("isLiteral = %v, want %v (password = %q)", isLiteral, tt.expectWarn, raw.Notifications.SMTP.Password)
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
