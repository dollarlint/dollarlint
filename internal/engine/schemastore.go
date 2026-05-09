package engine

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
)

const defaultSchemaStoreCatalogURL = "https://www.schemastore.org/api/json/catalog.json"

type schemaStoreCatalog struct {
	Schemas []schemaStoreEntry `json:"schemas"`

	indexed        bool
	exactPaths     map[string]schemaStoreEntry
	exactBasenames map[string]schemaStoreEntry
	globs          []schemaStorePattern
}

type schemaStoreEntry struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	FileMatch []string `json:"fileMatch"`
	URL       string   `json:"url"`
}

type schemaStorePattern struct {
	pattern string
	entry   schemaStoreEntry
}

func loadSchemaStoreCatalog(ctx context.Context, cache *SchemaCache, cfg Config) (*schemaStoreCatalog, *Warning, error) {
	if !catalogEnabled(cfg.Schemas) {
		return nil, nil, nil
	}
	catalog := &schemaStoreCatalog{}
	for _, source := range enabledSchemaStoreCatalogSources(cfg.Schemas) {
		loaded, warning, err := loadSchemaStoreCatalogSource(ctx, cache, cfg, source)
		if err != nil || warning != nil {
			return nil, warning, err
		}
		if loaded != nil {
			catalog.Schemas = append(catalog.Schemas, loaded.Schemas...)
		}
	}
	if len(catalog.Schemas) == 0 {
		return nil, nil, nil
	}
	catalog.buildIndex()
	return catalog, nil, nil
}

func enabledSchemaStoreCatalogSources(cfg SchemaConfig) []CatalogSource {
	sources := cfg.Catalogs.Sources
	if len(sources) == 0 {
		sources = []CatalogSource{defaultSchemaStoreCatalogSource()}
	}
	var out []CatalogSource
	for _, source := range sources {
		if source.Enabled != nil && !*source.Enabled {
			continue
		}
		if source.Format == "" {
			source.Format = "schemastore"
		}
		if source.Name == "" && source.Format == "schemastore" {
			source.Name = "schemastore"
		}
		if source.Format == "schemastore" {
			out = append(out, source)
		}
	}
	return out
}

func loadSchemaStoreCatalogSource(ctx context.Context, cache *SchemaCache, cfg Config, source CatalogSource) (*schemaStoreCatalog, *Warning, error) {
	catalogURL := source.URL
	if catalogURL == "" {
		catalogURL = source.Path
	}
	if catalogURL == "" {
		return nil, nil, nil
	}
	if !remoteFetchEnabled(cfg.Schemas) && isRemoteURI(catalogURL) {
		return schemaStoreCatalogError(cfg, source, "catalog %s requires remote schema fetching", catalogURL)
	}
	resolved, err := resolveCatalogURI(catalogURL)
	if err != nil {
		return schemaStoreCatalogError(cfg, source, "parse catalog URL %q: %v", catalogURL, err)
	}
	doc, err := cache.LoadContext(ctx, resolved)
	if err != nil {
		return schemaStoreCatalogError(cfg, source, "load catalog %s: %v", catalogURL, err)
	}
	baseURL, _ := url.Parse(resolved)
	raw, ok := doc.(map[string]any)
	if !ok {
		return schemaStoreCatalogError(cfg, source, "catalog %s is not an object", catalogURL)
	}
	catalog := &schemaStoreCatalog{}
	for _, rawEntry := range asSlice(raw["schemas"]) {
		entryObject, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		entry := schemaStoreEntry{
			Name:      asString(entryObject["name"]),
			Source:    source.Name,
			URL:       resolveCatalogEntryURL(baseURL, asString(entryObject["url"])),
			FileMatch: asStringSlice(entryObject["fileMatch"]),
		}
		if entry.URL == "" || len(entry.FileMatch) == 0 {
			continue
		}
		catalog.Schemas = append(catalog.Schemas, entry)
	}
	return catalog, nil, nil
}

func schemaStoreCatalogError(cfg Config, source CatalogSource, format string, args ...any) (*schemaStoreCatalog, *Warning, error) {
	message := fmt.Sprintf(format, args...)
	mode, err := catalogFailureMode(cfg.Schemas)
	if err != nil {
		return nil, nil, err
	}
	switch mode {
	case CatalogFailureError:
		return nil, nil, fmt.Errorf("%s", message)
	case CatalogFailureSkip:
		return nil, nil, nil
	default:
		sourceName := source.Name
		if sourceName == "" {
			sourceName = "catalog"
		}
		return nil, &Warning{
			Kind:    "schemaCatalogUnavailable",
			Source:  sourceName,
			Message: message,
		}, nil
	}
}

func resolveCatalogEntryURL(baseURL *url.URL, raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return baseURL.ResolveReference(parsed).String()
}

func resolveCatalogURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse catalog URL %q: %w", raw, err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	file, _ := fileURL(raw)
	return file.String(), nil
}

func applySchemaStoreAssociation(document *Document, catalog *schemaStoreCatalog, matchMode string) {
	if document.Schema != "" || catalog == nil {
		return
	}
	entry, ok := catalog.match(document.RelativePath, matchMode)
	if !ok {
		return
	}
	document.Schema = entry.URL
	document.SchemaSource = entry.schemaSource()
}

func (catalog *schemaStoreCatalog) buildIndex() {
	if catalog == nil || catalog.indexed {
		return
	}
	catalog.indexed = true
	catalog.exactPaths = map[string]schemaStoreEntry{}
	catalog.exactBasenames = map[string]schemaStoreEntry{}
	for _, entry := range catalog.Schemas {
		for _, rawPattern := range entry.FileMatch {
			pattern := cleanGlob(rawPattern)
			if pattern == "" {
				continue
			}
			if hasGlobMeta(pattern) {
				catalog.globs = append(catalog.globs, schemaStorePattern{pattern: pattern, entry: entry})
				continue
			}
			if strings.Contains(pattern, "/") {
				addSchemaStoreExact(catalog.exactPaths, pattern, entry)
				continue
			}
			addSchemaStoreExact(catalog.exactBasenames, pattern, entry)
		}
	}
}

func (catalog *schemaStoreCatalog) match(rel, matchMode string) (schemaStoreEntry, bool) {
	if catalog == nil {
		return schemaStoreEntry{}, false
	}
	if matchMode == "" {
		matchMode = CatalogMatchAuto
	}
	catalog.buildIndex()
	rel = cleanGlob(rel)
	if entry, ok := catalog.exactPaths[rel]; ok {
		return entry, true
	}
	base := path.Base(rel)
	if entry, ok := catalog.exactBasenames[base]; ok && (matchMode == CatalogMatchAll || !lowConfidenceSchemaStoreBasename(base)) {
		return entry, true
	}
	for _, candidate := range catalog.globs {
		if matchMode == CatalogMatchAuto && lowConfidenceSchemaStoreGlob(candidate.pattern) {
			continue
		}
		if matchPattern(candidate.pattern, rel) {
			return candidate.entry, true
		}
	}
	return schemaStoreEntry{}, false
}

func addSchemaStoreExact(entries map[string]schemaStoreEntry, key string, entry schemaStoreEntry) {
	if _, exists := entries[key]; exists {
		return
	}
	entries[key] = entry
}

func lowConfidenceSchemaStoreBasename(base string) bool {
	switch strings.ToLower(base) {
	case "config.json",
		"configuration.json",
		"extensions.json",
		"launch.json",
		"manifest.json",
		"schema.json",
		"settings.json",
		"task.json",
		"tasks.json":
		return true
	default:
		return false
	}
}

func lowConfidenceSchemaStoreGlob(pattern string) bool {
	if strings.Contains(pattern, "/") {
		return false
	}
	return strings.HasPrefix(pattern, "*")
}

func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[{")
}

func (entry schemaStoreEntry) schemaSource() string {
	source := "catalog"
	if entry.Source != "" {
		source += ":" + entry.Source
	}
	if entry.Name != "" {
		source += ":" + entry.Name
	}
	return source
}

func asSlice(value any) []any {
	typed, ok := value.([]any)
	if !ok {
		return nil
	}
	return typed
}

func asString(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

func asStringSlice(value any) []string {
	var out []string
	for _, item := range asSlice(value) {
		if text := asString(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}
