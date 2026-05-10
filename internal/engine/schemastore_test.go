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
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "company", Format: "schemastore", Path: catalogPath}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Discovered != 5 || result.Summary.Validated != 2 || result.Summary.Skipped != 3 || result.Summary.Issues.Total != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	var sawCatalogSource, sawSkipped bool
	for _, file := range result.Files {
		if file.RelativePath == "example.config.json" {
			if file.SchemaSource != "catalog:company:Example conventional config" {
				t.Fatalf("schema source = %q", file.SchemaSource)
			}
			if file.SchemaMatch == nil || file.SchemaMatch.Action != SchemaMatchActionMatched || file.SchemaMatch.MatchType != SchemaMatchTypeExactBasename || file.SchemaMatch.Confidence != SchemaMatchConfidenceMedium || !strings.Contains(file.SchemaMatch.Reason, "example.config.json") {
				t.Fatalf("schema match = %+v", file.SchemaMatch)
			}
			if !strings.Contains(file.SchemaMatch.SuggestedAssociation, `file = "example.config.json"`) || !strings.Contains(file.SchemaMatch.SuggestedAssociation, `schema = "file://`) {
				t.Fatalf("suggested association = %q", file.SchemaMatch.SuggestedAssociation)
			}
			sawCatalogSource = true
		}
		if file.RelativePath == "plain.json" && file.Status == StatusSkipped {
			sawSkipped = true
		}
	}
	if !sawCatalogSource || !sawSkipped {
		t.Fatalf("files = %+v", result.Files)
	}
	if len(result.Issues) != 1 || result.Issues[0].Property != "name" {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func TestSchemaStoreAutoSkipsGenericBasenameMatches(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	writeFile(t, catalogPath, `{
  "schemas": [
    {"name": "Generic tasks", "fileMatch": ["tasks.json"], "url": "./generic.schema.json"},
    {"name": "Path tasks", "fileMatch": ["config/tasks.json"], "url": "./generic.schema.json"},
    {"name": "Package", "fileMatch": ["package.json"], "url": "./package.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(dir, "generic.schema.json"), `{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(dir, "package.schema.json"), `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, "tasks.json"), `{}`)
	writeFile(t, filepath.Join(dir, "config", "tasks.json"), `{}`)
	writeFile(t, filepath.Join(dir, "package.json"), `{}`)

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: catalogPath}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint auto: %v", err)
	}
	if result.Summary.Validated != 2 || result.Summary.Issues.Total != 2 {
		t.Fatalf("auto summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	for _, file := range result.Files {
		switch file.RelativePath {
		case "tasks.json":
			if file.Status != StatusSkipped || file.SchemaSource != "" {
				t.Fatalf("generic basename should be skipped in auto mode: %+v", file)
			}
			if file.SchemaMatch == nil || file.SchemaMatch.Action != SchemaMatchActionSkippedLowConfidence || file.SchemaMatch.Confidence != SchemaMatchConfidenceLow || !strings.Contains(file.SchemaMatch.SuggestedCatalogIgnore, "[[schemas.catalogs.ignore]]") {
				t.Fatalf("generic basename skip match = %+v", file.SchemaMatch)
			}
		case "config/tasks.json":
			if file.SchemaSource != "catalog:test:Path tasks" || file.SchemaMatch == nil || file.SchemaMatch.MatchType != SchemaMatchTypeExactPath || file.SchemaMatch.Confidence != SchemaMatchConfidenceHigh {
				t.Fatalf("path-specific match was not applied: %+v", file)
			}
		case "package.json":
			if file.SchemaSource != "catalog:test:Package" || file.SchemaMatch == nil || file.SchemaMatch.MatchType != SchemaMatchTypeExactBasename || file.SchemaMatch.Confidence != SchemaMatchConfidenceMedium {
				t.Fatalf("distinctive basename match was not applied: %+v", file)
			}
		}
	}

	cfg.Schemas.Catalogs.Match = CatalogMatchAll
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint all: %v", err)
	}
	if result.Summary.Validated != 3 || result.Summary.Issues.Total != 3 {
		t.Fatalf("all summary = %+v issues=%+v", result.Summary, result.Issues)
	}
}

func TestSchemaStoreAutoSkipsLeadingWildcardBasenameGlobs(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	writeFile(t, catalogPath, `{
  "schemas": [
    {"name": "Broad app", "fileMatch": ["*.app.json"], "url": "./broad.schema.json"},
    {"name": "TypeScript", "fileMatch": ["tsconfig*.json"], "url": "./tsconfig.schema.json"},
    {"name": "Rubocop", "fileMatch": ["*.rubocop.yml"], "url": "./rubocop.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(dir, "broad.schema.json"), `{"type":"object","required":["protocol"]}`)
	writeFile(t, filepath.Join(dir, "tsconfig.schema.json"), `{"type":"object","properties":{"compilerOptions":{"type":"object"}}}`)
	writeFile(t, filepath.Join(dir, "rubocop.schema.json"), `{"type":"object"}`)
	writeFile(t, filepath.Join(dir, "tsconfig.app.json"), `{"compilerOptions":{}}`)
	writeFile(t, filepath.Join(dir, "custom.app.json"), `{}`)
	writeFile(t, filepath.Join(dir, ".rubocop.yml"), `{}`)

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: catalogPath}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint auto: %v", err)
	}
	if result.Summary.Issues.Total != 0 {
		t.Fatalf("auto summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	for _, file := range result.Files {
		switch file.RelativePath {
		case "tsconfig.app.json":
			if file.SchemaSource != "catalog:test:TypeScript" {
				t.Fatalf("tsconfig should use specific glob: %+v", file)
			}
		case "custom.app.json":
			if file.SchemaSource != "" || file.Status != StatusSkipped {
				t.Fatalf("leading wildcard glob should be skipped in auto mode: %+v", file)
			}
			if file.SchemaMatch == nil || file.SchemaMatch.Action != SchemaMatchActionSkippedLowConfidence || file.SchemaMatch.MatchType != SchemaMatchTypeBasenameGlob {
				t.Fatalf("leading wildcard skip match = %+v", file.SchemaMatch)
			}
		case ".rubocop.yml":
			if file.SchemaSource != "catalog:test:Rubocop" || file.SchemaMatch == nil || file.SchemaMatch.Action != SchemaMatchActionMatched {
				t.Fatalf("distinctive leading wildcard glob should be applied: %+v", file)
			}
		}
	}

	cfg.Schemas.Catalogs.Match = CatalogMatchAll
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint all: %v", err)
	}
	if result.Summary.Issues.Total != 2 {
		t.Fatalf("all summary = %+v issues=%+v", result.Summary, result.Issues)
	}
}

func TestSchemaStorePathPatternsOutrankBasenames(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	writeFile(t, catalogPath, `{
  "schemas": [
    {"name": "Generic OpenAPI", "fileMatch": ["openapi.yaml"], "url": "./generic.schema.json"},
    {"name": "Workflow", "fileMatch": [".github/workflows/*.yaml"], "url": "./workflow.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(dir, "generic.schema.json"), `{"type":"object","required":["openapi"]}`)
	writeFile(t, filepath.Join(dir, "workflow.schema.json"), `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, ".github", "workflows", "openapi.yaml"), `name: ci`)

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: catalogPath}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Issues.Total != 0 || result.Summary.Validated != 1 {
		t.Fatalf("summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	for _, file := range result.Files {
		if file.RelativePath == ".github/workflows/openapi.yaml" {
			if file.SchemaSource != "catalog:test:Workflow" || file.SchemaMatch == nil || file.SchemaMatch.MatchType != SchemaMatchTypePathGlob {
				t.Fatalf("path glob should outrank basename: %+v", file)
			}
		}
	}
}

func TestSchemaStoreCatalogIgnoreSuppressesInferredMatch(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	writeFile(t, catalogPath, `{
  "schemas": [
    {"name": "Known", "fileMatch": ["ignored.json", "explicit.json"], "url": "./known.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(dir, "known.schema.json"), `{"type":"object","required":["name"]}`)
	writeFile(t, filepath.Join(dir, "explicit.schema.json"), `{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	writeFile(t, filepath.Join(dir, "ignored.json"), `{}`)
	writeFile(t, filepath.Join(dir, "explicit.json"), `{"ok": true}`)

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: catalogPath}}
	cfg.Schemas.Catalogs.Ignore = []CatalogIgnoreRule{{File: "*.json", Reason: "application payloads"}}
	cfg.Schemas.Associations = []SchemaAssociation{{File: "explicit.json", Schema: filepath.Join(dir, "explicit.schema.json")}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Validated != 1 || result.Summary.Skipped != 4 || result.Summary.Issues.Total != 0 {
		t.Fatalf("summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	for _, file := range result.Files {
		switch file.RelativePath {
		case "ignored.json":
			if file.SchemaSource != "" || file.SchemaMatch == nil || file.SchemaMatch.Action != SchemaMatchActionIgnored || file.SchemaMatch.IgnorePattern != "*.json" || !strings.Contains(file.SkipDetail, "application payloads") {
				t.Fatalf("ignored catalog file = %+v", file)
			}
		case "explicit.json":
			if file.SchemaSource != "config-association" || file.SchemaMatch != nil {
				t.Fatalf("explicit association should win before catalog ignore: %+v", file)
			}
		}
	}
}

func TestSchemaStoreMatchHelperEdges(t *testing.T) {
	if match, ok := (*schemaStoreCatalog)(nil).match("anything.json", CatalogConfig{}); ok || match.action != "" {
		t.Fatalf("nil catalog match = %+v, %v", match, ok)
	}
	(*schemaStoreCatalog)(nil).buildIndex()
	emptyCatalog := &schemaStoreCatalog{}
	emptyCatalog.buildIndex()
	emptyCatalog.buildIndex()
	indexCatalog := &schemaStoreCatalog{Schemas: []schemaStoreEntry{{Name: "edges", FileMatch: []string{"", "nested/file.json", "*.json", "file.json"}}}}
	indexCatalog.buildIndex()
	if len(indexCatalog.exactPaths) != 1 || len(indexCatalog.exactBasenames) != 1 || len(indexCatalog.basenameGlobs) != 1 {
		t.Fatalf("edge catalog index = %+v", indexCatalog)
	}
	if match, ok := emptyCatalog.match("anything.json", CatalogConfig{}); ok || match.action != "" {
		t.Fatalf("empty catalog match = %+v, %v", match, ok)
	}
	if ignore, ok := catalogIgnoreMatch(nil, "file.json"); ok || ignore.File != "" {
		t.Fatalf("unexpected empty catalog ignore match = %+v, %v", ignore, ok)
	}
	if ignore, ok := catalogIgnoreMatch([]CatalogIgnoreRule{{File: "*.yaml"}, {}}, "file.json"); ok || ignore.File != "" {
		t.Fatalf("unexpected blank catalog ignore match = %+v, %v", ignore, ok)
	}
	if ignore, ok := catalogIgnoreMatch([]CatalogIgnoreRule{{}, {File: "*.json", Reason: "last"}}, "file.json"); !ok || ignore.Reason != "last" {
		t.Fatalf("catalog ignore match = %+v, %v", ignore, ok)
	}
	if ignore, ok := catalogIgnoreMatch([]CatalogIgnoreRule{{File: "*.yaml"}}, "file.json"); ok || ignore.File != "" {
		t.Fatalf("unexpected catalog ignore match = %+v, %v", ignore, ok)
	}
	if suggestedSchemaAssociation("", "schema.json") != "" || suggestedSchemaAssociation("file.json", "") != "" {
		t.Fatalf("empty association suggestions should be omitted")
	}
	if suggestedCatalogIgnore("") != "" {
		t.Fatalf("empty catalog ignore suggestion should be omitted")
	}
	if got := catalogPatternReason("file.json", "file.json", schemaStoreMatch{pattern: "*.json"}); !strings.Contains(got, `path "file.json" matched`) {
		t.Fatalf("fallback catalog pattern reason = %q", got)
	}
	if lowConfidenceSchemaStoreGlob("path/*.json") {
		t.Fatalf("path-qualified glob should not be low confidence")
	}
	entries := map[string]schemaStoreEntry{"file.json": {Name: "first"}}
	addSchemaStoreExact(entries, "file.json", schemaStoreEntry{Name: "second"})
	if entries["file.json"].Name != "first" {
		t.Fatalf("duplicate exact entry should keep first: %+v", entries)
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
	if result.Summary.Validated != 1 || result.Summary.Issues.Total != 0 {
		t.Fatalf("default summary = %+v issues=%+v", result.Summary, result.Issues)
	}

	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: filepath.Join(dir, "catalog.json")}}
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint catalog: %v", err)
	}
	if result.Summary.Issues.Total != 0 {
		t.Fatalf("explicit schema should win over catalog match: %+v", result.Issues)
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
	invalidPolicy := Config{Schemas: SchemaConfig{Catalogs: CatalogConfig{Failure: "explode"}}}
	if _, _, err := schemaStoreCatalogError(invalidPolicy, CatalogSource{Name: "test"}, "boom"); err == nil || !strings.Contains(err.Error(), "unsupported catalog failure policy") {
		t.Fatalf("expected invalid failure policy error, got %v", err)
	}
	if catalog, warning, err := loadSchemaStoreCatalog(context.Background(), cache, DefaultConfig()); err != nil || warning != nil || catalog != nil {
		t.Fatalf("disabled catalog = %+v, %+v, %v", catalog, warning, err)
	}
	emptyURLConfig := DefaultConfig()
	emptyURLConfig.Schemas.Catalogs.Enabled = true
	disabled := false
	emptyURLConfig.Schemas.Fetch.Enabled = &disabled
	if catalog, warning, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(DefaultConfig()), emptyURLConfig); err != nil || catalog != nil || warning == nil || !strings.Contains(warning.Message, "requires remote schema fetching") {
		t.Fatalf("warn remote-disabled catalog = %+v, %+v, %v", catalog, warning, err)
	}
	emptyURLConfig.Schemas.Catalogs.Failure = CatalogFailureSkip
	if catalog, warning, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(DefaultConfig()), emptyURLConfig); err != nil || warning != nil || catalog != nil {
		t.Fatalf("skip remote-disabled catalog = %+v, %+v, %v", catalog, warning, err)
	}
	emptyURLConfig.Schemas.Catalogs.Failure = CatalogFailureError
	if _, _, err := loadSchemaStoreCatalog(context.Background(), NewSchemaCache(DefaultConfig()), emptyURLConfig); err == nil || !strings.Contains(err.Error(), defaultSchemaStoreCatalogURL) {
		t.Fatalf("expected error-mode default catalog URL error, got %v", err)
	}

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", URL: "https://example.invalid/catalog.json"}}
	cfg.Schemas.Fetch.Enabled = &disabled
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("warn remote disabled should continue, got %v", err)
	}
	if result.Summary.Warnings != 1 || len(result.Warnings) != 1 {
		t.Fatalf("expected catalog warning, got %+v warnings=%+v", result.Summary, result.Warnings)
	}
	cfg.Schemas.Catalogs.Failure = CatalogFailureSkip
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("skip remote disabled should continue, got %v", err)
	}
	if result.Summary.Warnings != 0 || len(result.Warnings) != 0 {
		t.Fatalf("expected no catalog warnings, got %+v warnings=%+v", result.Summary, result.Warnings)
	}
	cfg.Schemas.Catalogs.Failure = CatalogFailureError
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "requires remote schema fetching") {
		t.Fatalf("expected remote disabled error, got %v", err)
	}
	if resolved, err := resolveCatalogURI("https://example.com/catalog.json"); err != nil || resolved != "https://example.com/catalog.json" {
		t.Fatalf("resolve remote catalog = %q, %v", resolved, err)
	}

	writeFile(t, filepath.Join(dir, "catalog.json"), `[]`)
	cfg.Schemas.Fetch.Enabled = nil
	cfg.Schemas.Catalogs.Failure = CatalogFailureWarn
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: filepath.Join(dir, "catalog.json")}}
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("warn invalid catalog should continue, got %v", err)
	}
	if result.Summary.Warnings != 1 || !strings.Contains(result.Warnings[0].Message, "is not an object") {
		t.Fatalf("expected invalid catalog warning, got %+v warnings=%+v", result.Summary, result.Warnings)
	}
	cfg.Schemas.Catalogs.Failure = CatalogFailureError
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "is not an object") {
		t.Fatalf("expected invalid catalog error, got %v", err)
	}

	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: filepath.Join(dir, "missing-catalog.json")}}
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "load catalog") {
		t.Fatalf("expected missing catalog error, got %v", err)
	}
	if _, err := resolveCatalogURI("%"); err == nil || !strings.Contains(err.Error(), "parse catalog URL") {
		t.Fatalf("expected bad catalog URL error, got %v", err)
	}
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", URL: "%"}}
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "parse catalog URL") {
		t.Fatalf("expected bad catalog URL lint error, got %v", err)
	}
}

func TestSchemaStoreMatchedSchemaFailurePolicy(t *testing.T) {
	dir := t.TempDir()
	catalogDir := filepath.Join(dir, "node_modules", "catalog-fixtures")
	writeFile(t, filepath.Join(catalogDir, "catalog.json"), `{
  "schemas": [
    {"name": "Broken", "fileMatch": ["broken.json"], "url": "./broken.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(catalogDir, "broken.schema.json"), `{"type":"not-a-json-schema-type"}`)
	writeFile(t, filepath.Join(dir, "broken.json"), `{}`)

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: filepath.Join(catalogDir, "catalog.json")}}

	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("warn catalog schema failure should continue, got %v", err)
	}
	if result.Summary.Issues.Total != 0 || result.Summary.Warnings != 1 || result.Summary.Validated != 0 || result.Summary.Skipped != 1 {
		t.Fatalf("warn catalog schema failure summary = %+v issues=%+v warnings=%+v", result.Summary, result.Issues, result.Warnings)
	}
	if result.Warnings[0].Kind != "schemaCatalogSchemaUnavailable" ||
		!strings.Contains(result.Warnings[0].Message, "Catalog schema could not be used for broken.json") ||
		!strings.Contains(result.Warnings[0].Message, "this is not a finding in the file") ||
		!strings.Contains(result.Warnings[0].Hint, "Technical details: catalog schema compile failed") {
		t.Fatalf("catalog schema warning = %+v", result.Warnings[0])
	}

	cfg.Schemas.Catalogs.Failure = CatalogFailureSkip
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("skip catalog schema failure should continue, got %v", err)
	}
	if result.Summary.Issues.Total != 0 || result.Summary.Warnings != 0 || result.Summary.Validated != 0 || result.Summary.Skipped != 1 {
		t.Fatalf("skip catalog schema failure summary = %+v issues=%+v warnings=%+v", result.Summary, result.Issues, result.Warnings)
	}

	cfg.Schemas.Catalogs.Failure = CatalogFailureError
	if _, err := Lint(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "catalog schema compile failed") {
		t.Fatalf("expected error catalog schema failure, got %v", err)
	}
}

func TestSchemaStoreCatalogSchemaAllowsVscodeReferences(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	writeFile(t, catalogPath, `{
  "schemas": [
    {"name": "Dev Container", "fileMatch": ["devcontainer.json"], "url": "./devcontainer.schema.json"}
  ]
}`)
	writeFile(t, filepath.Join(dir, "devcontainer.schema.json"), `{
  "type": "object",
  "properties": {
    "settings": {"$ref": "vscode://schemas/settings/machine"}
  }
}`)
	writeFile(t, filepath.Join(dir, "devcontainer.json"), `{"settings":{"editor.tabSize":2}}`)

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: catalogPath}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Warnings != 0 || result.Summary.Issues.Total != 0 || result.Summary.Validated != 1 {
		t.Fatalf("summary = %+v warnings=%+v issues=%+v", result.Summary, result.Warnings, result.Issues)
	}
}

func TestSchemaStoreCatalogSourceEdges(t *testing.T) {
	enabled := true
	disabled := false
	defaultSources := enabledSchemaStoreCatalogSources(SchemaConfig{})
	if len(defaultSources) != 1 || defaultSources[0].URL != defaultSchemaStoreCatalogURL {
		t.Fatalf("default sources = %+v", defaultSources)
	}
	sources := enabledSchemaStoreCatalogSources(SchemaConfig{
		Catalogs: CatalogConfig{
			Sources: []CatalogSource{
				{Name: "off", Format: "schemastore", URL: "./off.json", Enabled: &disabled},
				{URL: "./defaulted.json", Enabled: &enabled},
				{Name: "custom", Format: "custom", URL: "./custom.json"},
			},
		},
	})
	if len(sources) != 1 {
		t.Fatalf("enabled sources = %+v", sources)
	}
	if sources[0].Name != "schemastore" || sources[0].Format != "schemastore" || sources[0].URL != "./defaulted.json" {
		t.Fatalf("defaulted source = %+v", sources[0])
	}

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	if catalog, warning, err := loadSchemaStoreCatalogSource(context.Background(), NewSchemaCache(cfg), cfg, CatalogSource{Name: "empty"}); err != nil || warning != nil || catalog != nil {
		t.Fatalf("empty catalog source = %+v, %+v, %v", catalog, warning, err)
	}
	_, warning, err := schemaStoreCatalogError(cfg, CatalogSource{Path: "./catalog.json"}, "boom")
	if err != nil || warning == nil || warning.Source != "catalog" {
		t.Fatalf("catalog warning source fallback = %+v, %v", warning, err)
	}
}

func TestCuratedDefaultSchemaStoreAssociations(t *testing.T) {
	catalog := &schemaStoreCatalog{
		Schemas: []schemaStoreEntry{
			{
				Name:      "upstream rustfmt",
				Source:    "schemastore",
				FileMatch: []string{"rustfmt.toml"},
				URL:       "https://example.com/upstream-rustfmt.json",
			},
		},
	}
	addCuratedDefaultSchemaStoreAssociations(nil, CatalogSource{}, defaultSchemaStoreCatalogURL)
	addCuratedDefaultSchemaStoreAssociations(catalog, CatalogSource{}, "./catalog.json")
	if len(catalog.Schemas) != 1 {
		t.Fatalf("custom catalog should not get curated associations: %+v", catalog.Schemas)
	}

	addCuratedDefaultSchemaStoreAssociations(catalog, CatalogSource{}, defaultSchemaStoreCatalogURL)
	if len(catalog.Schemas) != 3 {
		t.Fatalf("curated associations = %+v", catalog.Schemas)
	}

	match, ok := catalog.match("tools/rustfmt.toml", CatalogConfig{})
	if !ok || match.entry.URL != "https://example.com/upstream-rustfmt.json" {
		t.Fatalf("upstream rustfmt should win when present: %+v ok=%v", match, ok)
	}

	match, ok = catalog.match("tools/.rustfmt.toml", CatalogConfig{})
	if !ok || match.entry.URL != "https://www.schemastore.org/rustfmt.json" || match.entry.Source != "schemastore" {
		t.Fatalf("curated rustfmt dotfile match = %+v ok=%v", match, ok)
	}
	schemaMatch := match.schemaMatch("tools/.rustfmt.toml")
	if schemaMatch.Action != SchemaMatchActionMatched || !strings.Contains(schemaMatch.SuggestedAssociation, `file = "tools/.rustfmt.toml"`) {
		t.Fatalf("curated rustfmt schemaMatch = %+v", schemaMatch)
	}

	match, ok = catalog.match(".release-plz.toml", CatalogConfig{})
	if !ok || match.entry.Name != "release-plz.toml" || !strings.Contains(match.entry.URL, "release-plz/main/.schema/latest.json") {
		t.Fatalf("curated release-plz match = %+v ok=%v", match, ok)
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
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: filepath.Join(dir, "catalog.json")}}
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
