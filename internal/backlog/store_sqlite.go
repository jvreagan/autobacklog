package backlog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

const createTableSQL = `
CREATE TABLE IF NOT EXISTS backlog_items (
	id          TEXT PRIMARY KEY,
	repo_url    TEXT NOT NULL DEFAULT '',
	title       TEXT NOT NULL,
	description TEXT NOT NULL,
	file_path   TEXT NOT NULL DEFAULT '',
	line_number INTEGER NOT NULL DEFAULT 0,
	priority    TEXT NOT NULL DEFAULT 'low',
	category    TEXT NOT NULL DEFAULT 'refactor',
	status      TEXT NOT NULL DEFAULT 'pending',
	attempts    INTEGER NOT NULL DEFAULT 0,
	pr_link     TEXT NOT NULL DEFAULT '',
	created_at  DATETIME NOT NULL,
	updated_at  DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_status ON backlog_items(status);
CREATE INDEX IF NOT EXISTS idx_priority ON backlog_items(priority);
CREATE INDEX IF NOT EXISTS idx_repo_url ON backlog_items(repo_url);
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

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	// Idempotent migration: add repo_url column to existing databases.
	// Ignore error — column already exists on fresh databases.
	db.Exec(migrateRepoURLSQL)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_repo_url ON backlog_items(repo_url)`)

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Insert(ctx context.Context, item *Item) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO backlog_items (id, repo_url, title, description, file_path, line_number, priority, category, status, attempts, pr_link, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.RepoURL, item.Title, item.Description, item.FilePath, item.LineNumber,
		item.Priority, item.Category, item.Status, item.Attempts, item.PRLink,
		item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) Update(ctx context.Context, item *Item) error {
	item.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE backlog_items SET title=?, description=?, file_path=?, line_number=?, priority=?, category=?, status=?, attempts=?, pr_link=?, updated_at=?
		 WHERE id=?`,
		item.Title, item.Description, item.FilePath, item.LineNumber,
		item.Priority, item.Category, item.Status, item.Attempts, item.PRLink,
		item.UpdatedAt, item.ID,
	)
	return err
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*Item, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, repo_url, title, description, file_path, line_number, priority, category, status, attempts, pr_link, created_at, updated_at
		 FROM backlog_items WHERE id=?`, id)
	return scanItem(row)
}

func (s *SQLiteStore) List(ctx context.Context, filter ListFilter) ([]*Item, error) {
	query := `SELECT id, repo_url, title, description, file_path, line_number, priority, category, status, attempts, pr_link, created_at, updated_at FROM backlog_items WHERE 1=1`
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

	query += ` ORDER BY CASE priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 END, created_at ASC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		item, err := scanItemFromRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backlog_items WHERE id=?`, id)
	return err
}

func (s *SQLiteStore) DeleteStale(ctx context.Context, repoURL string, days int) (int, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM backlog_items WHERE repo_url=? AND status IN ('done', 'failed', 'skipped') AND updated_at < ?`, repoURL, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row *sql.Row) (*Item, error) {
	item := &Item{}
	err := row.Scan(
		&item.ID, &item.RepoURL, &item.Title, &item.Description, &item.FilePath, &item.LineNumber,
		&item.Priority, &item.Category, &item.Status, &item.Attempts, &item.PRLink,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func scanItemFromRows(rows *sql.Rows) (*Item, error) {
	item := &Item{}
	err := rows.Scan(
		&item.ID, &item.RepoURL, &item.Title, &item.Description, &item.FilePath, &item.LineNumber,
		&item.Priority, &item.Category, &item.Status, &item.Attempts, &item.PRLink,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}
