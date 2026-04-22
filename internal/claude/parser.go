package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jvreagan/autobacklog/internal/backlog"
)

// ReviewFinding represents a single finding from Claude's review output.
type ReviewFinding struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Priority    string `json:"priority"`
	Category    string `json:"category"`
}

// TestDetection represents Claude's test detection output.
type TestDetection struct {
	Command   string `json:"command"`
	Framework string `json:"framework"`
}

// ClaudeResponse represents the JSON output from claude --output-format json.
type ClaudeResponse struct {
	Result string `json:"result"`
	Cost   Cost   `json:"cost_usd"`
}

// Cost represents cost information from Claude CLI output.
type Cost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
	Total  float64 `json:"total"`
}

// ParseReviewOutput parses Claude's review response into backlog items.
func ParseReviewOutput(output string) ([]*backlog.Item, float64, error) {
	response, err := parseClaudeResponse(output)
	if err != nil {
		// Try parsing the raw output as JSON array directly
		items, err2 := parseFindings(output)
		if err2 != nil {
			return nil, 0, fmt.Errorf("parsing claude response: %w; raw parse: %w", err, err2)
		}
		return items, 0, nil
	}

	items, err := parseFindings(response.Result)
	if err != nil {
		return nil, response.Cost.Total, err
	}

	return items, response.Cost.Total, nil
}

func parseClaudeResponse(output string) (*ClaudeResponse, error) {
	var resp ClaudeResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func parseFindings(text string) ([]*backlog.Item, error) {
	// Extract JSON array from text (Claude sometimes adds commentary)
	text = extractJSON(text)

	var findings []ReviewFinding
	if err := json.Unmarshal([]byte(text), &findings); err != nil {
		return nil, fmt.Errorf("parsing findings JSON: %w", err)
	}

	var items []*backlog.Item
	for _, f := range findings {
		item := backlog.NewItem(
			f.Title,
			f.Description,
			f.FilePath,
			normalizePriority(f.Priority),
			normalizeCategory(f.Category),
		)
		item.LineNumber = f.LineNumber
		items = append(items, item)
	}

	return items, nil
}

// ParseTestDetection parses Claude's test detection response.
func ParseTestDetection(output string) (*TestDetection, error) {
	response, err := parseClaudeResponse(output)
	if err != nil || response.Result == "" {
		// Try raw parse
		return parseTestDetectionRaw(output)
	}
	return parseTestDetectionRaw(response.Result)
}

func parseTestDetectionRaw(text string) (*TestDetection, error) {
	text = extractJSON(text)
	var det TestDetection
	if err := json.Unmarshal([]byte(text), &det); err != nil {
		return nil, fmt.Errorf("parsing test detection: %w", err)
	}
	return &det, nil
}

// extractJSON attempts to extract a JSON array or object from text that may contain
// surrounding commentary. It uses balanced bracket scanning to find the correct
// closing delimiter, avoiding false matches from nested brackets or commentary.
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// Try to find a balanced JSON array first, then object
	for _, pair := range [][2]byte{{'[', ']'}, {'{', '}'}} {
		if result := findBalanced(text, pair[0], pair[1]); result != "" {
			return result
		}
	}

	// #203: return empty string instead of raw input to avoid large error messages
	return ""
}

// findBalanced finds the first balanced occurrence of open/close delimiters,
// accounting for JSON strings (quoted content is ignored for bracket counting).
func findBalanced(text string, open, close byte) string {
	start := strings.IndexByte(text, open)
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func normalizePriority(s string) backlog.Priority {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high":
		return backlog.PriorityHigh
	case "medium":
		return backlog.PriorityMedium
	default:
		return backlog.PriorityLow
	}
}

func normalizeCategory(s string) backlog.Category {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bug":
		return backlog.CategoryBug
	case "security":
		return backlog.CategorySecurity
	case "performance":
		return backlog.CategoryPerformance
	case "refactor":
		return backlog.CategoryRefactor
	case "test":
		return backlog.CategoryTest
	case "docs":
		return backlog.CategoryDocs
	case "style":
		return backlog.CategoryStyle
	default:
		return backlog.CategoryRefactor
	}
}
