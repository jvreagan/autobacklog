package cli

import (
	"testing"
	"time"

	"github.com/jvreagan/autobacklog/internal/config"
)

// #185: test sanitizeConfig function
func TestSanitizeConfig(t *testing.T) {
	cfg := &config.Config{
		Repo: config.RepoConfig{
			URL:            "https://github.com/org/repo.git",
			Branch:         "main",
			WorkDir:        "/tmp/work",
			PRBranchPrefix: "autobacklog",
		},
		GitHub: config.GitHubConfig{
			PAT:        "ghp_secret_token",
			AutoMerge:  true,
			IssueLabel: "autobacklog",
		},
		Claude: config.ClaudeConfig{
			Model:            "sonnet",
			MaxBudgetPerCall: 5.0,
			MaxBudgetTotal:   50.0,
			Timeout:          10 * time.Minute,
		},
		Mode:       "oneshot",
		HelperMode: "buildbacklog",
		Notifications: config.NotificationsConfig{
			Enabled: true,
			SMTP: config.SMTPConfig{
				Host:     "smtp.example.com",
				Port:     587,
				Username: "user@example.com",
				Password: "smtp_secret",
				From:     "bot@example.com",
			},
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		WebUI: config.WebUIConfig{Port: 8080},
	}

	result := sanitizeConfig(cfg)

	// PAT should not appear anywhere
	github := result["github"].(map[string]any)
	if _, hasPAT := github["pat"]; hasPAT {
		t.Error("sanitizeConfig should not include github.pat")
	}

	// SMTP secrets should be redacted (#211)
	notif := result["notifications"].(map[string]any)
	smtp := notif["smtp"].(map[string]any)
	if smtp["password"] != "***" {
		t.Errorf("smtp.password should be redacted, got %q", smtp["password"])
	}
	if smtp["host"] != "***" {
		t.Errorf("smtp.host should be redacted, got %q", smtp["host"])
	}
	if smtp["from"] != "***" {
		t.Errorf("smtp.from should be redacted, got %q", smtp["from"])
	}

	// Non-secret fields should be preserved
	repo := result["repo"].(map[string]any)
	if repo["url"] != "https://github.com/org/repo.git" {
		t.Errorf("repo.url = %q", repo["url"])
	}
	if result["mode"] != "oneshot" {
		t.Errorf("mode = %q", result["mode"])
	}
}
