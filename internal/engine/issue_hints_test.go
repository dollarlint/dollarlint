package engine

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueHintRulesClassifyKnownPatterns(t *testing.T) {
	tests := []struct {
		name   string
		doc    *Document
		issue  Issue
		wantID string
	}{
		{
			name: "mkdocs inheritance",
			doc: &Document{
				Schema: "https://example.com/mkdocs.schema.json",
				Data:   map[string]any{"INHERIT": "../mkdocs.yml"},
			},
			issue:  Issue{Keyword: "required", Property: "site_name"},
			wantID: "mkdocs.inherited-required",
		},
		{
			name: "github funding blank provider",
			doc:  catalogHintDocument("catalog:schemastore:GitHub Funding", "https://www.schemastore.org/github-funding.json"),
			issue: Issue{
				Keyword:          "type",
				Property:         "github",
				InstanceLocation: "/github",
				Message:          "expected string, received null",
			},
			wantID: "github-funding.blank-provider",
		},
		{
			name: "github workflow strategy matrix",
			doc:  catalogHintDocument("catalog:schemastore:GitHub Workflow", "https://www.schemastore.org/github-workflow.json"),
			issue: Issue{
				Keyword:          "required",
				Property:         "matrix",
				InstanceLocation: "/jobs/build/strategy",
			},
			wantID: "github-workflow.strategy-matrix",
		},
		{
			name: "dependabot reviewers retired",
			doc:  catalogHintDocument("catalog:schemastore:dependabot-v2.json", "https://www.schemastore.org/dependabot-2.0.json"),
			issue: Issue{
				Keyword:  "additionalProperties",
				Property: "reviewers",
			},
			wantID: "dependabot.reviewers-retired",
		},
		{
			name: "legacy ci enum",
			doc:  catalogHintDocument("catalog:schemastore:Travis CI", "https://www.schemastore.org/travis.json"),
			issue: Issue{
				RelativePath: "repo/deps/legacy/.travis.yml",
				Keyword:      "enum",
			},
			wantID: "ci.legacy-enum",
		},
		{
			name: "catalog enum fallback",
			doc:  catalogHintDocument("catalog:schemastore:Ruff", "https://www.schemastore.org/ruff.json"),
			issue: Issue{
				Keyword: "enum",
				Message: "value \"UP999\" is not one of 100 allowed values",
			},
			wantID: "catalog.enum-drift",
		},
		{
			name: "catalog validation fallback",
			doc:  catalogHintDocument("catalog:schemastore:Package", "https://www.schemastore.org/package.json"),
			issue: Issue{
				Keyword:  "required",
				Property: "name",
			},
			wantID: "catalog.validation-context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := tt.issue
			applyValidationIssueHint(tt.doc, &issue, OutputConfig{})
			if issue.IssueHint == nil || issue.IssueHint.ID != tt.wantID || issue.Hint == "" {
				t.Fatalf("hint = %+v text=%q", issue.IssueHint, issue.Hint)
			}
		})
	}

	issue := Issue{Keyword: "required", Property: "name"}
	applyValidationIssueHint(&Document{SchemaSource: "$schema"}, &issue, OutputConfig{})
	if issue.IssueHint != nil || issue.Hint != "" {
		t.Fatalf("unexpected non-catalog hint = %+v", issue)
	}
}

func TestIssueHintsModesAndJSON(t *testing.T) {
	doc := catalogHintDocument("catalog:schemastore:GitHub Workflow", "https://www.schemastore.org/github-workflow.json")
	issue := Issue{Keyword: "required", Property: "matrix", InstanceLocation: "/jobs/test/strategy", Message: "must have required property \"matrix\""}

	disabled := issue
	applyValidationIssueHint(doc, &disabled, OutputConfig{IssueHints: IssueHintsOff})
	if disabled.Hint != "" || disabled.IssueHint != nil {
		t.Fatalf("issue hints off should suppress hints: %+v", disabled)
	}

	verbose := issue
	applyValidationIssueHint(doc, &verbose, OutputConfig{IssueHints: IssueHintsVerbose})
	if !strings.Contains(issueHintText(*verbose.IssueHint, true), "rule github-workflow.strategy-matrix") {
		t.Fatalf("verbose hint text = %q", issueHintText(*verbose.IssueHint, true))
	}

	data, err := FormatJSON(Result{Root: ".", Issues: []Issue{verbose}})
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	var decoded struct {
		Issues []struct {
			IssueHint *IssueHint `json:"issueHint"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json output: %v\n%s", err, string(data))
	}
	if len(decoded.Issues) != 1 || decoded.Issues[0].IssueHint == nil || decoded.Issues[0].IssueHint.ID != "github-workflow.strategy-matrix" {
		t.Fatalf("decoded issue hint = %+v", decoded)
	}
}

func TestFileIssueHintsUseSameLayer(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	writeFile(t, empty, "")
	emptyIssue := issueForError(DiscoveredFile{Path: empty, RelativePath: "empty.json"}, "", issueKeywordParse, errors.New("unexpected EOF"), OutputConfig{})
	if emptyIssue.IssueHint == nil || emptyIssue.IssueHint.ID != "parse.empty-json" || !strings.Contains(emptyIssue.Hint, "Empty JSON") {
		t.Fatalf("empty file hint = %+v", emptyIssue)
	}

	template := filepath.Join(dir, "deployment.yaml")
	writeFile(t, template, "name: {{ .Values.name }}\n")
	templateIssue := issueForError(DiscoveredFile{Path: template, RelativePath: "charts/app/templates/deployment.yaml"}, "", issueKeywordParse, errors.New("did not find expected key"), OutputConfig{})
	if templateIssue.IssueHint == nil || templateIssue.IssueHint.ID != "parse.templated-yaml" {
		t.Fatalf("template hint = %+v", templateIssue)
	}

	disabled := issueForError(DiscoveredFile{Path: empty, RelativePath: "empty.json"}, "", issueKeywordParse, errors.New("unexpected EOF"), OutputConfig{IssueHints: IssueHintsOff})
	if disabled.Hint != "" || disabled.IssueHint != nil {
		t.Fatalf("disabled file hint = %+v", disabled)
	}
}

func TestIssueHintRuleEdges(t *testing.T) {
	normalized := normalizedIssueHint(IssueHint{ID: "custom"})
	if normalized.GroupKey != "custom" || normalized.Confidence != IssueHintConfidenceMedium {
		t.Fatalf("normalized hint = %+v", normalized)
	}
	if schemaSourceOrURIContains(nil, "anything") {
		t.Fatalf("nil document should not match schema source")
	}
	if githubFundingProvider("unknown") {
		t.Fatalf("unexpected funding provider match")
	}
	if issueFileExt(DiscoveredFile{Path: "/tmp/config.toml"}) != ".toml" {
		t.Fatalf("issueFileExt should fall back to absolute path")
	}
	if appendHintSentence("", "only") != "only" {
		t.Fatalf("appendHintSentence empty mismatch")
	}
	hint := IssueHint{Detail: "detail", Suggestion: "suggestion"}
	appendCatalogMatchIssueHint(nil, &hint)
	appendCatalogMatchIssueHint(&Document{}, &hint)
	if hint.Detail != "detail" || hint.Suggestion != "suggestion" {
		t.Fatalf("unexpected catalog match mutation = %+v", hint)
	}

	funding := catalogHintDocument("catalog:schemastore:GitHub Funding", "https://www.schemastore.org/github-funding.json")
	if _, ok := githubFundingBlankProviderIssueHint(issueHintContext{Document: funding, Issue: Issue{Property: "unknown", Keyword: "type", Message: "expected string, received null"}}); ok {
		t.Fatalf("unknown funding provider should not match")
	}
	if _, ok := githubFundingBlankProviderIssueHint(issueHintContext{Document: funding, Issue: Issue{Property: "github", Keyword: "pattern", Message: "does not match pattern"}}); ok {
		t.Fatalf("nonblank funding provider issue should not match")
	}

	workflow := catalogHintDocument("catalog:schemastore:GitHub Workflow", "https://www.schemastore.org/github-workflow.json")
	if _, ok := githubWorkflowStrategyMatrixIssueHint(issueHintContext{Document: workflow, Issue: Issue{Keyword: "type", Property: "matrix", InstanceLocation: "/jobs/build/strategy"}}); ok {
		t.Fatalf("wrong workflow keyword should not match")
	}
	if _, ok := githubWorkflowStrategyMatrixIssueHint(issueHintContext{Document: workflow, Issue: Issue{Keyword: "required", Property: "matrix", InstanceLocation: "/jobs/build/env"}}); ok {
		t.Fatalf("wrong workflow location should not match")
	}

	dependabot := catalogHintDocument("catalog:schemastore:dependabot-v2.json", "https://www.schemastore.org/dependabot-2.0.json")
	if _, ok := dependabotReviewersRetiredIssueHint(issueHintContext{Document: dependabot, Issue: Issue{Keyword: "type", Property: "reviewers"}}); ok {
		t.Fatalf("wrong dependabot keyword should not match")
	}

	travis := catalogHintDocument("catalog:schemastore:Travis CI", "https://www.schemastore.org/travis.json")
	if _, ok := legacyCIEnumIssueHint(issueHintContext{Document: travis, Issue: Issue{RelativePath: ".travis.yml", Keyword: "enum"}}); ok {
		t.Fatalf("top-level CI enum should not match legacy vendor rule")
	}
}

func TestParseIssueHintRules(t *testing.T) {
	dir := t.TempDir()
	if hint, ok := classifyFileIssueHint(
		DiscoveredFile{Path: filepath.Join(dir, "fixture.baseline.jsonc"), RelativePath: "fixture.baseline.jsonc"},
		Issue{Keyword: issueKeywordParse, Message: "invalid"},
	); !ok || hint.ID != "parse.test-baseline" {
		t.Fatalf("baseline hint = %+v ok=%v", hint, ok)
	}
	if hint, ok := classifyFileIssueHint(
		DiscoveredFile{Path: filepath.Join(dir, "records.json"), RelativePath: "records.json"},
		Issue{Keyword: issueKeywordParse, Message: "multiple JSON values"},
	); !ok || hint.ID != "parse.multiple-json-values" {
		t.Fatalf("multiple values hint = %+v ok=%v", hint, ok)
	}
	if hint, ok := classifyFileIssueHint(
		DiscoveredFile{Path: filepath.Join(dir, "dup.yaml"), RelativePath: "dup.yaml"},
		Issue{Keyword: issueKeywordParse, Message: "mapping key \"name\" already defined"},
	); !ok || hint.ID != "parse.yaml-duplicate-key" {
		t.Fatalf("duplicate yaml hint = %+v ok=%v", hint, ok)
	}
	if hint, ok := classifyFileIssueHint(
		DiscoveredFile{Path: filepath.Join(dir, "bad.json"), RelativePath: "bad.json"},
		Issue{Keyword: issueKeywordParse, Message: "invalid literal"},
	); !ok || hint.ID != "parse.invalid-json-extension" {
		t.Fatalf("invalid json hint = %+v ok=%v", hint, ok)
	}
	if _, ok := parseInvalidJSONExtensionIssueHint(issueHintContext{
		File:  DiscoveredFile{Path: filepath.Join(dir, "valid-name.json"), RelativePath: "valid-name.json"},
		Issue: Issue{Keyword: issueKeywordParse, Message: "expected comma"},
	}); ok {
		t.Fatalf("non-extension-specific JSON parse issue should not match invalid extension hint")
	}
	if hint, ok := classifyFileIssueHint(
		DiscoveredFile{Path: filepath.Join(dir, "bad.toml"), RelativePath: "bad.toml"},
		Issue{Keyword: issueKeywordParse, Message: "expected value"},
	); !ok || hint.ID != "parse.invalid-toml" {
		t.Fatalf("invalid toml hint = %+v ok=%v", hint, ok)
	}

	plainYAML := filepath.Join(dir, "plain.yaml")
	writeFile(t, plainYAML, "name: ok\n")
	if _, ok := parseTemplatedYAMLIssueHint(issueHintContext{
		File:  DiscoveredFile{Path: plainYAML, RelativePath: "plain.yaml"},
		Issue: Issue{Keyword: issueKeywordParse, Message: "did not find expected key"},
	}); ok {
		t.Fatalf("plain YAML should not match templated YAML hint")
	}
}

func TestFormatTextGroupsRepeatedIssueHints(t *testing.T) {
	hint := IssueHint{
		ID:         "github-funding.blank-provider",
		Title:      "GitHub Funding provider entries cannot be blank.",
		Detail:     "Unused provider keys often come from templates.",
		Suggestion: "Remove unused provider keys.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "catalog:github-funding:blank-provider",
	}
	first := Issue{RelativePath: ".github/FUNDING.yml", Keyword: "type", Property: "github", Message: "expected string, received null"}
	applyIssueHint(&first, hint)
	second := Issue{RelativePath: ".github/FUNDING.yml", Keyword: "type", Property: "custom", Message: "expected string, received null"}
	applyIssueHint(&second, hint)

	result := Result{
		Summary: Summary{Issues: IssueSummary{Total: 2}, Duration: NewDuration(0)},
		Issues:  []Issue{first, second},
	}
	text := FormatText(result, OutputConfig{})
	assertContains(t, text, "issue hints")
	assertContains(t, text, "github-funding.blank-provider")
	assertContains(t, text, "2 issues across 1 file")
	if strings.Count(text, "GitHub Funding provider entries cannot be blank.") != 1 {
		t.Fatalf("expected grouped hint once:\n%s", text)
	}

	verbose := FormatText(result, OutputConfig{IssueHints: IssueHintsVerbose})
	assertContains(t, verbose, "rule github-funding.blank-provider")
	if strings.Count(verbose, "GitHub Funding provider entries cannot be blank.") < 3 {
		t.Fatalf("expected verbose grouped and per-issue hints:\n%s", verbose)
	}
}

func catalogHintDocument(source, schema string) *Document {
	return &Document{
		SchemaSource: source,
		Schema:       schema,
		SchemaMatch: &SchemaMatch{
			Action:               SchemaMatchActionMatched,
			Reason:               "basename matched catalog fileMatch",
			SuggestedAssociation: "[[schemas.associations]]\nfile = \"example.yml\"\nschema = \"" + schema + "\"",
		},
	}
}
