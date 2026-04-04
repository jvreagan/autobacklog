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

func TestSimilarText(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Fix bug", "Fix bug", true},
		{"Fix bug", "fix bug", true},
		{"Fix bug in handler", "Fix bug", true},
		{"Fix bug", "Fix bug in handler", true},
		{"Fix bug", "Add feature", false},
		{"  Fix bug  ", "fix bug", true},
		{"", "", true},
		{"abc", "xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := similarText(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("similarText(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
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

func TestSimilarText_NegativeCase(t *testing.T) {
	// "Fix bug" vs "Fix typo" — neither contains the other, not equal
	if similarText("Fix bug", "Fix typo") {
		t.Error("'Fix bug' and 'Fix typo' should NOT be similar")
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
