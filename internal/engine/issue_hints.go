package engine

import (
	"fmt"
	"path"
	"strings"
)

type issueHintContext struct {
	Document *Document
	File     DiscoveredFile
	Issue    Issue
}

type issueHintRule func(issueHintContext) (IssueHint, bool)

var validationIssueHintRules = []issueHintRule{
	mkDocsInheritedRequiredIssueHint,
	githubFundingBlankProviderIssueHint,
	githubWorkflowStrategyMatrixIssueHint,
	dependabotReviewersRetiredIssueHint,
	legacyCIEnumIssueHint,
	catalogEnumIssueHint,
	catalogValidationIssueHint,
}

var fileIssueHintRules = []issueHintRule{
	parseBaselineIssueHint,
	parseMultipleJSONValuesIssueHint,
	parseUnexpectedEOFIssueHint,
	parseTemplatedYAMLIssueHint,
	parseDuplicateYAMLKeyIssueHint,
	parseInvalidJSONExtensionIssueHint,
	parseInvalidTOMLIssueHint,
}

func applyValidationIssueHint(document *Document, issue *Issue, output OutputConfig) {
	if issue == nil || issue.Hint != "" || !issueHintsEnabled(output) {
		return
	}
	if hint, ok := classifyValidationIssueHint(document, *issue); ok {
		applyIssueHint(issue, hint)
	}
}

func applyFileIssueHint(file DiscoveredFile, issue *Issue, output OutputConfig) {
	if issue == nil || issue.Hint != "" || !issueHintsEnabled(output) {
		return
	}
	if hint, ok := classifyFileIssueHint(file, *issue); ok {
		applyIssueHint(issue, hint)
	}
}

func classifyValidationIssueHint(document *Document, issue Issue) (IssueHint, bool) {
	ctx := issueHintContext{Document: document, Issue: issue}
	for _, rule := range validationIssueHintRules {
		if hint, ok := rule(ctx); ok {
			return normalizedIssueHint(hint), true
		}
	}
	return IssueHint{}, false
}

func classifyFileIssueHint(file DiscoveredFile, issue Issue) (IssueHint, bool) {
	ctx := issueHintContext{File: file, Issue: issue}
	for _, rule := range fileIssueHintRules {
		if hint, ok := rule(ctx); ok {
			return normalizedIssueHint(hint), true
		}
	}
	return IssueHint{}, false
}

func applyIssueHint(issue *Issue, hint IssueHint) {
	normalized := normalizedIssueHint(hint)
	issue.IssueHint = &normalized
	issue.Hint = issueHintText(normalized, false)
}

func issueHintsEnabled(output OutputConfig) bool {
	mode, err := issueHintsMode(output)
	return err != nil || mode != IssueHintsOff
}

func issueHintsVerbose(output OutputConfig) bool {
	mode, err := issueHintsMode(output)
	return err == nil && mode == IssueHintsVerbose
}

func normalizedIssueHint(hint IssueHint) IssueHint {
	if hint.GroupKey == "" {
		hint.GroupKey = hint.ID
	}
	if hint.Confidence == "" {
		hint.Confidence = IssueHintConfidenceMedium
	}
	return hint
}

func issueHintText(hint IssueHint, verbose bool) string {
	var parts []string
	if hint.Title != "" {
		parts = append(parts, hint.Title)
	}
	if hint.Detail != "" {
		parts = append(parts, hint.Detail)
	}
	if hint.Suggestion != "" {
		parts = append(parts, hint.Suggestion)
	}
	if verbose {
		var metadata []string
		if hint.ID != "" {
			metadata = append(metadata, "rule "+hint.ID)
		}
		if hint.Confidence != "" {
			metadata = append(metadata, "confidence "+hint.Confidence)
		}
		if hint.Source != "" {
			metadata = append(metadata, "source "+hint.Source)
		}
		if len(metadata) > 0 {
			parts = append(parts, strings.Join(metadata, "; "))
		}
	}
	return strings.Join(parts, "\n")
}

func mkDocsInheritedRequiredIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if !isMkDocsInheritedRequiredIssue(ctx.Document, ctx.Issue) {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "mkdocs.inherited-required",
		Title:      "MkDocs inheritance may satisfy this required setting after render.",
		Detail:     "This MkDocs config declares top-level INHERIT; MkDocs merges inherited settings before use.",
		Suggestion: "Validate the rendered config or add a narrow ignore rule for the inherited required property.",
		Confidence: IssueHintConfidenceHigh,
		Source:     "https://www.mkdocs.org/user-guide/configuration/#configuration-inheritance",
		GroupKey:   "mkdocs:inherited-required",
	}, true
}

func githubFundingBlankProviderIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if !schemaSourceOrURIContains(ctx.Document, "github-funding") {
		return IssueHint{}, false
	}
	if ctx.Issue.Property == "" || !githubFundingProvider(ctx.Issue.Property) {
		return IssueHint{}, false
	}
	if !issueLooksBlankProvider(ctx.Issue) {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "github-funding.blank-provider",
		Title:      "GitHub Funding provider entries cannot be blank.",
		Detail:     fmt.Sprintf("The %q provider key is present but has an empty/null value. This often comes from leaving unused template keys in .github/FUNDING.yml.", ctx.Issue.Property),
		Suggestion: "Remove unused provider keys, or fill them with the username, project name, package name, or URL GitHub expects.",
		Confidence: IssueHintConfidenceHigh,
		Source:     "https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/displaying-a-sponsor-button-in-your-repository",
		GroupKey:   "catalog:github-funding:blank-provider",
	}, true
}

func githubWorkflowStrategyMatrixIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if !schemaSourceOrURIContains(ctx.Document, "github-workflow") {
		return IssueHint{}, false
	}
	if ctx.Issue.Keyword != "required" || ctx.Issue.Property != "matrix" {
		return IssueHint{}, false
	}
	if !strings.HasSuffix(normalizePointer(ctx.Issue.InstanceLocation), "/strategy") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "github-workflow.strategy-matrix",
		Title:      "SchemaStore requires matrix whenever a GitHub Actions strategy is present.",
		Detail:     "GitHub documents strategy as the matrix strategy surface, and fail-fast/max-parallel apply to matrix jobs.",
		Suggestion: "If GitHub accepts this workflow without matrix, this may be catalog schema drift; otherwise add matrix or remove the strategy block.",
		Confidence: IssueHintConfidenceMedium,
		Source:     "https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idstrategy",
		GroupKey:   "catalog:github-workflow:strategy-matrix",
	}, true
}

func dependabotReviewersRetiredIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if !schemaSourceOrURIContains(ctx.Document, "dependabot") {
		return IssueHint{}, false
	}
	if ctx.Issue.Keyword != "additionalProperties" || ctx.Issue.Property != "reviewers" {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "dependabot.reviewers-retired",
		Title:      "Dependabot reviewers was retired in favor of CODEOWNERS.",
		Detail:     "GitHub announced the dependabot.yml reviewers option would be removed on May 20, 2025.",
		Suggestion: "Move review assignment to CODEOWNERS, or keep a narrow ignore if this repository intentionally supports legacy Dependabot behavior.",
		Confidence: IssueHintConfidenceHigh,
		Source:     "https://github.blog/changelog/2025-04-29-dependabot-reviewers-configuration-option-being-replaced-by-code-owners/",
		GroupKey:   "catalog:dependabot:reviewers-retired",
	}, true
}

func legacyCIEnumIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != "enum" {
		return IssueHint{}, false
	}
	if !schemaSourceOrURIContains(ctx.Document, "travis") && !schemaSourceOrURIContains(ctx.Document, "circleci") {
		return IssueHint{}, false
	}
	rel := strings.ToLower(ctx.Issue.RelativePath)
	if !strings.Contains(rel, "/vendor/") && !strings.Contains(rel, "/deps/") && !strings.Contains(rel, "/third_party/") && !strings.Contains(rel, "/third-party/") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "ci.legacy-enum",
		Title:      "Legacy CI metadata in dependency/vendor paths can trip modern catalog enum checks.",
		Detail:     "The file is catalog-backed CI configuration under a low-actionability dependency subtree.",
		Suggestion: "Treat this as a legacy/vendor signal first; exclude the subtree or add a narrow ignore unless you actively maintain that CI file.",
		Confidence: IssueHintConfidenceMedium,
		GroupKey:   "catalog:ci:legacy-enum",
	}, true
}

func catalogEnumIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Document == nil || !isCatalogSchemaSource(ctx.Document.SchemaSource) || ctx.Issue.Keyword != "enum" {
		return IssueHint{}, false
	}
	hint := IssueHint{
		ID:         "catalog.enum-drift",
		Title:      fmt.Sprintf("This enum issue came from %s.", ctx.Document.SchemaSource),
		Detail:     "If this value is valid for the repo's tool version, the external catalog schema may be stale, version-mismatched, or incomplete.",
		Suggestion: "Confirm the tool's docs or runtime behavior before treating this as a repository configuration bug.",
		Confidence: IssueHintConfidenceMedium,
		GroupKey:   "catalog:enum-drift:" + ctx.Document.SchemaSource,
	}
	appendCatalogMatchIssueHint(ctx.Document, &hint)
	return hint, true
}

func catalogValidationIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Document == nil || !isCatalogSchemaSource(ctx.Document.SchemaSource) {
		return IssueHint{}, false
	}
	hint := IssueHint{
		ID:         "catalog.validation-context",
		Title:      fmt.Sprintf("This issue came from %s.", ctx.Document.SchemaSource),
		Detail:     "Catalog-backed schemas are inferred from file names and third-party schema metadata.",
		Suggestion: "Confirm the external catalog schema matches this repo's tool version and conventions before treating this as a config bug.",
		Confidence: IssueHintConfidenceMedium,
		GroupKey:   "catalog:validation-context:" + ctx.Document.SchemaSource,
	}
	appendCatalogMatchIssueHint(ctx.Document, &hint)
	return hint, true
}

func appendCatalogMatchIssueHint(document *Document, hint *IssueHint) {
	if document == nil || document.SchemaMatch == nil || document.SchemaMatch.Action != SchemaMatchActionMatched {
		return
	}
	if document.SchemaMatch.Reason != "" {
		hint.Detail = appendHintSentence(hint.Detail, "Matched because "+document.SchemaMatch.Reason+".")
	}
	if document.SchemaMatch.SuggestedAssociation != "" {
		hint.Suggestion = appendHintSentence(hint.Suggestion, "Suggested explicit association:\n"+document.SchemaMatch.SuggestedAssociation)
	}
}

func parseBaselineIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse {
		return IssueHint{}, false
	}
	rel := strings.ToLower(ctx.File.RelativePath)
	if !strings.HasSuffix(rel, ".baseline.jsonc") && !strings.Contains(rel, "/tests/baselines/") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "parse.test-baseline",
		Title:      "This looks like a test baseline artifact rather than a standalone JSON document.",
		Suggestion: "Exclude generated or baseline fixtures if they are not meant to be validated directly.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "parse:test-baseline",
	}, true
}

func parseMultipleJSONValuesIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse {
		return IssueHint{}, false
	}
	lower := strings.ToLower(ctx.Issue.Message)
	if !strings.Contains(lower, "multiple json values") && !strings.Contains(lower, "after top-level value") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "parse.multiple-json-values",
		Title:      "This file contains content after the first JSON value.",
		Suggestion: "Use .jsonl/.ndjson for line-delimited data, or exclude generated/test fixtures.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "parse:multiple-json-values",
	}, true
}

func parseUnexpectedEOFIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse {
		return IssueHint{}, false
	}
	lower := strings.ToLower(ctx.Issue.Message)
	if !strings.Contains(lower, "unexpected eof") && lower != "eof" && !strings.HasSuffix(lower, ": eof") {
		return IssueHint{}, false
	}
	if fileIsEmpty(ctx.File.Path) {
		return IssueHint{
			ID:         "parse.empty-json",
			Title:      "Empty JSON files are not valid documents.",
			Suggestion: "Add a value such as {} or exclude placeholder fixtures.",
			Confidence: IssueHintConfidenceHigh,
			GroupKey:   "parse:empty-json",
		}, true
	}
	return IssueHint{
		ID:         "parse.unexpected-eof",
		Title:      "The document ends before a complete value is parsed.",
		Suggestion: "Check for an unfinished object, array, string, or comment.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "parse:unexpected-eof",
	}, true
}

func parseTemplatedYAMLIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse {
		return IssueHint{}, false
	}
	ext := issueFileExt(ctx.File)
	if ext != ".yaml" && ext != ".yml" {
		return IssueHint{}, false
	}
	if !fileContainsToken(ctx.File.Path, "<%") && !fileContainsToken(ctx.File.Path, "{{") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "parse.templated-yaml",
		Title:      "This YAML file appears to contain template tags.",
		Suggestion: "Exclude it or validate the rendered YAML instead.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "parse:templated-yaml",
	}, true
}

func parseDuplicateYAMLKeyIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse {
		return IssueHint{}, false
	}
	ext := issueFileExt(ctx.File)
	lower := strings.ToLower(ctx.Issue.Message)
	if (ext != ".yaml" && ext != ".yml") || !strings.Contains(lower, "mapping key") || !strings.Contains(lower, "already defined") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "parse.yaml-duplicate-key",
		Title:      "This YAML parser rejects duplicate mapping keys.",
		Suggestion: "Exclude deliberately invalid fixtures or remove the duplicate key.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "parse:yaml-duplicate-key",
	}, true
}

func parseInvalidJSONExtensionIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse || issueFileExt(ctx.File) != ".json" {
		return IssueHint{}, false
	}
	lower := strings.ToLower(ctx.Issue.Message)
	if !strings.Contains(lower, "invalid literal") && !strings.Contains(lower, "looking for beginning of value") {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "parse.invalid-json-extension",
		Title:      "This file is not valid JSON/JSONC despite its .json extension.",
		Suggestion: "Rename it, exclude it, or use .jsonl/.ndjson for record streams.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "parse:invalid-json-extension",
	}, true
}

func parseInvalidTOMLIssueHint(ctx issueHintContext) (IssueHint, bool) {
	if ctx.Issue.Keyword != issueKeywordParse || issueFileExt(ctx.File) != ".toml" {
		return IssueHint{}, false
	}
	return IssueHint{
		ID:         "parse.invalid-toml",
		Title:      "This TOML file is syntactically invalid.",
		Suggestion: "Exclude it if it is an intentionally broken test fixture.",
		Confidence: IssueHintConfidenceMedium,
		GroupKey:   "parse:invalid-toml",
	}, true
}

func schemaSourceOrURIContains(document *Document, needle string) bool {
	if document == nil {
		return false
	}
	needle = strings.ToLower(needle)
	return strings.Contains(strings.ToLower(document.SchemaSource), needle) ||
		strings.Contains(strings.ToLower(document.Schema), needle)
}

func githubFundingProvider(property string) bool {
	switch property {
	case "community_bridge", "github", "issuehunt", "ko_fi", "liberapay", "open_collective", "patreon", "tidelift", "polar", "buy_me_a_coffee", "thanks_dev", "custom":
		return true
	default:
		return false
	}
}

func issueLooksBlankProvider(issue Issue) bool {
	lower := strings.ToLower(issue.Message)
	return strings.Contains(lower, "received null") ||
		strings.Contains(lower, "must be at least 1") ||
		strings.Contains(lower, "minlength") ||
		issue.Keyword == "oneOf" ||
		issue.Keyword == "type" ||
		issue.Keyword == "minLength"
}

func issueFileExt(file DiscoveredFile) string {
	ext := strings.ToLower(path.Ext(file.RelativePath))
	if ext == "" {
		ext = strings.ToLower(path.Ext(file.Path))
	}
	return ext
}

func appendHintSentence(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "\n" + addition
}
