package backlog

import (
	"context"
	"testing"
)

func TestEvaluateThreshold_HighPriority(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewItem("Critical bug", "desc", "f.go", PriorityHigh, CategoryBug))

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if !result.ShouldImplement {
		t.Error("ShouldImplement = false, want true (high priority item exists)")
	}
	if len(result.SelectedItems) != 1 {
		t.Errorf("SelectedItems = %d, want 1", len(result.SelectedItems))
	}
	if result.SelectedItems[0].Priority != PriorityHigh {
		t.Error("selected item should be high priority")
	}
}

func TestEvaluateThreshold_MediumBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		store.Insert(ctx, NewItem("Medium item", "desc", "f.go", PriorityMedium, CategoryRefactor))
	}

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if !result.ShouldImplement {
		t.Error("ShouldImplement = false, want true (medium threshold met)")
	}
	if len(result.SelectedItems) != 3 {
		t.Errorf("SelectedItems = %d, want 3", len(result.SelectedItems))
	}
}

func TestEvaluateThreshold_LowBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		store.Insert(ctx, NewItem("Low item", "desc", "f.go", PriorityLow, CategoryStyle))
	}

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if !result.ShouldImplement {
		t.Error("ShouldImplement = false, want true (low threshold met)")
	}
	if len(result.SelectedItems) != 5 {
		t.Errorf("SelectedItems = %d, want 5", len(result.SelectedItems))
	}
}

func TestEvaluateThreshold_BelowThreshold(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// 2 medium items (below threshold of 3)
	store.Insert(ctx, NewItem("Med 1", "", "", PriorityMedium, CategoryRefactor))
	store.Insert(ctx, NewItem("Med 2", "", "", PriorityMedium, CategoryRefactor))

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if result.ShouldImplement {
		t.Error("ShouldImplement = true, want false (below threshold)")
	}
}

func TestEvaluateThreshold_MaxPerCycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert 10 high priority items
	for i := 0; i < 10; i++ {
		store.Insert(ctx, NewItem("High item", "desc", "f.go", PriorityHigh, CategoryBug))
	}

	// Max 3 per cycle
	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 3)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if !result.ShouldImplement {
		t.Error("ShouldImplement = false")
	}
	if len(result.SelectedItems) != 3 {
		t.Errorf("SelectedItems = %d, want 3 (capped at maxPerCycle)", len(result.SelectedItems))
	}
}

func TestEvaluateThreshold_EmptyBacklog(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if result.ShouldImplement {
		t.Error("ShouldImplement = true, want false (empty backlog)")
	}
	if result.Reason != "no thresholds met" {
		t.Errorf("Reason = %q", result.Reason)
	}
}

func TestEvaluateThreshold_IgnoresNonPending(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert a done high-priority item
	done := NewItem("Done item", "", "", PriorityHigh, CategoryBug)
	done.Status = StatusDone
	store.Insert(ctx, done)

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if result.ShouldImplement {
		t.Error("ShouldImplement = true, want false (only done items)")
	}
}

func TestEvaluateThreshold_Combined(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// 1 high + 3 medium
	store.Insert(ctx, NewItem("High", "", "", PriorityHigh, CategoryBug))
	for i := 0; i < 3; i++ {
		store.Insert(ctx, NewItem("Medium", "", "", PriorityMedium, CategoryRefactor))
	}

	result, err := EvaluateThreshold(ctx, store, 1, 3, 5, 10)
	if err != nil {
		t.Fatalf("EvaluateThreshold: %v", err)
	}

	if !result.ShouldImplement {
		t.Error("ShouldImplement = false")
	}
	if len(result.SelectedItems) != 4 {
		t.Errorf("SelectedItems = %d, want 4 (1 high + 3 medium)", len(result.SelectedItems))
	}
}
