package engine

import (
	"encoding/json"
	"sort"
)

const sarifVersion = "2.1.0"

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	ShortDescription sarifMessage      `json:"shortDescription,omitempty"`
	Properties       sarifRuleProperty `json:"properties,omitempty"`
}

type sarifRuleProperty struct {
	Keyword string `json:"keyword,omitempty"`
}

type sarifResult struct {
	RuleID       string             `json:"ruleId"`
	Level        string             `json:"level"`
	Message      sarifMessage       `json:"message"`
	Locations    []sarifLocation    `json:"locations,omitempty"`
	Properties   sarifProperties    `json:"properties,omitempty"`
	Suppressions []sarifSuppression `json:"suppressions,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}

type sarifProperties struct {
	Schema           string `json:"schema,omitempty"`
	Keyword          string `json:"keyword,omitempty"`
	KeywordLocation  string `json:"keywordLocation,omitempty"`
	Property         string `json:"property,omitempty"`
	InstanceLocation string `json:"instanceLocation,omitempty"`
	WarningKind      string `json:"warningKind,omitempty"`
	WarningSource    string `json:"warningSource,omitempty"`
}

type sarifSuppression struct {
	Kind          string      `json:"kind"`
	Justification string      `json:"justification,omitempty"`
	Status        string      `json:"status,omitempty"`
	Properties    sarifIgnore `json:"properties,omitempty"`
}

type sarifIgnore struct {
	Ignored bool `json:"ignored,omitempty"`
}

func FormatSARIF(result Result) ([]byte, error) {
	log := sarifLog{
		Version: sarifVersion,
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "dollarlint",
				InformationURI: "https://github.com/dollarlint/dollarlint",
				Rules:          sarifRules(result),
			}},
			Results: sarifResults(result),
		}},
	}
	return json.MarshalIndent(log, "", "  ")
}

func sarifRules(result Result) []sarifRule {
	seen := map[string]Issue{}
	for _, issue := range result.Issues {
		ruleID := sarifRuleID(issue)
		if _, ok := seen[ruleID]; !ok {
			seen[ruleID] = issue
		}
	}
	warnings := map[string]Warning{}
	for _, warning := range result.Warnings {
		ruleID := sarifWarningRuleID(warning)
		if _, ok := warnings[ruleID]; !ok {
			warnings[ruleID] = warning
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	for id := range warnings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rules := make([]sarifRule, 0, len(ids))
	for _, id := range ids {
		if warning, ok := warnings[id]; ok {
			rules = append(rules, sarifRule{
				ID:   id,
				Name: id,
				ShortDescription: sarifMessage{
					Text: sarifWarningDescription(warning),
				},
			})
			continue
		}
		issue := seen[id]
		rules = append(rules, sarifRule{
			ID:   id,
			Name: id,
			ShortDescription: sarifMessage{
				Text: sarifRuleDescription(issue),
			},
			Properties: sarifRuleProperty{Keyword: issue.Keyword},
		})
	}
	return rules
}

func sarifResults(result Result) []sarifResult {
	results := make([]sarifResult, 0, len(result.Issues)+len(result.Warnings))
	for _, issue := range result.Issues {
		result := sarifResult{
			RuleID:  sarifRuleID(issue),
			Level:   sarifLevel(issue),
			Message: sarifMessage{Text: issue.Message},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: issue.RelativePath},
					Region:           sarifIssueRegion(issue),
				},
			}},
			Properties: sarifProperties{
				Schema:           issue.Schema,
				Keyword:          issue.Keyword,
				KeywordLocation:  issue.KeywordLocation,
				Property:         issue.Property,
				InstanceLocation: issue.InstanceLocation,
			},
		}
		if issue.Ignored {
			result.Suppressions = []sarifSuppression{{
				Kind:          "inSource",
				Justification: issue.IgnoreReason,
				Status:        "accepted",
				Properties:    sarifIgnore{Ignored: true},
			}}
		}
		results = append(results, result)
	}
	for _, warning := range result.Warnings {
		results = append(results, sarifResult{
			RuleID:  sarifWarningRuleID(warning),
			Level:   "warning",
			Message: sarifMessage{Text: warning.Message},
			Properties: sarifProperties{
				WarningKind:   warning.Kind,
				WarningSource: warning.Source,
			},
		})
	}
	return results
}

func sarifRuleID(issue Issue) string {
	if issue.Keyword != "" {
		return issue.Keyword
	}
	return "dollarlint"
}

func sarifRuleDescription(issue Issue) string {
	if issue.Keyword != "" {
		return "JSON Schema " + issue.Keyword + " validation"
	}
	return "dollarlint issue"
}

func sarifLevel(issue Issue) string {
	if issue.Ignored {
		return "none"
	}
	return "error"
}

func sarifWarningRuleID(warning Warning) string {
	if warning.Kind != "" {
		return "dollarlint.warning." + warning.Kind
	}
	return "dollarlint.warning"
}

func sarifWarningDescription(warning Warning) string {
	if warning.Source != "" {
		return "dollarlint " + warning.Source + " warning"
	}
	return "dollarlint warning"
}

func sarifIssueRegion(issue Issue) *sarifRegion {
	if issue.Line <= 0 {
		return nil
	}
	region := &sarifRegion{StartLine: issue.Line}
	if issue.Column > 0 {
		region.StartColumn = issue.Column
	}
	return region
}
