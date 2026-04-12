package backlog

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// queryable is satisfied by both *sql.DB and *sql.Tx, allowing shared CRUD
// logic between SQLiteStore and txStore.
type queryable interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// storeOps holds the shared CRUD implementations that work against any queryable.
type storeOps struct {
	q queryable
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
	storeOps
}

// txStore implements Store within a transaction. Close and RunInTx are no-ops.
type txStore struct {
	storeOps
}

const createTableSQL = `
CREATE TABLE IF NOT EXISTS backlog_items (
	id           TEXT PRIMARY KEY,
	repo_url     TEXT NOT NULL DEFAULT '',
	title        TEXT NOT NULL,
	description  TEXT NOT NULL,
	file_path    TEXT NOT NULL DEFAULT '',
	line_number  INTEGER NOT NULL DEFAULT 0,
	issue_number INTEGER NOT NULL DEFAULT 0,
	priority     TEXT NOT NULL DEFAULT 'low',
	category     TEXT NOT NULL DEFAULT 'refactor',
	status       TEXT NOT NULL DEFAULT 'pending',
	attempts     INTEGER NOT NULL DEFAULT 0,
	pr_link      TEXT NOT NULL DEFAULT '',
	created_at   DATETIME NOT NULL,
	updated_at   DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_status ON backlog_items(status);
CREATE INDEX IF NOT EXISTS idx_priority ON backlog_items(priority);
`

const migrateRepoURLSQL = `
ALTER TABLE backlog_items ADD COLUMN repo_url TEXT NOT NULL DEFAULT '';
`

// NewSQLiteStore opens or creates a SQLite database at the given path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Limit to one open connection to serialize all writes and prevent
	// SQLITE_BUSY errors. SQLite supports only one writer at a time.
	db.SetMaxOpenConns(1)

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// #208: set busy_timeout so SQLite retries internally on lock contention
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting busy_timeout: %w", err)
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	log := slog.Default()

	// Idempotent migrations: add columns to existing databases.
	// "already exists" errors are expected on fresh DBs; real errors are logged.
	execMigration(db, migrateRepoURLSQL, log)
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_repo_url ON backlog_items(repo_url)`, log)
	execMigration(db, `ALTER TABLE backlog_items ADD COLUMN issue_number INTEGER NOT NULL DEFAULT 0`, log)
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_issue_number ON backlog_items(issue_number)`, log)
	// #207: composite index for common dedup query pattern
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_repo_status ON backlog_items(repo_url, status)`, log)

	// Cost analytics table
	execMigration(db, `CREATE TABLE IF NOT EXISTS cost_records (
		id          TEXT PRIMARY KEY,
		repo_url    TEXT NOT NULL,
		item_id     TEXT NOT NULL DEFAULT '',
		timestamp   DATETIME NOT NULL,
		model       TEXT NOT NULL DEFAULT '',
		prompt_type TEXT NOT NULL DEFAULT '',
		cost_total  REAL NOT NULL DEFAULT 0
	)`, log)
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_cost_repo_time ON cost_records(repo_url, timestamp)`, log)

	// API stats table
	execMigration(db, `CREATE TABLE IF NOT EXISTS api_stats_records (
		id          TEXT PRIMARY KEY,
		repo_url    TEXT NOT NULL,
		timestamp   DATETIME NOT NULL,
		calls       INTEGER NOT NULL DEFAULT 0,
		retries     INTEGER NOT NULL DEFAULT 0,
		rate_limits INTEGER NOT NULL DEFAULT 0,
		failures    INTEGER NOT NULL DEFAULT 0
	)`, log)
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_apistats_repo_time ON api_stats_records(repo_url, timestamp)`, log)

	return &SQLiteStore{db: db, storeOps: storeOps{q: db}}, nil
}

// execMigration runs a migration statement, ignoring "already exists" errors.
// #209: uses slog instead of fmt.Fprintf(os.Stderr, ...).
func execMigration(db *sql.DB, stmt string, log *slog.Logger) {
	if _, err := db.Exec(stmt); err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "duplicate column") {
			log.Warn("migration warning", "error", err, "statement", stmt)
		}
	}
}

// Insert adds a new item to the store.
// #134: validates required fields before insertion.
func (s *storeOps) Insert(ctx context.Context, item *Item) error {
	if item.ID == "" {
		return fmt.Errorf("inserting item: ID is required")
	}
	if item.Title == "" {
		return fmt.Errorf("inserting item: title is required")
	}
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO backlog_items (id, repo_url, title, description, file_path, line_number, issue_number, priority, category, status, attempts, pr_link, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.RepoURL, item.Title, item.Description, item.FilePath, item.LineNumber, item.IssueNumber,
		item.Priority, item.Category, item.Status, item.Attempts, item.PRLink,
		item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting item %q: %w", item.ID, err)
	}
	return nil
}

// Update modifies an existing item in the store.
// Returns an error if the item does not exist (zero rows affected).
// #130: now also updates repo_url so changes are not silently dropped.
// #132: sets UpdatedAt only after successful exec to avoid side effects on error.
func (s *storeOps) Update(ctx context.Context, item *Item) error {
	now := time.Now().UTC()
	result, err := s.q.ExecContext(ctx,
		`UPDATE backlog_items SET repo_url=?, title=?, description=?, file_path=?, line_number=?, issue_number=?, priority=?, category=?, status=?, attempts=?, pr_link=?, updated_at=?
		 WHERE id=?`,
		item.RepoURL, item.Title, item.Description, item.FilePath, item.LineNumber, item.IssueNumber,
		item.Priority, item.Category, item.Status, item.Attempts, item.PRLink,
		now, item.ID,
	)
	if err != nil {
		return fmt.Errorf("updating item %q: %w", item.ID, err)
	}
	// #133: check RowsAffected error instead of discarding
	n, raErr := result.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("checking rows affected for item %q: %w", item.ID, raErr)
	}
	if n == 0 {
		return fmt.Errorf("updating item %q: not found", item.ID)
	}
	// #132: only mutate caller's struct after confirmed success
	item.UpdatedAt = now
	return nil
}

// Get retrieves an item by ID from the store.
func (s *storeOps) Get(ctx context.Context, id string) (*Item, error) {
	row := s.q.QueryRowContext(ctx,
		`SELECT id, repo_url, title, description, file_path, line_number, issue_number, priority, category, status, attempts, pr_link, created_at, updated_at
		 FROM backlog_items WHERE id=?`, id)
	item, err := scanRow(row)
	if err != nil {
		return nil, fmt.Errorf("getting item %q: %w", id, err)
	}
	return item, nil
}

// List returns items matching the given filter from the store.
func (s *storeOps) List(ctx context.Context, filter ListFilter) ([]*Item, error) {
	query := `SELECT id, repo_url, title, description, file_path, line_number, issue_number, priority, category, status, attempts, pr_link, created_at, updated_at FROM backlog_items WHERE 1=1`
	args := []any{}

	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for i, s := range filter.Statuses {
			placeholders[i] = "?"
			args = append(args, s)
		}
		query += ` AND status IN (` + strings.Join(placeholders, ",") + `)`
	} else if filter.Status != nil {
		query += ` AND status=?`
		args = append(args, *filter.Status)
	}
	if filter.Priority != nil {
		query += ` AND priority=?`
		args = append(args, *filter.Priority)
	}
	if filter.Category != nil {
		query += ` AND category=?`
		args = append(args, *filter.Category)
	}
	if filter.RepoURL != nil {
		query += ` AND repo_url=?`
		args = append(args, *filter.RepoURL)
	}
	if filter.IssueNumber != nil {
		query += ` AND issue_number=?`
		args = append(args, *filter.IssueNumber)
	}

	query += ` ORDER BY CASE priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 END, created_at ASC`

	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing items: %w", err)
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		item, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning item row: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Delete removes an item by ID from the store.
// Returns an error if the item does not exist (zero rows affected).
func (s *storeOps) Delete(ctx context.Context, id string) error {
	result, err := s.q.ExecContext(ctx, `DELETE FROM backlog_items WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("deleting item %q: %w", id, err)
	}
	// #133: check RowsAffected error instead of discarding
	n, raErr := result.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("checking rows affected for delete %q: %w", id, raErr)
	}
	if n == 0 {
		return fmt.Errorf("deleting item %q: not found", id)
	}
	return nil
}

// DeleteStale removes items in terminal status older than the given number of days.
func (s *storeOps) DeleteStale(ctx context.Context, repoURL string, days int) (int, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	result, err := s.q.ExecContext(ctx,
		`DELETE FROM backlog_items WHERE repo_url=? AND status IN ('done', 'failed', 'skipped') AND updated_at < ?`, repoURL, cutoff)
	if err != nil {
		return 0, fmt.Errorf("deleting stale items: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// InsertCost records a cost entry.
func (s *storeOps) InsertCost(ctx context.Context, record *CostRecord) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO cost_records (id, repo_url, item_id, timestamp, model, prompt_type, cost_total)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.RepoURL, record.ItemID, record.Timestamp, record.Model, record.PromptType, record.CostTotal,
	)
	if err != nil {
		return fmt.Errorf("inserting cost record: %w", err)
	}
	return nil
}

// ListCosts returns cost records for the given repo since the given time.
func (s *storeOps) ListCosts(ctx context.Context, repoURL string, since time.Time) ([]*CostRecord, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, repo_url, item_id, timestamp, model, prompt_type, cost_total
		 FROM cost_records WHERE repo_url=? AND timestamp >= ? ORDER BY timestamp ASC`, repoURL, since)
	if err != nil {
		return nil, fmt.Errorf("listing cost records: %w", err)
	}
	defer rows.Close()

	var records []*CostRecord
	for rows.Next() {
		r := &CostRecord{}
		if err := rows.Scan(&r.ID, &r.RepoURL, &r.ItemID, &r.Timestamp, &r.Model, &r.PromptType, &r.CostTotal); err != nil {
			return nil, fmt.Errorf("scanning cost record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// InsertAPIStats records a GitHub API usage entry.
func (s *storeOps) InsertAPIStats(ctx context.Context, record *APIStatsRecord) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO api_stats_records (id, repo_url, timestamp, calls, retries, rate_limits, failures)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.RepoURL, record.Timestamp, record.Calls, record.Retries, record.RateLimits, record.Failures,
	)
	if err != nil {
		return fmt.Errorf("inserting api stats record: %w", err)
	}
	return nil
}

// ListAPIStats returns API stats records for the given repo since the given time.
func (s *storeOps) ListAPIStats(ctx context.Context, repoURL string, since time.Time) ([]*APIStatsRecord, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, repo_url, timestamp, calls, retries, rate_limits, failures
		 FROM api_stats_records WHERE repo_url=? AND timestamp >= ? ORDER BY timestamp ASC`, repoURL, since)
	if err != nil {
		return nil, fmt.Errorf("listing api stats records: %w", err)
	}
	defer rows.Close()

	var records []*APIStatsRecord
	for rows.Next() {
		r := &APIStatsRecord{}
		if err := rows.Scan(&r.ID, &r.RepoURL, &r.Timestamp, &r.Calls, &r.Retries, &r.RateLimits, &r.Failures); err != nil {
			return nil, fmt.Errorf("scanning api stats record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// RunInTx executes fn inside a database transaction. If fn returns an error
// the transaction is rolled back; otherwise it is committed.
func (s *SQLiteStore) RunInTx(ctx context.Context, fn func(tx Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	txS := &txStore{storeOps: storeOps{q: tx}}
	if err := fn(txS); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback after error (%w): %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// RunInTx on a txStore is a no-op passthrough (nested transactions not supported).
func (s *txStore) RunInTx(_ context.Context, fn func(tx Store) error) error {
	return fn(s)
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Close on a txStore is a no-op.
func (s *txStore) Close() error {
	return nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanRow scans a single row into an Item. Works with both *sql.Row and *sql.Rows.
func scanRow(s scanner) (*Item, error) {
	item := &Item{}
	err := s.Scan(
		&item.ID, &item.RepoURL, &item.Title, &item.Description, &item.FilePath, &item.LineNumber, &item.IssueNumber,
		&item.Priority, &item.Category, &item.Status, &item.Attempts, &item.PRLink,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}
