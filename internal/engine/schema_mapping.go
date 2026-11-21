package engine

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func validateSchemaMapping(mapping SchemaMapping) error {
	raw := strings.TrimSpace(mapping.URL)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("schemas.mappings url %q must be an absolute http or https URL", mapping.URL)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || strings.HasSuffix(raw, "#") {
		return fmt.Errorf("schemas.mappings url %q must not have a query or fragment", mapping.URL)
	}
	if strings.TrimSpace(mapping.Path) == "" {
		return fmt.Errorf("schemas.mappings path is required for url %q", mapping.URL)
	}
	return nil
}

// mappedSchemaPath returns the local file serving a schema URL (without
// fragment) when a mapping covers it. A mapping whose URL ends in "/" covers
// every URL below that prefix, resolved below its directory; any other mapping
// covers exactly its URL. The longest matching mapping URL wins.
func mappedSchemaPath(raw string, mappings []SchemaMapping) (string, bool, error) {
	best := -1
	for i, mapping := range mappings {
		prefix := strings.TrimSpace(mapping.URL)
		matched := raw == prefix || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(raw, prefix))
		if matched && (best < 0 || len(prefix) > len(strings.TrimSpace(mappings[best].URL))) {
			best = i
		}
	}
	if best < 0 {
		return "", false, nil
	}
	mapping := mappings[best]
	prefix := strings.TrimSpace(mapping.URL)
	if raw == prefix && !strings.HasSuffix(prefix, "/") {
		return mapping.Path, true, nil
	}
	rest := strings.TrimPrefix(raw, prefix)
	parsed, err := url.Parse(rest)
	if err != nil || parsed.RawQuery != "" || parsed.IsAbs() {
		return "", true, fmt.Errorf("mapped schema URL %s has no local path below %s", raw, prefix)
	}
	clean := path.Clean("/" + parsed.Path)
	// Backslashes and colons would let a decoded path leave the directory on Windows (`..\`, `C:`).
	if rest == "" || clean == "/" || clean != "/"+parsed.Path || strings.ContainsAny(parsed.Path, "\\:\x00") {
		return "", true, fmt.Errorf("mapped schema URL %s has no local path below %s", raw, prefix)
	}
	return filepath.Join(mapping.Path, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), true, nil
}

func isMappedSchemaURI(raw string, mappings []SchemaMapping) bool {
	key, err := withoutFragment(raw)
	if err != nil {
		return false
	}
	_, ok, _ := mappedSchemaPath(key, mappings)
	return ok
}
