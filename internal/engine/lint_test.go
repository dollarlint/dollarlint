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
	if result.Summary.Ignored != 1 || result.Summary.Issues != 3 || !result.HasIssues() {
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
	if !strings.Contains(text, "dollarlint found 3 issues in 2 files") || !strings.Contains(text, "skipped: skip.yaml") {
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
	if result.Summary.Issues == 0 {
		t.Fatalf("expected parse/schema issues")
	}
	var parseIssue, compileIssue bool
	for _, issue := range result.Issues {
		if strings.Contains(issue.Message, "parse") {
			parseIssue = true
		}
		if strings.Contains(issue.Message, "compile schema") {
			compileIssue = true
		}
	}
	if !parseIssue || !compileIssue {
		t.Fatalf("issues = %+v", result.Issues)
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
	if result.Summary.Failed != 1 || result.Summary.Issues != 1 {
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

func TestIgnoreMatching(t *testing.T) {
	issue := Issue{RelativePath: "nested/file.json", Keyword: "type", KeywordLocation: "/properties/name/type", Property: "name", InstanceLocation: "/name"}
	if !ignoreMatches(issue, IgnoreRule{File: "**/*.json", Keyword: "/properties/name/type", Property: "/name"}) {
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
	issue := issueForError(DiscoveredFile{Path: "/tmp/a.json", RelativePath: "a.json"}, "schema", err)
	if issue.Message != "plain" || issue.Schema != "schema" {
		t.Fatalf("issueForError = %+v", issue)
	}
	issues := issuesFromSchemaError(&Document{Path: "/tmp/a.json", RelativePath: "a.json", Schema: "schema"}, err)
	if len(issues) != 1 || issues[0].Message != "plain" {
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
