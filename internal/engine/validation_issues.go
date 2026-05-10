package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

func issuesFromSchemaError(document *Document, err error, output OutputConfig) []Issue {
	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) {
		return issuesFromValidationErrorWithOutput(document, validationErr, output)
	}
	return []Issue{{
		File:         document.Path,
		RelativePath: document.RelativePath,
		Schema:       document.Schema,
		Keyword:      issueKeywordSchema,
		Message:      err.Error(),
	}}
}

func issuesFromValidationError(document *Document, err *jsonschema.ValidationError) []Issue {
	return issuesFromValidationErrorWithOutput(document, err, OutputConfig{BranchErrors: BranchErrorsBest})
}

func issuesFromValidationErrorWithOutput(document *Document, err *jsonschema.ValidationError, output OutputConfig) []Issue {
	mode, modeErr := branchErrorMode(output)
	if modeErr != nil {
		mode = BranchErrorsBest
	}
	if mode == BranchErrorsAll {
		return collectAllValidationIssues(document, err, nil)
	}
	return collectValidationIssues(document, err, nil)
}

func collectAllValidationIssues(document *Document, err *jsonschema.ValidationError, issues []Issue) []Issue {
	if len(err.Causes) > 0 {
		for _, cause := range err.Causes {
			issues = collectAllValidationIssues(document, cause, issues)
		}
		return issues
	}
	return append(issues, leafValidationIssues(document, err)...)
}

func collectValidationIssues(document *Document, err *jsonschema.ValidationError, issues []Issue) []Issue {
	return append(issues, compactValidationIssues(document, err)...)
}

func compactValidationIssues(document *Document, err *jsonschema.ValidationError) []Issue {
	if len(err.Causes) > 0 {
		if isChoiceValidationError(err) {
			return bestChoiceIssues(document, err)
		}
		var issues []Issue
		for _, cause := range err.Causes {
			issues = append(issues, compactValidationIssues(document, cause)...)
		}
		return dedupeIssues(issues)
	}
	return leafValidationIssues(document, err)
}

func leafValidationIssues(document *Document, err *jsonschema.ValidationError) []Issue {
	base := Issue{
		File:             document.Path,
		RelativePath:     document.RelativePath,
		Schema:           document.Schema,
		SchemaSource:     document.SchemaSource,
		SchemaMatch:      document.SchemaMatch,
		Keyword:          keywordName(err),
		KeywordLocation:  keywordLocation(err),
		Property:         propertyName(err),
		InstanceLocation: instanceLocation(err.InstanceLocation),
		Message:          validationMessage(err),
	}
	applyIssuePosition(document, &base)
	var issues []Issue
	switch typed := err.ErrorKind.(type) {
	case *kind.Required:
		for _, missing := range typed.Missing {
			issue := base
			issue.Property = missing
			issue.Message = fmt.Sprintf("must have required property %q", missing)
			applyValidationIssueHint(document, &issue)
			issues = append(issues, issue)
		}
	case *kind.AdditionalProperties:
		for _, property := range typed.Properties {
			issue := base
			issue.Property = property
			issue.InstanceLocation = joinPointer(issue.InstanceLocation, property)
			issue.Line = 0
			issue.Column = 0
			applyIssuePosition(document, &issue)
			issue.Message = fmt.Sprintf("must not have additional property %q", property)
			applyValidationIssueHint(document, &issue)
			issues = append(issues, issue)
		}
	default:
		applyValidationIssueHint(document, &base)
		issues = append(issues, base)
	}
	return issues
}

func applyValidationIssueHint(document *Document, issue *Issue) {
	if issue.Hint != "" {
		return
	}
	if isMkDocsInheritedRequiredIssue(document, *issue) {
		issue.Hint = "This MkDocs config declares top-level INHERIT; MkDocs merges inherited settings before use, so validate the rendered config or add a narrow ignore rule for the inherited required property."
		return
	}
	if hint := catalogValidationIssueHint(document, *issue); hint != "" {
		issue.Hint = hint
	}
}

func catalogValidationIssueHint(document *Document, issue Issue) string {
	if document == nil || !isCatalogSchemaSource(document.SchemaSource) {
		return ""
	}
	if issue.Keyword == "enum" {
		return appendCatalogMatchHint(document,
			fmt.Sprintf("This issue came from %s; if this value is valid for the repo's tool version, the external catalog schema may be stale, version-mismatched, or incomplete.", document.SchemaSource))
	}
	return appendCatalogMatchHint(document,
		fmt.Sprintf("This issue came from %s; confirm the external catalog schema matches this repo's tool version and conventions before treating it as a config bug.", document.SchemaSource))
}

func appendCatalogMatchHint(document *Document, hint string) string {
	if document == nil || document.SchemaMatch == nil || document.SchemaMatch.Action != SchemaMatchActionMatched {
		return hint
	}
	if document.SchemaMatch.Reason != "" {
		hint += " Matched because " + document.SchemaMatch.Reason + "."
	}
	if document.SchemaMatch.SuggestedAssociation != "" {
		hint += "\nSuggested explicit association:\n" + document.SchemaMatch.SuggestedAssociation
	}
	return hint
}

func isMkDocsInheritedRequiredIssue(document *Document, issue Issue) bool {
	if document == nil || issue.Keyword != "required" || issue.Property == "" {
		return false
	}
	source := strings.ToLower(document.SchemaSource)
	schema := strings.ToLower(document.Schema)
	if !strings.Contains(source, "mkdocs") && !strings.Contains(schema, "mkdocs") {
		return false
	}
	root, ok := document.Data.(map[string]any)
	if !ok {
		return false
	}
	_, ok = root["INHERIT"]
	return ok
}

func isChoiceValidationError(err *jsonschema.ValidationError) bool {
	switch err.ErrorKind.(type) {
	case *kind.OneOf, *kind.AnyOf:
		return true
	default:
		return false
	}
}

type choiceIssueScore struct {
	discriminatorFailures int
	contextTypeFailures   int
	maxDepth              int
	issues                int
}

func bestChoiceIssues(document *Document, err *jsonschema.ValidationError) []Issue {
	if len(err.Causes) == 0 {
		return leafValidationIssues(document, err)
	}
	context := instanceLocation(err.InstanceLocation)
	var best []Issue
	var bestScore choiceIssueScore
	for i, cause := range err.Causes {
		issues := compactValidationIssues(document, cause)
		score := scoreChoiceIssues(context, issues)
		if i == 0 || betterChoiceScore(score, bestScore) {
			best = issues
			bestScore = score
		}
	}
	return dedupeIssues(best)
}

func scoreChoiceIssues(context string, issues []Issue) choiceIssueScore {
	score := choiceIssueScore{issues: len(issues)}
	for _, issue := range issues {
		score.maxDepth = max(score.maxDepth, pointerDepth(issue.InstanceLocation))
		if isImmediateDiscriminatorIssue(context, issue) {
			score.discriminatorFailures++
		}
		if issue.Keyword == "type" && normalizePointer(issue.InstanceLocation) == normalizePointer(context) {
			score.contextTypeFailures++
		}
	}
	return score
}

func betterChoiceScore(left, right choiceIssueScore) bool {
	if left.discriminatorFailures != right.discriminatorFailures {
		return left.discriminatorFailures < right.discriminatorFailures
	}
	if left.contextTypeFailures != right.contextTypeFailures {
		return left.contextTypeFailures < right.contextTypeFailures
	}
	if left.maxDepth != right.maxDepth {
		return left.maxDepth > right.maxDepth
	}
	return left.issues < right.issues
}

func isImmediateDiscriminatorIssue(context string, issue Issue) bool {
	switch issue.Property {
	case "type", "apiVersion", "kind":
	default:
		return false
	}
	if issue.Keyword != "enum" && issue.Keyword != "const" {
		return false
	}
	return normalizePointer(issue.InstanceLocation) == joinPointer(context, issue.Property)
}

func pointerDepth(pointer string) int {
	pointer = strings.Trim(normalizePointer(pointer), "/")
	if pointer == "" {
		return 0
	}
	return len(strings.Split(pointer, "/"))
}

func dedupeIssues(issues []Issue) []Issue {
	seen := map[string]bool{}
	deduped := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		key := issueDedupeKey(issue)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, issue)
	}
	return deduped
}

func issueDedupeKey(issue Issue) string {
	return strings.Join([]string{
		issue.Keyword,
		issue.KeywordLocation,
		issue.Property,
		issue.InstanceLocation,
		issue.Message,
	}, "\x00")
}

func applyIssuePosition(document *Document, issue *Issue) {
	if issue.Line > 0 && issue.Column > 0 {
		return
	}
	pointer := issue.InstanceLocation
	if pointer == "" {
		pointer = "/"
	}
	if pos, ok := document.SourceMap.Position(pointer); ok {
		issue.Line = pos.Line
		issue.Column = pos.Column
	}
}

func keywordName(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return ""
	}
	keywordPath := err.ErrorKind.KeywordPath()
	if len(keywordPath) == 0 {
		return ""
	}
	return keywordPath[0]
}

func keywordLocation(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return ""
	}
	return "/" + strings.Join(err.ErrorKind.KeywordPath(), "/")
}

func propertyName(err *jsonschema.ValidationError) string {
	if name := propertyFromKind(err.ErrorKind); name != "" {
		return name
	}
	if len(err.InstanceLocation) == 0 {
		return ""
	}
	return err.InstanceLocation[len(err.InstanceLocation)-1]
}

func propertyFromKind(errorKind jsonschema.ErrorKind) string {
	switch typed := errorKind.(type) {
	case *kind.PropertyNames:
		return typed.Property
	case *kind.Dependency:
		return typed.Prop
	case *kind.DependentRequired:
		return typed.Prop
	default:
		return ""
	}
}

func instanceLocation(parts []string) string {
	if len(parts) == 0 {
		return "/"
	}
	return "/" + path.Join(parts...)
}

func validationMessage(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return "validation failed"
	}
	switch typed := err.ErrorKind.(type) {
	case *kind.Type:
		return fmt.Sprintf("expected %s, received %s", strings.Join(typed.Want, " or "), typed.Got)
	case *kind.MinProperties:
		return fmt.Sprintf("must have at least %d %s", typed.Want, countNoun(typed.Want, "property", "properties"))
	case *kind.MaxProperties:
		return fmt.Sprintf("must have at most %d %s", typed.Want, countNoun(typed.Want, "property", "properties"))
	case *kind.MinItems:
		return fmt.Sprintf("must have at least %d %s", typed.Want, countNoun(typed.Want, "item", "items"))
	case *kind.MaxItems:
		return fmt.Sprintf("must have at most %d %s", typed.Want, countNoun(typed.Want, "item", "items"))
	case *kind.MinLength:
		return fmt.Sprintf("must be at least %d characters", typed.Want)
	case *kind.MaxLength:
		return fmt.Sprintf("must be at most %d characters", typed.Want)
	case *kind.Minimum:
		return fmt.Sprintf("must be >= %s", ratString(typed.Want))
	case *kind.Maximum:
		return fmt.Sprintf("must be <= %s", ratString(typed.Want))
	case *kind.ExclusiveMinimum:
		return fmt.Sprintf("must be > %s", ratString(typed.Want))
	case *kind.ExclusiveMaximum:
		return fmt.Sprintf("must be < %s", ratString(typed.Want))
	case *kind.MultipleOf:
		return fmt.Sprintf("must be a multiple of %s", ratString(typed.Want))
	case *kind.Enum:
		return enumValidationMessage(typed)
	}
	return err.BasicOutput().Error.String()
}

func enumValidationMessage(typed *kind.Enum) string {
	if typed == nil {
		return "value does not match any allowed enum value"
	}
	got := displayValidationValue(typed.Got)
	switch len(typed.Want) {
	case 0:
		return fmt.Sprintf("value %s does not match any allowed enum value", got)
	case 1:
		return fmt.Sprintf("value %s must be %s", got, displayValidationValue(typed.Want[0]))
	}
	if len(typed.Want) <= 10 {
		return fmt.Sprintf("value %s must be one of %s", got, joinedValidationValues(typed.Want))
	}
	return fmt.Sprintf("value %s is not one of %d allowed values (examples: %s)", got, len(typed.Want), summarizedValidationValues(typed.Want))
}

func joinedValidationValues(values []any) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, displayValidationValue(value))
	}
	return strings.Join(out, ", ")
}

func summarizedValidationValues(values []any) string {
	const head = 5
	const tail = 2
	if len(values) <= head+tail {
		return joinedValidationValues(values)
	}
	parts := make([]string, 0, head+tail+1)
	for _, value := range values[:head] {
		parts = append(parts, displayValidationValue(value))
	}
	parts = append(parts, "...")
	for _, value := range values[len(values)-tail:] {
		parts = append(parts, displayValidationValue(value))
	}
	return strings.Join(parts, ", ")
}

func displayValidationValue(value any) string {
	switch typed := value.(type) {
	case string:
		return fmt.Sprintf("%q", typed)
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		data, err := json.Marshal(typed)
		if err == nil {
			return string(data)
		}
	}
	return fmt.Sprintf("%v", value)
}

func countNoun(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func ratString(value *big.Rat) string {
	if value == nil {
		return "0"
	}
	if value.IsInt() {
		return value.Num().String()
	}
	return strings.TrimRight(strings.TrimRight(value.FloatString(6), "0"), ".")
}

func issueForError(file DiscoveredFile, schema, keyword string, err error) Issue {
	message := err.Error()
	return Issue{
		File:         file.Path,
		RelativePath: file.RelativePath,
		Schema:       schema,
		Keyword:      keyword,
		Message:      message,
		Hint:         hintForIssue(file, keyword, message),
	}
}

func issueForMissingSchemaCoverage(document *Document) Issue {
	issue := Issue{
		File:         document.Path,
		RelativePath: document.RelativePath,
		SchemaMatch:  document.SchemaMatch,
		Keyword:      "schemaCoverage",
		Message:      "file must declare a schema or match a configured schema association, built-in association, or catalog entry",
	}
	if document.SchemaMatch != nil && document.SchemaMatch.Reason != "" {
		issue.Hint = document.SchemaMatch.Reason
		if document.SchemaMatch.SuggestedAssociation != "" {
			issue.Hint += "\nSuggested explicit association:\n" + document.SchemaMatch.SuggestedAssociation
		}
		if document.SchemaMatch.SuggestedCatalogIgnore != "" {
			issue.Hint += "\nSuggested catalog ignore:\n" + document.SchemaMatch.SuggestedCatalogIgnore
		}
	}
	return issue
}

func issuesForDocumentParseErrors(document *Document) []Issue {
	if document == nil || len(document.ParseErrors) == 0 {
		return nil
	}
	issues := make([]Issue, 0, len(document.ParseErrors))
	for _, parseErr := range document.ParseErrors {
		issues = append(issues, Issue{
			File:         document.Path,
			RelativePath: document.RelativePath,
			Keyword:      issueKeywordParse,
			Line:         parseErr.Line,
			Column:       parseErr.Column,
			Message:      parseErr.Message,
			Hint: hintForIssue(DiscoveredFile{
				Path:         document.Path,
				RelativePath: document.RelativePath,
			}, issueKeywordParse, parseErr.Message),
		})
	}
	return issues
}

func hintForIssue(file DiscoveredFile, keyword, message string) string {
	if keyword != issueKeywordParse {
		return ""
	}
	lower := strings.ToLower(message)
	ext := strings.ToLower(path.Ext(file.RelativePath))
	if ext == "" {
		ext = strings.ToLower(path.Ext(file.Path))
	}
	if strings.HasSuffix(strings.ToLower(file.RelativePath), ".baseline.jsonc") ||
		strings.Contains(strings.ToLower(file.RelativePath), "/tests/baselines/") {
		return "This looks like a test baseline artifact rather than a standalone JSON document; exclude generated or baseline fixtures."
	}
	if strings.Contains(lower, "multiple json values") ||
		strings.Contains(lower, "after top-level value") {
		return "This file contains content after the first JSON value; use .jsonl/.ndjson for line-delimited data, or exclude generated/test fixtures."
	}
	if strings.Contains(lower, "unexpected eof") || lower == "eof" || strings.HasSuffix(lower, ": eof") {
		if fileIsEmpty(file.Path) {
			return "Empty JSON files are not valid documents; add a value such as {} or exclude placeholder fixtures."
		}
		return "The document ends before a complete value is parsed; check for an unfinished object, array, string, or comment."
	}
	if (ext == ".yaml" || ext == ".yml") && fileContainsToken(file.Path, "<%") {
		return "This YAML file appears to contain template tags; exclude it or validate the rendered YAML instead."
	}
	if (ext == ".yaml" || ext == ".yml") && strings.Contains(lower, "mapping key") && strings.Contains(lower, "already defined") {
		return "This YAML parser rejects duplicate mapping keys; exclude deliberately invalid fixtures or remove the duplicate key."
	}
	if ext == ".json" && (strings.Contains(lower, "invalid literal") || strings.Contains(lower, "looking for beginning of value")) {
		return "This file is not valid JSON/JSONC despite its .json extension; rename it, exclude it, or use .jsonl/.ndjson for record streams."
	}
	if ext == ".toml" {
		return "This TOML file is syntactically invalid; exclude it if it is an intentionally broken test fixture."
	}
	return ""
}

func fileIsEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() == 0
}

func fileContainsToken(path, token string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	buffer := make([]byte, 8192)
	n, _ := file.Read(buffer)
	return bytes.Contains(buffer[:n], []byte(token))
}

func addWarning(result *Result, warning Warning) {
	result.Warnings = append(result.Warnings, warning)
	result.Summary.Warnings++
}

func addIssue(result *Result, issue Issue) {
	result.Issues = append(result.Issues, issue)
	if issue.Ignored {
		result.Summary.Ignored++
		return
	}
	addIssueSummary(&result.Summary.Issues, issue)
}

func addIssueSummary(summary *IssueSummary, issue Issue) {
	summary.Total++
	switch issueCategory(issue) {
	case "parsing":
		summary.Parsing++
	case "schema":
		summary.Schema++
	case "coverage":
		summary.Coverage++
	default:
		summary.Validation++
	}
}

func issueCategory(issue Issue) string {
	switch issue.Keyword {
	case issueKeywordParse:
		return "parsing"
	case issueKeywordSchema:
		return "schema"
	case "schemaCoverage":
		return "coverage"
	default:
		return "validation"
	}
}
