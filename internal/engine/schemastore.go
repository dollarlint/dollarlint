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
	FileMatch []string `json:"fileMatch"`
	URL       string   `json:"url"`
}

func loadSchemaStoreCatalog(ctx context.Context, cache *SchemaCache, cfg Config) (*schemaStoreCatalog, *Warning, error) {
	if !schemaStoreEnabled(cfg.Schema) {
		return nil, nil, nil
	}
	catalogURL := cfg.Schema.SchemaStore.URL
	if catalogURL == "" {
		catalogURL = cfg.Schema.SchemaStoreCatalogURL
	}
	if catalogURL == "" {
		catalogURL = defaultSchemaStoreCatalogURL
	}
	if !remoteFetchEnabled(cfg.Schema) && isRemoteURI(catalogURL) {
		return schemaStoreCatalogError(cfg, "schemastore catalog %s requires remote schema fetching", catalogURL)
	}
	resolved, err := resolveCatalogURI(catalogURL)
	if err != nil {
		return schemaStoreCatalogError(cfg, "parse schemastore catalog URL %q: %v", catalogURL, err)
	}
	doc, err := cache.LoadContext(ctx, resolved)
	if err != nil {
		return schemaStoreCatalogError(cfg, "load schemastore catalog %s: %v", catalogURL, err)
	}
	baseURL, _ := url.Parse(resolved)
	raw, ok := doc.(map[string]any)
	if !ok {
		return schemaStoreCatalogError(cfg, "schemastore catalog %s is not an object", catalogURL)
	}
	catalog := &schemaStoreCatalog{}
	for _, rawEntry := range asSlice(raw["schemas"]) {
		entryObject, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		entry := schemaStoreEntry{
			Name:      asString(entryObject["name"]),
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

func schemaStoreCatalogError(cfg Config, format string, args ...any) (*schemaStoreCatalog, *Warning, error) {
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
		return nil, &Warning{
			Kind:    "schemaStoreCatalogUnavailable",
			Source:  "schemastore",
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
		return "", fmt.Errorf("parse schemastore catalog URL %q: %w", raw, err)
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
				document.SchemaSource = "schemastore"
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
