package engine

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestFormatTextGroupedDefault(t *testing.T) {
	result := textFixtureResult()
	text := FormatText(result, OutputConfig{})
	assertContains(t, text, "dollarlint found 5 validation issues in 2 files after 123ms")
	assertContains(t, text, "\na.json\n")
	assertContains(t, text, "/name     type")
	assertContains(t, text, "/count    minimum")
	assertContains(t, text, "\nb.toml\n")
	assertContains(t, text, "/enabled  type")
	assertContains(t, text, "Summary: 4 discovered, 3 validated, 1 skipped, 5 validation issues in 123ms")
	if strings.Contains(text, "schema:") {
		t.Fatalf("default output should not include verbose schema details:\n%s", text)
	}
}

func TestFormatTextLocationsVerboseSkippedAndQuiet(t *testing.T) {
	result := textFixtureResult()
	result.Warnings = []Warning{{Kind: "schemaCatalogUnavailable", Source: "schemastore", Message: "catalog unavailable"}}
	result.Summary.Warnings = 1
	text := FormatText(result, OutputConfig{Locations: true, Verbose: true, ShowSkipped: true})
	assertContains(t, text, "2:7       type")
	assertContains(t, text, "3:10      minimum")
	assertContains(t, text, "location: /name")
	assertContains(t, text, "schema: file:///schema.json")
	assertContains(t, text, "warnings")
	assertContains(t, text, "catalog unavailable")
	assertContains(t, text, "1 warning")
	assertContains(t, text, "skipped: skipped.json (no schema)")
	locationOnly := FormatText(result, OutputConfig{Locations: true})
	assertContains(t, locationOnly, "2:7       type")
	assertContains(t, locationOnly, "expected string, received number  /name")
	quiet := FormatText(result, OutputConfig{Quiet: true})
	assertContains(t, quiet, "dollarlint found 5 validation issues in 2 files after 123ms")
	assertContains(t, quiet, "1 warning")
	if strings.Contains(quiet, "Summary:") {
		t.Fatalf("quiet output should omit summary:\n%s", quiet)
	}
	passed := FormatText(Result{}, OutputConfig{Quiet: true})
	if passed != "dollarlint passed in 0s\n" {
		t.Fatalf("quiet pass output = %q", passed)
	}
	passed = FormatText(Result{Summary: Summary{Discovered: 1, Validated: 1, Duration: NewDuration(123 * time.Millisecond)}}, OutputConfig{})
	assertContains(t, passed, "dollarlint passed in 123ms: 1 discovered, 1 validated, 0 skipped")
	passed = FormatText(Result{Summary: Summary{Warnings: 1, Duration: NewDuration(123 * time.Millisecond)}, Warnings: []Warning{{Kind: "x", Message: "careful"}}}, OutputConfig{})
	assertContains(t, passed, "dollarlint passed with 1 warning in 123ms")
	assertContains(t, passed, "careful")
}

func TestFormatTextIssueBreakdownAndHints(t *testing.T) {
	result := Result{
		Summary: Summary{Issues: IssueSummary{Total: 2, Parsing: 1, Validation: 1}, Duration: NewDuration(123 * time.Millisecond), DurationNanos: int64(123 * time.Millisecond)},
		Issues: []Issue{
			{
				RelativePath: "data.json",
				Keyword:      issueKeywordParse,
				Message:      "parse data.json: multiple JSON values",
				Hint:         "Use .jsonl for line-delimited data.",
			},
			{
				RelativePath:     "config.json",
				Keyword:          "type",
				InstanceLocation: "/name",
				Message:          "expected string, received number",
			},
		},
	}
	text := FormatText(result, OutputConfig{})
	assertContains(t, text, "dollarlint found 2 issues (1 parsing, 1 validation) in 2 files after 123ms")
	assertContains(t, text, "hint: Use .jsonl for line-delimited data.")
	assertContains(t, text, "Summary: 0 discovered, 0 validated, 0 skipped, 2 issues (1 parsing, 1 validation) in 123ms")
}

func TestFormatJSONContract(t *testing.T) {
	result := Result{
		Root: "/tmp/repo",
		Summary: Summary{
			Discovered:    1,
			Validated:     1,
			Issues:        IssueSummary{Total: 1, Validation: 1},
			Ignored:       1,
			Warnings:      1,
			Duration:      NewDuration(123 * time.Millisecond),
			DurationNanos: int64(123 * time.Millisecond),
		},
		Files: []FileResult{{
			Path:         "/tmp/repo/config.json",
			RelativePath: "config.json",
			Format:       DocumentFormatJSON,
			Schema:       "file:///tmp/repo/schema.json",
			SchemaSource: "$schema",
			Status:       StatusError,
			Issues:       1,
			Ignored:      1,
		}},
		Issues: []Issue{
			{
				File:             "/tmp/repo/config.json",
				RelativePath:     "config.json",
				Schema:           "file:///tmp/repo/schema.json",
				Keyword:          "type",
				InstanceLocation: "/name",
				Line:             2,
				Column:           11,
				Message:          "expected string, received number",
			},
			{
				File:             "/tmp/repo/config.json",
				RelativePath:     "config.json",
				Schema:           "file:///tmp/repo/schema.json",
				Keyword:          "additionalProperties",
				Property:         "extra",
				InstanceLocation: "/extra",
				Message:          "must not have additional property \"extra\"",
				Ignored:          true,
				IgnoreReason:     "known extra",
			},
		},
		Warnings: []Warning{{
			Kind:         "schemaCatalogSchemaUnavailable",
			Source:       "catalog:schemastore:devcontainer.json",
			Path:         "config.json",
			Schema:       "https://example.com/schema.json",
			SchemaSource: "catalog:schemastore:devcontainer.json",
			Message:      "catalog schema load failed",
			Hint:         "Try again later.",
		}},
	}

	data, err := FormatJSON(result)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	text := string(data)
	for _, removed := range []string{`"duration":`, `"relativePath":`, `"file":`, `"file:///tmp/repo"`, `"ignored": true`} {
		if strings.Contains(text, removed) {
			t.Fatalf("json output contains removed field %q:\n%s", removed, text)
		}
	}
	var decoded struct {
		Schema        string `json:"$schema"`
		FormatVersion int    `json:"formatVersion"`
		Summary       struct {
			DurationNanos int64        `json:"durationNanos"`
			Issues        IssueSummary `json:"issues"`
		} `json:"summary"`
		Files []struct {
			Path         string `json:"path"`
			Schema       string `json:"schema"`
			SchemaSource string `json:"schemaSource"`
			Issues       int    `json:"issues"`
			Ignored      int    `json:"ignored"`
		} `json:"files"`
		Issues []struct {
			Path         string `json:"path"`
			Schema       string `json:"schema"`
			Category     string `json:"category"`
			SchemaSource string `json:"schemaSource"`
			Line         int    `json:"line"`
			Column       int    `json:"column"`
		} `json:"issues"`
		IgnoredIssues []struct {
			Path         string `json:"path"`
			Category     string `json:"category"`
			IgnoreReason string `json:"ignoreReason"`
		} `json:"ignoredIssues"`
		Warnings []struct {
			Kind         string `json:"kind"`
			Path         string `json:"path"`
			Schema       string `json:"schema"`
			SchemaSource string `json:"schemaSource"`
			Hint         string `json:"hint"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json output invalid: %v\n%s", err, text)
	}
	if decoded.Schema != JSONResultSchema || decoded.FormatVersion != JSONFormatVersion || decoded.Summary.DurationNanos != int64(123*time.Millisecond) || decoded.Summary.Issues.Total != 1 {
		t.Fatalf("json summary = %+v version=%d", decoded.Summary, decoded.FormatVersion)
	}
	if len(decoded.Files) != 1 || decoded.Files[0].Path != "config.json" || decoded.Files[0].Schema != "schema.json" || decoded.Files[0].SchemaSource != "$schema" || decoded.Files[0].Issues != 1 || decoded.Files[0].Ignored != 1 {
		t.Fatalf("json files = %+v", decoded.Files)
	}
	if len(decoded.Issues) != 1 || decoded.Issues[0].Path != "config.json" || decoded.Issues[0].Schema != "schema.json" || decoded.Issues[0].Category != "validation" || decoded.Issues[0].SchemaSource != "$schema" || decoded.Issues[0].Line != 2 || decoded.Issues[0].Column != 11 {
		t.Fatalf("json issues = %+v", decoded.Issues)
	}
	if len(decoded.IgnoredIssues) != 1 || decoded.IgnoredIssues[0].Category != "validation" || decoded.IgnoredIssues[0].IgnoreReason != "known extra" {
		t.Fatalf("json ignored issues = %+v", decoded.IgnoredIssues)
	}
	if len(decoded.Warnings) != 1 || decoded.Warnings[0].Path != "config.json" || decoded.Warnings[0].Schema == "" || decoded.Warnings[0].SchemaSource == "" || decoded.Warnings[0].Hint == "" {
		t.Fatalf("json warnings = %+v", decoded.Warnings)
	}
	validateResultSchema(t, data)

	data, err = FormatJSON(Result{})
	if err != nil {
		t.Fatalf("FormatJSON empty: %v", err)
	}
	text = string(data)
	for _, expected := range []string{`"files": []`, `"issues": []`, `"ignoredIssues": []`, `"warnings": []`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("empty json missing %q:\n%s", expected, text)
		}
	}
	validateResultSchema(t, data)
}

func validateResultSchema(t *testing.T, data []byte) {
	t.Helper()
	schemaPath, err := filepath.Abs("../../schemas/dollarlint-result.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schemaURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(schemaPath)}).String()
	schema, err := jsonschema.NewCompiler().Compile(schemaURI)
	if err != nil {
		t.Fatalf("compile result schema: %v", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("result json does not match schema: %v\n%s", err, string(data))
	}
}

func TestTextHelpers(t *testing.T) {
	if plural(1) != "" || plural(2) != "s" {
		t.Fatalf("plural mismatch")
	}
	if fallback("", "x") != "x" || fallback("y", "x") != "y" {
		t.Fatalf("fallback mismatch")
	}
	if issueLocation(Issue{Line: 4}, OutputConfig{Locations: true}) != "4" {
		t.Fatalf("line-only location mismatch")
	}
	if issueLocation(Issue{}, OutputConfig{}) != "/" {
		t.Fatalf("fallback issue location mismatch")
	}
	if widths := issueColumnWidths([]Issue{{Message: "unknown"}}, OutputConfig{}); widths.Keyword != len("keyword") {
		t.Fatalf("fallback keyword width should stay at header width, got %+v", widths)
	}
	if styledCell(textStyleMuted, "x", 0) != textStyleMuted.Render("x") {
		t.Fatalf("unstyled-width cell mismatch")
	}
	cases := map[time.Duration]string{
		0:                                    "0s",
		500 * time.Microsecond:               "500µs",
		1234 * time.Microsecond:              "1ms",
		12345 * time.Millisecond:             "12.35s",
		75*time.Second + 12*time.Millisecond: "1m15s",
	}
	for duration, expected := range cases {
		if got := formatElapsed(duration); got != expected {
			t.Fatalf("formatElapsed(%v) = %q, want %q", duration, got, expected)
		}
	}
}

func textFixtureResult() Result {
	return Result{
		Summary: Summary{Discovered: 4, Validated: 3, Skipped: 1, Issues: IssueSummary{Total: 5, Validation: 5}, Duration: NewDuration(123 * time.Millisecond), DurationNanos: int64(123 * time.Millisecond)},
		Files: []FileResult{
			{RelativePath: "skipped.json", Status: StatusSkipped},
		},
		Issues: []Issue{
			{
				RelativePath:     "a.json",
				Schema:           "file:///schema.json",
				Keyword:          "minimum",
				KeywordLocation:  "/minimum",
				Property:         "count",
				InstanceLocation: "/count",
				Line:             3,
				Column:           10,
				Message:          "must be >= 1",
			},
			{
				RelativePath:     "a.json",
				Schema:           "file:///schema.json",
				Keyword:          "type",
				KeywordLocation:  "/type",
				Property:         "name",
				InstanceLocation: "/name",
				Line:             2,
				Column:           7,
				Message:          "expected string, received number",
			},
			{
				RelativePath:     "b.toml",
				Keyword:          "type",
				InstanceLocation: "/enabled",
				Line:             4,
				Column:           11,
				Message:          "expected boolean, received string",
			},
			{
				RelativePath:     "b.toml",
				Keyword:          "enum",
				InstanceLocation: "/mode",
				Line:             4,
				Column:           12,
				Message:          "value must be one of test, prod",
			},
			{
				RelativePath:     "b.toml",
				Keyword:          "required",
				InstanceLocation: "/",
				Line:             4,
				Column:           12,
				Message:          "must have required property \"name\"",
			},
			{
				RelativePath:     "ignored.json",
				Keyword:          "required",
				InstanceLocation: "/",
				Message:          "ignored",
				Ignored:          true,
			},
		},
	}
}

func assertContains(t *testing.T, value, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("expected %q in:\n%s", substring, value)
	}
}
