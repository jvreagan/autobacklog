package backlog

import (
	"time"

	"github.com/google/uuid"
)

// Priority levels for backlog items.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// Category classifies the type of improvement.
type Category string

const (
	CategoryBug         Category = "bug"
	CategorySecurity    Category = "security"
	CategoryPerformance Category = "performance"
	CategoryRefactor    Category = "refactor"
	CategoryTest        Category = "test"
	CategoryDocs        Category = "docs"
	CategoryStyle       Category = "style"
)

// Status tracks the lifecycle of a backlog item.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
	StatusSkipped    Status = "skipped"
)

// Item represents a single backlog improvement item.
type Item struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	FilePath    string    `json:"file_path"`
	LineNumber  int       `json:"line_number,omitempty"`
	Priority    Priority  `json:"priority"`
	Category    Category  `json:"category"`
	Status      Status    `json:"status"`
	Attempts    int       `json:"attempts"`
	PRLink      string    `json:"pr_link,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewItem creates a new backlog item with a generated ID and timestamps.
func NewItem(title, description, filePath string, priority Priority, category Category) *Item {
	now := time.Now().UTC()
	return &Item{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		FilePath:    filePath,
		Priority:    priority,
		Category:    category,
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
