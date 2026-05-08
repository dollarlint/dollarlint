package dollarlint

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
		abs = filepath.ToSlash(abs)
		if !strings.HasPrefix(abs, "/") {
			abs = "/" + abs
		}
	} else {
		abs = filepath.ToSlash(abs)
	}
	if strings.HasSuffix(originalPath, string(filepath.Separator)) && !strings.HasSuffix(abs, "/") {
		abs += "/"
	}
	return &url.URL{Scheme: "file", Path: abs}
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
