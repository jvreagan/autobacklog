package backlog

import "context"

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
	// that are in a terminal status (done, failed, skipped).
	DeleteStale(ctx context.Context, days int) (int, error)

	// Close closes the store.
	Close() error
}

// ListFilter specifies criteria for listing backlog items.
type ListFilter struct {
	Status   *Status
	Priority *Priority
	Category *Category
	Limit    int
}
