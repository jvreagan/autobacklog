package backlog

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"
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

// Ingest takes a slice of new items, deduplicates against existing items for the given repo, and inserts new ones.
// All inserts are performed atomically within a transaction — if any insert
// fails, the entire batch is rolled back.
// Returns the number of new items inserted.
func (m *Manager) Ingest(ctx context.Context, repoURL string, newItems []*Item) (int, error) {
	// Only fetch active (non-terminal) items for deduplication — avoids a full
	// table scan of done/failed/skipped items in long-running daemons.
	active, err := m.store.List(ctx, ListFilter{
		RepoURL:  &repoURL,
		Statuses: []Status{StatusPending, StatusInProgress},
	})
	if err != nil {
		return 0, err
	}

	// Build a lookup map keyed on normalized "title|filepath" for O(1) dedup.
	type dedupKey struct{ title, file string }
	seen := make(map[dedupKey]bool, len(active))
	for _, ex := range active {
		seen[dedupKey{strings.ToLower(strings.TrimSpace(ex.Title)), ex.FilePath}] = true
	}

	// Filter to items that need inserting (dedup pass).
	var toInsert []*Item
	for _, item := range newItems {
		// #187: skip nil items to prevent panics
		if item == nil {
			continue
		}
		item.RepoURL = repoURL

		key := dedupKey{strings.ToLower(strings.TrimSpace(item.Title)), item.FilePath}
		if seen[key] {
			m.log.Debug("skipping duplicate item (exact match)", "title", item.Title, "file", item.FilePath)
			continue
		}

		// Fuzzy check: only use substring containment for titles of meaningful length
		if m.isFuzzyDuplicate(item, active) {
			m.log.Debug("skipping duplicate item (fuzzy match)", "title", item.Title, "file", item.FilePath)
			continue
		}

		toInsert = append(toInsert, item)
		seen[key] = true
		active = append(active, item)
	}

	if len(toInsert) == 0 {
		return 0, nil
	}

	// Insert all new items atomically within a transaction.
	err = m.store.RunInTx(ctx, func(tx Store) error {
		for _, item := range toInsert {
			if err := tx.Insert(ctx, item); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	for _, item := range toInsert {
		m.log.Info("ingested backlog item", "title", item.Title, "priority", item.Priority, "category", item.Category)
	}

	return len(toInsert), nil
}

// isFuzzyDuplicate checks if item is similar to any existing item via substring containment.
// Only applies when both titles are at least 20 characters to avoid overly aggressive dedup
// (e.g., "Fix bug" matching "Fix bug in authentication handler").
func (m *Manager) isFuzzyDuplicate(newItem *Item, existing []*Item) bool {
	newTitle := strings.ToLower(strings.TrimSpace(newItem.Title))
	// #136: use rune count instead of byte length for multi-byte UTF-8 titles
	if utf8.RuneCountInString(newTitle) < 20 {
		return false
	}
	for _, ex := range existing {
		if newItem.FilePath != ex.FilePath {
			continue
		}
		exTitle := strings.ToLower(strings.TrimSpace(ex.Title))
		if utf8.RuneCountInString(exTitle) < 20 {
			continue
		}
		if strings.Contains(newTitle, exTitle) || strings.Contains(exTitle, newTitle) {
			return true
		}
	}
	return false
}

// CleanStale removes stale items from the store, scoped to a specific repo.
func (m *Manager) CleanStale(ctx context.Context, repoURL string, days int) (int, error) {
	n, err := m.store.DeleteStale(ctx, repoURL, days)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		m.log.Info("cleaned stale backlog items", "count", n)
	}
	return n, nil
}
