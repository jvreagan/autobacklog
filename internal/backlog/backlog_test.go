package backlog

import (
	"context"
	"log/slog"
	"testing"
)

func newTestManager(t *testing.T) (*Manager, *SQLiteStore) {
	t.Helper()
	store := newTestStore(t)
	log := slog.Default()
	mgr := NewManager(store, log)
	return mgr, store
}

func TestManager_IngestAllNew(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	items := []*Item{
		NewItem("Bug 1", "desc", "a.go", PriorityHigh, CategoryBug),
		NewItem("Bug 2", "desc", "b.go", PriorityMedium, CategoryBug),
	}

	n, err := mgr.Ingest(ctx, "", items)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 2 {
		t.Errorf("inserted = %d, want 2", n)
	}
}

func TestManager_IngestDedup(t *testing.T) {
	mgr, store := newTestManager(t)
	ctx := context.Background()

	existing := NewItem("Fix null pointer", "desc", "handler.go", PriorityHigh, CategoryBug)
	store.Insert(ctx, existing)

	items := []*Item{
		NewItem("Fix null pointer", "same issue", "handler.go", PriorityHigh, CategoryBug),
		NewItem("New issue", "different", "other.go", PriorityLow, CategoryRefactor),
	}

	n, err := mgr.Ingest(ctx, "", items)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 1 {
		t.Errorf("inserted = %d, want 1 (one duplicate skipped)", n)
	}
}

func TestManager_IngestSkipsDoneItems(t *testing.T) {
	mgr, store := newTestManager(t)
	ctx := context.Background()

	// An item that's already done shouldn't block new items with same title
	done := NewItem("Fix bug", "old", "file.go", PriorityHigh, CategoryBug)
	done.Status = StatusDone
	store.Insert(ctx, done)

	items := []*Item{
		NewItem("Fix bug", "new occurrence", "file.go", PriorityHigh, CategoryBug),
	}

	n, err := mgr.Ingest(ctx, "", items)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 1 {
		t.Errorf("inserted = %d, want 1 (done items should not count as duplicates)", n)
	}
}

func TestManager_IngestDifferentFiles(t *testing.T) {
	mgr, store := newTestManager(t)
	ctx := context.Background()

	existing := NewItem("Fix bug", "desc", "a.go", PriorityHigh, CategoryBug)
	store.Insert(ctx, existing)

	// Same title but different file — not a duplicate
	items := []*Item{
		NewItem("Fix bug", "desc", "b.go", PriorityHigh, CategoryBug),
	}

	n, err := mgr.Ingest(ctx, "", items)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 1 {
		t.Errorf("inserted = %d, want 1 (different file is not a duplicate)", n)
	}
}

func TestManager_CleanStale(t *testing.T) {
	mgr, store := newTestManager(t)
	ctx := context.Background()

	// Insert items with various statuses
	for _, s := range []Status{StatusPending, StatusDone, StatusFailed} {
		item := NewItem("Item "+string(s), "", "", PriorityLow, CategoryRefactor)
		item.Status = s
		store.Insert(ctx, item)
	}

	// With recent items, nothing should be cleaned
	n, err := mgr.CleanStale(ctx, "", 30)
	if err != nil {
		t.Fatalf("CleanStale: %v", err)
	}
	if n != 0 {
		t.Errorf("cleaned %d, want 0 (all items are recent)", n)
	}
}

func TestManager_Ingest_EmptySlice(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	n, err := mgr.Ingest(ctx, "", []*Item{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("inserted = %d, want 0", n)
	}
}

func TestManager_Ingest_NilSlice(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	n, err := mgr.Ingest(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("inserted = %d, want 0", n)
	}
}

// #187: Ingest with nil item in slice should not panic.
func TestManager_Ingest_NilItem(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	items := []*Item{
		NewItem("Valid", "desc", "a.go", PriorityHigh, CategoryBug),
		nil, // should not panic
	}

	// This would panic before the nil guard was added
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Ingest panicked on nil item: %v", r)
		}
	}()
	mgr.Ingest(ctx, "", items)
}

// #188: isFuzzyDuplicate edge cases.
func TestManager_Ingest_FuzzyDuplicateEdgeCases(t *testing.T) {
	mgr, store := newTestManager(t)
	ctx := context.Background()

	// Existing item with a long title
	existing := NewItem("Fix authentication handler null pointer dereference", "desc", "handler.go", PriorityHigh, CategoryBug)
	if err := store.Insert(ctx, existing); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		title     string
		file      string
		wantInsert bool
	}{
		{"substring match same file", "Fix authentication handler null pointer", "handler.go", false},
		{"different file not matched", "Fix authentication handler null pointer", "other.go", true},
		{"short title bypasses fuzzy", "Fix auth", "handler.go", true},
		{"exact match same file", "Fix authentication handler null pointer dereference", "handler.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []*Item{NewItem(tt.title, "desc", tt.file, PriorityHigh, CategoryBug)}
			n, err := mgr.Ingest(ctx, "", items)
			if err != nil {
				t.Fatal(err)
			}
			inserted := n == 1
			if inserted != tt.wantInsert {
				t.Errorf("inserted = %v, want %v", inserted, tt.wantInsert)
			}
		})
	}
}

func TestManager_Ingest_CrossRepoDedupIsolation(t *testing.T) {
	mgr, store := newTestManager(t)
	ctx := context.Background()

	repoA := "https://github.com/org/repo-a.git"
	repoB := "https://github.com/org/repo-b.git"

	// Ingest an item into repoA
	itemsA := []*Item{
		NewItem("Fix null pointer", "desc", "handler.go", PriorityHigh, CategoryBug),
	}
	n, err := mgr.Ingest(ctx, repoA, itemsA)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("repoA inserted = %d, want 1", n)
	}

	// Same title+file into repoB should NOT be deduped
	itemsB := []*Item{
		NewItem("Fix null pointer", "desc", "handler.go", PriorityHigh, CategoryBug),
	}
	n, err = mgr.Ingest(ctx, repoB, itemsB)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("repoB inserted = %d, want 1 (cross-repo should not dedup)", n)
	}

	// Total items across both repos
	all, _ := store.List(ctx, ListFilter{})
	if len(all) != 2 {
		t.Errorf("total items = %d, want 2", len(all))
	}
}
