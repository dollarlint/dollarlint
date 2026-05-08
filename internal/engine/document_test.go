package dollarlint

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestParseDocumentSchemaConventions(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "data.json")
	writeFile(t, jsonPath, `{"$schema":"./schema.json","name":"ok"}`)
	doc, err := ParseDocument(DiscoveredFile{Path: jsonPath, RelativePath: "data.json"})
	if err != nil {
		t.Fatalf("ParseDocument json: %v", err)
	}
	if doc.Format != DocumentFormatJSON || doc.Schema != "./schema.json" || doc.SchemaSource != "$schema" {
		t.Fatalf("json doc = %+v", doc)
	}
	yamlPath := filepath.Join(dir, "data.yaml")
	writeFile(t, yamlPath, `
# yaml-language-server: $schema=./yaml-schema.json
name: ok
`)
	doc, err = ParseDocument(DiscoveredFile{Path: yamlPath, RelativePath: "data.yaml"})
	if err != nil {
		t.Fatalf("ParseDocument yaml: %v", err)
	}
	if doc.Schema != "./yaml-schema.json" || doc.SchemaSource != "yaml-language-server" {
		t.Fatalf("yaml doc = %+v", doc)
	}
	tomlDirectivePath := filepath.Join(dir, "taplo.toml")
	writeFile(t, tomlDirectivePath, `
#:schema ./taplo-schema.json
name = "ok"
`)
	doc, err = ParseDocument(DiscoveredFile{Path: tomlDirectivePath, RelativePath: "taplo.toml"})
	if err != nil {
		t.Fatalf("ParseDocument toml directive: %v", err)
	}
	if doc.Schema != "./taplo-schema.json" || doc.SchemaSource != "taplo-directive" {
		t.Fatalf("toml directive doc = %+v", doc)
	}
	tomlRootPath := filepath.Join(dir, "root.toml")
	writeFile(t, tomlRootPath, `
"$schema" = "./root-schema.json"
name = "ok"
`)
	doc, err = ParseDocument(DiscoveredFile{Path: tomlRootPath, RelativePath: "root.toml"})
	if err != nil {
		t.Fatalf("ParseDocument toml root: %v", err)
	}
	if doc.Schema != "./root-schema.json" || doc.SchemaSource != "$schema" {
		t.Fatalf("toml root doc = %+v", doc)
	}
}

func TestParseDocumentErrorsAndHelpers(t *testing.T) {
	dir := t.TempDir()
	badJSON := filepath.Join(dir, "bad.json")
	writeFile(t, badJSON, `{`)
	if _, err := ParseDocument(DiscoveredFile{Path: badJSON, RelativePath: "bad.json"}); err == nil {
		t.Fatalf("expected parse error")
	}
	unsupported := filepath.Join(dir, "bad.txt")
	writeFile(t, unsupported, "")
	if _, err := ParseDocument(DiscoveredFile{Path: unsupported, RelativePath: "bad.txt"}); err == nil {
		t.Fatalf("expected unsupported format error")
	}
	if _, err := decodeDocument(nil, "wat"); err == nil {
		t.Fatalf("expected unsupported decode format")
	}
	if schema := rootSchema([]any{}); schema != "" {
		t.Fatalf("unexpected schema %q", schema)
	}
	if schema := rootSchema(map[string]any{"$schema": 123}); schema != "" {
		t.Fatalf("unexpected non-string schema %q", schema)
	}
	if directive := yamlSchemaDirective([]byte("name: ok\n# yaml-language-server: $schema=late\n")); directive != "late" {
		t.Fatalf("yaml directive = %q", directive)
	}
	if directive := tomlSchemaDirective([]byte("# just a comment\nname = \"ok\"\n#:schema late\n")); directive != "" {
		t.Fatalf("late toml directive = %q", directive)
	}
	if len(firstLines([]byte("a\nb\nc"), 2)) != 2 {
		t.Fatalf("firstLines did not limit")
	}
	normalized := normalizeYAML(map[any]any{"a": []any{map[any]any{1: "one"}}})
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("marshal normalized: %v", err)
	}
	if string(data) != `{"a":[{"1":"one"}]}` {
		t.Fatalf("normalized = %s", data)
	}
}
