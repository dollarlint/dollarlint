package engine

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
)

type jsonResult struct {
	Schema        string           `json:"$schema"`
	FormatVersion int              `json:"formatVersion"`
	Root          string           `json:"root"`
	Summary       jsonSummary      `json:"summary"`
	Files         []jsonFileResult `json:"files"`
	Issues        []jsonIssue      `json:"issues"`
	IgnoredIssues []jsonIssue      `json:"ignoredIssues"`
	Warnings      []jsonWarning    `json:"warnings"`
}

type jsonSummary struct {
	Discovered    int          `json:"discovered"`
	Validated     int          `json:"validated"`
	Skipped       int          `json:"skipped"`
	Failed        int          `json:"failed"`
	Issues        IssueSummary `json:"issues"`
	Ignored       int          `json:"ignored"`
	Warnings      int          `json:"warnings"`
	DurationNanos int64        `json:"durationNanos"`
}

type jsonFileResult struct {
	Path         string `json:"path"`
	Format       string `json:"format"`
	Schema       string `json:"schema,omitempty"`
	SchemaSource string `json:"schemaSource,omitempty"`
	Status       string `json:"status"`
	Issues       int    `json:"issues"`
	Ignored      int    `json:"ignored"`
	Message      string `json:"message,omitempty"`
}

type jsonIssue struct {
	Path             string `json:"path"`
	Schema           string `json:"schema,omitempty"`
	SchemaSource     string `json:"schemaSource,omitempty"`
	Category         string `json:"category"`
	Keyword          string `json:"keyword,omitempty"`
	KeywordLocation  string `json:"keywordLocation,omitempty"`
	Property         string `json:"property,omitempty"`
	InstanceLocation string `json:"instanceLocation,omitempty"`
	Line             int    `json:"line,omitempty"`
	Column           int    `json:"column,omitempty"`
	Message          string `json:"message"`
	Hint             string `json:"hint,omitempty"`
	IgnoreReason     string `json:"ignoreReason,omitempty"`
}

type jsonWarning struct {
	Kind         string `json:"kind"`
	Source       string `json:"source,omitempty"`
	Path         string `json:"path,omitempty"`
	Schema       string `json:"schema,omitempty"`
	SchemaSource string `json:"schemaSource,omitempty"`
	Message      string `json:"message"`
	Hint         string `json:"hint,omitempty"`
}

func (result Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(newJSONResult(result))
}

func newJSONResult(result Result) jsonResult {
	root := schemaDisplayRoot(result.Root)
	files := make([]jsonFileResult, 0, len(result.Files))
	schemaSources := map[string]string{}
	for _, file := range result.Files {
		path := resultPath(file.RelativePath, file.Path)
		if file.SchemaSource != "" {
			schemaSources[path] = file.SchemaSource
		}
		files = append(files, jsonFileResult{
			Path:         path,
			Format:       file.Format,
			Schema:       displaySchema(file.Schema, root),
			SchemaSource: file.SchemaSource,
			Status:       file.Status,
			Issues:       file.Issues,
			Ignored:      file.Ignored,
			Message:      file.Message,
		})
	}

	issues := make([]jsonIssue, 0, len(result.Issues))
	ignoredIssues := make([]jsonIssue, 0)
	for _, issue := range result.Issues {
		out := newJSONIssue(issue, schemaSources, root)
		if issue.Ignored {
			ignoredIssues = append(ignoredIssues, out)
			continue
		}
		issues = append(issues, out)
	}

	warnings := make([]jsonWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, jsonWarning{
			Kind:         warning.Kind,
			Source:       warning.Source,
			Path:         warning.Path,
			Schema:       displaySchema(warning.Schema, root),
			SchemaSource: warning.SchemaSource,
			Message:      warning.Message,
			Hint:         warning.Hint,
		})
	}

	return jsonResult{
		Schema:        JSONResultSchema,
		FormatVersion: JSONFormatVersion,
		Root:          result.Root,
		Summary:       newJSONSummary(result.Summary),
		Files:         files,
		Issues:        issues,
		IgnoredIssues: ignoredIssues,
		Warnings:      warnings,
	}
}

func newJSONSummary(summary Summary) jsonSummary {
	return jsonSummary{
		Discovered:    summary.Discovered,
		Validated:     summary.Validated,
		Skipped:       summary.Skipped,
		Failed:        summary.Failed,
		Issues:        summary.Issues,
		Ignored:       summary.Ignored,
		Warnings:      summary.Warnings,
		DurationNanos: summaryDurationNanos(summary),
	}
}

func newJSONIssue(issue Issue, schemaSources map[string]string, root string) jsonIssue {
	path := resultPath(issue.RelativePath, issue.File)
	return jsonIssue{
		Path:             path,
		Schema:           displaySchema(issue.Schema, root),
		SchemaSource:     schemaSources[path],
		Category:         issueCategory(issue),
		Keyword:          issue.Keyword,
		KeywordLocation:  issue.KeywordLocation,
		Property:         issue.Property,
		InstanceLocation: issue.InstanceLocation,
		Line:             issue.Line,
		Column:           issue.Column,
		Message:          issue.Message,
		Hint:             issue.Hint,
		IgnoreReason:     issue.IgnoreReason,
	}
}

func resultPath(relativePath, path string) string {
	if relativePath != "" {
		return relativePath
	}
	return path
}

func summaryDurationNanos(summary Summary) int64 {
	if summary.DurationNanos != 0 {
		return summary.DurationNanos
	}
	return summary.Duration.Nanoseconds()
}

func schemaDisplayRoot(root string) string {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(configSearchRoot(root))
	if err != nil {
		return ""
	}
	return abs
}

func displaySchema(schema, root string) string {
	if schema == "" || root == "" {
		return schema
	}
	parsed, err := url.Parse(schema)
	if err != nil || parsed.Scheme != "file" {
		return schema
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return schema
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return schema
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return schema
	}
	rel = filepath.ToSlash(rel)
	if parsed.Fragment != "" {
		rel += "#" + parsed.EscapedFragment()
	}
	return rel
}
