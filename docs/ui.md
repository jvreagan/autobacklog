# Web UI

Autobacklog includes an optional real-time web dashboard that provides visibility into a running cycle without parsing terminal output.

## Enabling

The web UI is disabled by default (port `0`). Enable it via config or CLI flag:

**Config:**

```yaml
webui:
  port: 8080
```

**CLI flag** (overrides config):

```bash
autobacklog run --config autobacklog.yaml --webui-port 8080
autobacklog daemon --config autobacklog.yaml --webui-port 8080
```

Once started, open `http://localhost:8080` in a browser.

## Dashboard Layout

The single-page dashboard has three sections:

### Configuration Overview (top)

A grid showing the current config values (secrets like PAT and SMTP password are redacted). Loaded once from `/api/config` on page load.

### App Logs (bottom left)

Real-time structured log output from `slog`. Lines are color-coded by level:

| Level | Color |
|-------|-------|
| DEBUG | gray |
| INFO | white |
| WARN | orange |
| ERROR | red |

### Claude Output (bottom right)

Real-time stdout from Claude CLI subprocess calls. This is the same output you'd see in the terminal during `RunPrint` invocations (implementation and fix attempts).

## How It Works

- **Server-Sent Events (SSE)** stream log and Claude output to the browser in real time
- A **Hub** broadcasts events to all connected browser tabs with non-blocking fan-out
- A **TeeWriter** (`io.Writer`) intercepts output, sending it to both the terminal and the SSE hub
- The server binds the port eagerly on startup — if the port is taken, autobacklog exits with a clear error before doing any work
- On shutdown (Ctrl-C or cycle completion in oneshot mode), the HTTP server is gracefully stopped

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Dashboard HTML (embedded in binary) |
| `GET /api/events` | SSE stream; all events by default |
| `GET /api/events?type=log` | SSE stream filtered to app logs only |
| `GET /api/events?type=claude` | SSE stream filtered to Claude output only |
| `GET /api/config` | Sanitized config as JSON |

## SSE Event Format

Each SSE message has an `event` type (`log` or `claude`) and a `data` field containing one line of output:

```
event: log
data: time=2025-01-15T10:30:00Z level=INFO msg="starting cycle" dry_run=false

event: claude
data: I'll implement the fix by modifying src/handler.go...
```

## Browser Behavior

- Auto-reconnects on disconnect with exponential backoff (1s to 8s)
- Auto-scrolls each pane only when the user is already at the bottom
- Caps each pane at 5000 DOM elements to prevent memory bloat on long runs
- New subscribers receive the last 1000 events as history on connect
- Connection status badge shows connected (green) or disconnected (red)

## Architecture

```
Terminal ──────────────────────────────────────┐
                                               │
slog handler ──→ TeeWriter(stderr, hub) ──→ Hub ──→ SSE ──→ Browser
                                               │
Claude CLI ────→ TeeWriter(stdout, hub) ──→────┘
```

The logging package and Claude client have no dependency on the webui package. They accept plain `io.Writer` interfaces, and the CLI layer wires in `TeeWriter` instances when the web UI is enabled.
