# Configuration Reference

Autobacklog is configured via a YAML file. All `${VAR}` references are interpolated from environment variables.

## `repo`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Git clone URL (HTTPS) |
| `branch` | string | `main` | Target branch |
| `work_dir` | string | `/tmp/autobacklog` | Local clone directory |
| `pr_branch_prefix` | string | `autobacklog` | Prefix for PR branches |

## `github`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `pat` | string | | GitHub Personal Access Token (inline) |
| `pat_file` | string | | Path to file containing PAT |
| `auto_merge` | bool | `false` | Auto-merge PRs via `gh pr merge --squash --auto` after CI passes |
| `create_issues` | bool | `false` | Create a GitHub issue for each new backlog item during ingest |
| `issue_label` | string | `autobacklog` | Label used to import issues and tag created issues |

Falls back to `GITHUB_TOKEN` environment variable if neither is set.

### GitHub Issues Integration

When `create_issues` is enabled, autobacklog creates a GitHub issue for every new finding ingested into the backlog. The issue includes the description, file path, priority, and category.

Conversely, any open issue with the configured `issue_label` is automatically imported into the backlog during the IMPORT_ISSUES state. This enables team members to file issues with the label for autobacklog to pick up and implement.

When a PR is created for an item that has a linked issue, the PR body includes `Fixes #N` so GitHub auto-closes the issue on merge.

## `claude`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `binary` | string | `claude` | Path to Claude Code CLI |
| `model` | string | `sonnet` | Model to use |
| `max_budget_per_call` | float | `10.00` | USD cap per CLI invocation |
| `max_budget_total` | float | `100.00` | USD cap across all invocations |
| `timeout` | duration | `10m` | Timeout per invocation |
| `dangerously_skip_permissions` | bool | `false` | Pass `--dangerously-skip-permissions` to Claude CLI |

## `backlog`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `high_threshold` | int | `1` | Min high items to trigger implementation |
| `medium_threshold` | int | `3` | Min medium items to trigger batch |
| `low_threshold` | int | `5` | Min low items to trigger batch |
| `max_per_cycle` | int | `5` | Max items to implement per cycle |
| `stale_days` | int | `30` | Days before cleaning terminal items (scoped to current repo) |

## `testing`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto_detect` | bool | `true` | Auto-detect test framework |
| `override_command` | string | | Override test command |
| `timeout` | duration | `5m` | Test execution timeout |
| `max_retries` | int | `3` | Max fix attempts on test failure |

## `mode`

String: `oneshot` (default) or `daemon`.

## `helper_mode`

String: `buildbacklog` (default) or `burndown`.

- **`buildbacklog`** — full pipeline: review → ingest → evaluate → implement
- **`burndown`** — skip review and ingest, only implement existing backlog items

Can also be set via `--helper-mode` CLI flag.

## `daemon`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `interval` | duration | `1h` | Time between cycles |
| `quiet_start` | string | | Start of quiet hours (`HH:MM`) |
| `quiet_end` | string | | End of quiet hours (`HH:MM`) |

## `notifications`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable email notifications |

### `notifications.smtp`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | | SMTP server hostname |
| `port` | int | `587` | SMTP port |
| `username` | string | | SMTP username |
| `password` | string | | SMTP password |
| `from` | string | | Sender email address |

### `notifications.recipients`

List of email addresses.

### `notifications.events`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `on_cycle_complete` | bool | `false` | Notify on cycle completion |
| `on_stuck` | bool | `false` | Notify when item is stuck |
| `on_out_of_tokens` | bool | `false` | Notify when budget exceeded |
| `on_pr_created` | bool | `false` | Notify when PR is created |
| `on_error` | bool | `false` | Notify on unexpected errors |

## `logging`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `info` | Log level: debug, info, warn, error |
| `file` | string | | Log file path (also logs to stderr) |
| `format` | string | `text` | Output format: text or json |

## Duration Format

Durations use Go's format: `10s`, `5m`, `1h`, `2h30m`.

## Environment Variables

Any `${VAR}` in the YAML is replaced with the corresponding environment variable value. Unset variables are left as-is.
