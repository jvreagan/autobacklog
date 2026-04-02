package claude

import (
	"fmt"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/backlog"
)

// ReviewPrompt generates a prompt for Claude to review a codebase.
func ReviewPrompt() string {
	return `Review this entire codebase for improvements. For each finding, output a JSON array of objects with these fields:
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
func ImplementPrompt(item *backlog.Item) string {
	return fmt.Sprintf(`Implement the following improvement:

Title: %s
Description: %s
File: %s
Category: %s
Priority: %s

Make the necessary code changes to fix this issue. Follow existing code conventions and style.
If new tests are needed, add them. If existing tests need updating, update them.
Make minimal, focused changes — don't refactor unrelated code.`, item.Title, item.Description, item.FilePath, item.Category, item.Priority)
}

// FixTestPrompt generates a prompt for Claude to fix failing tests.
func FixTestPrompt(testOutput string) string {
	return fmt.Sprintf(`The tests are failing after the recent changes. Here is the test output:

%s

Fix the code so that all tests pass. Make minimal changes — only fix what's broken.
Do not disable or skip tests.`, testOutput)
}

// DocumentPrompt generates a prompt for Claude to update documentation.
func DocumentPrompt(changes []string) string {
	changeList := strings.Join(changes, "\n- ")
	return fmt.Sprintf(`The following changes were made to the codebase:
- %s

Review and update any documentation (README, doc comments, etc.) that should reflect these changes.
Only update documentation that is directly affected by the changes. Don't add documentation where none existed before unless the changes warrant it.`, changeList)
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
