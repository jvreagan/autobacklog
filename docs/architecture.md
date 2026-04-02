# Architecture

## Overview

Autobacklog is a state machine that cycles through discrete phases. Each phase is isolated and communicates via the SQLite backlog store and transient in-memory state.

## State Machine

```
CLONE → REVIEW → INGEST → EVALUATE_THRESHOLD → IMPLEMENT → TEST → PR → DOCUMENT → DONE
```

### States

**CLONE** — Clones the target repo (first run) or pulls latest changes. Uses PAT-authenticated HTTPS.

**REVIEW** — Invokes Claude Code CLI with a structured review prompt. Claude analyzes the entire codebase and outputs a JSON array of findings with title, description, file path, priority, and category.

**INGEST** — Deduplicates new findings against the existing backlog (matching by title + file path) and inserts new items into SQLite.

**EVALUATE_THRESHOLD** — Checks whether the backlog meets implementation thresholds:
- Any high-priority item → implement immediately
- Medium-priority count ≥ threshold → implement batch
- Low-priority count ≥ threshold → implement batch
- Results capped at `max_per_cycle`

**IMPLEMENT** — For each selected item:
1. Creates a feature branch (`autobacklog/<category>/<title-slug>`)
2. Invokes Claude to implement the fix
3. Runs tests (with retry loop)
4. Stages, commits, pushes
5. Creates a PR via `gh`
6. Updates item status and PR link

**TEST** — Integrated into the implement loop. Auto-detects the test framework by checking for known files (go.mod, package.json, pytest.ini, etc.). If tests fail, Claude is asked to fix them (up to `max_retries` attempts). If still failing, changes are reverted and the item is marked failed.

**PR** — Creates a GitHub pull request via `gh pr create` with a structured body including summary, category, and test results.

**DOCUMENT** — Optionally invokes Claude to update project documentation reflecting the changes made.

## Package Structure

```
internal/
├── app/          Orchestrator — drives the state machine
├── backlog/      Domain types, Store interface, SQLite impl, threshold logic
├── claude/       Claude Code CLI subprocess wrapper, prompts, JSON parser, budget tracker
├── config/       YAML loading, env var interpolation, validation, defaults
├── git/          Git operations: clone, branch, commit, push
├── github/       PR creation via gh CLI
├── notify/       Notifier interface, SMTP email implementation
├── runner/       Test framework detection, test execution
├── cli/          Cobra commands (run, daemon, status, init, version)
└── logging/      Structured logging setup (slog)
```

## Key Design Decisions

### CGo-free SQLite
Uses `modernc.org/sqlite` (pure Go) instead of `github.com/mattn/go-sqlite3` to avoid CGo dependency, making cross-compilation simpler.

### Claude Code CLI as Subprocess
Rather than using the API directly, we invoke the `claude` CLI as a subprocess. This leverages Claude Code's built-in capabilities (file editing, context management) and respects per-invocation budget caps.

### Budget Safety
Two-level budget control:
1. `max_budget_per_call` — passed to Claude CLI via `--max-budget-usd`
2. `max_budget_total` — tracked in-process; stops the daemon when exceeded

### Always PR, Never Direct Push
All changes go through pull requests. The daemon never pushes to the main branch directly.

### Deduplication
Items are deduplicated by comparing title similarity + file path against non-terminal items in the backlog. This prevents the same issue from being filed repeatedly across cycles.
