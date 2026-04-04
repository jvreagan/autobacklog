# Autobacklog

Autonomous code improvement daemon. Point it at a GitHub repo and it continuously reviews code with AI, builds a prioritized backlog, implements improvements, runs tests, and creates PRs — all without human involvement.

## How It Works

```
CLONE → IMPORT_ISSUES → REVIEW → INGEST → EVALUATE → IMPLEMENT → TEST → PR → DOCUMENT → (loop)
```

1. **Clone/Pull** the target repo
2. **Import Issues** — pull in open GitHub issues labeled `autobacklog` as work items
3. **Review** the codebase using Claude Code CLI, producing structured findings
4. **Ingest** findings into a local SQLite backlog, deduplicating per-repo against existing items; optionally create GitHub issues for each new finding
5. **Evaluate** whether per-repo thresholds are met (high=immediate, medium≥3, low≥5)
6. **Implement** selected items by invoking Claude to make code changes
7. **Test** — auto-detect and run the test suite; retry up to 3x if tests fail
8. **PR** — create a GitHub pull request with description and test results (auto-closes linked issues via `Fixes #N`)
9. **Document** — update docs if needed
10. **Loop** (daemon mode) or exit (oneshot mode)

## Quick Start

```bash
# Install
go install github.com/jamesreagan/autobacklog/cmd/autobacklog@latest

# Generate config
autobacklog init

# Edit autobacklog.yaml with your repo URL and GitHub token

# Run one cycle
autobacklog run --config autobacklog.yaml

# Or run as a daemon
autobacklog daemon --config autobacklog.yaml
```

## Prerequisites

- **Go 1.22+**
- **Claude Code CLI** (`claude`) installed and authenticated
- **GitHub CLI** (`gh`) installed and authenticated (or provide a PAT)
- **Git**

## CLI

```
autobacklog run      --config ./autobacklog.yaml   # One-shot cycle
autobacklog daemon   --config ./autobacklog.yaml   # Continuous daemon
autobacklog status   --config ./autobacklog.yaml   # Show backlog state
autobacklog init                                    # Generate example config
autobacklog version                                 # Print version
```

### Global Flags

| Flag | Description |
|------|-------------|
| `--config, -c` | Config file path (default: `autobacklog.yaml`) |
| `--verbose, -v` | Enable debug logging |
| `--dry-run` | Run through the state machine without making changes |

## Configuration

See [`configs/autobacklog.example.yaml`](configs/autobacklog.example.yaml) for a fully annotated example.

Key sections:

| Section | Purpose |
|---------|---------|
| `repo` | Target repository URL, branch, working directory |
| `github` | PAT authentication (inline, file, or env var) |
| `claude` | CLI binary, model, budget limits, timeout |
| `backlog` | Priority thresholds, max items per cycle, stale cleanup |
| `testing` | Auto-detection, override command, timeout, retries |
| `mode` | `oneshot` or `daemon` |
| `helper_mode` | `buildbacklog` (full pipeline) or `burndown` (implement only) |
| `daemon` | Cycle interval, quiet hours |
| `notifications` | SMTP email notifications with event toggles |
| `logging` | Level, file output, format (text/json) |

Environment variables are interpolated with `${VAR}` syntax.

## GitHub Issues Integration

Autobacklog supports bidirectional sync with GitHub Issues:

- **Inbound**: Open issues labeled `autobacklog` are imported as work items each cycle
- **Outbound**: When `github.create_issues: true`, a GitHub issue is created for each new finding
- **Auto-close**: PRs include `Fixes #N` to auto-close linked issues on merge

```yaml
github:
  create_issues: true
  issue_label: "autobacklog"
```

## Test Framework Auto-Detection

Autobacklog detects and runs tests automatically:

| Detected File | Framework | Command |
|---------------|-----------|---------|
| `go.mod` | Go | `go test ./...` |
| `package.json` (with test script) | Node.js | `npm test` |
| `pytest.ini` / `setup.cfg` / `pyproject.toml` | pytest | `pytest` |
| `pom.xml` | Maven | `mvn test` |
| `build.gradle` / `build.gradle.kts` | Gradle | `gradle test` |
| `Cargo.toml` | Rust | `cargo test` |
| `Makefile` (test target) | Make | `make test` |

If no framework is detected, Claude is asked to determine the test command.

## Notifications

Email notifications are sent for these events (individually toggleable):

- **cycle_complete** — Summary of each cycle
- **stuck** — Item failed after max retries
- **out_of_tokens** — Budget exceeded, daemon paused
- **pr_created** — New PR with link
- **error** — Unexpected errors

## Architecture

```
cmd/autobacklog/main.go           Entry point
internal/
  app/                            State machine orchestrator
  backlog/                        Item types, SQLite store, threshold logic
  claude/                         Claude Code CLI wrapper, prompts, parser
  config/                         YAML config loading with env var interpolation
  git/                            Clone, branch, commit operations
  github/                         PR creation, auto-merge, issue sync via gh CLI
  notify/                         Email notifications
  runner/                         Test framework detection and execution
  cli/                            Cobra CLI commands
  logging/                        Structured logging (slog)
```

## Building

```bash
make build      # Build binary
make test       # Run tests
make vet        # Run go vet
make all        # Clean + build + test + vet
```

## License

MIT
