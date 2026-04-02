package backlog

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteStore_InsertAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("Test item", "A description", "file.go", PriorityHigh, CategoryBug)
	item.LineNumber = 42

	if err := store.Insert(ctx, item); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := store.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != item.ID {
		t.Errorf("ID = %q, want %q", got.ID, item.ID)
	}
	if got.Title != "Test item" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Description != "A description" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.FilePath != "file.go" {
		t.Errorf("FilePath = %q", got.FilePath)
	}
	if got.LineNumber != 42 {
		t.Errorf("LineNumber = %d", got.LineNumber)
	}
	if got.Priority != PriorityHigh {
		t.Errorf("Priority = %q", got.Priority)
	}
	if got.Category != CategoryBug {
		t.Errorf("Category = %q", got.Category)
	}
	if got.Status != StatusPending {
		t.Errorf("Status = %q", got.Status)
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("Original", "Desc", "f.go", PriorityLow, CategoryRefactor)
	store.Insert(ctx, item)

	item.Title = "Updated"
	item.Status = StatusInProgress
	item.Attempts = 2

	if err := store.Update(ctx, item); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := store.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Title != "Updated" {
		t.Errorf("Title = %q, want Updated", got.Title)
	}
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q", got.Status)
	}
	if got.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", got.Attempts)
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("To delete", "", "", PriorityLow, CategoryStyle)
	store.Insert(ctx, item)

	if err := store.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, item.ID)
	if err == nil {
		t.Error("Get after Delete should error")
	}
}

func TestSQLiteStore_ListNoFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewItem("A", "", "", PriorityLow, CategoryRefactor))
	store.Insert(ctx, NewItem("B", "", "", PriorityHigh, CategoryBug))
	store.Insert(ctx, NewItem("C", "", "", PriorityMedium, CategoryTest))

	items, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("len = %d, want 3", len(items))
	}

	// Should be ordered by priority: high, medium, low
	if items[0].Priority != PriorityHigh {
		t.Errorf("first item priority = %q, want high", items[0].Priority)
	}
	if items[1].Priority != PriorityMedium {
		t.Errorf("second item priority = %q, want medium", items[1].Priority)
	}
	if items[2].Priority != PriorityLow {
		t.Errorf("third item priority = %q, want low", items[2].Priority)
	}
}

func TestSQLiteStore_ListWithFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewItem("Pending", "", "", PriorityLow, CategoryRefactor))
	done := NewItem("Done", "", "", PriorityHigh, CategoryBug)
	done.Status = StatusDone
	store.Insert(ctx, done)

	status := StatusPending
	items, err := store.List(ctx, ListFilter{Status: &status})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Title != "Pending" {
		t.Errorf("Title = %q, want Pending", items[0].Title)
	}
}

func TestSQLiteStore_ListWithLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		store.Insert(ctx, NewItem("Item", "", "", PriorityLow, CategoryRefactor))
	}

	items, err := store.List(ctx, ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("len = %d, want 3", len(items))
	}
}

func TestSQLiteStore_DeleteStale(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert an old done item
	old := NewItem("Old done", "", "", PriorityLow, CategoryRefactor)
	old.Status = StatusDone
	old.UpdatedAt = time.Now().UTC().AddDate(0, 0, -60)
	store.Insert(ctx, old)
	// Force the old UpdatedAt
	store.db.ExecContext(ctx, `UPDATE backlog_items SET updated_at=? WHERE id=?`, old.UpdatedAt, old.ID)

	// Insert a recent done item
	recent := NewItem("Recent done", "", "", PriorityLow, CategoryRefactor)
	recent.Status = StatusDone
	store.Insert(ctx, recent)

	// Insert a pending item (should not be deleted)
	pending := NewItem("Pending", "", "", PriorityHigh, CategoryBug)
	store.Insert(ctx, pending)

	n, err := store.DeleteStale(ctx, 30)
	if err != nil {
		t.Fatalf("DeleteStale: %v", err)
	}

	if n != 1 {
		t.Errorf("deleted %d items, want 1", n)
	}

	// pending should still exist
	_, err = store.Get(ctx, pending.ID)
	if err != nil {
		t.Error("pending item should still exist")
	}

	// recent done should still exist
	_, err = store.Get(ctx, recent.ID)
	if err != nil {
		t.Error("recent done item should still exist")
	}

	// old done should be gone
	_, err = store.Get(ctx, old.ID)
	if err == nil {
		t.Error("old done item should be deleted")
	}
}
