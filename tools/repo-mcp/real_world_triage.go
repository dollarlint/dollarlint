package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type realWorldTriageArgs struct {
	Title                  string                           `json:"title"`
	CorpusDir              string                           `json:"corpusDir"`
	CacheDir               string                           `json:"cacheDir"`
	Command                string                           `json:"command"`
	OutputArtifact         string                           `json:"outputArtifact"`
	ManifestPath           string                           `json:"manifestPath"`
	Repositories           []realWorldRepository            `json:"repositories"`
	DependencyPrep         []realWorldDependencyPrep        `json:"dependencyPrep"`
	ProductRecommendations []realWorldProductRecommendation `json:"productRecommendations"`
	ProductDecisions       []string                         `json:"productDecisions"`
	FollowUp               []string                         `json:"followUp"`
}

type realWorldOutputDetails struct {
	Summary  *realWorldResult       `json:"summary,omitempty"`
	Issues   []realWorldOutputIssue `json:"issues,omitempty"`
	Warnings []map[string]any       `json:"warnings,omitempty"`
	Files    []realWorldOutputFile  `json:"files,omitempty"`
}

type realWorldOutputIssue struct {
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
}

type realWorldOutputFile struct {
	Path         string `json:"path"`
	Format       string `json:"format,omitempty"`
	Schema       string `json:"schema,omitempty"`
	SchemaSource string `json:"schemaSource,omitempty"`
	Status       string `json:"status"`
	Issues       int    `json:"issues,omitempty"`
	Ignored      int    `json:"ignored,omitempty"`
	Message      string `json:"message,omitempty"`
}

type realWorldTriage struct {
	OutputArtifact         string                           `json:"outputArtifact"`
	Summary                *realWorldResult                 `json:"summary,omitempty"`
	PerRepository          []realWorldRepositoryTriage      `json:"perRepository"`
	IssueGroups            []realWorldIssueGroup            `json:"issueGroups"`
	WarningGroups          []realWorldWarningGroup          `json:"warningGroups"`
	Findings               []string                         `json:"findings"`
	ProductRecommendations []realWorldProductRecommendation `json:"productRecommendations"`
	ProductDecisions       []string                         `json:"productDecisions"`
	FollowUp               []string                         `json:"followUp"`
	FinalResponseContract  map[string]any                   `json:"finalResponseContract"`
}

type realWorldRepositoryTriage struct {
	Repository              string                `json:"repository"`
	Path                    string                `json:"path,omitempty"`
	CloneURL                string                `json:"cloneURL,omitempty"`
	Discovered              int                   `json:"discovered"`
	Validated               int                   `json:"validated"`
	Skipped                 int                   `json:"skipped"`
	Failed                  int                   `json:"failed"`
	IssueCount              int                   `json:"issueCount"`
	FixtureIssueCount       int                   `json:"fixtureIssueCount"`
	ProductSignalIssueCount int                   `json:"productSignalIssueCount"`
	WarningCount            int                   `json:"warningCount"`
	IssueCategories         realWorldIssueSummary `json:"issueCategories"`
	StatusCounts            map[string]int        `json:"statusCounts,omitempty"`
}

type realWorldIssueGroup struct {
	Repository    string   `json:"repository"`
	Category      string   `json:"category"`
	Signal        string   `json:"signal"`
	SchemaSource  string   `json:"schemaSource,omitempty"`
	Keyword       string   `json:"keyword,omitempty"`
	Count         int      `json:"count"`
	Paths         []string `json:"paths,omitempty"`
	Messages      []string `json:"messages,omitempty"`
	ProductSignal bool     `json:"productSignal"`
	FixtureSignal bool     `json:"fixtureSignal"`
}

type realWorldWarningGroup struct {
	Kind         string   `json:"kind"`
	Schema       string   `json:"schema,omitempty"`
	SchemaSource string   `json:"schemaSource,omitempty"`
	Count        int      `json:"count"`
	Paths        []string `json:"paths,omitempty"`
	Messages     []string `json:"messages,omitempty"`
}

type realWorldRepoIdentity struct {
	Name     string
	Path     string
	CloneURL string
}

func (s *repoServer) handleRealWorldTriageOutput(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldTriageArgs
	_ = request.BindArguments(&args)

	manifestPath := args.ManifestPath
	if manifestPath == "" && args.CorpusDir != "" {
		manifestPath = filepath.Join(args.CorpusDir, realWorldManifestName)
	}
	repositories := append([]realWorldRepository{}, args.Repositories...)
	if manifestPath != "" {
		manifest, err := readRealWorldManifest(manifestPath)
		if err == nil {
			if args.Title == "" {
				args.Title = manifest.Title
			}
			if args.CorpusDir == "" {
				args.CorpusDir = manifest.CorpusDir
			}
			if args.CacheDir == "" {
				args.CacheDir = manifest.CacheDir
			}
			if args.OutputArtifact == "" {
				args.OutputArtifact = manifest.OutputArtifact
			}
			if len(repositories) == 0 {
				repositories = manifest.Repositories
			}
		}
	}
	if args.OutputArtifact == "" {
		return mcp.NewToolResultError("outputArtifact is required; call real_world_finish_validation first or pass manifestPath"), nil
	}
	if args.Command == "" && args.CorpusDir != "" && args.CacheDir != "" {
		args.Command = realWorldValidationCommand(args.CorpusDir, args.CacheDir, args.OutputArtifact, true, "warn", 1, "1ms", "1ms", nil)
	}

	triage, err := triageRealWorldOutput(args.OutputArtifact, repositories)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(args.ProductRecommendations) > 0 {
		triage.ProductRecommendations = args.ProductRecommendations
	}
	if len(args.ProductDecisions) > 0 {
		triage.ProductDecisions = args.ProductDecisions
	}
	if len(args.FollowUp) > 0 {
		triage.FollowUp = args.FollowUp
	}

	draftRecord := map[string]any{
		"title":                  args.Title,
		"corpus":                 args.CorpusDir,
		"cacheDir":               args.CacheDir,
		"command":                args.Command,
		"outputArtifact":         args.OutputArtifact,
		"manifestPath":           manifestPath,
		"repositories":           repositories,
		"dependencyPrep":         args.DependencyPrep,
		"findings":               triage.Findings,
		"productRecommendations": triage.ProductRecommendations,
		"productDecisions":       triage.ProductDecisions,
		"followUp":               triage.FollowUp,
	}
	missingArgs := realWorldTriageDraftRecordMissing(args, repositories)
	return structured(map[string]any{
		"ok":                    true,
		"outputArtifact":        args.OutputArtifact,
		"manifestPath":          manifestPath,
		"summary":               triage.Summary,
		"perRepository":         triage.PerRepository,
		"issueGroups":           triage.IssueGroups,
		"warningGroups":         triage.WarningGroups,
		"draftRecord":           draftRecord,
		"draftRecordMissing":    missingArgs,
		"finalResponseContract": triage.FinalResponseContract,
		"nextStep":              realWorldNextRecordTriagedResult(draftRecord, missingArgs),
	})
}

func triageRealWorldOutput(outputArtifact string, repositories []realWorldRepository) (realWorldTriage, error) {
	details, err := readRealWorldOutputDetails(outputArtifact)
	if err != nil {
		return realWorldTriage{}, err
	}
	if err := validateRealWorldOutputDetails(outputArtifact, details); err != nil {
		return realWorldTriage{}, err
	}
	repoIndex := realWorldRepositoryIndex(repositories)
	perRepo := realWorldPerRepositoryTriage(details, repoIndex)
	issueGroups := realWorldGroupIssues(details.Issues, repoIndex)
	warningGroups := realWorldGroupWarnings(details.Warnings)
	findings := realWorldDraftFindings(details, perRepo, issueGroups, warningGroups)
	productRecommendations := realWorldDraftProductRecommendations(details, issueGroups, warningGroups)
	productDecisions := []string{"No product changes were made during this sweep; record the triage and decide follow-up from the structured result."}
	followUp := realWorldDraftFollowUp(details, productRecommendations)
	return realWorldTriage{
		OutputArtifact:         outputArtifact,
		Summary:                details.Summary,
		PerRepository:          perRepo,
		IssueGroups:            issueGroups,
		WarningGroups:          warningGroups,
		Findings:               findings,
		ProductRecommendations: productRecommendations,
		ProductDecisions:       productDecisions,
		FollowUp:               followUp,
		FinalResponseContract:  realWorldFinalResponseContract(),
	}, nil
}

func readRealWorldOutputDetails(path string) (realWorldOutputDetails, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return realWorldOutputDetails{}, err
	}
	var payload struct {
		Summary *struct {
			Discovered    int                   `json:"discovered"`
			Validated     int                   `json:"validated"`
			Skipped       int                   `json:"skipped"`
			Failed        int                   `json:"failed"`
			Issues        realWorldIssueSummary `json:"issues"`
			Ignored       int                   `json:"ignored"`
			Warnings      int                   `json:"warnings"`
			DurationNanos int64                 `json:"durationNanos"`
		} `json:"summary"`
		Issues   []realWorldOutputIssue `json:"issues"`
		Warnings []map[string]any       `json:"warnings"`
		Files    []realWorldOutputFile  `json:"files"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return realWorldOutputDetails{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if payload.Summary == nil {
		return realWorldOutputDetails{}, fmt.Errorf("parse %s: missing summary", path)
	}
	summary := &realWorldResult{
		Discovered: payload.Summary.Discovered,
		Validated:  payload.Summary.Validated,
		Skipped:    payload.Summary.Skipped,
		Failed:     payload.Summary.Failed,
		Issues:     payload.Summary.Issues,
		Ignored:    payload.Summary.Ignored,
		Warnings:   payload.Summary.Warnings,
	}
	if payload.Summary.DurationNanos > 0 {
		summary.Duration = &realWorldDurationInfo{Nanos: payload.Summary.DurationNanos}
	}
	return realWorldOutputDetails{
		Summary:  summary,
		Issues:   payload.Issues,
		Warnings: payload.Warnings,
		Files:    payload.Files,
	}, nil
}

func validateRealWorldOutputDetails(path string, details realWorldOutputDetails) error {
	if details.Summary == nil {
		return fmt.Errorf("triage %s: missing summary", path)
	}
	if len(details.Files) > 0 && details.Summary.Discovered != len(details.Files) {
		return fmt.Errorf("triage %s: summary.discovered=%d but files has %d entries", path, details.Summary.Discovered, len(details.Files))
	}
	if details.Summary.Issues.Total != len(details.Issues) {
		return fmt.Errorf("triage %s: summary.issues.total=%d but issues has %d entries", path, details.Summary.Issues.Total, len(details.Issues))
	}
	if details.Summary.Warnings != len(details.Warnings) {
		return fmt.Errorf("triage %s: summary.warnings=%d but warnings has %d entries", path, details.Summary.Warnings, len(details.Warnings))
	}
	return nil
}

func realWorldPerRepositoryTriage(details realWorldOutputDetails, repoIndex map[string]realWorldRepoIdentity) []realWorldRepositoryTriage {
	repos := map[string]*realWorldRepositoryTriage{}
	for _, file := range details.Files {
		repo := realWorldRepoForPath(file.Path, repoIndex)
		item := realWorldEnsureRepoTriage(repos, repo, repoIndex)
		item.Discovered++
		if item.StatusCounts == nil {
			item.StatusCounts = map[string]int{}
		}
		item.StatusCounts[file.Status]++
		switch file.Status {
		case "validated":
			item.Validated++
		case "skipped":
			item.Skipped++
		case "failed", "error":
			item.Failed++
		}
	}
	for _, issue := range details.Issues {
		repo := realWorldRepoForPath(issue.Path, repoIndex)
		item := realWorldEnsureRepoTriage(repos, repo, repoIndex)
		item.IssueCount++
		switch issue.Category {
		case "parsing":
			item.IssueCategories.Parsing++
		case "validation":
			item.IssueCategories.Validation++
		case "schema":
			item.IssueCategories.Schema++
		case "coverage":
			item.IssueCategories.Coverage++
		}
		item.IssueCategories.Total++
		if realWorldIsFixturePath(issue.Path) {
			item.FixtureIssueCount++
		}
		if realWorldIssueIsProductSignal(issue) {
			item.ProductSignalIssueCount++
		}
	}
	for _, warning := range details.Warnings {
		repo := realWorldRepoForPath(realWorldMapString(warning, "path"), repoIndex)
		item := realWorldEnsureRepoTriage(repos, repo, repoIndex)
		item.WarningCount++
	}
	out := make([]realWorldRepositoryTriage, 0, len(repos))
	for _, item := range repos {
		if len(item.StatusCounts) == 0 {
			item.StatusCounts = nil
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IssueCount != out[j].IssueCount {
			return out[i].IssueCount > out[j].IssueCount
		}
		return out[i].Repository < out[j].Repository
	})
	return out
}

func realWorldGroupIssues(issues []realWorldOutputIssue, repoIndex map[string]realWorldRepoIdentity) []realWorldIssueGroup {
	groups := map[string]*realWorldIssueGroup{}
	for _, issue := range issues {
		repo := realWorldRepoForPath(issue.Path, repoIndex)
		signal := realWorldIssueSignal(issue)
		key := strings.Join([]string{repo, issue.Category, signal, issue.SchemaSource, issue.Keyword}, "\x00")
		group := groups[key]
		if group == nil {
			group = &realWorldIssueGroup{
				Repository:    repo,
				Category:      issue.Category,
				Signal:        signal,
				SchemaSource:  issue.SchemaSource,
				Keyword:       issue.Keyword,
				ProductSignal: realWorldIssueIsProductSignal(issue),
				FixtureSignal: realWorldIsFixturePath(issue.Path),
			}
			groups[key] = group
		}
		group.Count++
		group.Paths = appendUniqueLimit(group.Paths, issue.Path, 8)
		group.Messages = appendUniqueLimit(group.Messages, issue.Message, 4)
	}
	out := make([]realWorldIssueGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.Join([]string{out[i].Repository, out[i].Signal, out[i].Category, out[i].SchemaSource, out[i].Keyword}, "\x00") <
			strings.Join([]string{out[j].Repository, out[j].Signal, out[j].Category, out[j].SchemaSource, out[j].Keyword}, "\x00")
	})
	return out
}

func realWorldGroupWarnings(warnings []map[string]any) []realWorldWarningGroup {
	groups := map[string]*realWorldWarningGroup{}
	for _, warning := range warnings {
		kind := realWorldMapString(warning, "kind")
		schema := realWorldMapString(warning, "schema")
		schemaSource := realWorldMapString(warning, "schemaSource")
		key := strings.Join([]string{kind, schema, schemaSource}, "\x00")
		group := groups[key]
		if group == nil {
			group = &realWorldWarningGroup{Kind: kind, Schema: schema, SchemaSource: schemaSource}
			groups[key] = group
		}
		group.Count++
		group.Paths = appendUniqueLimit(group.Paths, realWorldMapString(warning, "path"), 8)
		group.Messages = appendUniqueLimit(group.Messages, realWorldMapString(warning, "message"), 3)
	}
	out := make([]realWorldWarningGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.Join([]string{out[i].Kind, out[i].SchemaSource, out[i].Schema}, "\x00") <
			strings.Join([]string{out[j].Kind, out[j].SchemaSource, out[j].Schema}, "\x00")
	})
	return out
}

func realWorldDraftFindings(details realWorldOutputDetails, perRepo []realWorldRepositoryTriage, issueGroups []realWorldIssueGroup, warningGroups []realWorldWarningGroup) []string {
	summary := details.Summary
	findings := []string{
		fmt.Sprintf("DollarLint discovered %d files, validated %d, skipped %d, failed %d, reported %d issues, and emitted %d warnings.",
			summary.Discovered, summary.Validated, summary.Skipped, summary.Failed, summary.Issues.Total, summary.Warnings),
	}
	if len(perRepo) > 0 {
		var parts []string
		for _, repo := range perRepo {
			if repo.Repository == "corpus-root" && repo.IssueCount == 0 {
				continue
			}
			if repo.IssueCount > 0 || repo.WarningCount > 0 {
				parts = append(parts, fmt.Sprintf("%s: %d issues, %d warnings, %d skipped", repo.Repository, repo.IssueCount, repo.WarningCount, repo.Skipped))
			}
		}
		if len(parts) > 0 {
			findings = append(findings, "Repository concentration: "+strings.Join(parts, "; ")+".")
		}
	}
	if len(issueGroups) > 0 {
		var parts []string
		for _, group := range issueGroups {
			parts = append(parts, fmt.Sprintf("%s %s/%s: %d", group.Repository, group.Signal, nonEmpty(group.Keyword, group.Category), group.Count))
			if len(parts) == 4 {
				break
			}
		}
		findings = append(findings, "Issue groups: "+strings.Join(parts, "; ")+".")
	}
	if len(warningGroups) > 0 {
		var parts []string
		for _, group := range warningGroups {
			parts = append(parts, fmt.Sprintf("%s %s: %d", nonEmpty(group.Kind, "warning"), nonEmpty(group.SchemaSource, group.Schema), group.Count))
			if len(parts) == 3 {
				break
			}
		}
		findings = append(findings, "Warning groups: "+strings.Join(parts, "; ")+".")
	}
	if summary.Discovered > 0 && summary.Skipped > 0 {
		findings = append(findings, fmt.Sprintf("Skipped coverage: %d of %d discovered files were skipped (%s).", summary.Skipped, summary.Discovered, percentString(summary.Skipped, summary.Discovered)))
	}
	if summary.Issues.Total == 0 && summary.Warnings == 0 && summary.Failed == 0 {
		findings = append(findings, "No crashes, output-contract failures, validation issues, or warnings were observed.")
	}
	return findings
}

func realWorldDraftProductRecommendations(details realWorldOutputDetails, issueGroups []realWorldIssueGroup, warningGroups []realWorldWarningGroup) []realWorldProductRecommendation {
	var recommendations []realWorldProductRecommendation
	if len(warningGroups) > 0 {
		recommendations = append(recommendations, realWorldProductRecommendation{
			Strength:       "low",
			Recommendation: "Consider summarizing repeated catalog schema compile warnings by schema/source in real-world output and workflow summaries.",
			Rationale:      "The sweep emitted schema catalog warnings that are not file findings; grouping them keeps the user focused on product signals without hiding schema availability problems.",
		})
	}
	if details.Summary != nil && details.Summary.Discovered > 0 && details.Summary.Skipped*100/details.Summary.Discovered >= 10 {
		recommendations = append(recommendations, realWorldProductRecommendation{
			Strength:       "low",
			Recommendation: "Consider surfacing skipped-file coverage by repository and reason in the MCP triage output.",
			Rationale:      fmt.Sprintf("This sweep skipped %d of %d discovered files (%s), which is useful context when judging corpus coverage.", details.Summary.Skipped, details.Summary.Discovered, percentString(details.Summary.Skipped, details.Summary.Discovered)),
		})
	}
	for _, group := range issueGroups {
		if group.Signal == "parser-compatibility" && group.ProductSignal {
			recommendations = append(recommendations, realWorldProductRecommendation{
				Strength:       "med",
				Recommendation: "Investigate non-fixture parser compatibility failures from the real-world corpus.",
				Rationale:      "A parse failure outside fixture/test data is likely to affect normal users validating repository configuration files.",
			})
			break
		}
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, realWorldProductRecommendation{
			Strength:       "low",
			Recommendation: "No product change is recommended from this sweep.",
			Rationale:      "The triage did not find crashes, output-contract failures, repeated confusing warnings, or non-fixture product behavior signals.",
		})
	}
	return recommendations
}

func realWorldDraftFollowUp(details realWorldOutputDetails, recommendations []realWorldProductRecommendation) []string {
	for _, recommendation := range recommendations {
		if !strings.HasPrefix(strings.ToLower(recommendation.Recommendation), "no product change") {
			return []string{
				"Review the structured productRecommendations and decide whether to open implementation work.",
				"Continue the next real-world sweep with fresh repositories rather than retesting the same corpus by default.",
			}
		}
	}
	if details.Summary != nil && details.Summary.Issues.Total == 0 && details.Summary.Warnings == 0 {
		return []string{"Continue the next real-world sweep with fresh repositories."}
	}
	return []string{"Keep this structured result for comparison with the next sweep."}
}

func realWorldFinalResponseContract() map[string]any {
	return map[string]any{
		"required":                 "The agent's final message to the user must choose exactly one of these outcomes: productChangesToConsider or productBehavedReasonably.",
		"productChangesToConsider": "Use this when the sweep found concrete product changes worth considering; list each recommendation with strength and rationale grounded in the MCP triage/record result.",
		"productBehavedReasonably": "Use this when no genuine product change is recommended; say the product behaved reasonably and briefly explain the observed evidence.",
		"doNot": []string{
			"Do not end with only raw counts or a generic run summary.",
			"Do not create or update Markdown report files for repository memory.",
			"Do not publish a GitHub Discussion unless the run is inside a GitHub Agentic Workflow.",
		},
	}
}

func realWorldTriageDraftRecordMissing(args realWorldTriageArgs, repositories []realWorldRepository) []string {
	var missing []string
	if args.Title == "" {
		missing = append(missing, "title")
	}
	if args.CorpusDir == "" {
		missing = append(missing, "corpus")
	}
	if args.CacheDir == "" {
		missing = append(missing, "cacheDir")
	}
	if args.Command == "" {
		missing = append(missing, "command")
	}
	if args.OutputArtifact == "" {
		missing = append(missing, "outputArtifact")
	}
	if len(repositories) == 0 {
		missing = append(missing, "repositories")
	}
	if len(args.DependencyPrep) == 0 {
		missing = append(missing, "dependencyPrep")
	}
	return missing
}

func realWorldRepositoryIndex(repositories []realWorldRepository) map[string]realWorldRepoIdentity {
	index := map[string]realWorldRepoIdentity{}
	for _, repo := range repositories {
		name := nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path))
		if name == "" {
			continue
		}
		identity := realWorldRepoIdentity{Name: name, Path: repo.Path, CloneURL: repo.CloneURL}
		for _, key := range []string{name, slugify(name), filepath.Base(repo.Path), repoNameFromURL(repo.CloneURL)} {
			key = strings.TrimSpace(key)
			if key != "" {
				index[key] = identity
			}
		}
	}
	return index
}

func realWorldRepoForPath(path string, repoIndex map[string]realWorldRepoIdentity) string {
	clean := strings.Trim(strings.TrimPrefix(filepath.ToSlash(path), "./"), "/")
	if clean == "" {
		return "unknown"
	}
	first, _, hasSlash := strings.Cut(clean, "/")
	if identity, ok := repoIndex[first]; ok && identity.Name != "" {
		return identity.Name
	}
	if !hasSlash {
		return "corpus-root"
	}
	return first
}

func realWorldEnsureRepoTriage(repos map[string]*realWorldRepositoryTriage, repo string, repoIndex map[string]realWorldRepoIdentity) *realWorldRepositoryTriage {
	item := repos[repo]
	if item != nil {
		return item
	}
	identity := repoIndex[repo]
	item = &realWorldRepositoryTriage{
		Repository: repo,
		Path:       identity.Path,
		CloneURL:   identity.CloneURL,
	}
	repos[repo] = item
	return item
}

func realWorldIssueSignal(issue realWorldOutputIssue) string {
	if realWorldIsFixturePath(issue.Path) {
		return "fixture-data"
	}
	source := strings.ToLower(issue.SchemaSource)
	if strings.HasPrefix(source, "catalog:schemastore") {
		return "catalog-schema-validation"
	}
	switch issue.Category {
	case "parsing":
		return "parser-compatibility"
	case "schema":
		return "schema-resolution"
	case "coverage":
		return "coverage"
	default:
		return "product-behavior"
	}
}

func realWorldIssueIsProductSignal(issue realWorldOutputIssue) bool {
	return realWorldIssueSignal(issue) != "fixture-data"
}

func realWorldIsFixturePath(path string) bool {
	for _, part := range strings.Split(strings.ToLower(filepath.ToSlash(path)), "/") {
		switch part {
		case "testdata", "fixtures", "fixture", "tests", "test", "__tests__", "spec", "specs", "samples", "sample-data":
			return true
		}
	}
	return false
}

func realWorldMapString(value map[string]any, key string) string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func appendUniqueLimit(values []string, value string, limit int) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	if limit > 0 && len(values) >= limit {
		return values
	}
	return append(values, value)
}

func percentString(part, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(part)*100/float64(total))
}
