package engine

import (
	"context"
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
