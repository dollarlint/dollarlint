package engine

import (
	"context"
	"fmt"
	"net/url"
)

const defaultSchemaStoreCatalogURL = "https://www.schemastore.org/api/json/catalog.json"

type schemaStoreCatalog struct {
	Schemas []schemaStoreEntry `json:"schemas"`
}

type schemaStoreEntry struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	FileMatch []string `json:"fileMatch"`
	URL       string   `json:"url"`
}

func loadSchemaStoreCatalog(ctx context.Context, cache *SchemaCache, cfg Config) (*schemaStoreCatalog, *Warning, error) {
	if !schemaStoreEnabled(cfg.Schema) {
		return nil, nil, nil
	}
	catalog := &schemaStoreCatalog{}
	for _, source := range enabledSchemaStoreCatalogSources(cfg.Schema) {
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
	return catalog, nil, nil
}

func enabledSchemaStoreCatalogSources(cfg SchemaConfig) []CatalogSource {
	sources := cfg.Catalogs.Sources
	if len(sources) == 0 {
		source := defaultSchemaStoreCatalogSource()
		source.URL = cfg.SchemaStore.URL
		if source.URL == "" {
			source.URL = cfg.SchemaStoreCatalogURL
		}
		if source.URL == "" {
			source.URL = defaultSchemaStoreCatalogURL
		}
		sources = []CatalogSource{source}
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
	if !remoteFetchEnabled(cfg.Schema) && isRemoteURI(catalogURL) {
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
	mode, err := schemaStoreFailureMode(cfg.Schema)
	if err != nil {
		return nil, nil, err
	}
	switch mode {
	case SchemaStoreFailureError:
		return nil, nil, fmt.Errorf("%s", message)
	case SchemaStoreFailureSkip:
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

func applySchemaStoreAssociation(document *Document, catalog *schemaStoreCatalog) {
	if document.Schema != "" || catalog == nil {
		return
	}
	for _, entry := range catalog.Schemas {
		for _, pattern := range entry.FileMatch {
			if matchPattern(pattern, document.RelativePath) {
				document.Schema = entry.URL
				document.SchemaSource = "catalog"
				if entry.Source != "" {
					document.SchemaSource += ":" + entry.Source
				}
				if entry.Name != "" {
					document.SchemaSource += ":" + entry.Name
				}
				return
			}
		}
	}
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
