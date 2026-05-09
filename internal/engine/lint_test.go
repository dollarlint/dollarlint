package engine

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

func TestLintEndToEndWithIgnoresAndAssociations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.json"), `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["name"],
  "additionalProperties": false,
  "properties": {
    "$schema": {"type": "string"},
    "name": {"type": "string"},
    "count": {"type": "integer"}
  }
}`)
	writeFile(t, filepath.Join(dir, "valid.json"), `{"$schema":"./schema.json","name":"ok","count":1}`)
	writeFile(t, filepath.Join(dir, "invalid.json"), `{"$schema":"./schema.json","count":"no","extra":true}`)
	writeFile(t, filepath.Join(dir, "skip.yaml"), `name: no schema`)
	writeFile(t, filepath.Join(dir, "associated.toml"), `name = 42`)
	cfg := configWithoutSchemaStore()
	cfg.Schemas.Associations = []SchemaAssociation{{File: "*.toml", Schema: "./schema.json"}}
	cfg.Ignore = []IgnoreRule{{File: "invalid.json", Keyword: "additionalProperties", Property: "extra", Reason: "known extra"}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Discovered != 5 || result.Summary.Validated != 4 || result.Summary.Skipped != 1 {
		t.Fatalf("summary counts = %+v", result.Summary)
	}
	if result.Summary.Ignored != 1 || result.Summary.Issues.Total != 3 || result.Summary.Issues.Validation != 3 || result.Summary.Issues.Parsing != 0 || !result.HasIssues() {
		t.Fatalf("issue counts = %+v issues=%+v", result.Summary, result.Issues)
	}
	if result.Summary.Duration.Duration <= 0 || result.Summary.DurationNanos <= 0 {
		t.Fatalf("duration was not recorded: %+v", result.Summary)
	}
	var sawRequired, sawType, sawIgnored bool
	for _, issue := range result.Issues {
		switch {
		case issue.Keyword == "required" && issue.Property == "name":
			sawRequired = true
		case issue.Keyword == "type" && issue.Property == "count":
			sawType = true
		case issue.Ignored && issue.Property == "extra":
			sawIgnored = true
		}
	}
	if !sawRequired || !sawType || !sawIgnored {
		t.Fatalf("missing expected issues: %+v", result.Issues)
	}
	text := FormatText(result, OutputConfig{ShowSkipped: true})
	if !strings.Contains(text, "dollarlint found 3 validation issues in 2 files") || !strings.Contains(text, "skipped: skip.yaml") {
		t.Fatalf("text output = %s", text)
	}
	data, err := FormatJSON(result)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json output invalid: %v", err)
	}
}

func TestLintDurationCanUseExternalStart(t *testing.T) {
	dir := t.TempDir()
	cfg := configWithoutSchemaStore()
	cfg.Discovery.Include = []string{"*.nothing"}
	startedAt := time.Now().Add(-2 * time.Second)

	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg, StartedAt: startedAt})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Duration.Duration < 2*time.Second || result.Summary.DurationNanos < int64(2*time.Second) {
		t.Fatalf("duration did not include external start: %+v", result.Summary)
	}
}

func TestLintNearestNestedConfigAppliesChildPolicy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "root.schema"), `{
  "type": "object",
  "required": ["root"],
  "properties": {"$schema": {"type": "string"}, "root": {"type": "boolean"}}
}`)
	writeFile(t, filepath.Join(dir, "root.json"), `{"root": true}`)
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
[configs]
mode = "nearest"

[discovery]
include = ["*.json"]

[[schemas.associations]]
file = "*.json"
schema = "./root.schema"
`)
	writeFile(t, filepath.Join(dir, "packages", "api", "child.schema"), `{
  "type": "object",
  "required": ["child"],
  "properties": {"$schema": {"type": "string"}, "child": {"type": "boolean"}}
}`)
	writeFile(t, filepath.Join(dir, "packages", "api", "child.json"), `{"child": true}`)
	writeFile(t, filepath.Join(dir, "packages", "api", ".dollarlint.toml"), `
[discovery]
include = ["*.json"]

[[schemas.associations]]
file = "*.json"
schema = "./child.schema"
`)

	cfg, configPath, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg, ConfigPath: configPath})
	if err != nil {
		t.Fatalf("Lint nearest: %v", err)
	}
	if result.Summary.Discovered != 2 || result.Summary.Validated != 2 || result.Summary.Issues.Total != 0 {
		t.Fatalf("nearest summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	for _, file := range result.Files {
		if file.RelativePath == "packages/api/child.json" {
			if !strings.HasSuffix(file.Schema, "/packages/api/child.schema") {
				t.Fatalf("child file used wrong schema: %+v", file)
			}
			return
		}
	}
	t.Fatalf("missing child file: %+v", result.Files)
}

func TestLintExplicitConfigSuppressesNearestDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "root.schema"), `{
  "type": "object",
  "required": ["root"],
  "properties": {"$schema": {"type": "string"}, "root": {"type": "boolean"}}
}`)
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), `
[configs]
mode = "nearest"

[discovery]
include = ["*.json"]

[[schemas.associations]]
file = "*.json"
schema = "./root.schema"
`)
	writeFile(t, filepath.Join(dir, "nested", "child.schema"), `{
  "type": "object",
  "required": ["child"],
  "properties": {"$schema": {"type": "string"}, "child": {"type": "boolean"}}
}`)
	writeFile(t, filepath.Join(dir, "nested", ".dollarlint.toml"), `
[[schemas.associations]]
file = "*.json"
schema = "./child.schema"
`)
	writeFile(t, filepath.Join(dir, "nested", "child.json"), `{"child": true}`)

	cfg, configPath, err := LoadConfig(dir, "")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg, ConfigPath: configPath, ExplicitConfig: true})
	if err != nil {
		t.Fatalf("Lint explicit: %v", err)
	}
	if result.Summary.Issues.Total == 0 {
		t.Fatalf("expected explicit config to ignore nested config, result=%+v", result)
	}
}

func TestLintRequiresSchemaCoverage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.schema"), `{"type":"object"}`)
	writeFile(t, filepath.Join(dir, "catalog.catalog"), `{"schemas":[{"name":"Custom","fileMatch":["cataloged.json"],"url":"./schema.schema"}]}`)
	writeFile(t, filepath.Join(dir, "inline.json"), `{"$schema":"./schema.schema","name":"ok"}`)
	writeFile(t, filepath.Join(dir, "associated.yaml"), `name: ok`)
	writeFile(t, filepath.Join(dir, "cataloged.json"), `{"name":"ok"}`)
	writeFile(t, filepath.Join(dir, "uncovered.toml"), `name = "missing"`)

	cfg := DefaultConfig()
	cfg.Discovery.Include = []string{"*.json", "*.yaml", "*.toml"}
	cfg.Schemas.RequireCoverage = true
	cfg.Schemas.Associations = []SchemaAssociation{{File: "associated.yaml", Schema: "./schema.schema"}}
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "company", Format: "schemastore", Path: filepath.Join(dir, "catalog.catalog")}}

	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Discovered != 4 || result.Summary.Validated != 3 || result.Summary.Skipped != 0 || result.Summary.Failed != 1 || result.Summary.Issues.Total != 1 {
		t.Fatalf("summary counts = %+v", result.Summary)
	}
	if len(result.Issues) != 1 || result.Issues[0].RelativePath != "uncovered.toml" || result.Issues[0].Keyword != "schemaCoverage" {
		t.Fatalf("coverage issue = %+v", result.Issues)
	}
	for _, file := range result.Files {
		if file.RelativePath == "uncovered.toml" {
			if file.Status != StatusError || !strings.Contains(file.Message, "not covered") {
				t.Fatalf("uncovered file result = %+v", file)
			}
			return
		}
	}
	t.Fatalf("missing uncovered file result: %+v", result.Files)
}

func TestLintAppliesBuiltinDollarlintConfigSchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), "version = 1\n")

	result, err := Lint(context.Background(), Options{Root: dir, Config: DefaultConfig()})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Discovered != 1 || result.Summary.Validated != 1 || result.Summary.Skipped != 0 || result.Summary.Issues.Total != 0 {
		t.Fatalf("summary counts = %+v issues=%+v", result.Summary, result.Issues)
	}
	if len(result.Files) != 1 || result.Files[0].Schema != builtinDollarlintConfigSchemaURI || result.Files[0].SchemaSource != builtinDollarlintConfigSchemaSource {
		t.Fatalf("builtin schema file = %+v", result.Files)
	}

	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), "version = 1\nunknown = true\n")
	result, err = Lint(context.Background(), Options{Root: dir, Config: DefaultConfig()})
	if err != nil {
		t.Fatalf("Lint invalid config: %v", err)
	}
	if result.Summary.Issues.Total == 0 {
		t.Fatalf("expected invalid config issue, result = %+v", result)
	}
}

func TestLintUserAssociationWinsOverBuiltinDollarlintConfigSchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "allow.schema.json"), `{"type":"object"}`)
	writeFile(t, filepath.Join(dir, ".dollarlint.toml"), "unknown = true\n")

	cfg := DefaultConfig()
	cfg.Schemas.Associations = []SchemaAssociation{{File: ".dollarlint.toml", Schema: "./allow.schema.json"}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	for _, file := range result.Files {
		if file.RelativePath == ".dollarlint.toml" {
			if file.SchemaSource != "config-association" || strings.Contains(file.Schema, "dollarlint://") {
				t.Fatalf("config association should win: %+v", file)
			}
			return
		}
	}
	t.Fatalf("missing .dollarlint.toml result: %+v", result.Files)
}

func TestLintOnlyBuildsRequestedSourceLocations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.json"), `{"type":"object","required":["name"],"properties":{"$schema":{"type":"string"},"name":{"type":"string"}}}`)
	writeFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json"}`)
	result, err := Lint(context.Background(), Options{Root: dir, Config: configWithoutSchemaStore()})
	if err != nil {
		t.Fatalf("Lint without source locations: %v", err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Line != 0 || result.Issues[0].Column != 0 {
		t.Fatalf("issue should not have source location by default: %+v", result.Issues)
	}
	cfg := DefaultConfig()
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg, SourceLocations: true})
	if err != nil {
		t.Fatalf("Lint with source locations: %v", err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Line == 0 || result.Issues[0].Column == 0 {
		t.Fatalf("issue should have source location when requested: %+v", result.Issues)
	}
}

func TestLintValidatesJSONVariants(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.jsonc"), `{
  // schemas may also be JSONC
  "type": "object",
  "required": ["name"],
  "properties": {"$schema": {"type": "string"}, "name": {"type": "string"}}
}`)
	writeFile(t, filepath.Join(dir, "schema.json5"), `{
  // schemas may also be JSON5
  type: 'object',
  required: ['active'],
  properties: {$schema: {type: 'string'}, active: {type: 'boolean'}},
}`)
	writeFile(t, filepath.Join(dir, "config.jsonc"), `{
  "$schema": "./schema.jsonc",
  // comments and trailing commas
  "name": 42,
}`)
	writeFile(t, filepath.Join(dir, "other.json5"), `{
  $schema: './schema.json5',
  active: true,
}`)
	cfg := configWithoutSchemaStore()
	cfg.Output.Locations = true
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint json variants: %v", err)
	}
	if result.Summary.Discovered != 4 || result.Summary.Validated != 2 || result.Summary.Skipped != 2 || result.Summary.Issues.Total != 1 {
		t.Fatalf("json variants summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	if len(result.Issues) != 1 || result.Issues[0].RelativePath != "config.jsonc" || result.Issues[0].Property != "name" || result.Issues[0].Line != 4 || result.Issues[0].Column == 0 {
		t.Fatalf("jsonc issue = %+v", result.Issues)
	}
	for _, file := range result.Files {
		if file.RelativePath == "other.json5" && file.Format != DocumentFormatJSON5 {
			t.Fatalf("json5 file format = %+v", file)
		}
	}
}

func TestLintValidatesJSONLinesAsIndependentDocuments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "record.schema.json"), `{
  "type": "object",
  "required": ["name"],
  "additionalProperties": false,
  "properties": {
    "name": {"type": "string"},
    "count": {"type": "integer"}
  }
}`)
	writeFile(t, filepath.Join(dir, "events.jsonl"), `
{"name":"ok","count":1}
{"name":42}
{"count":2}
{"name":"extra","extra":true}
{"name":}
`)
	cfg := configWithoutSchemaStore()
	cfg.Discovery.Include = []string{"*.jsonl"}
	cfg.Schemas.Associations = []SchemaAssociation{{File: "*.jsonl", Schema: "./record.schema.json"}}
	cfg.Output.Locations = true
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint jsonl: %v", err)
	}
	if result.Summary.Discovered != 1 || result.Summary.Validated != 1 || result.Summary.Failed != 1 || result.Summary.Issues.Total != 4 {
		t.Fatalf("jsonl summary = %+v issues=%+v", result.Summary, result.Issues)
	}
	if len(result.Files) != 1 || result.Files[0].Status != StatusError || result.Files[0].Format != DocumentFormatJSONLines {
		t.Fatalf("jsonl file result = %+v", result.Files)
	}
	typeIssue := findIssue(result.Issues, "type", "name")
	if typeIssue.Line != 2 || typeIssue.Column == 0 || typeIssue.InstanceLocation != "/name" {
		t.Fatalf("jsonl type issue = %+v", typeIssue)
	}
	requiredIssue := findIssue(result.Issues, "required", "name")
	if requiredIssue.Line != 3 || requiredIssue.Column == 0 {
		t.Fatalf("jsonl required issue = %+v", requiredIssue)
	}
	additionalIssue := findIssue(result.Issues, "additionalProperties", "extra")
	if additionalIssue.Line != 4 || additionalIssue.Column == 0 || additionalIssue.InstanceLocation != "/extra" {
		t.Fatalf("jsonl additional issue = %+v", additionalIssue)
	}
	parseIssue := findMessageIssue(result.Issues, "parse line 5")
	if parseIssue.Keyword != issueKeywordParse || parseIssue.Line != 5 || parseIssue.Column == 0 {
		t.Fatalf("jsonl parse issue = %+v", parseIssue)
	}
}

func TestLintParseSchemaAndPrimeErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.json"), `{`)
	writeFile(t, filepath.Join(dir, "bad-schema.json"), `{"$schema":"./missing.json"}`)
	writeFile(t, filepath.Join(dir, "invalid-schema.json"), `{"$schema":"./schema.json","name":"ok"}`)
	writeFile(t, filepath.Join(dir, "schema.json"), `{"type": 42}`)
	result, err := Lint(context.Background(), Options{Root: dir, Config: configWithoutSchemaStore()})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if result.Summary.Issues.Total == 0 {
		t.Fatalf("expected parse/schema issues")
	}
	var parseFile FileResult
	for _, file := range result.Files {
		if file.RelativePath == "bad.json" {
			parseFile = file
			break
		}
	}
	if parseFile.Format == "" || parseFile.Status != StatusError {
		t.Fatalf("parse error file result = %+v", parseFile)
	}
	var parseIssue, loadIssue, compileIssue bool
	for _, issue := range result.Issues {
		if issue.Keyword == issueKeywordParse && strings.Contains(issue.Message, "parse") {
			parseIssue = true
		}
		if issue.Keyword == issueKeywordSchema && strings.Contains(issue.Message, "schema load failed") {
			loadIssue = true
		}
		if issue.Keyword == issueKeywordSchema && strings.Contains(issue.Message, "schema compile failed") {
			compileIssue = true
		}
	}
	if !parseIssue || !loadIssue || !compileIssue {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func TestLintMissingDependencySchemaHint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "project.json"), `{"$schema":"./node_modules/tool/schema.json","name":"app"}`)
	result, err := Lint(context.Background(), Options{Root: dir, Config: configWithoutSchemaStore()})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	issue := result.Issues[0]
	if issue.Keyword != issueKeywordSchema || !strings.Contains(issue.Message, "schema load failed") {
		t.Fatalf("schema issue = %+v", issue)
	}
	if !strings.Contains(issue.Hint, "install dependencies before validating") ||
		!strings.Contains(issue.Hint, "tool schemas are unavailable") {
		t.Fatalf("hint = %q", issue.Hint)
	}
}

func TestLintRootAndDiscoveryErrorEdges(t *testing.T) {
	result, err := Lint(context.Background(), Options{Config: Config{Discovery: DiscoveryConfig{Include: []string{"*.nothing"}}}})
	if err != nil {
		t.Fatalf("Lint default root: %v", err)
	}
	if result.Root != "." {
		t.Fatalf("default root = %q", result.Root)
	}
	_, err = Lint(context.Background(), Options{Root: filepath.Join(t.TempDir(), "missing"), Config: DefaultConfig()})
	if err == nil {
		t.Fatalf("expected discovery error")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad-uri.json"), `{"$schema":"%zz"}`)
	result, err = Lint(context.Background(), Options{Root: dir, Config: DefaultConfig()})
	if err != nil {
		t.Fatalf("Lint bad URI: %v", err)
	}
	if result.Summary.Failed != 1 || result.Summary.Issues.Total != 1 {
		t.Fatalf("bad URI result = %+v", result)
	}
}

func TestCompileSchemaTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`{"type":"string"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	writeFile(t, schemaPath, `{"$ref":`+strconv.Quote(server.URL+`/slow.json`)+`}`)
	schemaURL, err := fileURL(schemaPath)
	if err != nil {
		t.Fatalf("fileURL: %v", err)
	}
	cfg := DefaultConfig()
	cfg.Schemas.Compile.Timeout = NewDuration(time.Millisecond)
	cfg.Schemas.Fetch.Timeout = NewDuration(time.Second)
	cache := NewSchemaCache(cfg)
	_, err = compileSchema(context.Background(), cache, cfg, schemaURL.String(), nil)
	if err == nil {
		t.Fatalf("expected compile timeout")
	}
}

func TestValidationIssueHelpers(t *testing.T) {
	doc := &Document{Path: "/tmp/file.json", RelativePath: "file.json", Schema: "file:///tmp/schema.json"}
	err := &jsonschema.ValidationError{
		SchemaURL:        "file:///tmp/schema.json",
		InstanceLocation: []string{"name"},
		ErrorKind:        &kind.PropertyNames{Property: "bad"},
	}
	issues := issuesFromValidationError(doc, err)
	if len(issues) != 1 || issues[0].Property != "bad" || issues[0].InstanceLocation != "/name" {
		t.Fatalf("property issue = %+v", issues)
	}
	noKind := &jsonschema.ValidationError{}
	if keywordName(noKind) != "" || keywordLocation(noKind) != "" {
		t.Fatalf("expected empty keyword helpers")
	}
	if instanceLocation(nil) != "/" {
		t.Fatalf("root instance location mismatch")
	}
	if validationMessage(noKind) == "" {
		t.Fatalf("expected fallback validation message")
	}
	mkdocs := &Document{
		Path:         "/tmp/mkdocs.yml",
		RelativePath: "mkdocs.yml",
		Schema:       "https://www.schemastore.org/mkdocs-1.6.json",
		SchemaSource: "catalog:schemastore:mkdocs.yml",
		Data:         map[string]any{"INHERIT": "../en/mkdocs.yml"},
	}
	mkdocsIssues := issuesFromValidationError(mkdocs, &jsonschema.ValidationError{
		InstanceLocation: nil,
		ErrorKind:        &kind.Required{Missing: []string{"site_name"}},
	})
	if len(mkdocsIssues) != 1 || !strings.Contains(mkdocsIssues[0].Hint, "top-level INHERIT") {
		t.Fatalf("mkdocs inherited hint = %+v", mkdocsIssues)
	}
	messageCases := map[jsonschema.ErrorKind]string{
		&kind.Type{Got: "number", Want: []string{"string"}}: "expected string, received number",
		&kind.MinProperties{Want: 2}:                        "must have at least 2 properties",
		&kind.MaxProperties{Want: 3}:                        "must have at most 3 properties",
		&kind.MinItems{Want: 1}:                             "must have at least 1 item",
		&kind.MaxItems{Want: 4}:                             "must have at most 4 items",
		&kind.MinLength{Want: 2}:                            "must be at least 2 characters",
		&kind.MaxLength{Want: 5}:                            "must be at most 5 characters",
		&kind.Minimum{Want: big.NewRat(1, 1)}:               "must be >= 1",
		&kind.Maximum{Want: big.NewRat(5, 1)}:               "must be <= 5",
		&kind.ExclusiveMinimum{Want: big.NewRat(1, 2)}:      "must be > 0.5",
		&kind.ExclusiveMaximum{Want: big.NewRat(3, 2)}:      "must be < 1.5",
		&kind.MultipleOf{Want: big.NewRat(25, 10)}:          "must be a multiple of 2.5",
		&kind.Minimum{}:                                     "must be >= 0",
	}
	for errorKind, expected := range messageCases {
		if got := validationMessage(&jsonschema.ValidationError{ErrorKind: errorKind}); got != expected {
			t.Fatalf("validationMessage(%T) = %q, want %q", errorKind, got, expected)
		}
	}
	dep := propertyFromKind(&kind.Dependency{Prop: "a"})
	depReq := propertyFromKind(&kind.DependentRequired{Prop: "b"})
	if dep != "a" || depReq != "b" || propertyFromKind(nil) != "" {
		t.Fatalf("dependency properties = %q %q", dep, depReq)
	}
}

func TestValidationIssuesCompactChoiceBranches(t *testing.T) {
	doc := &Document{Path: "/tmp/file.json", RelativePath: "file.json", Schema: "file:///tmp/schema.json"}
	err := &jsonschema.ValidationError{
		InstanceLocation: []string{"resources"},
		ErrorKind:        &kind.OneOf{},
		Causes: []*jsonschema.ValidationError{
			{
				InstanceLocation: []string{"resources", "0"},
				ErrorKind:        &kind.Group{},
				Causes: []*jsonschema.ValidationError{
					{
						InstanceLocation: []string{"resources", "0"},
						ErrorKind:        &kind.OneOf{},
						Causes: []*jsonschema.ValidationError{
							{
								InstanceLocation: []string{"resources", "0"},
								ErrorKind:        &kind.Group{},
								Causes: []*jsonschema.ValidationError{
									{
										InstanceLocation: []string{"resources", "0", "sku", "name"},
										ErrorKind: &kind.Enum{Got: "NotARealSku", Want: []any{
											"Standard_LRS",
										}},
									},
									{
										InstanceLocation: []string{"resources", "0", "properties", "allowBlobPublicAccess"},
										ErrorKind:        &kind.Pattern{Got: "nope", Want: "^\\[([^\\[].*)?\\]$"},
									},
								},
							},
							{
								InstanceLocation: []string{"resources", "0"},
								ErrorKind:        &kind.Group{},
								Causes: []*jsonschema.ValidationError{
									{
										InstanceLocation: []string{"resources", "0", "type"},
										ErrorKind: &kind.Enum{Got: "Microsoft.Storage/storageAccounts", Want: []any{
											"Microsoft.Storage/storageAccounts/blobServices",
										}},
									},
									{
										InstanceLocation: []string{"resources", "0", "name"},
										ErrorKind:        &kind.Pattern{Got: "invalidstorage", Want: "^.*/default$"},
									},
								},
							},
						},
					},
				},
			},
			{
				InstanceLocation: []string{"resources"},
				ErrorKind:        &kind.Type{Got: "array", Want: []string{"object"}},
			},
		},
	}
	issues := issuesFromValidationError(doc, err)
	if len(issues) != 2 {
		t.Fatalf("compacted issues = %+v", issues)
	}
	for _, issue := range issues {
		if issue.InstanceLocation == "/resources" || issue.InstanceLocation == "/resources/0/type" {
			t.Fatalf("reported unrelated branch issue: %+v", issues)
		}
	}
	if issues[0].InstanceLocation != "/resources/0/sku/name" || issues[1].InstanceLocation != "/resources/0/properties/allowBlobPublicAccess" {
		t.Fatalf("unexpected compacted issues = %+v", issues)
	}
	allIssues := issuesFromValidationErrorWithOutput(doc, err, OutputConfig{BranchErrors: BranchErrorsAll})
	if len(allIssues) != 5 {
		t.Fatalf("all branch issues = %+v", allIssues)
	}
	var sawRootType, sawResourceType bool
	for _, issue := range allIssues {
		if issue.InstanceLocation == "/resources" {
			sawRootType = true
		}
		if issue.InstanceLocation == "/resources/0/type" {
			sawResourceType = true
		}
	}
	if !sawRootType || !sawResourceType {
		t.Fatalf("all branch issues omitted branch failures: %+v", allIssues)
	}
}

func TestValidationIssuesDedupeRepeatedLeaves(t *testing.T) {
	doc := &Document{Path: "/tmp/file.json", RelativePath: "file.json", Schema: "file:///tmp/schema.json"}
	duplicate := &jsonschema.ValidationError{
		InstanceLocation: []string{"name"},
		ErrorKind:        &kind.Pattern{Got: "bad", Want: "^[a-z]+$"},
	}
	err := &jsonschema.ValidationError{
		ErrorKind: &kind.Group{},
		Causes: []*jsonschema.ValidationError{
			duplicate,
			{
				InstanceLocation: []string{"name"},
				ErrorKind:        &kind.Pattern{Got: "bad", Want: "^[a-z]+$"},
			},
		},
	}
	issues := issuesFromValidationError(doc, err)
	if len(issues) != 1 || issues[0].InstanceLocation != "/name" {
		t.Fatalf("deduped issues = %+v", issues)
	}
}

func TestLintBranchErrorsOutputMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "schema.json"), `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "oneOf": [
    {
      "type": "object",
      "required": ["name"],
      "properties": {
        "$schema": {"type": "string"},
        "kind": {"const": "widget"},
        "name": {"type": "string"}
      }
    },
    {
      "type": "object",
      "required": ["label"],
      "properties": {
        "$schema": {"type": "string"},
        "kind": {"const": "gadget"},
        "label": {"type": "string"}
      }
    }
  ]
}`)
	writeFile(t, filepath.Join(dir, "bad.json"), `{"$schema":"./schema.json","kind":"widget"}`)

	cfg := configWithoutSchemaStore()
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint branch best: %v", err)
	}
	if result.Summary.Issues.Total != 1 || result.Issues[0].Property != "name" {
		t.Fatalf("best branch result = %+v issues=%+v", result.Summary, result.Issues)
	}

	cfg.Output.BranchErrors = BranchErrorsAll
	result, err = Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint branch all: %v", err)
	}
	if result.Summary.Issues.Total != 3 {
		t.Fatalf("all branch result = %+v issues=%+v", result.Summary, result.Issues)
	}
	var sawName, sawKind, sawLabel bool
	for _, issue := range result.Issues {
		switch issue.Property {
		case "name":
			sawName = true
		case "kind":
			sawKind = true
		case "label":
			sawLabel = true
		}
	}
	if !sawName || !sawKind || !sawLabel {
		t.Fatalf("all branch issues = %+v", result.Issues)
	}
}

func TestIgnoreMatching(t *testing.T) {
	issue := Issue{RelativePath: "nested/file.json", Keyword: "type", KeywordLocation: "/properties/name/type", SchemaSource: "catalog:schemastore:Example", Property: "name", InstanceLocation: "/name"}
	if !ignoreMatches(issue, IgnoreRule{File: "**/*.json", Keyword: "/properties/name/type", SchemaSource: "catalog:schemastore:Example", Property: "/name"}) {
		t.Fatalf("expected ignore by keyword location and instance pointer")
	}
	if ignoreMatches(issue, IgnoreRule{File: "*.yaml"}) {
		t.Fatalf("unexpected file match")
	}
	if ignoreMatches(issue, IgnoreRule{Keyword: "required"}) {
		t.Fatalf("unexpected keyword match")
	}
	if ignoreMatches(issue, IgnoreRule{Property: "/other"}) {
		t.Fatalf("unexpected property pointer match")
	}
	if ignoreMatches(issue, IgnoreRule{SchemaSource: "config-association"}) {
		t.Fatalf("unexpected schema source match")
	}
	applyIgnore(&issue, []IgnoreRule{{Property: "na*"}})
	if !issue.Ignored || issue.IgnoreReason != "matched ignore rule" {
		t.Fatalf("applyIgnore = %+v", issue)
	}
}

func TestApplySchemaAssociationSkipsIncompleteRules(t *testing.T) {
	doc := &Document{RelativePath: "file.json"}
	applySchemaAssociation(doc, []SchemaAssociation{{File: ""}, {File: "*.yaml", Schema: "schema.json"}}, "config-association")
	if doc.Schema != "" {
		t.Fatalf("unexpected association = %+v", doc)
	}
}

func TestLintAppliesSchemaStoreAssociationsWhenEnabled(t *testing.T) {
	var catalogRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog.json":
			catalogRequests++
			w.Write([]byte(`{"schemas":[{"fileMatch":["example.yaml"],"url":"` + "http://" + r.Host + `/schema.json"}]}`))
		case "/schema.json":
			w.Write([]byte(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "example.yaml"), `name: 42`)
	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Enabled = true
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "schemastore", Format: "schemastore", URL: server.URL + "/catalog.json"}}
	cache := false
	cfg.Schemas.Fetch.Cache = &cache
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if catalogRequests != 1 {
		t.Fatalf("catalog requests = %d", catalogRequests)
	}
	if result.Summary.Validated != 1 || result.Summary.Skipped != 0 || len(result.Issues) != 1 {
		t.Fatalf("schemastore result = %+v issues=%+v", result.Summary, result.Issues)
	}
	if result.Files[0].SchemaSource != "catalog:schemastore" || !strings.HasSuffix(result.Files[0].Schema, "/schema.json") {
		t.Fatalf("file schema = %+v", result.Files[0])
	}
}

func TestLintCanDisableSchemaStoreAssociations(t *testing.T) {
	var catalogRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalogRequests++
		w.Write([]byte(`{"schemas":[{"fileMatch":["example.yaml"],"url":"` + "http://" + r.Host + `/schema.json"}]}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "example.yaml"), `name: 42`)
	cfg := DefaultConfig()
	cfg.Schemas.Catalogs.Sources = []CatalogSource{{Name: "schemastore", Format: "schemastore", URL: server.URL}}
	result, err := Lint(context.Background(), Options{Root: dir, Config: cfg})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if catalogRequests != 0 {
		t.Fatalf("catalog should not have been fetched")
	}
	if result.Summary.Validated != 0 || result.Summary.Skipped != 1 {
		t.Fatalf("disabled schemastore result = %+v", result.Summary)
	}
}

func TestValidateDocumentNonValidationError(t *testing.T) {
	err := errors.New("plain")
	issue := issueForError(DiscoveredFile{Path: "/tmp/a.json", RelativePath: "a.json"}, "schema", issueKeywordSchema, err)
	if issue.Message != "plain" || issue.Schema != "schema" || issue.Keyword != issueKeywordSchema {
		t.Fatalf("issueForError = %+v", issue)
	}
	issues := issuesFromSchemaError(&Document{Path: "/tmp/a.json", RelativePath: "a.json", Schema: "schema"}, err, OutputConfig{})
	if len(issues) != 1 || issues[0].Message != "plain" || issues[0].Keyword != issueKeywordSchema {
		t.Fatalf("issuesFromSchemaError = %+v", issues)
	}
	issue = Issue{}
	applyIssuePosition(&Document{SourceMap: SourceMap{"/": {Line: 9, Column: 2}}}, &issue)
	if issue.Line != 9 || issue.Column != 2 {
		t.Fatalf("empty pointer position = %+v", issue)
	}
	applyIssuePosition(&Document{}, &issue)
	if issue.Line != 9 || issue.Column != 2 {
		t.Fatalf("existing position should not be overwritten: %+v", issue)
	}
}

func configWithoutSchemaStore() Config {
	cfg := DefaultConfig()
	return cfg
}

func findIssue(issues []Issue, keyword, property string) Issue {
	for _, issue := range issues {
		if issue.Keyword == keyword && issue.Property == property {
			return issue
		}
	}
	return Issue{}
}

func findMessageIssue(issues []Issue, messagePart string) Issue {
	for _, issue := range issues {
		if strings.Contains(issue.Message, messagePart) {
			return issue
		}
	}
	return Issue{}
}
