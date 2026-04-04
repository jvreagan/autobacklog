# Quick Start

## 1. Install

```bash
go install github.com/jamesreagan/autobacklog/cmd/autobacklog@latest
```

Or build from source:

```bash
git clone https://github.com/jamesreagan/autobacklog.git
cd autobacklog
make build
```

## 2. Prerequisites

Ensure you have:
- **Claude Code CLI** — `claude --version` should work
- **GitHub CLI** — `gh auth status` should show authenticated
- **Git** — configured with push access to your target repo

## 3. Generate Config

```bash
autobacklog init
```

This creates `autobacklog.yaml` in the current directory.

## 4. Configure

Edit `autobacklog.yaml`:

```yaml
repo:
  url: "https://github.com/your-org/your-repo.git"

github:
  pat: "${GITHUB_TOKEN}"
  # create_issues: true              # optional: sync findings to GitHub Issues
  # issue_label: "autobacklog"       # label for bidirectional issue sync
```

Set your GitHub token:

```bash
export GITHUB_TOKEN=ghp_your_token_here
```

## 5. Dry Run

Test the pipeline without making changes:

```bash
autobacklog run --config autobacklog.yaml --dry-run --verbose
```

## 6. Run

Execute a single improvement cycle:

```bash
autobacklog run --config autobacklog.yaml
```

## 7. Daemon Mode

Run continuously:

```bash
autobacklog daemon --config autobacklog.yaml
```

## 8. Check Status

View the backlog for the current repo (scoped by `repo.url` from your config):

```bash
autobacklog status --config autobacklog.yaml
```

Without `--config`, all items across repos are shown.
