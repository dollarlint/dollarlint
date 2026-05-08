package engine

import (
	"encoding/json"
	"testing"
)

func TestFormatSARIF(t *testing.T) {
	result := Result{
		Issues: []Issue{
			{
				RelativePath:     "config.json",
				Schema:           "https://example.com/schema.json",
				Keyword:          "type",
				KeywordLocation:  "/properties/name/type",
				Property:         "name",
				InstanceLocation: "/name",
				Line:             3,
				Column:           12,
				Message:          "got number, want string",
			},
			{
				RelativePath:     "legacy.json",
				Keyword:          "required",
				InstanceLocation: "/",
				Line:             1,
				Message:          "missing property \"name\"",
				Ignored:          true,
				IgnoreReason:     "legacy",
			},
			{
				RelativePath: "schema.json",
				Message:      "compile schema failed",
			},
		},
		Warnings: []Warning{{Kind: "schemaStoreCatalogUnavailable", Source: "schemastore", Message: "catalog unavailable"}},
	}
	data, err := FormatSARIF(result)
	if err != nil {
		t.Fatalf("FormatSARIF: %v", err)
	}
	var log sarifLog
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatalf("invalid sarif json: %v", err)
	}
	if log.Version != sarifVersion || len(log.Runs) != 1 {
		t.Fatalf("log = %+v", log)
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "dollarlint" || len(run.Tool.Driver.Rules) != 4 || len(run.Results) != 4 {
		t.Fatalf("run = %+v", run)
	}
	if run.Results[0].RuleID != "type" || run.Results[0].Level != "error" {
		t.Fatalf("first result = %+v", run.Results[0])
	}
	region := run.Results[0].Locations[0].PhysicalLocation.Region
	if region == nil || region.StartLine != 3 || region.StartColumn != 12 {
		t.Fatalf("region = %+v", region)
	}
	if run.Results[1].Level != "none" || len(run.Results[1].Suppressions) != 1 {
		t.Fatalf("ignored result = %+v", run.Results[1])
	}
	if run.Results[2].RuleID != "dollarlint" || run.Results[2].Locations[0].PhysicalLocation.Region != nil {
		t.Fatalf("file-level result = %+v", run.Results[2])
	}
	if run.Results[3].RuleID != "dollarlint.warning.schemaStoreCatalogUnavailable" || run.Results[3].Level != "warning" || len(run.Results[3].Locations) != 0 {
		t.Fatalf("warning result = %+v", run.Results[3])
	}
	if sarifRuleDescription(Issue{}) != "dollarlint issue" {
		t.Fatalf("default rule description mismatch")
	}
	if sarifWarningRuleID(Warning{}) != "dollarlint.warning" || sarifWarningDescription(Warning{}) != "dollarlint warning" {
		t.Fatalf("default warning helpers mismatch")
	}
	if sarifIssueRegion(Issue{Line: 2}).StartColumn != 0 {
		t.Fatalf("column should be omitted when unknown")
	}
}
