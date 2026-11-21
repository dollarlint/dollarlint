package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintServesMappedSchemaURLsFromLocalFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schemas", "item", "v0.json"), `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://schemas.example.test/item/v0.json",
  "type": "object",
  "required": ["part"],
  "properties": {"part": {"$ref": "https://schemas.example.test/part/v0.json"}}
}`)
	writeFile(t, filepath.Join(dir, "schemas", "part", "v0.json"), `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://schemas.example.test/part/v0.json",
  "type": "string"
}`)
	writeFile(t, filepath.Join(dir, "exact.json"), `{"type": "object", "required": ["name"]}`)
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
[[schemas.associations]]
file = "data/*.json"
schema = "./schemas/item/v0.json"

[[schemas.associations]]
file = "named/*.json"
schema = "https://schemas.example.test/item/v0.json"

[[schemas.associations]]
file = "exact/*.json"
schema = "https://exact.example.test/schema.json"

[[schemas.mappings]]
url = "https://schemas.example.test/"
path = "schemas"

[[schemas.mappings]]
url = "https://exact.example.test/schema.json"
path = "./exact.json"

[schemas.fetch]
enabled = false
`)
	writeFile(t, filepath.Join(dir, "data", "valid.json"), `{"part": "a"}`)
	writeFile(t, filepath.Join(dir, "data", "invalid.json"), `{"part": 1}`)
	writeFile(t, filepath.Join(dir, "named", "invalid.json"), `{}`)
	writeFile(t, filepath.Join(dir, "exact", "invalid.json"), `{}`)
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Discovery.Include = []string{"data/*.json", "named/*.json", "exact/*.json"}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Validated != 4 || len(result.Warnings) != 0 {
		t.Fatalf("summary = %+v warnings = %+v", result.Summary, result.Warnings)
	}
	got := map[string]string{}
	for _, issue := range result.Issues {
		got[filepath.ToSlash(issue.RelativePath)] = issue.Keyword
	}
	want := map[string]string{"data/invalid.json": "type", "named/invalid.json": "required", "exact/invalid.json": "required"}
	if len(got) != len(want) {
		t.Fatalf("issues = %+v", result.Issues)
	}
	for file, keyword := range want {
		if got[file] != keyword {
			t.Fatalf("issue for %s = %q, want %q (issues %+v)", file, got[file], keyword, result.Issues)
		}
	}
}

func TestMappedSchemaPath(t *testing.T) {
	mappings := []SchemaMapping{
		{URL: "https://example.test/", Path: "/schemas"},
		{URL: "https://example.test/deep/", Path: "/deep"},
		{URL: "https://example.test/one.json", Path: "/one.json"},
	}
	cases := map[string]string{
		"https://example.test/a/v0.json":      filepath.Join("/schemas", "a", "v0.json"),
		"https://example.test/deep/b.json":    filepath.Join("/deep", "b.json"),
		"https://example.test/one.json":       "/one.json",
		"https://example.test/one.json.other": filepath.Join("/schemas", "one.json.other"),
	}
	for raw, want := range cases {
		got, ok, err := mappedSchemaPath(raw, mappings)
		if err != nil || !ok || got != want {
			t.Fatalf("mappedSchemaPath(%q) = %q, %v, %v; want %q", raw, got, ok, err, want)
		}
	}
	for _, raw := range []string{
		"https://example.test/../x.json",
		"https://example.test/a/./b.json",
		"https://example.test/",
		"https://example.test/a//b.json",
		"https://example.test/%2e%2e%5coutside.json",
		"https://example.test/a%5c..%5c..%5coutside.json",
		"https://example.test/C:%5coutside.json",
	} {
		if _, ok, err := mappedSchemaPath(raw, mappings); !ok || err == nil {
			t.Fatalf("mappedSchemaPath(%q) should refuse to leave the mapped directory, got ok=%v err=%v", raw, ok, err)
		}
	}
	if got, ok, err := mappedSchemaPath("https://upper.test/a.json", []SchemaMapping{{URL: "HTTPS://upper.test/", Path: "/upper"}}); err != nil || !ok || got != filepath.Join("/upper", "a.json") {
		t.Fatalf("an uppercase-scheme mapping should match: %q, %v, %v", got, ok, err)
	}
	if _, ok, _ := mappedSchemaPath("https://other.test/a.json", mappings); ok {
		t.Fatalf("unmapped URL matched")
	}
}

func TestLoadConfigRejectsInvalidSchemaMappings(t *testing.T) {
	for _, body := range []string{
		"[[schemas.mappings]]\nurl = \"./relative/\"\npath = \"schemas\"\n",
		"[[schemas.mappings]]\nurl = \"https://example.test/a.json#x\"\npath = \"a.json\"\n",
		"[[schemas.mappings]]\nurl = \"https://example.test/\"\npath = \"\"\n",
	} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".dollarlint.toml"), body)
		if _, _, err := LoadConfig(dir, ""); err == nil || !strings.Contains(err.Error(), "schemas.mappings") {
			t.Fatalf("expected schemas.mappings error for %q, got %v", body, err)
		}
	}
}

func TestLoadConfigExtendsAppendsSchemaMappings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base", ".dollarlint.toml"), "[[schemas.mappings]]\nurl = \"https://a.test/\"\npath = \"a\"\n")
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), "extends = \"base/.dollarlint.toml\"\n[[schemas.mappings]]\nurl = \"https://b.test/\"\npath = \"b\"\n")
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Schemas.Mappings) != 2 || filepath.Base(cfg.Schemas.Mappings[0].Path) != "a" || filepath.Dir(cfg.Schemas.Mappings[0].Path) != filepath.Join(dir, "base") || filepath.Base(cfg.Schemas.Mappings[1].Path) != "b" {
		t.Fatalf("mappings = %+v", cfg.Schemas.Mappings)
	}
}

func TestLintReportsMappedRootSchemaCompileFailureAsIssue(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schemas", "broken.json"), `{"type": 42}`)
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
[[schemas.associations]]
file = "data/*.json"
schema = "https://schemas.example.test/broken.json"

[[schemas.mappings]]
url = "https://schemas.example.test/"
path = "schemas"
`)
	writeFile(t, filepath.Join(dir, "data", "file.json"), `{}`)
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Discovery.Include = []string{"data/*.json"}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(result.Warnings) != 0 || result.Summary.Skipped != 0 || result.Summary.Issues.Schema != 1 {
		t.Fatalf("a mapped schema that fails to compile should be a schema issue, not a warning or skip: summary %+v warnings %+v issues %+v", result.Summary, result.Warnings, result.Issues)
	}
}

func TestLoadConfigChildSchemaMappingWinsOverParentForTheSameURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base", ".dollarlint.toml"), "[[schemas.mappings]]\nurl = \"https://a.test/\"\npath = \"parent\"\n")
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), "extends = \"base/.dollarlint.toml\"\n[[schemas.mappings]]\nurl = \"https://a.test/\"\npath = \"child\"\n")
	cfg, _, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	got, ok, err := mappedSchemaPath("https://a.test/x.json", cfg.Schemas.Mappings)
	if err != nil || !ok || got != filepath.Join(dir, "child", "x.json") {
		t.Fatalf("mappedSchemaPath = %q, %v, %v; want the child's mapping", got, ok, err)
	}
}

func TestLintMappedRelativeRefsAndFragments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schemas", "root.json"), `{"$id":"https://schemas.test/root.json","$ref":"nested/part.json#/$defs/value"}`)
	writeFile(t, filepath.Join(dir, "schemas", "nested", "part.json"), `{"$defs":{"value":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, "data.json"), `42`)
	cfg := DefaultConfig()
	disabled := false
	cfg.Schemas.Fetch.Enabled = &disabled
	cfg.Discovery.Include = []string{"data.json"}
	cfg.Schemas.Associations = []SchemaAssociation{{File: "data.json", Schema: "https://schemas.test/root.json"}}
	cfg.Schemas.Mappings = []SchemaMapping{{URL: "https://schemas.test/", Path: filepath.Join(dir, "schemas")}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil || result.Summary.Issues.Validation != 1 || len(result.Warnings) != 0 {
		t.Fatalf("err=%v result=%+v", err, result)
	}
}

func TestMappedSchemaPathHostnameCase(t *testing.T) {
	for _, pair := range [][2]string{
		{"HTTPS://SCHEMAS.example.test/", "https://schemas.example.test/Item.json"},
		{"https://schemas.example.test/", "https://SCHEMAS.example.test/Item.json"},
		{"https://SCHEMAS.example.test/Item.json", "https://schemas.example.test/Item.json"},
		{"https://schemas.example.test/Item.json", "https://SCHEMAS.example.test/Item.json"},
	} {
		local := t.TempDir()
		got, ok, err := mappedSchemaPath(pair[1], []SchemaMapping{{URL: pair[0], Path: local}})
		want := local
		if strings.HasSuffix(pair[0], "/") {
			want = filepath.Join(local, "Item.json")
		}
		if err != nil || !ok || got != want {
			t.Errorf("mapping %s for %s: got=%q matched=%v err=%v; want %q", pair[0], pair[1], got, ok, err, want)
		}
	}
	for _, pair := range [][2]string{
		{"https://schemas.example.test/Item.json", "https://SCHEMAS.example.test/item.json"},
		{"https://schemas.example.test/Item/", "https://SCHEMAS.example.test/item/part.json"},
	} {
		if _, ok, _ := mappedSchemaPath(pair[1], []SchemaMapping{{URL: pair[0], Path: t.TempDir()}}); ok {
			t.Errorf("mapping %s must preserve path case", pair[0])
		}
	}
	mappings := []SchemaMapping{{URL: "https://SCHEMAS.example.test/", Path: "parent"}, {URL: "https://schemas.example.test/", Path: "child"}}
	if got, ok, err := mappedSchemaPath("https://Schemas.example.test/Item.json", mappings); err != nil || !ok || got != filepath.Join("child", "Item.json") {
		t.Fatalf("equal normalized mappings must prefer the last: %q %v %v", got, ok, err)
	}
}

func TestLintReportsMappedDependencyFailuresAsIssues(t *testing.T) {
	for name, schema := range map[string]string{
		"missing":         "",
		"invalid JSON":    `{`,
		"invalid schema":  `{"type":42}`,
		"invalid regex":   `{"pattern":"["}`,
		"duplicate ID":    `{"$defs":{"a":{"$id":"same.json"},"b":{"$id":"same.json"}}}`,
		"missing pointer": `{"$ref":"#/$defs/missing"}`,
		"missing anchor":  `{"$ref":"#missing"}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if schema != "" {
				writeFile(t, filepath.Join(dir, "broken.json"), schema)
			}
			writeFile(t, filepath.Join(dir, "data.json"), `{}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"$ref":"https://SCHEMAS.test/broken.json"}`)
			}))
			defer server.Close()
			cfg := DefaultConfig()
			cfg.Discovery.Include = []string{"data.json"}
			cfg.Schemas.RequireCoverage = true
			disabled := false
			cfg.Schemas.Fetch.Cache = &disabled
			cfg.Schemas.Associations = []SchemaAssociation{{File: "data.json", Schema: server.URL + "/root.json"}}
			cfg.Schemas.Mappings = []SchemaMapping{{URL: "https://schemas.test/broken.json", Path: filepath.Join(dir, "broken.json")}}
			result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			if !result.HasIssues() || result.Summary.Issues.Schema != 1 || result.Summary.Skipped != 0 || len(result.Warnings) != 0 {
				t.Fatalf("mapped dependency must produce a schema issue: summary=%+v warnings=%+v", result.Summary, result.Warnings)
			}
			for _, mode := range []string{CatalogFailureWarn, CatalogFailureSkip, CatalogFailureError} {
				cfg.Schemas.Catalogs.Failure = mode
				doc := &Document{Path: filepath.Join(dir, "data.json"), RelativePath: "data.json", Schema: server.URL + "/root.json", SchemaSource: "catalog:test", Data: map[string]any{}}
				for _, uri := range []string{server.URL + "/root.json", "https://schemas.test/broken.json"} {
					doc.Schema = uri
					validation := validateDocument(context.Background(), NewSchemaCache(cfg), cfg, doc)
					if len(validation.issues) != 0 {
						t.Fatalf("catalog failures must follow catalog policy: %+v", validation)
					}
					switch mode {
					case CatalogFailureWarn:
						if validation.err != nil || !validation.skipped || validation.schemaWarning == nil {
							t.Fatalf("expected catalog warning: %+v", validation)
						}
					case CatalogFailureSkip:
						if validation.err != nil || !validation.skipped || validation.schemaWarning != nil {
							t.Fatalf("expected catalog skip: %+v", validation)
						}
					case CatalogFailureError:
						if validation.err == nil {
							t.Fatalf("expected catalog error: %+v", validation)
						}
					}
				}
			}
		})
	}
}

func TestMappedDependencyDoesNotChangeUnmappedRemoteFailurePolicy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "valid.json"), `{"type":"object"}`)
	writeFile(t, filepath.Join(dir, "data.json"), `{}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/root.json" {
			fmt.Fprint(w, `{"allOf":[{"$ref":"https://schemas.test/valid.json"},{"$ref":"broken.json"}]}`)
		} else {
			fmt.Fprint(w, `{"type":42}`)
		}
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.Discovery.Include = []string{"data.json"}
	disabled := false
	cfg.Schemas.Fetch.Cache = &disabled
	cfg.Schemas.Associations = []SchemaAssociation{{File: "data.json", Schema: server.URL + "/root.json"}}
	cfg.Schemas.Mappings = []SchemaMapping{{URL: "https://schemas.test/valid.json", Path: filepath.Join(dir, "valid.json")}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if result.HasIssues() || result.Summary.Skipped != 1 || len(result.Warnings) != 1 {
		t.Fatalf("unmapped remote failure must retain availability policy: summary=%+v warnings=%+v", result.Summary, result.Warnings)
	}
}
