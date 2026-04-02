package backlog

import (
	"testing"
	"time"
)

func TestNewItem(t *testing.T) {
	item := NewItem("Fix bug", "Description", "main.go", PriorityHigh, CategoryBug)

	if item.ID == "" {
		t.Error("ID should not be empty")
	}
	if item.Title != "Fix bug" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.Description != "Description" {
		t.Errorf("Description = %q", item.Description)
	}
	if item.FilePath != "main.go" {
		t.Errorf("FilePath = %q", item.FilePath)
	}
	if item.Priority != PriorityHigh {
		t.Errorf("Priority = %q", item.Priority)
	}
	if item.Category != CategoryBug {
		t.Errorf("Category = %q", item.Category)
	}
	if item.Status != StatusPending {
		t.Errorf("Status = %q, want pending", item.Status)
	}
	if item.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", item.Attempts)
	}
	if time.Since(item.CreatedAt) > time.Second {
		t.Error("CreatedAt should be recent")
	}
	if time.Since(item.UpdatedAt) > time.Second {
		t.Error("UpdatedAt should be recent")
	}
}

func TestNewItemUniqueIDs(t *testing.T) {
	a := NewItem("A", "", "", PriorityLow, CategoryRefactor)
	b := NewItem("B", "", "", PriorityLow, CategoryRefactor)

	if a.ID == b.ID {
		t.Error("two items should have different IDs")
	}
}
