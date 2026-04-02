# Autobacklog — Project Conventions

## Language & Build
- Go 1.22+, build with `make build` or `go build ./cmd/autobacklog`
- Run tests: `make test` or `go test ./...`
- Run vet: `go vet ./...`

## Project Structure
- `cmd/autobacklog/main.go` — entry point only, delegates to `internal/cli`
- `internal/` — all application code, organized by domain
- `configs/` — example config files
- `docs/` — documentation

## Code Style
- Standard Go formatting (`gofmt`)
- Use `log/slog` for structured logging — never `fmt.Println` for operational output
- Error wrapping with `%w` and context: `fmt.Errorf("doing thing: %w", err)`
- Interfaces defined in the consumer package (e.g., `Store` in `backlog/`)
- CGo-free: use `modernc.org/sqlite` not `mattn/go-sqlite3`

## Dependencies
- CLI: `github.com/spf13/cobra`
- Config: `gopkg.in/yaml.v3`
- SQLite: `modernc.org/sqlite`
- UUIDs: `github.com/google/uuid`
- Everything else: standard library

## Testing
- Unit tests go in `*_test.go` next to the source
- Use table-driven tests where appropriate
- No test frameworks — standard `testing` package only

## Git
- Never push to main directly — always PR
- Branch naming: `autobacklog/<category>/<short-title>`
- Commits: imperative mood, prefixed with area (e.g., `backlog: add dedup logic`)
