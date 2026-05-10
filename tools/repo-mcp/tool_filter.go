package main

import (
	"os"
	"path"
	"strings"
	"unicode"
)

const toolFilterEnv = "DOLLARLINT_MCP_TOOLS"

type toolNameFilter struct {
	patterns []string
}

func toolNameFilterFromEnv() toolNameFilter {
	return parseToolNameFilter(os.Getenv(toolFilterEnv))
}

func parseToolNameFilter(raw string) toolNameFilter {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	patterns := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		patterns = append(patterns, part)
	}
	return toolNameFilter{patterns: patterns}
}

func (f toolNameFilter) Allows(name string) bool {
	if len(f.patterns) == 0 {
		return true
	}
	for _, pattern := range f.patterns {
		if pattern == "*" || pattern == name {
			return true
		}
		if matched, err := path.Match(pattern, name); err == nil && matched {
			return true
		}
	}
	return false
}
