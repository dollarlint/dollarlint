package engine

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateSchemaMapping(mapping SchemaMapping) error {
	raw := strings.TrimSpace(mapping.URL)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("schemas.mappings url %q must be an absolute http or https URL", mapping.URL)
	}
	if parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" || strings.HasSuffix(raw, "#") {
		return fmt.Errorf("schemas.mappings url %q must not have a query or fragment", mapping.URL)
	}
	if strings.TrimSpace(mapping.Path) == "" {
		return fmt.Errorf("schemas.mappings path is required for url %q", mapping.URL)
	}
	local := strings.TrimSpace(mapping.Path)
	if pathURL, err := url.Parse(local); err == nil && pathURL.IsAbs() && !isWindowsDriveAbsolutePath(local) {
		return fmt.Errorf("schemas.mappings path %q must be a filesystem path, not a URL", mapping.Path)
	}
	if info, err := os.Stat(local); err == nil {
		if info.IsDir() && !strings.HasSuffix(raw, "/") {
			return fmt.Errorf("schemas.mappings url %q must end in / when path %q is a directory", mapping.URL, mapping.Path)
		}
		if !info.IsDir() && strings.HasSuffix(raw, "/") {
			return fmt.Errorf("schemas.mappings path %q must be a directory when url %q ends in /", mapping.Path, mapping.URL)
		}
	}
	return nil
}

// mappedSchemaPath returns the local file serving a schema URL (without
// fragment) when a mapping covers it. A mapping whose URL ends in "/" covers
// every URL below that prefix, resolved below its directory; any other mapping
// covers exactly its URL. The longest matching mapping URL wins, and of equal ones the last.
func mappedSchemaPath(raw string, mappings []SchemaMapping) (string, bool, error) {
	raw = normalizedMappingURL(raw)
	best, bestPrefix := -1, ""
	for i, mapping := range mappings {
		prefix := normalizedMappingURL(mapping.URL)
		matched := raw == prefix || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(raw, prefix))
		// Later mappings (a child config's, after `extends`) win ties, as associations do.
		if matched && (best < 0 || len(prefix) >= len(bestPrefix)) {
			best, bestPrefix = i, prefix
		}
	}
	if best < 0 {
		return "", false, nil
	}
	mapping, prefix := mappings[best], bestPrefix
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
	relative := filepath.FromSlash(strings.TrimPrefix(clean, "/"))
	if !filepath.IsLocal(relative) {
		return "", true, fmt.Errorf("mapped schema URL %s has an invalid local path below %s", raw, prefix)
	}
	return filepath.Join(mapping.Path, relative), true, nil
}

func isMappedSchemaURI(raw string, mappings []SchemaMapping) bool {
	key, err := withoutFragment(raw)
	if err != nil {
		return false
	}
	_, ok, _ := mappedSchemaPath(key, mappings)
	return ok
}

// normalizedMappingURL compares schemes and hosts case-insensitively while
// preserving case-sensitive paths and the original URI used by the compiler.
func normalizedMappingURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if parsed, err := url.Parse(raw); err == nil {
		parsed.Host = strings.ToLower(parsed.Host)
		return parsed.String()
	}
	return raw
}

// isMappedSchemaFailure identifies the resource that actually failed, which can
// be a mapped dependency of an unmapped remote or catalog schema. Compiler
// errors retain resource URLs even after the document has loaded successfully.
// The cases below cover jsonschema v6.0.3's URL-bearing compiler errors; review
// new error types and the mapped-dependency regression tests when upgrading.
func isMappedSchemaFailure(err error, mappings []SchemaMapping) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		var uri string
		switch e := err.(type) {
		case *mappedSchemaLoadError:
			return true
		case interface{ Unwrap() []error }:
			for _, cause := range e.Unwrap() {
				if isMappedSchemaFailure(cause, mappings) {
					return true
				}
			}
		case *jsonschema.LoadURLError:
			uri = e.URL
		case *jsonschema.SchemaValidationError:
			uri = e.URL
		case *jsonschema.InvalidRegexError:
			uri = e.URL
		case *jsonschema.InvalidMetaSchemaURLError:
			uri = e.URL
		case *jsonschema.UnsupportedDraftError:
			uri = e.URL
		case *jsonschema.MetaSchemaCycleError:
			uri = e.URL
		case *jsonschema.MetaSchemaMismatchError:
			uri = e.URL
		case *jsonschema.ParseIDError:
			uri = e.URL
		case *jsonschema.ParseAnchorError:
			uri = e.URL
		case *jsonschema.DuplicateIDError:
			uri = e.URL
		case *jsonschema.DuplicateAnchorError:
			uri = e.URL
		case *jsonschema.UnsupportedVocabularyError:
			uri = e.URL
		case *jsonschema.AnchorNotFoundError:
			uri = e.URL
		case *jsonschema.ParseURLError:
			uri = e.URL
		case *jsonschema.InvalidJsonPointerError:
			uri = e.URL
		case *jsonschema.JSONPointerNotFoundError:
			uri = e.URL
		}
		if uri != "" && isMappedSchemaURI(uri, mappings) {
			return true
		}
	}
	return false
}

// mappedSchemaLoadError keeps the public URI and its local source together,
// including in optimized loaders that run before the schema compiler.
type mappedSchemaLoadError struct {
	URI  string
	Path string
	Err  error
}

func (e *mappedSchemaLoadError) Error() string {
	return fmt.Sprintf("mapped schema %s (from %s): %v", e.URI, e.Path, e.Err)
}

func (e *mappedSchemaLoadError) Unwrap() error { return e.Err }

func mappedSchemaLoadFailure(err error) *mappedSchemaLoadError {
	var mapped *mappedSchemaLoadError
	if errors.As(err, &mapped) {
		return mapped
	}
	// jsonschema's LoadURLError exposes Err but does not implement Unwrap.
	var load *jsonschema.LoadURLError
	if errors.As(err, &load) {
		return mappedSchemaLoadFailure(load.Err)
	}
	return nil
}
