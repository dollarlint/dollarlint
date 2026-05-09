package engine

import (
	"strings"
	"testing"
	"time"
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
