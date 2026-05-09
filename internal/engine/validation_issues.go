package engine

import (
	"errors"
	"fmt"
	"math/big"
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
			issues = append(issues, issue)
		}
	default:
		issues = append(issues, base)
	}
	return issues
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
	}
	return err.BasicOutput().Error.String()
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
	return Issue{
		File:         file.Path,
		RelativePath: file.RelativePath,
		Schema:       schema,
		Keyword:      keyword,
		Message:      err.Error(),
	}
}

func issueForMissingSchemaCoverage(document *Document) Issue {
	return Issue{
		File:         document.Path,
		RelativePath: document.RelativePath,
		Keyword:      "schemaCoverage",
		Message:      "file must declare a schema or match a configured schema association, built-in association, or catalog entry",
	}
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
		})
	}
	return issues
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
	result.Summary.Issues++
}
