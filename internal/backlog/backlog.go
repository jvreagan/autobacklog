package backlog

import (
	"context"
	"log/slog"
	"strings"
)

// Manager handles ingestion, deduplication, and selection of backlog items.
type Manager struct {
	store Store
	log   *slog.Logger
}

// NewManager creates a new backlog manager.
func NewManager(store Store, log *slog.Logger) *Manager {
	return &Manager{store: store, log: log}
}

// Ingest takes a slice of new items, deduplicates against existing items, and inserts new ones.
// Returns the number of new items inserted.
func (m *Manager) Ingest(ctx context.Context, newItems []*Item) (int, error) {
	existing, err := m.store.List(ctx, ListFilter{})
	if err != nil {
		return 0, err
	}

	inserted := 0
	for _, item := range newItems {
		if m.isDuplicate(item, existing) {
			m.log.Debug("skipping duplicate item", "title", item.Title, "file", item.FilePath)
			continue
		}
		if err := m.store.Insert(ctx, item); err != nil {
			return inserted, err
		}
		existing = append(existing, item)
		inserted++
		m.log.Info("ingested backlog item", "title", item.Title, "priority", item.Priority, "category", item.Category)
	}

	return inserted, nil
}

// isDuplicate checks if a new item is similar to any existing non-terminal item.
func (m *Manager) isDuplicate(newItem *Item, existing []*Item) bool {
	for _, ex := range existing {
		if ex.Status == StatusDone || ex.Status == StatusSkipped {
			continue
		}
		if similarText(newItem.Title, ex.Title) && newItem.FilePath == ex.FilePath {
			return true
		}
	}
	return false
}

// similarText checks if two strings are similar enough to be considered duplicates.
func similarText(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return true
	}
	// Check if one contains the other
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	return false
}

// CleanStale removes stale items from the store.
func (m *Manager) CleanStale(ctx context.Context, days int) (int, error) {
	n, err := m.store.DeleteStale(ctx, days)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		m.log.Info("cleaned stale backlog items", "count", n)
	}
	return n, nil
}
