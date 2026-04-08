package backlog

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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

// #193: helper checks Insert errors
func insertOrFatal(t *testing.T, store *SQLiteStore, ctx context.Context, item *Item) {
	t.Helper()
	if err := store.Insert(ctx, item); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("Original", "Desc", "f.go", PriorityLow, CategoryRefactor)
	insertOrFatal(t, store, ctx, item)

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

	n, err := store.DeleteStale(ctx, "", 30)
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

func TestSQLiteStore_ListWithPriorityFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewItem("High", "", "", PriorityHigh, CategoryBug))
	store.Insert(ctx, NewItem("Low", "", "", PriorityLow, CategoryRefactor))

	p := PriorityHigh
	items, err := store.List(ctx, ListFilter{Priority: &p})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Title != "High" {
		t.Errorf("Title = %q, want High", items[0].Title)
	}
}

func TestSQLiteStore_ListWithCategoryFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewItem("Bug", "", "", PriorityHigh, CategoryBug))
	store.Insert(ctx, NewItem("Refactor", "", "", PriorityLow, CategoryRefactor))

	c := CategoryBug
	items, err := store.List(ctx, ListFilter{Category: &c})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Title != "Bug" {
		t.Errorf("Title = %q, want Bug", items[0].Title)
	}
}

func TestSQLiteStore_Get_NonExistentID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent-id")
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

func TestSQLiteStore_Insert_DuplicateID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("First", "desc", "f.go", PriorityHigh, CategoryBug)
	if err := store.Insert(ctx, item); err != nil {
		t.Fatal(err)
	}

	// Insert another item with the same ID
	dup := &Item{
		ID:        item.ID,
		Title:     "Duplicate",
		Priority:  PriorityLow,
		Category:  CategoryRefactor,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err := store.Insert(ctx, dup)
	if err == nil {
		t.Error("expected error for duplicate ID insertion")
	}
}

func TestSQLiteStore_DeleteStale_OnlyTerminalStatuses(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert old items with various statuses
	statuses := []Status{StatusPending, StatusInProgress, StatusDone, StatusFailed, StatusSkipped}
	for _, s := range statuses {
		item := NewItem("Item "+string(s), "", "", PriorityLow, CategoryRefactor)
		item.Status = s
		store.Insert(ctx, item)
		// Force old updated_at
		store.db.ExecContext(ctx, `UPDATE backlog_items SET updated_at=? WHERE id=?`,
			time.Now().UTC().AddDate(0, 0, -60), item.ID)
	}

	n, err := store.DeleteStale(ctx, "", 30)
	if err != nil {
		t.Fatal(err)
	}
	// Only done, failed, skipped should be deleted (3 terminal statuses)
	if n != 3 {
		t.Errorf("deleted %d items, want 3 (done+failed+skipped)", n)
	}

	// Verify pending and in_progress still exist
	items, _ := store.List(ctx, ListFilter{})
	if len(items) != 2 {
		t.Errorf("remaining items = %d, want 2 (pending + in_progress)", len(items))
	}
}

func TestSQLiteStore_IssueNumber_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("Issue item", "desc", "f.go", PriorityHigh, CategoryBug)
	item.IssueNumber = 42

	if err := store.Insert(ctx, item); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := store.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.IssueNumber != 42 {
		t.Errorf("IssueNumber = %d, want 42", got.IssueNumber)
	}

	// Update the issue number
	got.IssueNumber = 99
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got2, _ := store.Get(ctx, item.ID)
	if got2.IssueNumber != 99 {
		t.Errorf("IssueNumber after update = %d, want 99", got2.IssueNumber)
	}
}

func TestSQLiteStore_ListFilterByIssueNumber(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item1 := NewItem("No issue", "desc", "a.go", PriorityHigh, CategoryBug)
	item1.IssueNumber = 0
	store.Insert(ctx, item1)

	item2 := NewItem("Has issue", "desc", "b.go", PriorityHigh, CategoryBug)
	item2.IssueNumber = 10
	store.Insert(ctx, item2)

	// Filter for items with issue_number=0 (no issue)
	zero := 0
	items, err := store.List(ctx, ListFilter{IssueNumber: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Title != "No issue" {
		t.Errorf("Title = %q, want 'No issue'", items[0].Title)
	}

	// Filter for items with issue_number=10
	ten := 10
	items2, err := store.List(ctx, ListFilter{IssueNumber: &ten})
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 1 {
		t.Fatalf("len = %d, want 1", len(items2))
	}
	if items2[0].Title != "Has issue" {
		t.Errorf("Title = %q, want 'Has issue'", items2[0].Title)
	}
}

// #189: Update of a non-existent item should return an error.
func TestSQLiteStore_Update_NonExistent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item := NewItem("Ghost", "desc", "f.go", PriorityHigh, CategoryBug)
	// Don't insert — update directly
	err := store.Update(ctx, item)
	if err == nil {
		t.Error("expected error when updating non-existent item")
	}
}

// #190: Delete of a non-existent item should return an error.
func TestSQLiteStore_Delete_NonExistent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, "non-existent-id")
	if err == nil {
		t.Error("expected error when deleting non-existent item")
	}
}

// #191: NewSQLiteStore with an invalid path should return an error.
func TestNewSQLiteStore_InvalidPath(t *testing.T) {
	_, err := NewSQLiteStore("/nonexistent/deeply/nested/path/test.db")
	if err == nil {
		t.Error("expected error for invalid DB path")
	}
}

// #192: List with multiple combined filters.
func TestSQLiteStore_ListCombinedFilters(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	repoURL := "https://github.com/org/repo.git"

	i1 := NewItem("High Bug Pending", "", "a.go", PriorityHigh, CategoryBug)
	i1.RepoURL = repoURL
	insertOrFatal(t, store, ctx, i1)

	i2 := NewItem("High Bug Done", "", "b.go", PriorityHigh, CategoryBug)
	i2.Status = StatusDone
	i2.RepoURL = repoURL
	insertOrFatal(t, store, ctx, i2)

	i3 := NewItem("Low Refactor Pending", "", "c.go", PriorityLow, CategoryRefactor)
	i3.RepoURL = repoURL
	insertOrFatal(t, store, ctx, i3)

	// Filter: high priority + pending status + scoped to repo
	status := StatusPending
	pri := PriorityHigh
	items, err := store.List(ctx, ListFilter{
		RepoURL:  &repoURL,
		Status:   &status,
		Priority: &pri,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].Title != "High Bug Pending" {
		t.Errorf("Title = %q, want 'High Bug Pending'", items[0].Title)
	}
}

func TestSQLiteStore_MultiTenantIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	repoA := "https://github.com/org/repo-a.git"
	repoB := "https://github.com/org/repo-b.git"

	itemA := NewItem("Bug in A", "desc", "a.go", PriorityHigh, CategoryBug)
	itemA.RepoURL = repoA
	insertOrFatal(t, store, ctx, itemA)

	itemB := NewItem("Bug in B", "desc", "b.go", PriorityHigh, CategoryBug)
	itemB.RepoURL = repoB
	insertOrFatal(t, store, ctx, itemB)

	// List scoped to repoA
	itemsA, err := store.List(ctx, ListFilter{RepoURL: &repoA})
	if err != nil {
		t.Fatal(err)
	}
	if len(itemsA) != 1 {
		t.Errorf("repoA items = %d, want 1", len(itemsA))
	}
	if itemsA[0].Title != "Bug in A" {
		t.Errorf("Title = %q, want 'Bug in A'", itemsA[0].Title)
	}

	// List scoped to repoB
	itemsB, err := store.List(ctx, ListFilter{RepoURL: &repoB})
	if err != nil {
		t.Fatal(err)
	}
	if len(itemsB) != 1 {
		t.Errorf("repoB items = %d, want 1", len(itemsB))
	}

	// Unscoped list returns both
	all, err := store.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("all items = %d, want 2", len(all))
	}

	// DeleteStale scoped to repoA only deletes repoA items
	itemA.Status = StatusDone
	store.Update(ctx, itemA)
	store.db.ExecContext(ctx, `UPDATE backlog_items SET updated_at=? WHERE id=?`,
		time.Now().UTC().AddDate(0, 0, -60), itemA.ID)

	n, err := store.DeleteStale(ctx, repoA, 30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("deleted %d, want 1", n)
	}

	// repoB item should still exist
	_, err = store.Get(ctx, itemB.ID)
	if err != nil {
		t.Error("repoB item should still exist after repoA stale cleanup")
	}
}

func TestSQLiteStore_RunInTx_Commit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item1 := NewItem("Tx Item 1", "desc", "a.go", PriorityHigh, CategoryBug)
	item2 := NewItem("Tx Item 2", "desc", "b.go", PriorityLow, CategoryRefactor)

	err := store.RunInTx(ctx, func(tx Store) error {
		if err := tx.Insert(ctx, item1); err != nil {
			return err
		}
		return tx.Insert(ctx, item2)
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}

	// Both items should be visible after commit.
	got1, err := store.Get(ctx, item1.ID)
	if err != nil {
		t.Fatalf("Get item1 after commit: %v", err)
	}
	if got1.Title != "Tx Item 1" {
		t.Errorf("Title = %q, want 'Tx Item 1'", got1.Title)
	}

	got2, err := store.Get(ctx, item2.ID)
	if err != nil {
		t.Fatalf("Get item2 after commit: %v", err)
	}
	if got2.Title != "Tx Item 2" {
		t.Errorf("Title = %q, want 'Tx Item 2'", got2.Title)
	}
}

func TestSQLiteStore_RunInTx_Rollback(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	item1 := NewItem("Will Rollback 1", "desc", "a.go", PriorityHigh, CategoryBug)
	item2 := NewItem("Will Rollback 2", "desc", "b.go", PriorityLow, CategoryRefactor)

	err := store.RunInTx(ctx, func(tx Store) error {
		if err := tx.Insert(ctx, item1); err != nil {
			return err
		}
		if err := tx.Insert(ctx, item2); err != nil {
			return err
		}
		return fmt.Errorf("simulated failure")
	})
	if err == nil {
		t.Fatal("expected error from RunInTx")
	}
	if !strings.Contains(err.Error(), "simulated failure") {
		t.Errorf("unexpected error: %v", err)
	}

	// Neither item should exist after rollback.
	_, err = store.Get(ctx, item1.ID)
	if err == nil {
		t.Error("item1 should not exist after rollback")
	}
	_, err = store.Get(ctx, item2.ID)
	if err == nil {
		t.Error("item2 should not exist after rollback")
	}
}

func TestSQLiteStore_RunInTx_AtomicIngestFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Pre-insert an item so we can trigger a duplicate ID error inside the tx.
	existing := NewItem("Existing", "desc", "x.go", PriorityMedium, CategoryRefactor)
	insertOrFatal(t, store, ctx, existing)

	item1 := NewItem("New Item", "desc", "a.go", PriorityHigh, CategoryBug)

	err := store.RunInTx(ctx, func(tx Store) error {
		if err := tx.Insert(ctx, item1); err != nil {
			return err
		}
		// Insert with duplicate ID — should fail
		dup := &Item{
			ID:        existing.ID,
			Title:     "Duplicate",
			Priority:  PriorityLow,
			Category:  CategoryRefactor,
			Status:    StatusPending,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		return tx.Insert(ctx, dup)
	})
	if err == nil {
		t.Fatal("expected error from duplicate insert in tx")
	}

	// item1 should be rolled back because the tx failed.
	_, err = store.Get(ctx, item1.ID)
	if err == nil {
		t.Error("item1 should not exist after failed tx (atomic rollback)")
	}

	// existing item should still be there.
	got, err := store.Get(ctx, existing.ID)
	if err != nil {
		t.Fatalf("existing item should still exist: %v", err)
	}
	if got.Title != "Existing" {
		t.Errorf("Title = %q, want 'Existing'", got.Title)
	}
}
