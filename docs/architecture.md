# Architecture

## Overview

Autobacklog is a state machine that cycles through discrete phases. Each phase is isolated and communicates via the SQLite backlog store and transient in-memory state.

## State Machine

```
CLONE → IMPORT_ISSUES → REVIEW → INGEST → EVALUATE_THRESHOLD → IMPLEMENT → (TEST) → (PR) → DOCUMENT → DONE
```

### States

**CLONE** — Clones the target repo (first run) or pulls latest changes. Uses PAT-authenticated HTTPS.

**IMPORT_ISSUES** — Imports open GitHub issues with the configured label (default: `autobacklog`) into the backlog. Runs before REVIEW so Claude's findings dedup against already-imported issues. Each imported issue is tracked by its `issue_number` to prevent re-importing. Non-fatal on errors.

**REVIEW** — Invokes Claude Code CLI with a structured review prompt. Claude analyzes the entire codebase and outputs a JSON array of findings with title, description, file path, priority, and category.

**INGEST** — Deduplicates new findings against the existing backlog for the same repo (matching by title + file path) and inserts new items into SQLite. Each item is tagged with the configured `repo.url` to ensure isolation when multiple repos share a database. When `github.create_issues` is enabled, a GitHub issue is created for each newly ingested item and the issue number is stored on the backlog item.

**EVALUATE_THRESHOLD** — Checks whether the backlog for the current repo meets implementation thresholds:
- Any high-priority item → implement immediately
- Medium-priority count ≥ threshold → implement batch
- Low-priority count ≥ threshold → implement batch
- Results capped at `max_per_cycle`

**IMPLEMENT** — For each selected item:
1. Creates a feature branch (`autobacklog/<category>/<title-slug>`)
2. Invokes Claude to implement the fix
3. Runs tests with retry loop (auto-detect or override command)
4. On test failure, asks Claude to fix (up to `max_retries` attempts); if still failing, reverts and marks failed
5. Stages, commits, pushes
6. Creates a PR via `gh pr create` (includes `Fixes #N` when the item has a linked issue)
7. Optionally enables auto-merge via `gh pr merge --squash --auto`
8. Updates item status and PR link

> **Note:** The `TEST` and `PR` states exist in the state enum but are **skipped** during the main loop — testing and PR creation are integrated into the `IMPLEMENT` phase's `implementItem()` method.

**DOCUMENT** — Optionally invokes Claude to update project documentation reflecting the changes made. Non-fatal if it fails.

## Package Structure

```
internal/
├── app/          Orchestrator — drives the state machine, defines dependency interfaces
├── backlog/      Domain types, Store interface, SQLite impl, threshold logic
├── claude/       Claude Code CLI subprocess wrapper, prompts, JSON parser, budget tracker
├── config/       YAML loading, env var interpolation, validation, defaults
├── git/          Git operations: clone, branch, commit, push
├── github/       PR creation, auto-merge, and issue sync via gh CLI
├── notify/       Notifier interface, SMTP email implementation
├── runner/       Test framework detection, test execution
├── cli/          Cobra commands (run, daemon, status, init, version)
└── logging/      Structured logging setup (slog)
```

## Key Design Decisions

### Dependency Injection via Interfaces

The `app` package defines interfaces for its external dependencies, following the Go convention of interfaces in the consumer package:

- **`Repository`** — abstracts git operations (clone, branch, commit, push, revert)
- **`AIClient`** — abstracts Claude CLI invocations and budget tracking
- **`TestRunner`** — abstracts test execution
- **`PRCreator`** — abstracts GitHub PR creation and auto-merge
- **`IssueManager`** — abstracts GitHub issue creation and listing

The production constructor `New()` builds concrete implementations and delegates to `NewWithDeps()`, which accepts these interfaces. This enables comprehensive testing of the orchestrator with mock dependencies.

### CGo-free SQLite
Uses `modernc.org/sqlite` (pure Go) instead of `github.com/mattn/go-sqlite3` to avoid CGo dependency, making cross-compilation simpler.

### Claude Code CLI as Subprocess
Rather than using the API directly, we invoke the `claude` CLI as a subprocess. This leverages Claude Code's built-in capabilities (file editing, context management) and respects per-invocation budget caps. CLI output is streamed to the terminal in real time so the user can follow progress — implementation calls stream both stdout and stderr, while review calls stream stderr only since stdout contains structured JSON.

### Budget Safety
Two-level budget control:
1. `max_budget_per_call` — passed to Claude CLI via `--max-budget-usd`
2. `max_budget_total` — tracked in-process; stops the daemon when exceeded

### Always PR, Never Direct Push
All changes go through pull requests. The daemon never pushes to the main branch directly. When `auto_merge` is enabled, PRs are set to auto-merge via `gh pr merge --squash --auto` once all required CI checks pass.

### Multi-Tenant Isolation
All backlog items are tagged with their `repo_url`. Every query — listing, deduplication, threshold evaluation, and stale cleanup — is scoped to the current repo. This means multiple repos can safely share a single SQLite database without cross-contamination.

### Deduplication
Items are deduplicated by comparing title similarity + file path against non-terminal items in the backlog for the same repo. This prevents the same issue from being filed repeatedly across cycles while allowing identical findings in different repos to coexist.
