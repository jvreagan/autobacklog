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

// Ingest takes a slice of new items, deduplicates against existing items for the given repo, and inserts new ones.
// Returns the number of new items inserted.
func (m *Manager) Ingest(ctx context.Context, repoURL string, newItems []*Item) (int, error) {
	existing, err := m.store.List(ctx, ListFilter{RepoURL: &repoURL})
	if err != nil {
		return 0, err
	}

	// Build a lookup map keyed on normalized "title|filepath" for O(1) dedup.
	type dedupKey struct{ title, file string }
	seen := make(map[dedupKey]bool, len(existing))
	var active []*Item
	for _, ex := range existing {
		if ex.Status == StatusDone || ex.Status == StatusSkipped || ex.Status == StatusFailed {
			continue
		}
		seen[dedupKey{strings.ToLower(strings.TrimSpace(ex.Title)), ex.FilePath}] = true
		active = append(active, ex)
	}

	inserted := 0
	for _, item := range newItems {
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

		if err := m.store.Insert(ctx, item); err != nil {
			return inserted, err
		}
		seen[key] = true
		active = append(active, item)
		inserted++
		m.log.Info("ingested backlog item", "title", item.Title, "priority", item.Priority, "category", item.Category)
	}

	return inserted, nil
}

// isFuzzyDuplicate checks if item is similar to any existing item via substring containment.
// Only applies when both titles are at least 20 characters to avoid overly aggressive dedup
// (e.g., "Fix bug" matching "Fix bug in authentication handler").
func (m *Manager) isFuzzyDuplicate(newItem *Item, existing []*Item) bool {
	newTitle := strings.ToLower(strings.TrimSpace(newItem.Title))
	if len(newTitle) < 20 {
		return false
	}
	for _, ex := range existing {
		if newItem.FilePath != ex.FilePath {
			continue
		}
		exTitle := strings.ToLower(strings.TrimSpace(ex.Title))
		if len(exTitle) < 20 {
			continue
		}
		if strings.Contains(newTitle, exTitle) || strings.Contains(exTitle, newTitle) {
			return true
		}
	}
	return false
}

// similarText checks if two strings are similar enough to be considered duplicates.
// Uses exact normalized equality only — substring matching is handled separately
// with a minimum length requirement to avoid false positives.
func similarText(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return true
	}
	// Substring containment only for titles of meaningful length
	if len(a) >= 20 && len(b) >= 20 {
		if strings.Contains(a, b) || strings.Contains(b, a) {
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
