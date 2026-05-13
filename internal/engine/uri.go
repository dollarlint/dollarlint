package engine

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func resolveSchemaURI(schema, documentPath string) (string, error) {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return "", fmt.Errorf("empty schema URI")
	}
	if isLocalAbsolutePath(schema, runtime.GOOS) {
		file, _ := fileURL(schema)
		return file.String(), nil
	}
	ref, err := url.Parse(schema)
	if err != nil {
		return "", fmt.Errorf("parse schema URI %q: %w", schema, err)
	}
	if ref.IsAbs() {
		return ref.String(), nil
	}
	basePath := filepath.Dir(documentPath)
	base, _ := fileURL(basePath + string(filepath.Separator))
	return base.ResolveReference(ref).String(), nil
}

func fileURL(path string) (*url.URL, error) {
	abs, _ := filepath.Abs(path)
	return fileURLFromAbs(path, abs, runtime.GOOS), nil
}

func fileURLFromAbs(originalPath, abs, goos string) *url.URL {
	if goos == "windows" {
		abs = slashPathForOS(abs, goos)
		if !strings.HasPrefix(abs, "/") {
			abs = "/" + abs
		}
	} else {
		abs = slashPathForOS(abs, goos)
	}
	if hasTrailingPathSeparator(originalPath, goos) && !strings.HasSuffix(abs, "/") {
		abs += "/"
	}
	return &url.URL{Scheme: "file", Path: abs}
}

func slashPathForOS(path, goos string) string {
	if goos == "windows" {
		return strings.ReplaceAll(path, `\`, "/")
	}
	return filepath.ToSlash(path)
}

func hasTrailingPathSeparator(path, goos string) bool {
	if goos == "windows" {
		return strings.HasSuffix(path, `\`) || strings.HasSuffix(path, "/")
	}
	return strings.HasSuffix(path, "/")
}

func isLocalAbsolutePath(path, goos string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if goos != "windows" {
		return false
	}
	return isWindowsDriveAbsolutePath(path) || strings.HasPrefix(path, `\\`)
}

func isWindowsDriveAbsolutePath(path string) bool {
	if len(path) < 3 || path[1] != ':' {
		return false
	}
	if path[2] != '\\' && path[2] != '/' {
		return false
	}
	return isASCIIAlpha(path[0])
}

func isWindowsDrive(path string) bool {
	return len(path) == 2 && path[1] == ':' && isASCIIAlpha(path[0])
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func withoutFragment(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func isRemoteURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
