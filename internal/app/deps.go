package app

import (
	"context"

	"github.com/jamesreagan/autobacklog/internal/claude"
	gh "github.com/jamesreagan/autobacklog/internal/github"
	"github.com/jamesreagan/autobacklog/internal/runner"
)

// Repository abstracts git operations for testability.
type Repository interface {
	WorkDir() string
	CloneOrPull(ctx context.Context) error
	CreateBranch(ctx context.Context, prefix, category, title string) (string, error)
	CheckoutBranch(ctx context.Context, branch string) error
	Push(ctx context.Context, branch string) error
	StageAll(ctx context.Context) error
	Commit(ctx context.Context, message string) error
	HasChanges(ctx context.Context) (bool, error)
	RevertToClean(ctx context.Context) error
	DeleteBranch(ctx context.Context, branch string) error
}

// AIClient abstracts the Claude CLI client for testability.
type AIClient interface {
	Run(ctx context.Context, workDir, prompt string) (string, error)
	RunPrint(ctx context.Context, workDir, prompt string) (string, error)
	Budget() *claude.Budget
}

// TestRunner abstracts test execution for testability.
type TestRunner interface {
	Run(ctx context.Context, workDir, command string, args []string) (*runner.Result, error)
}

// PRCreator abstracts GitHub PR operations for testability.
type PRCreator interface {
	CreatePR(ctx context.Context, workDir string, req gh.PRRequest) (string, error)
	EnableAutoMerge(ctx context.Context, workDir string, prURL string) error
}

// IssueManager abstracts GitHub issue operations for testability.
type IssueManager interface {
	EnsureLabel(ctx context.Context, workDir, label string) error
	CreateIssue(ctx context.Context, workDir, title, body string, labels []string) (int, error)
	ListIssues(ctx context.Context, workDir, label string) ([]gh.Issue, error)
}
