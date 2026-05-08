package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const defaultSchemaStoreCatalogURL = "https://www.schemastore.org/api/json/catalog.json"

type schemaStoreCatalog struct {
	Schemas []schemaStoreCatalogEntry `json:"schemas"`
}

type schemaStoreCatalogEntry struct {
	FileMatch []string `json:"fileMatch"`
	URL       string   `json:"url"`
}

func fetchSchemaStoreAssociations(ctx context.Context, cfg Config) ([]SchemaAssociation, error) {
	if err := checkRemoteDomainPolicy(cfg.Schema.SchemaStoreCatalogURL, cfg.Schema); err != nil {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, cfg.Timeouts.Fetch.Duration)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, cfg.Schema.SchemaStoreCatalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create SchemaStore catalog request: %w", err)
	}
	req.Header.Set("User-Agent", "dollarlint")
	resp, err := (&http.Client{Timeout: cfg.Timeouts.Fetch.Duration}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch SchemaStore catalog %s: %w", cfg.Schema.SchemaStoreCatalogURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fetch SchemaStore catalog %s: HTTP %d", cfg.Schema.SchemaStoreCatalogURL, resp.StatusCode)
	}
	var catalog schemaStoreCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode SchemaStore catalog %s: %w", cfg.Schema.SchemaStoreCatalogURL, err)
	}
	associations := make([]SchemaAssociation, 0, len(catalog.Schemas))
	for _, schema := range catalog.Schemas {
		if schema.URL == "" {
			continue
		}
		for _, fileMatch := range schema.FileMatch {
			if fileMatch == "" {
				continue
			}
			associations = append(associations, SchemaAssociation{
				File:   fileMatch,
				Schema: schema.URL,
			})
		}
	}
	return associations, nil
}
