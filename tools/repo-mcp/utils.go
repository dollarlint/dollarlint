package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dollarlint/dollarlint"
	"github.com/mark3labs/mcp-go/mcp"
)

func structured(value map[string]any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultStructuredOnly(value), nil
}

func findRepoRoot() (string, error) {
	if root := os.Getenv("DOLLARLINT_REPO_ROOT"); root != "" {
		return filepath.Abs(root)
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("find repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func schemaObject(properties map[string]any) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
}

func enumSchema(values []string, description string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

func arrayStringSchema(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "string"}}
}

func mustRawJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func lines(text string) []string {
	raw := strings.Split(strings.TrimSpace(text), "\n")
	if len(raw) == 1 && raw[0] == "" {
		return nil
	}
	return raw
}

func existingPaths(root string, paths []string) []string {
	var existing []string
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			existing = append(existing, path)
		}
	}
	return existing
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func hasDirtyStatus(status string) bool {
	for _, line := range lines(status) {
		if strings.HasPrefix(line, "## ") {
			continue
		}
		return true
	}
	return false
}

func truncate(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return text[:max] + "\n... truncated ..."
}

func cleanRelativePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative to the repo root")
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("path must stay inside the repo")
	}
	return clean, nil
}

func classifyError(message string) string {
	switch {
	case strings.Contains(message, "compile schema"):
		return "schema_compile"
	case strings.Contains(message, "fetch") || strings.Contains(message, "http"):
		return "schema_fetch"
	case strings.Contains(message, "catalog"):
		return "catalog"
	case strings.Contains(message, "config"):
		return "config"
	default:
		return "operational"
	}
}

func classifyResult(result dollarlint.Result) map[string]any {
	counts := map[string]int{}
	for _, file := range result.Files {
		counts[string(file.Status)]++
	}
	return map[string]any{
		"statuses":  counts,
		"hasIssues": result.HasIssues(),
		"warnings":  len(result.Warnings),
	}
}

func resultIssueDigest(result dollarlint.Result, limit int) []map[string]any {
	var digest []map[string]any
	for _, issue := range result.Issues {
		digest = append(digest, map[string]any{
			"file":             issue.RelativePath,
			"instanceLocation": issue.InstanceLocation,
			"keyword":          issue.Keyword,
			"message":          issue.Message,
			"line":             issue.Line,
			"column":           issue.Column,
			"property":         issue.Property,
		})
		if len(digest) >= limit {
			return digest
		}
	}
	return digest
}
