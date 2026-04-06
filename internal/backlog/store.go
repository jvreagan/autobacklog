package backlog

import "context"

// Store is the persistence interface for backlog items.
//
// Note: individual operations are not wrapped in transactions. Multi-step
// operations (e.g., Manager.Ingest inserting multiple items) are not atomic.
// If atomicity is needed in the future, add a RunInTx method.
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

	// Close closes the store.
	Close() error
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
