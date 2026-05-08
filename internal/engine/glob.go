package engine

import (
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func matchAny(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		if matchPattern(pattern, rel) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, rel string) bool {
	pattern = cleanGlob(pattern)
	rel = cleanGlob(rel)
	if pattern == "" {
		return false
	}
	if ok, _ := doublestar.PathMatch(pattern, rel); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := doublestar.PathMatch(pattern, path.Base(rel)); ok {
			return true
		}
	}
	return false
}

func cleanGlob(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "./")
	value = path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if value == "." {
		return ""
	}
	return value
}
