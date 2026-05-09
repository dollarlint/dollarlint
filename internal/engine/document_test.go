package engine

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
	jsoncPath := filepath.Join(dir, "data.jsonc")
	writeFile(t, jsoncPath, `{
  // comments and trailing commas are allowed
  "$schema": "./jsonc-schema.json",
  "name": "ok",
}`)
	doc, err = ParseDocument(DiscoveredFile{Path: jsoncPath, RelativePath: "data.jsonc"})
	if err != nil {
		t.Fatalf("ParseDocument jsonc: %v", err)
	}
	if doc.Format != DocumentFormatJSONC || doc.Schema != "./jsonc-schema.json" || doc.SchemaSource != "$schema" {
		t.Fatalf("jsonc doc = %+v", doc)
	}
	json5Path := filepath.Join(dir, "data.json5")
	writeFile(t, json5Path, `{
  // JSON5 object keys and strings
  $schema: './json5-schema.json',
  name: 'ok',
  count: 0x10,
}`)
	doc, err = ParseDocument(DiscoveredFile{Path: json5Path, RelativePath: "data.json5"})
	if err != nil {
		t.Fatalf("ParseDocument json5: %v", err)
	}
	if doc.Format != DocumentFormatJSON5 || doc.Schema != "./json5-schema.json" || doc.SchemaSource != "$schema" {
		t.Fatalf("json5 doc = %+v", doc)
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

func TestParseDocumentJSONParsingModes(t *testing.T) {
	dir := t.TempDir()
	tsconfig := filepath.Join(dir, "tsconfig.app.json")
	writeFile(t, tsconfig, `{
  // TypeScript config files commonly use JSON with comments.
  "$schema": "./tsconfig.schema.json",
  "compilerOptions": {},
}`)
	doc, err := ParseDocument(DiscoveredFile{Path: tsconfig, RelativePath: "tsconfig.app.json"})
	if err != nil {
		t.Fatalf("ParseDocument auto tsconfig: %v", err)
	}
	if doc.Format != DocumentFormatJSONC || doc.Schema != "./tsconfig.schema.json" {
		t.Fatalf("auto tsconfig doc = %+v", doc)
	}
	strict := DefaultConfig().Parsing
	strict.JSON.Mode = JSONParsingStrict
	if _, err := parseDocument(DiscoveredFile{Path: tsconfig, RelativePath: "tsconfig.app.json"}, strict, false); err == nil {
		t.Fatalf("expected strict mode to reject commented .json")
	}
	ordinary := filepath.Join(dir, "settings.json")
	writeFile(t, ordinary, `{
  // Ordinary .json stays strict in auto mode.
  "$schema": "./settings.schema.json"
}`)
	if _, err := ParseDocument(DiscoveredFile{Path: ordinary, RelativePath: "settings.json"}); err == nil {
		t.Fatalf("expected auto mode to keep ordinary .json strict")
	}
	jsonc := DefaultConfig().Parsing
	jsonc.JSON.Mode = JSONParsingJSONC
	doc, err = parseDocument(DiscoveredFile{Path: ordinary, RelativePath: "settings.json"}, jsonc, false)
	if err != nil {
		t.Fatalf("ParseDocument jsonc mode ordinary json: %v", err)
	}
	if doc.Format != DocumentFormatJSONC {
		t.Fatalf("jsonc mode doc = %+v", doc)
	}
}

func TestParseDocumentCanAttachSourceMapFromInitialRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	writeFile(t, path, `{
  "$schema": "./schema.json",
  "name": 42
}`)
	doc, err := parseDocument(DiscoveredFile{Path: path, RelativePath: "bad.json"}, DefaultConfig().Parsing, true)
	if err != nil {
		t.Fatalf("parseDocument with source map: %v", err)
	}
	if doc.SourceMap == nil {
		t.Fatalf("expected source map to be attached")
	}
	if pos, ok := doc.SourceMap.Position("/name"); !ok || pos.Line != 3 || pos.Column == 0 {
		t.Fatalf("source map position = %+v ok=%v", pos, ok)
	}

	linesPath := filepath.Join(dir, "events.jsonl")
	writeFile(t, linesPath, "{\"name\":\"first\"}\n{\"name\":\"second\"}\n")
	lines, err := parseDocument(DiscoveredFile{Path: linesPath, RelativePath: "events.jsonl"}, DefaultConfig().Parsing, true)
	if err != nil {
		t.Fatalf("parseDocument jsonl with source map: %v", err)
	}
	if lines.SourceMap == nil || len(lines.LineDocuments) != 2 || lines.LineDocuments[0].SourceMap == nil {
		t.Fatalf("expected jsonl source maps to be attached: %+v", lines)
	}
}

func TestParseDocumentErrorsAndHelpers(t *testing.T) {
	dir := t.TempDir()
	badJSON := filepath.Join(dir, "bad.json")
	writeFile(t, badJSON, `{`)
	if _, err := ParseDocument(DiscoveredFile{Path: badJSON, RelativePath: "bad.json"}); err == nil {
		t.Fatalf("expected parse error")
	}
	badJSON5 := filepath.Join(dir, "bad.json5")
	writeFile(t, badJSON5, `{bad: NaN}`)
	if _, err := ParseDocument(DiscoveredFile{Path: badJSON5, RelativePath: "bad.json5"}); err == nil {
		t.Fatalf("expected non-json json5 value error")
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
	if directive := tomlSchemaDirective([]byte("#:schemaFOO ./bad.json\n#:schema ./good.json\n")); directive != "./good.json" {
		t.Fatalf("toml directive after malformed directive = %q", directive)
	}
	if directive := tomlSchemaDirective([]byte("#:schema\t./tabbed.json\n")); directive != "./tabbed.json" {
		t.Fatalf("tabbed toml directive = %q", directive)
	}
	if len(firstLines([]byte("a\nb\nc"), 2)) != 2 {
		t.Fatalf("firstLines did not limit")
	}
	if lines := firstLines([]byte("a\r\nb"), 2); len(lines) != 2 || lines[0] != "a" {
		t.Fatalf("firstLines CRLF = %#v", lines)
	}
	if lines := firstLines([]byte("a"), 0); lines != nil {
		t.Fatalf("zero limit firstLines = %#v", lines)
	}
	if refs := ((*Document)(nil)).azureResourceRefs(); refs != nil {
		t.Fatalf("nil document refs = %#v", refs)
	}
	azureDoc := &Document{Data: map[string]any{"resources": []any{map[string]any{"type": "Microsoft.Good/widgets", "apiVersion": "2023-01-01"}}}}
	if refs := azureDoc.azureResourceRefs(); len(refs) != 1 {
		t.Fatalf("azure refs = %#v", refs)
	}
	azureDoc.Data = nil
	if refs := azureDoc.azureResourceRefs(); len(refs) != 1 {
		t.Fatalf("cached azure refs = %#v", refs)
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

func TestParseDocumentJSONLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeFile(t, path, "\n{\"name\":\"first\"}\r\n  {\"name\":\"second\",\"count\":2}\n{\"name\":}\n")
	doc, err := ParseDocument(DiscoveredFile{Path: path, RelativePath: "events.jsonl"})
	if err != nil {
		t.Fatalf("ParseDocument jsonl: %v", err)
	}
	if doc.Format != DocumentFormatJSONLines || !doc.isLineDelimited() {
		t.Fatalf("jsonl format = %+v", doc)
	}
	if len(doc.LineDocuments) != 2 || len(doc.ParseErrors) != 1 {
		t.Fatalf("jsonl records/errors = %d/%d: %+v", len(doc.LineDocuments), len(doc.ParseErrors), doc.ParseErrors)
	}
	if doc.LineDocuments[0].Line != 1 || doc.LineDocuments[1].Line != 2 {
		t.Fatalf("jsonl line numbers = %+v", doc.LineDocuments)
	}
	if doc.ParseErrors[0].Line != 3 || doc.ParseErrors[0].Column == 0 || !strings.Contains(doc.ParseErrors[0].Message, "parse line 3") {
		t.Fatalf("jsonl parse error = %+v", doc.ParseErrors[0])
	}
	values, ok := doc.Data.([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("jsonl data = %#v", doc.Data)
	}
}
