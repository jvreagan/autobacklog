package backlog

import (
	"context"
	"time"
)

// Store is the persistence interface for backlog items.
type Store interface {
	// Insert adds a new item to the store.
	Insert(ctx context.Context, item *Item) error

	// Update modifies an existing item.
	Update(ctx context.Context, item *Item) error

	// Get retrieves an item by ID.
	Get(ctx context.Context, id string) (*Item, error)

	// List returns items matching the given filter.
	List(ctx context.Context, filter ListFilter) ([]*Item, error)

	// Delete removes an item by ID.
	Delete(ctx context.Context, id string) error

	// DeleteStale removes items older than the given number of days
	// that are in a terminal status (done, failed, skipped),
	// scoped to a specific repo URL.
	DeleteStale(ctx context.Context, repoURL string, days int) (int, error)

	// InsertCost records a cost entry for analytics.
	InsertCost(ctx context.Context, record *CostRecord) error

	// ListCosts returns cost records for the given repo within the date range.
	ListCosts(ctx context.Context, repoURL string, since time.Time) ([]*CostRecord, error)

	// InsertAPIStats records a GitHub API usage entry.
	InsertAPIStats(ctx context.Context, record *APIStatsRecord) error

	// ListAPIStats returns API stats records for the given repo since the given time.
	ListAPIStats(ctx context.Context, repoURL string, since time.Time) ([]*APIStatsRecord, error)

	// RunInTx executes fn inside a database transaction. If fn returns an
	// error the transaction is rolled back; otherwise it is committed.
	// The Store passed to fn operates within the transaction.
	RunInTx(ctx context.Context, fn func(tx Store) error) error

	// Close closes the store.
	Close() error
}

// CostRecord represents a single Claude invocation cost entry.
type CostRecord struct {
	ID         string    `json:"id"`
	RepoURL    string    `json:"repo_url"`
	ItemID     string    `json:"item_id"`
	Timestamp  time.Time `json:"timestamp"`
	Model      string    `json:"model"`
	PromptType string    `json:"prompt_type"`
	CostTotal  float64   `json:"cost_total"`
}

// APIStatsRecord represents a single GitHub API usage snapshot per cycle.
type APIStatsRecord struct {
	ID         string    `json:"id"`
	RepoURL    string    `json:"repo_url"`
	Timestamp  time.Time `json:"timestamp"`
	Calls      int       `json:"calls"`
	Retries    int       `json:"retries"`
	RateLimits int       `json:"rate_limits"`
	Failures   int       `json:"failures"`
}

// ListFilter specifies criteria for listing backlog items.
type ListFilter struct {
	Status      *Status   // single status filter (mutually exclusive with Statuses)
	Statuses    []Status  // multiple status filter (OR); takes precedence over Status
	Priority    *Priority
	Category    *Category
	RepoURL     *string
	IssueNumber *int
	Limit       int
}
