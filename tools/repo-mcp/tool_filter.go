package main

import (
	"os"
	"path"
	"strings"
	"unicode"
)

const toolFilterEnv = "DOLLARLINT_MCP_TOOLS"

type toolNameFilter struct {
	includePatterns []string
	excludePatterns []string
}

func toolNameFilterFromEnv() toolNameFilter {
	return parseToolNameFilter(os.Getenv(toolFilterEnv))
}

func parseToolNameFilter(raw string) toolNameFilter {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	includePatterns := make([]string, 0, len(parts))
	excludePatterns := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		exclude := strings.HasPrefix(part, "!")
		if exclude {
			part = strings.TrimSpace(strings.TrimPrefix(part, "!"))
			if part == "" {
				continue
			}
			excludePatterns = append(excludePatterns, part)
			continue
		}
		includePatterns = append(includePatterns, part)
	}
	return toolNameFilter{includePatterns: includePatterns, excludePatterns: excludePatterns}
}

func (f toolNameFilter) Allows(name string) bool {
	if toolNameMatchesAny(f.excludePatterns, name) {
		return false
	}
	if len(f.includePatterns) == 0 {
		return true
	}
	return toolNameMatchesAny(f.includePatterns, name)
}

func toolNameMatchesAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if toolNameMatches(pattern, name) {
			return true
		}
	}
	return false
}

func toolNameMatches(pattern, name string) bool {
	if pattern == "*" || pattern == name {
		return true
	}
	matched, err := path.Match(pattern, name)
	return err == nil && matched
}
