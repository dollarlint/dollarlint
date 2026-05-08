package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaStoreCatalogAssociations(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	writeFile(t, catalogPath, `{
  "version": 1,
  "schemas": [
    {
      "name": "Example conventional config",
      "fileMatch": ["example.config.json", "**/.example/config.yaml"],
      "url": "./config.schema.json"
    }
  ]
}`)
	writeFile(t, filepath.Join(dir, "config.schema.json"), `{
  "type": "object",
  "required": ["name"],
  "additionalProperties": false,
  "properties": {
    "name": {"type": "string"}
  }
}`)
	writeFile(t, filepath.Join(dir, "example.config.json"), `{"name": 42}`)
	writeFile(t, filepath.Join(dir, ".example", "config.yaml"), `name: ok`)
	writeFile(t, filepath.Join(dir, "plain.json"), `{}`)

	cfg := DefaultConfig()
	cfg.Schema.SchemaStore.Enabled = true
	cfg.Schema.SchemaStore.URL = catalogPath
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Discovered != 5 || result.Summary.Validated != 2 || result.Summary.Skipped != 3 || result.Summary.Issues != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	var sawSchemaStoreSource, sawSkipped bool
	for _, file := range result.Files {
		if file.RelativePath == "example.config.json" {
			if file.SchemaSource != "schemastore:Example conventional config" {
				t.Fatalf("schema source = %q", file.SchemaSource)
			}
			sawSchemaStoreSource = true
		}
		if file.RelativePath == "plain.json" && file.Status == StatusSkipped {
			sawSkipped = true
		}
	}
	if !sawSchemaStoreSource || !sawSkipped {
		t.Fatalf("files = %+v", result.Files)
	}
	if len(result.Issues) != 1 || result.Issues[0].Property != "name" {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func TestSchemaStorePrecedenceAndDisabledDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "catalog.json"), `{
  "schemas": [
    {"name": "Bad match", "fileMatch": ["settings.json"], "url": "./bad.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(dir, "bad.schema.json"), `{"type":"object","required":["bad"]}`)
	writeFile(t, filepath.Join(dir, "explicit.schema.json"), `{"type":"object","required":["ok"],"properties":{"$schema":{"type":"string"},"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(dir, "settings.json"), `{"$schema":"./explicit.schema.json","ok": true}`)

	cfg := DefaultConfig()
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint default: %v", err)
	}
	if result.Summary.Validated != 1 || result.Summary.Issues != 0 {
		t.Fatalf("default summary = %+v issues=%+v", result.Summary, result.Issues)
	}

	cfg.Schema.SchemaStore.Enabled = true
	cfg.Schema.SchemaStore.URL = filepath.Join(dir, "catalog.json")
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint schemastore: %v", err)
	}
	if result.Summary.Issues != 0 {
		t.Fatalf("explicit schema should win over schemastore: %+v", result.Issues)
	}
	for _, file := range result.Files {
		if file.RelativePath == "settings.json" && file.SchemaSource != "$schema" {
			t.Fatalf("schema source = %q", file.SchemaSource)
		}
	}
}

func TestSchemaStoreErrors(t *testing.T) {
	dir := t.TempDir()
	cache := NewSchemaCache(DefaultConfig())
	invalidPolicy := Config{Schema: SchemaConfig{SchemaStore: SchemaStoreConfig{Failure: "explode"}}}
	if _, _, err := schemaStoreCatalogError(invalidPolicy, "boom"); err == nil || !strings.Contains(err.Error(), "unsupported schemaStore failure policy") {
		t.Fatalf("expected invalid failure policy error, got %v", err)
	}
	if catalog, warning, err := loadSchemaStoreCatalog(context.Background(), cache, DefaultConfig()); err != nil || warning != nil || catalog != nil {
		t.Fatalf("disabled catalog = %+v, %+v, %v", catalog, warning, err)
	}
	emptyURLConfig := Config{Schema: SchemaConfig{SchemaStore: SchemaStoreConfig{Enabled: true}}}
	disabled := false
	emptyURLConfig.Schema.FetchRemote = &disabled
	if catalog, warning, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(DefaultConfig()), emptyURLConfig); err != nil || catalog != nil || warning == nil || !strings.Contains(warning.Message, "requires remote schema fetching") {
		t.Fatalf("warn remote-disabled catalog = %+v, %+v, %v", catalog, warning, err)
	}
	emptyURLConfig.Schema.SchemaStore.Failure = SchemaStoreFailureSkip
	if catalog, warning, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(DefaultConfig()), emptyURLConfig); err != nil || warning != nil || catalog != nil {
		t.Fatalf("skip remote-disabled catalog = %+v, %+v, %v", catalog, warning, err)
	}
	emptyURLConfig.Schema.SchemaStore.Strict = true
	if _, _, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(DefaultConfig()), emptyURLConfig); err == nil || !strings.Contains(err.Error(), defaultSchemaStoreCatalogURL) {
		t.Fatalf("expected strict default catalog URL error, got %v", err)
	}

	cfg := DefaultConfig()
	cfg.Schema.SchemaStore.Enabled = true
	cfg.Schema.SchemaStore.URL = "https://example.invalid/catalog.json"
	cfg.Schema.FetchRemote = &disabled
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("warn remote disabled should continue, got %v", err)
	}
	if result.Summary.Warnings != 1 || len(result.Warnings) != 1 {
		t.Fatalf("expected schemastore warning, got %+v warnings=%+v", result.Summary, result.Warnings)
	}
	cfg.Schema.SchemaStore.Failure = SchemaStoreFailureSkip
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("skip remote disabled should continue, got %v", err)
	}
	if result.Summary.Warnings != 0 || len(result.Warnings) != 0 {
		t.Fatalf("expected no schemastore warnings, got %+v warnings=%+v", result.Summary, result.Warnings)
	}
	cfg.Schema.SchemaStore.Failure = ""
	cfg.Schema.SchemaStore.Strict = true
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "requires remote schema fetching") {
		t.Fatalf("expected remote disabled error, got %v", err)
	}
	if resolved, err := resolveCatalogURI("https://example.com/catalog.json"); err != nil || resolved != "https://example.com/catalog.json" {
		t.Fatalf("resolve remote catalog = %q, %v", resolved, err)
	}

	writeFile(t, filepath.Join(dir, "catalog.json"), `[]`)
	cfg.Schema.FetchRemote = nil
	cfg.Schema.SchemaStore.URL = filepath.Join(dir, "catalog.json")
	cfg.Schema.SchemaStore.Strict = false
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("warn invalid catalog should continue, got %v", err)
	}
	if result.Summary.Warnings != 1 || !strings.Contains(result.Warnings[0].Message, "is not an object") {
		t.Fatalf("expected invalid catalog warning, got %+v warnings=%+v", result.Summary, result.Warnings)
	}
	cfg.Schema.SchemaStore.Strict = true
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "is not an object") {
		t.Fatalf("expected invalid catalog error, got %v", err)
	}

	cfg.Schema.SchemaStore.URL = filepath.Join(dir, "missing-catalog.json")
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "load schemastore catalog") {
		t.Fatalf("expected missing catalog error, got %v", err)
	}
	if _, err := resolveCatalogURI("%"); err == nil || !strings.Contains(err.Error(), "parse schemastore catalog URL") {
		t.Fatalf("expected bad catalog URL error, got %v", err)
	}
	cfg.Schema.SchemaStore.URL = "%"
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "parse schemastore catalog URL") {
		t.Fatalf("expected bad catalog URL lint error, got %v", err)
	}
}

func TestSchemaStoreCatalogParsingEdges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "catalog.json"), `{
  "schemas": [
    "not an entry",
    {"name": "missing url", "fileMatch": ["missing-url.json"]},
    {"name": "missing match", "url": "./missing-match.schema.json"},
    {"name": 42, "fileMatch": "nope", "url": 99},
    {"fileMatch": ["valid.json"], "url": "./valid.schema.json"},
    {"name": "Invalid URL", "fileMatch": ["invalid-url.json"], "url": "%"}
  ]
}`)
	cfg := DefaultConfig()
	cfg.Schema.SchemaStore.Enabled = true
	cfg.Schema.SchemaStore.URL = filepath.Join(dir, "catalog.json")
	catalog, warning, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(cfg), cfg)
	if err != nil {
		t.Fatalf("loadSchemaStoreCatalog: %v", err)
	}
	if warning != nil {
		t.Fatalf("unexpected warning: %+v", warning)
	}
	if len(catalog.Schemas) != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}
	if catalog.Schemas[0].Name != "" || catalog.Schemas[0].URL == "./valid.schema.json" {
		t.Fatalf("relative URL was not resolved or name was not defaulted: %+v", catalog.Schemas[0])
	}
	if catalog.Schemas[1].URL != "%" {
		t.Fatalf("invalid entry URL should be preserved: %+v", catalog.Schemas[1])
	}
}
