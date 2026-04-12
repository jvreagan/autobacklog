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
	ID             string    `json:"id"`
	RepoURL        string    `json:"repo_url"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	FilePath       string    `json:"file_path"`
	LineNumber     int       `json:"line_number,omitempty"`
	IssueNumber    int       `json:"issue_number,omitempty"`
	Priority       Priority  `json:"priority"`
	Category       Category  `json:"category"`
	Status         Status    `json:"status"`
	Attempts       int       `json:"attempts"`
	PRLink         string    `json:"pr_link,omitempty"`
	LastReviewHash string    `json:"last_review_hash,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ValidPriority returns true if p is a recognized priority value.
func ValidPriority(p Priority) bool {
	switch p {
	case PriorityHigh, PriorityMedium, PriorityLow:
		return true
	}
	return false
}

// ValidCategory returns true if c is a recognized category value.
func ValidCategory(c Category) bool {
	switch c {
	case CategoryBug, CategorySecurity, CategoryPerformance, CategoryRefactor, CategoryTest, CategoryDocs, CategoryStyle:
		return true
	}
	return false
}

// NewItem creates a new backlog item with a generated ID and timestamps.
// Invalid priority defaults to PriorityLow; invalid category defaults to CategoryRefactor (#135).
func NewItem(title, description, filePath string, priority Priority, category Category) *Item {
	if !ValidPriority(priority) {
		priority = PriorityLow
	}
	if !ValidCategory(category) {
		category = CategoryRefactor
	}
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
