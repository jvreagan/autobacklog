package claude

import (
	"fmt"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/backlog"
)

// docsDirective instructs Claude to read the docs/ directory for project context.
const docsDirective = `Before starting, read all files in the docs/ directory (if it exists) to understand the project's architecture, requirements, and conventions. Use that context to inform your analysis.

`

// maxPromptTestOutput caps test output embedded in prompts to avoid wasting
// tokens on massive test logs (#152).
const maxPromptTestOutput = 10000

// ReviewPrompt generates a prompt for Claude to review a codebase.
func ReviewPrompt() string {
	return docsDirective + `Review this entire codebase for improvements. For each finding, output a JSON array of objects with these fields:
- "title": short description of the issue (string)
- "description": detailed explanation and suggested fix (string)
- "file_path": relative path to the file (string)
- "line_number": approximate line number (integer, 0 if N/A)
- "priority": "high", "medium", or "low" (string)
- "category": one of "bug", "security", "performance", "refactor", "test", "docs", "style" (string)

Focus on:
1. Bugs and correctness issues (high priority)
2. Security vulnerabilities (high priority)
3. Performance problems (medium priority)
4. Missing error handling (medium priority)
5. Code duplication and refactoring opportunities (low-medium priority)
6. Missing or inadequate tests (low-medium priority)
7. Documentation gaps (low priority)

Output ONLY the JSON array, no other text. Example:
[
  {
    "title": "SQL injection in user query",
    "description": "The query in handlers/user.go uses string concatenation instead of parameterized queries.",
    "file_path": "handlers/user.go",
    "line_number": 42,
    "priority": "high",
    "category": "security"
  }
]`
}

// ImplementPrompt generates a prompt for Claude to implement a fix for a backlog item.
// User-supplied fields are clearly delimited to reduce prompt injection risk (#121).
func ImplementPrompt(item *backlog.Item) string {
	return docsDirective + fmt.Sprintf(`Implement the following improvement:

<backlog-item>
Title: %s
Description: %s
File: %s
Category: %s
Priority: %s
</backlog-item>

Make the necessary code changes to fix this issue. Follow existing code conventions and style.
If new tests are needed, add them. If existing tests need updating, update them.
Make minimal, focused changes — don't refactor unrelated code.`, item.Title, item.Description, item.FilePath, item.Category, item.Priority)
}

// FixTestPrompt generates a prompt for Claude to fix failing tests.
// Truncates test output to maxPromptTestOutput to control token usage (#152).
func FixTestPrompt(testOutput string) string {
	if len(testOutput) > maxPromptTestOutput {
		testOutput = "... (truncated) ...\n" + testOutput[len(testOutput)-maxPromptTestOutput:]
	}
	return fmt.Sprintf(`The tests are failing after the recent changes. Here is the test output:

<test-output>
%s
</test-output>

Fix the code so that all tests pass. Make minimal changes — only fix what's broken.
Do not disable or skip tests.`, testOutput)
}

// DocumentPrompt generates a prompt for Claude to update documentation.
// Returns empty string if changes slice is empty (#202).
func DocumentPrompt(changes []string) string {
	if len(changes) == 0 {
		return ""
	}
	changeList := strings.Join(changes, "\n- ")
	return docsDirective + fmt.Sprintf(`The following changes were made to the codebase:
- %s

Review and update any documentation (README, doc comments, etc.) that should reflect these changes.
Only update documentation that is directly affected by the changes. Don't add documentation where none existed before unless the changes warrant it.`, changeList)
}

// AddressReviewPrompt generates a prompt for Claude to address PR review feedback.
// Truncates feedback to maxPromptTestOutput to control token usage.
func AddressReviewPrompt(itemTitle, feedback string) string {
	if len(feedback) > maxPromptTestOutput {
		feedback = "... (truncated) ...\n" + feedback[len(feedback)-maxPromptTestOutput:]
	}
	return docsDirective + fmt.Sprintf(`A pull request for the following item has received review feedback:

<pr-title>%s</pr-title>

<review-feedback>
%s
</review-feedback>

Address the review feedback by making the requested code changes. Follow existing code conventions and style.
Make minimal, focused changes — only address what the reviewers asked for.
Do not refactor unrelated code.`, itemTitle, feedback)
}

// BatchImplementPrompt generates a prompt for Claude to implement multiple items at once.
func BatchImplementPrompt(items []*backlog.Item) string {
	var b strings.Builder
	b.WriteString(docsDirective)
	b.WriteString("Implement the following backlog items. Make all necessary code changes for each item.\n")
	b.WriteString("Follow existing code conventions and style. If new tests are needed, add them.\n")
	b.WriteString("Make minimal, focused changes — don't refactor unrelated code.\n\n")

	for i, item := range items {
		fmt.Fprintf(&b, `<backlog-item index="%d">
Title: %s
Description: %s
File: %s
Category: %s
Priority: %s
</backlog-item>

`, i+1, item.Title, item.Description, item.FilePath, item.Category, item.Priority)
	}

	return b.String()
}

// DetectTestPrompt generates a prompt for Claude to figure out how to test a project.
func DetectTestPrompt() string {
	return `Analyze this project and determine how to run its test suite.
Output a JSON object with:
- "command": the shell command to run tests (string)
- "framework": the name of the test framework (string)

Output ONLY the JSON object, no other text. Example:
{"command": "npm test", "framework": "jest"}`
}
