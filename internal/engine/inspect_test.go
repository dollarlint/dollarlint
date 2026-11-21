package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInspectExplainsDiscoveredSchemaAssociations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.schema"), `{"type":"object"}`)
	writeFile(t, filepath.Join(dir, "catalog.catalog"), `{"schemas":[
  {"name":"Cataloged","fileMatch":["cataloged.json"],"url":"./schema.schema"},
  {"name":"Generic tasks","fileMatch":["tasks.json"],"url":"./schema.schema"}
]}`)
	writeFile(t, filepath.Join(dir, "inline.json"), `{"$schema":"./schema.schema","name":"ok"}`)
	writeFile(t, filepath.Join(dir, "directive.yaml"), `# yaml-language-server: $schema=./schema.schema
name: ok`)
	writeFile(t, filepath.Join(dir, "directive.toml"), `#:schema ./schema.schema
name = "ok"`)
	writeFile(t, filepath.Join(dir, "associated.yaml"), `name: ok`)
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `version = 1`)
	writeFile(t, filepath.Join(dir, "cataloged.json"), `{}`)
	writeFile(t, filepath.Join(dir, "tasks.json"), `{}`)
	writeFile(t, filepath.Join(dir, "plain.toml"), `name = "ok"`)
	writeFile(t, filepath.Join(dir, "bad.json"), `{`)
	writeFile(t, filepath.Join(dir, "bad-schema.json"), `{"$schema":"%"}`)

	cfg := DefaultConfig()
	cfg.Schemas.Associations = []SchemaAssociation{{File: "associated.yaml", Schema: "./schema.schema"}}
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "test", Format: "schemastore", Path: filepath.Join(dir, "catalog.catalog")}}

	result, err := Inspect(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.FormatVersion != InspectFormatVersion || result.Summary.Discovered != 10 || result.Summary.Associated != 6 || result.Summary.Unassociated != 4 || result.Summary.Errors != 2 {
		t.Fatalf("inspect summary = %+v", result.Summary)
	}

	inline := findInspectFile(result.Files, "inline.json")
	if inline.Schema != "schema.schema" || inline.SchemaSource != "$schema" || !strings.Contains(inline.Reason, "$schema property") {
		t.Fatalf("inline inspect file = %+v", inline)
	}
	associated := findInspectFile(result.Files, "associated.yaml")
	if associated.Schema != "schema.schema" || associated.SchemaSource != "config-association" || !strings.Contains(associated.Reason, `pattern "associated.yaml"`) {
		t.Fatalf("associated inspect file = %+v", associated)
	}
	directiveYAML := findInspectFile(result.Files, "directive.yaml")
	if directiveYAML.SchemaSource != "yaml-language-server" || !strings.Contains(directiveYAML.Reason, "yaml-language-server directive") {
		t.Fatalf("yaml directive inspect file = %+v", directiveYAML)
	}
	directiveTOML := findInspectFile(result.Files, "directive.toml")
	if directiveTOML.SchemaSource != "taplo-directive" || !strings.Contains(directiveTOML.Reason, "Taplo #:schema directive") {
		t.Fatalf("toml directive inspect file = %+v", directiveTOML)
	}
	builtin := findInspectFile(result.Files, ".dollarlint.toml")
	if builtin.Schema != builtinDollarlintConfigSchemaURI || builtin.SchemaSource != builtinDollarlintConfigSchemaSource || !strings.Contains(builtin.Reason, "built-in association") {
		t.Fatalf("builtin inspect file = %+v", builtin)
	}
	cataloged := findInspectFile(result.Files, "cataloged.json")
	if cataloged.SchemaMatch == nil || cataloged.SchemaSource != "catalog:test:Cataloged" || !strings.Contains(cataloged.Reason, "catalog fileMatch") {
		t.Fatalf("catalog inspect file = %+v", cataloged)
	}
	tasks := findInspectFile(result.Files, "tasks.json")
	if tasks.AssociationStatus != InspectAssociationStatusUnassociated || tasks.Schema != "" || tasks.SchemaMatch == nil || tasks.SchemaMatch.Action != SchemaMatchActionSkippedLowConfidence || tasks.SuggestedCatalogIgnore == "" {
		t.Fatalf("tasks inspect file = %+v", tasks)
	}
	plain := findInspectFile(result.Files, "plain.toml")
	if plain.AssociationStatus != InspectAssociationStatusUnassociated || !strings.Contains(plain.Reason, "catalog match") {
		t.Fatalf("plain inspect file = %+v", plain)
	}
	bad := findInspectFile(result.Files, "bad.json")
	if bad.AssociationStatus != InspectAssociationStatusError || bad.Message == "" || !strings.Contains(bad.Reason, "parse failed") {
		t.Fatalf("bad inspect file = %+v", bad)
	}
	badSchema := findInspectFile(result.Files, "bad-schema.json")
	if badSchema.AssociationStatus != InspectAssociationStatusError || badSchema.Message == "" || !strings.Contains(badSchema.Reason, "$schema property") {
		t.Fatalf("bad schema inspect file = %+v", badSchema)
	}

	text := FormatInspectText(result)
	assertContains(t, text, "dollarlint inspected 10 discovered files: 6 associated, 4 without schema, 2 errors")
	assertContains(t, text, "schema: none")
	assertContains(t, text, "suggested catalog ignore:")

	data, err := FormatInspectJSON(result)
	if err != nil {
		t.Fatalf("FormatInspectJSON: %v", err)
	}
	var decoded InspectResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("inspect json: %v\n%s", err, string(data))
	}
	if decoded.FormatVersion != InspectFormatVersion || len(decoded.Files) != 10 || decoded.Files[0].Path == "" {
		t.Fatalf("decoded inspect json = %+v", decoded)
	}
}

func TestInspectWarningsNearestConfigAndHelperEdges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
[configs]
mode = "nearest"

[schemas.catalogs]
enabled = true
failure = "warn"
[[schemas.catalogs.sources]]
url = "https://example.com/catalog.json"
`)
	writeFile(t, filepath.Join(dir, "root.json"), `{}`)
	writeFile(t, filepath.Join(dir, "packages", "api", ".dollarlint.toml"), `
[[schemas.associations]]
file = "*.json"
schema = "../../schema.schema"
`)
	writeFile(t, filepath.Join(dir, "packages", "api", "child.json"), `{}`)
	disabled := false
	cfg, configPath, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.Schemas.Fetch.Enabled = &disabled

	result, err := Inspect(context.Background(), Options{Root: dir, Config: cfg, ConfigPath: configPath})
	if err != nil {
		t.Fatalf("Inspect nearest: %v", err)
	}
	if result.Summary.Warnings != 1 || result.Summary.Discovered != 4 || result.Summary.Associated != 3 || result.Summary.Unassociated != 1 {
		t.Fatalf("nearest inspect summary = %+v warnings=%+v files=%+v", result.Summary, result.Warnings, result.Files)
	}
	child := findInspectFile(result.Files, "packages/api/child.json")
	if child.Schema != "schema.schema" || !strings.Contains(child.Reason, `pattern "packages/api/**/*.json"`) {
		t.Fatalf("nearest child inspect file = %+v", child)
	}
	root := findInspectFile(result.Files, "root.json")
	if root.AssociationStatus != InspectAssociationStatusUnassociated || !strings.Contains(root.Reason, "catalog match") {
		t.Fatalf("nearest root inspect file = %+v", root)
	}
	text := FormatInspectText(result)
	assertContains(t, text, "warnings")
	assertContains(t, text, "requires remote schema fetching")
	if schemaDeclarationReason("") != "schema declaration found in file" {
		t.Fatalf("default schema declaration reason mismatch")
	}
	if noSchemaAssociationReason(Config{}) != "no inline schema, config association, or built-in association; catalog matching is disabled" {
		t.Fatalf("catalog-disabled no schema reason mismatch")
	}
}

func TestInspectReportsKnownSchemaGap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "netlify.toml"), "[build]\ncommand = \"npm test\"\n")

	result, err := Inspect(context.Background(), Options{Root: dir, Config: DefaultConfig()})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	file := findInspectFile(result.Files, "netlify.toml")
	if file.AssociationStatus != InspectAssociationStatusUnassociated || file.SchemaGap == nil || file.SchemaGap.Name != "Netlify deploy config" || !strings.Contains(file.Reason, "SchemaStore's Netlify schema") {
		t.Fatalf("inspect schema gap file = %+v", file)
	}
	text := FormatInspectText(result)
	assertContains(t, text, "SchemaStore's Netlify schema")
	assertContains(t, text, "https://docs.netlify.com/build/configure-builds/file-based-configuration/")
}

func TestInspectDefaultsRootAndReturnsConfigurationErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.json"), `{}`)
	t.Chdir(dir)

	result, err := Inspect(context.Background(), Options{Config: DefaultConfig()})
	if err != nil {
		t.Fatalf("Inspect default root: %v", err)
	}
	if result.Root != "." || result.Summary.Discovered != 1 {
		t.Fatalf("default-root inspect result = %+v", result)
	}

	cfg := DefaultConfig()
	cfg.Schemas.MaxDepth = -1
	if _, err := Inspect(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "schemas.maxDepth") {
		t.Fatalf("expected config validation error, got %v", err)
	}
}

func TestInspectReturnsDiscoveryAndCatalogErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.json"), `{}`)

	if _, err := Inspect(context.Background(), Options{Root: filepath.Join(dir, "missing"), Config: DefaultConfig()}); err == nil || !strings.Contains(err.Error(), "stat root") {
		t.Fatalf("expected discovery error, got %v", err)
	}

	nearest := DefaultConfig()
	nearest.Configs.Mode = ConfigModeNearest
	if _, err := Inspect(context.Background(), Options{Root: filepath.Join(dir, "missing-nearest"), Config: nearest, ConfigPath: filepath.Join(dir, ".dollarlint.toml")}); err == nil || !strings.Contains(err.Error(), "stat root") {
		t.Fatalf("expected nearest discovery error, got %v", err)
	}

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Failure = CatalogFailureError
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{
		Name:   "missing",
		Format: "schemastore",
		Path:   filepath.Join(dir, "missing-catalog.json"),
	}}
	if _, err := Inspect(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "missing-catalog.json") {
		t.Fatalf("expected catalog load error, got %v", err)
	}

	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `version = 1`)
	cfg.Configs.Mode = ConfigModeNearest
	if _, err := Inspect(context.Background(), Options{Root: dir, Config: cfg}); err == nil || !strings.Contains(err.Error(), "missing-catalog.json") {
		t.Fatalf("expected nearest catalog load error, got %v", err)
	}
}

func TestInspectReturnsConfigResolutionErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not reliably block child stat on Windows")
	}
	dir := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatalf("chmod blocked dir: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	cfg := DefaultConfig()
	cfg.Configs.Mode = ConfigModeNearest
	_, err := Inspect(context.Background(), Options{Root: dir, Config: cfg})
	if err == nil {
		t.Skip("permission bits did not block config discovery")
	}
	if !strings.Contains(err.Error(), ".dollarlint.toml") {
		t.Fatalf("expected config resolution error, got %v", err)
	}
}

func TestInspectInternalErrorEdges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.json")
	writeFile(t, path, `{}`)
	file := DiscoveredFile{Path: path, RelativePath: "plain.json"}

	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Match = "maybe"
	if _, err := inspectDiscoveredFiles(context.Background(), dir, []DiscoveredFile{file}, cfg); err == nil || !strings.Contains(err.Error(), "unsupported catalog match mode") {
		t.Fatalf("expected catalog match error, got %v", err)
	}

	cfg = DefaultConfig()
	result, err := inspectConfiguredFileGroups(context.Background(), dir, []configuredFile{{file: file, config: cfg}})
	if err != nil {
		t.Fatalf("inspectConfiguredFileGroups: %v", err)
	}
	if result.Summary.Discovered != 1 || result.Files[0].Path != "plain.json" {
		t.Fatalf("configured file group result = %+v", result)
	}
}

func findInspectFile(files []InspectFile, path string) InspectFile {
	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	return InspectFile{}
}
