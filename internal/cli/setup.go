package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jamesreagan/autobacklog/internal/app"
	"github.com/jamesreagan/autobacklog/internal/backlog"
	"github.com/jamesreagan/autobacklog/internal/config"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/logging"
	"github.com/jamesreagan/autobacklog/internal/notify"
	"github.com/jamesreagan/autobacklog/internal/webui"
)

// setupResult bundles the objects created during CLI setup.
type setupResult struct {
	cfg          *config.Config
	store        *backlog.SQLiteStore
	orchestrator *app.App
	log          *slog.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	hub          *webui.Hub
	uiServer     *webui.Server
}

// setup loads config, opens the DB, sets up auth, and creates the orchestrator.
// Callers must defer result.store.Close(), result.cancel(), and logging.Cleanup().
func setup() (*setupResult, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if verbose {
		cfg.Logging.Level = "debug"
	}
	if helperMode != "" {
		cfg.HelperMode = helperMode
	}

	// CLI flag overrides config for webui port
	if webuiPort != 0 {
		cfg.WebUI.Port = webuiPort
	}

	// Start web UI server before logging so port conflicts fail fast.
	// orchestratorPtr is set after the orchestrator is created so the
	// statsFn closure can reference it via deferred binding.
	var orchestratorPtr **app.App
	var storePtr *backlog.SQLiteStore
	var hub *webui.Hub
	var uiServer *webui.Server
	if cfg.WebUI.Port > 0 {
		hub = webui.NewHub(1000)
		bootLog := slog.New(slog.NewTextHandler(os.Stderr, nil))
		var appRef *app.App
		orchestratorPtr = &appRef
		uiServer = webui.NewServer(cfg.WebUI.Port, hub, func() any {
			return sanitizeConfig(cfg)
		}, func() any {
			if *orchestratorPtr == nil {
				return nil
			}
			return (*orchestratorPtr).LastStats()
		}, func() any {
			if storePtr == nil {
				return nil
			}
			since := time.Now().UTC().AddDate(0, 0, -30)
			records, err := storePtr.ListCycles(context.Background(), cfg.Repo.URL, since)
			if err != nil {
				return nil
			}
			return records
		}, bootLog)
		if err := uiServer.Start(); err != nil {
			return nil, err
		}
	}

	// Set up logging, optionally teeing to the web UI hub.
	var log *slog.Logger
	if hub != nil {
		logWriter := webui.NewTeeWriter(os.Stderr, hub, webui.EventLog)
		log, err = logging.SetupWithExtraWriter(cfg.Logging, logWriter)
	} else {
		log, err = logging.Setup(cfg.Logging)
	}
	if err != nil {
		if uiServer != nil {
			uiServer.Shutdown(context.Background())
		}
		return nil, fmt.Errorf("setting up logging: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determining home directory: %w", err)
	}
	dbPath := filepath.Join(home, ".autobacklog", "backlog.db")
	store, err := backlog.NewSQLiteStore(dbPath)
	if err != nil {
		// #155: shut down UI server if it was already started
		if uiServer != nil {
			uiServer.Shutdown(context.Background())
		}
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Wire up deferred store reference for the cyclesFn closure.
	storePtr = store

	// Create cancellable context before any work so all operations
	// (including auth setup) respect shutdown signals.
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			log.Info("received shutdown signal, finishing current operation...")
			cancel()
		case <-ctx.Done():
			signal.Stop(sigCh)
			return
		}
		// Wait for a second signal → force quit
		select {
		case <-sigCh:
			log.Warn("received second signal, forcing exit")
			os.Exit(1)
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	if !dryRun {
		pat, err := cfg.ResolveGitHubPAT()
		if err != nil {
			log.Warn("cannot resolve GitHub PAT", "error", err)
		}
		if err := gh.SetupAuth(ctx, pat, log); err != nil {
			log.Warn("GitHub auth setup failed", "error", err)
		}
	}

	var notifier notify.Notifier
	if cfg.Notifications.Enabled {
		notifier = notify.NewEmailNotifier(cfg.Notifications, log)
	} else {
		notifier = notify.NoopNotifier{}
	}

	// The orchestrator is NOT safe for concurrent use. The daemon loop calls
	// RunCycle sequentially — never call it from multiple goroutines.
	orchestrator, err := app.New(cfg, store, notifier, log, dryRun)
	if err != nil {
		store.Close()
		cancel()
		// #155: shut down UI server on orchestrator creation failure
		if uiServer != nil {
			uiServer.Shutdown(context.Background())
		}
		return nil, fmt.Errorf("creating orchestrator: %w", err)
	}

	// Wire up deferred orchestrator reference for the statsFn closure.
	if orchestratorPtr != nil {
		*orchestratorPtr = orchestrator
	}

	// Tee Claude CLI output to the web UI hub if enabled.
	if hub != nil {
		claudeTee := webui.NewTeeWriter(os.Stdout, hub, webui.EventClaude)
		orchestrator.SetClaudeOutputWriters(claudeTee, os.Stderr)
	}

	return &setupResult{
		cfg:          cfg,
		store:        store,
		orchestrator: orchestrator,
		log:          log,
		ctx:          ctx,
		cancel:       cancel,
		hub:          hub,
		uiServer:     uiServer,
	}, nil
}

// sanitizeConfig returns a copy of the config with secrets redacted.
func sanitizeConfig(cfg *config.Config) map[string]any {
	redact := func(s string) string {
		if s == "" {
			return ""
		}
		return "***"
	}
	return map[string]any{
		"repo": map[string]any{
			"url":              cfg.Repo.URL,
			"branch":           cfg.Repo.Branch,
			"work_dir":         cfg.Repo.WorkDir,
			"pr_branch_prefix": cfg.Repo.PRBranchPrefix,
		},
		"github": map[string]any{
			"auto_merge":    cfg.GitHub.AutoMerge,
			"create_issues": cfg.GitHub.CreateIssues,
			"issue_label":   cfg.GitHub.IssueLabel,
		},
		"claude": map[string]any{
			"model":              cfg.Claude.Model,
			"max_budget_per_call": cfg.Claude.MaxBudgetPerCall,
			"max_budget_total":    cfg.Claude.MaxBudgetTotal,
			"timeout":            cfg.Claude.Timeout.String(),
		},
		"backlog": map[string]any{
			"high_threshold":   cfg.Backlog.HighThreshold,
			"medium_threshold": cfg.Backlog.MediumThreshold,
			"low_threshold":    cfg.Backlog.LowThreshold,
			"max_per_cycle":    cfg.Backlog.MaxPerCycle,
			"max_concurrent":   cfg.Backlog.MaxConcurrent,
			"stale_days":       cfg.Backlog.StaleDays,
		},
		"mode":        cfg.Mode,
		"helper_mode": cfg.HelperMode,
		"notifications": map[string]any{
			"enabled": cfg.Notifications.Enabled,
			"smtp": map[string]any{
				"host":     redact(cfg.Notifications.SMTP.Host), // #211: redact infrastructure details
				"port":     0,                                   // #211: redact port
				"username": redact(cfg.Notifications.SMTP.Username),
				"password": redact(cfg.Notifications.SMTP.Password),
				"from":     redact(cfg.Notifications.SMTP.From), // #211: redact from address
			},
		},
		"logging": map[string]any{
			"level":  cfg.Logging.Level,
			"file":   cfg.Logging.File,
			"format": cfg.Logging.Format,
		},
		"webui": map[string]any{
			"port": cfg.WebUI.Port,
		},
	}
}
