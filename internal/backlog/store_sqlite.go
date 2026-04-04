package backlog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
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

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	// Idempotent migrations: add columns to existing databases.
	// "already exists" errors are expected on fresh DBs; real errors are logged.
	execMigration(db, migrateRepoURLSQL)
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_repo_url ON backlog_items(repo_url)`)
	execMigration(db, `ALTER TABLE backlog_items ADD COLUMN issue_number INTEGER NOT NULL DEFAULT 0`)
	execMigration(db, `CREATE INDEX IF NOT EXISTS idx_issue_number ON backlog_items(issue_number)`)

	return &SQLiteStore{db: db}, nil
}

// execMigration runs a migration statement, ignoring "already exists" errors.
func execMigration(db *sql.DB, stmt string) {
	if _, err := db.Exec(stmt); err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "duplicate column") {
			// Genuine failure — log via stderr since slog isn't available here
			fmt.Fprintf(os.Stderr, "autobacklog: migration warning: %v\n", err)
		}
	}
}

// Insert adds a new item to the SQLite store.
func (s *SQLiteStore) Insert(ctx context.Context, item *Item) error {
	_, err := s.db.ExecContext(ctx,
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

// Update modifies an existing item in the SQLite store.
// Returns an error if the item does not exist (zero rows affected).
func (s *SQLiteStore) Update(ctx context.Context, item *Item) error {
	item.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE backlog_items SET title=?, description=?, file_path=?, line_number=?, issue_number=?, priority=?, category=?, status=?, attempts=?, pr_link=?, updated_at=?
		 WHERE id=?`,
		item.Title, item.Description, item.FilePath, item.LineNumber, item.IssueNumber,
		item.Priority, item.Category, item.Status, item.Attempts, item.PRLink,
		item.UpdatedAt, item.ID,
	)
	if err != nil {
		return fmt.Errorf("updating item %q: %w", item.ID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("updating item %q: not found", item.ID)
	}
	return nil
}

// Get retrieves an item by ID from the SQLite store.
func (s *SQLiteStore) Get(ctx context.Context, id string) (*Item, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_url, title, description, file_path, line_number, issue_number, priority, category, status, attempts, pr_link, created_at, updated_at
		 FROM backlog_items WHERE id=?`, id)
	item, err := scanRow(row)
	if err != nil {
		return nil, fmt.Errorf("getting item %q: %w", id, err)
	}
	return item, nil
}

// List returns items matching the given filter from the SQLite store.
func (s *SQLiteStore) List(ctx context.Context, filter ListFilter) ([]*Item, error) {
	query := `SELECT id, repo_url, title, description, file_path, line_number, issue_number, priority, category, status, attempts, pr_link, created_at, updated_at FROM backlog_items WHERE 1=1`
	args := []any{}

	if filter.Status != nil {
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

	rows, err := s.db.QueryContext(ctx, query, args...)
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

// Delete removes an item by ID from the SQLite store.
func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backlog_items WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("deleting item %q: %w", id, err)
	}
	return nil
}

// DeleteStale removes items in terminal status older than the given number of days.
func (s *SQLiteStore) DeleteStale(ctx context.Context, repoURL string, days int) (int, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM backlog_items WHERE repo_url=? AND status IN ('done', 'failed', 'skipped') AND updated_at < ?`, repoURL, cutoff)
	if err != nil {
		return 0, fmt.Errorf("deleting stale items: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
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
