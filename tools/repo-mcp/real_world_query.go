package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type realWorldArtifactQueryArgs struct {
	EntryID        string `json:"entryID"`
	OutputArtifact string `json:"outputArtifact"`
	Repository     string `json:"repository"`
	Focus          string `json:"focus"`
	Recommendation string `json:"recommendation"`
	Limit          int    `json:"limit"`
}

type realWorldRecommendationBacklogArgs struct {
	Limit           int    `json:"limit"`
	MinOccurrences  int    `json:"minOccurrences"`
	IncludeNoChange bool   `json:"includeNoChange"`
	Topic           string `json:"topic"`
}

type realWorldArtifactQueryTarget struct {
	Entry        *realWorldEntry
	ArtifactPath string
}

type realWorldRecommendationBacklogCluster struct {
	Key                         string                                  `json:"key"`
	Title                       string                                  `json:"title"`
	Count                       int                                     `json:"count"`
	HighestStrength             string                                  `json:"highestStrength"`
	Strengths                   map[string]int                          `json:"strengths"`
	Entries                     []string                                `json:"entries"`
	Repositories                []string                                `json:"repositories,omitempty"`
	Examples                    []realWorldRecommendationBacklogExample `json:"examples"`
	SuggestedRegressionFixtures []string                                `json:"suggestedRegressionFixtures,omitempty"`
	NextAction                  string                                  `json:"nextAction,omitempty"`
}

type realWorldRecommendationBacklogExample struct {
	EntryID        string `json:"entryID"`
	EntryTitle     string `json:"entryTitle,omitempty"`
	Date           string `json:"date,omitempty"`
	Repository     string `json:"repository,omitempty"`
	Outcome        string `json:"outcome,omitempty"`
	Strength       string `json:"strength"`
	Recommendation string `json:"recommendation"`
	Rationale      string `json:"rationale"`
	EntryPath      string `json:"entryPath,omitempty"`
	ArtifactPath   string `json:"artifactPath,omitempty"`
}

func (s *repoServer) handleRealWorldArtifactQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldArtifactQueryArgs
	_ = request.BindArguments(&args)
	out, err := s.queryRealWorldArtifact(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) handleRealWorldRecommendationBacklog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldRecommendationBacklogArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldRecommendationBacklog(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) queryRealWorldArtifact(args realWorldArtifactQueryArgs) (map[string]any, error) {
	limit := boundedRealWorldQueryLimit(args.Limit)
	target, err := s.resolveRealWorldArtifactQueryTarget(args)
	if err != nil {
		return nil, err
	}
	details, err := readRealWorldOutputDetails(target.ArtifactPath)
	if err != nil {
		return nil, err
	}
	repositories := []realWorldRepository{}
	entrySummary := map[string]any{}
	if target.Entry != nil {
		repositories = target.Entry.Repositories
		entrySummary = realWorldArtifactQueryEntrySummary(s.root, *target.Entry)
	}
	repoIndex := realWorldRepositoryIndex(repositories)
	filteredDetails := details
	if strings.TrimSpace(args.Repository) != "" {
		filteredDetails = filterRealWorldOutputDetailsByRepository(details, repoIndex, args.Repository)
	}
	perRepo := realWorldPerRepositoryTriage(filteredDetails, repoIndex)
	issueGroups := realWorldGroupIssues(filteredDetails.Issues, repoIndex)
	warningGroups := realWorldGroupWarnings(filteredDetails.Warnings)
	skippedSignals := realWorldSkippedFileSignals(filteredDetails.Files)
	skippedGroups := realWorldSkippedFileGroups(skippedSignals)
	issueExamples := realWorldIssueExamples(filteredDetails, limit)
	focus := nonEmpty(args.Focus, "all")
	out := map[string]any{
		"ok":             true,
		"entry":          entrySummary,
		"outputArtifact": displayRealWorldArtifactPath(s.root, target.ArtifactPath),
		"query": map[string]any{
			"repository":     args.Repository,
			"focus":          focus,
			"recommendation": args.Recommendation,
			"limit":          limit,
		},
		"summary":     filteredDetails.Summary,
		"uxSignals":   realWorldUXSignals(filteredDetails, issueExamples, skippedGroups),
		"outcomeHint": realWorldEvidenceOutcomeHint(filteredDetails.Summary, issueGroups, warningGroups, skippedGroups, issueExamples),
		"nextStep":    "Use these grouped artifact details as evidence for product recommendations, team discussion, or follow-up implementation work.",
	}
	if focus == "all" || focus == "overview" {
		out["perRepository"] = firstRepositoryTriage(perRepo, limit)
	}
	if focus == "all" || focus == "issues" || focus == "recommendation" {
		out["issueGroups"] = firstIssueGroups(issueGroups, limit)
		out["exampleIssues"] = issueExamples
	}
	if focus == "all" || focus == "warnings" || focus == "recommendation" {
		out["warningGroups"] = firstWarningGroups(warningGroups, limit)
	}
	if focus == "all" || focus == "skipped" || focus == "recommendation" {
		out["skippedGroups"] = firstSkippedGroups(skippedGroups, limit)
		out["skippedCoverageByRepository"] = realWorldSkippedCoverageByRepository(filteredDetails, repoIndex, limit)
	}
	if focus == "all" || focus == "cli" {
		out["cliPreview"] = realWorldCLIPreview(filteredDetails.Styled)
	}
	if strings.TrimSpace(args.Recommendation) != "" || focus == "recommendation" {
		out["recommendationExamples"] = realWorldRecommendationExamples(filteredDetails, repoIndex, args.Recommendation, limit)
	}
	return out, nil
}

func (s *repoServer) resolveRealWorldArtifactQueryTarget(args realWorldArtifactQueryArgs) (realWorldArtifactQueryTarget, error) {
	if strings.TrimSpace(args.OutputArtifact) != "" {
		path, err := resolveRealWorldArtifactPath(s.root, args.OutputArtifact)
		return realWorldArtifactQueryTarget{ArtifactPath: path}, err
	}
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return realWorldArtifactQueryTarget{}, err
	}
	if strings.TrimSpace(args.EntryID) != "" {
		for i := range history.Entries {
			if history.Entries[i].ID == args.EntryID {
				path, err := realWorldEntryArtifactPath(s.root, history.Entries[i])
				return realWorldArtifactQueryTarget{Entry: &history.Entries[i], ArtifactPath: path}, err
			}
		}
		return realWorldArtifactQueryTarget{}, fmt.Errorf("real-world entry %q was not found", args.EntryID)
	}
	for i := len(history.Entries) - 1; i >= 0; i-- {
		path, err := realWorldEntryArtifactPath(s.root, history.Entries[i])
		if err == nil {
			return realWorldArtifactQueryTarget{Entry: &history.Entries[i], ArtifactPath: path}, nil
		}
	}
	return realWorldArtifactQueryTarget{}, fmt.Errorf("no recorded real-world entry has a readable persisted output artifact")
}

func resolveRealWorldArtifactPath(root, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("outputArtifact is required")
	}
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	clean, err := cleanRelativePath(raw)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, clean), nil
}

func realWorldEntryArtifactPath(root string, entry realWorldEntry) (string, error) {
	candidates := []string{entry.PersistedOutputArtifact, entry.OutputArtifact}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		path, err := resolveRealWorldArtifactPath(root, candidate)
		if err != nil {
			continue
		}
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("real-world entry %q has no readable output artifact", entry.ID)
}

func realWorldArtifactQueryEntrySummary(root string, entry realWorldEntry) map[string]any {
	out := map[string]any{
		"id":        entry.ID,
		"title":     entry.Title,
		"date":      entry.Date,
		"repoCount": len(entry.Repositories),
		"entryPath": realWorldEntryRelPath(entry),
	}
	if entry.PersistedOutputArtifact != "" {
		out["persistedOutputArtifact"] = entry.PersistedOutputArtifact
	}
	return out
}

func displayRealWorldArtifactPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return path
}

func filterRealWorldOutputDetailsByRepository(details realWorldOutputDetails, repoIndex map[string]realWorldRepoIdentity, repository string) realWorldOutputDetails {
	repository = strings.ToLower(strings.TrimSpace(repository))
	if repository == "" {
		return details
	}
	matchesRepo := func(path string) bool {
		repo := strings.ToLower(realWorldRepoForPath(path, repoIndex))
		return repo == repository || slugify(repo) == slugify(repository)
	}
	filtered := realWorldOutputDetails{Root: details.Root, Styled: details.Styled}
	for _, file := range details.Files {
		if matchesRepo(file.Path) {
			filtered.Files = append(filtered.Files, file)
		}
	}
	for _, issue := range details.Issues {
		if matchesRepo(issue.Path) {
			filtered.Issues = append(filtered.Issues, issue)
		}
	}
	for _, warning := range details.Warnings {
		if matchesRepo(realWorldMapString(warning, "path")) {
			filtered.Warnings = append(filtered.Warnings, warning)
		}
	}
	filtered.Summary = realWorldSummaryForDetails(filtered)
	return filtered
}

func realWorldSummaryForDetails(details realWorldOutputDetails) *realWorldResult {
	summary := &realWorldResult{
		Discovered: len(details.Files),
		Warnings:   len(details.Warnings),
		Issues:     realWorldIssueSummary{Total: len(details.Issues)},
	}
	for _, file := range details.Files {
		switch file.Status {
		case "validated":
			summary.Validated++
		case "skipped":
			summary.Skipped++
		case "failed", "error":
			summary.Failed++
		}
		summary.Ignored += file.Ignored
	}
	for _, issue := range details.Issues {
		switch issue.Category {
		case "parsing":
			summary.Issues.Parsing++
		case "validation":
			summary.Issues.Validation++
		case "schema":
			summary.Issues.Schema++
		case "coverage":
			summary.Issues.Coverage++
		}
	}
	return summary
}

func firstRepositoryTriage(items []realWorldRepositoryTriage, limit int) []realWorldRepositoryTriage {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func firstSkippedGroups(groups []realWorldSkippedFileGroup, limit int) []realWorldSkippedFileGroup {
	if limit <= 0 || len(groups) <= limit {
		return groups
	}
	return groups[:limit]
}

func realWorldSkippedCoverageByRepository(details realWorldOutputDetails, repoIndex map[string]realWorldRepoIdentity, limit int) []map[string]any {
	type coverage struct {
		repository string
		discovered int
		skipped    int
		classes    map[string]int
	}
	byRepo := map[string]*coverage{}
	for _, file := range details.Files {
		repo := realWorldRepoForPath(file.Path, repoIndex)
		item := byRepo[repo]
		if item == nil {
			item = &coverage{repository: repo, classes: map[string]int{}}
			byRepo[repo] = item
		}
		item.discovered++
		if file.Status == "skipped" {
			class, _, _ := realWorldClassifySkippedFile(file)
			item.skipped++
			item.classes[class]++
		}
	}
	items := make([]*coverage, 0, len(byRepo))
	for _, item := range byRepo {
		if item.skipped > 0 {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].skipped != items[j].skipped {
			return items[i].skipped > items[j].skipped
		}
		return items[i].repository < items[j].repository
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"repository": item.repository,
			"discovered": item.discovered,
			"skipped":    item.skipped,
			"ratio":      percentString(item.skipped, item.discovered),
			"classes":    item.classes,
		})
	}
	return out
}

func realWorldRecommendationExamples(details realWorldOutputDetails, repoIndex map[string]realWorldRepoIdentity, recommendation string, limit int) map[string]any {
	terms := realWorldRecommendationTerms(recommendation)
	issueGroups := realWorldGroupIssues(details.Issues, repoIndex)
	warningGroups := realWorldGroupWarnings(details.Warnings)
	skippedGroups := realWorldSkippedFileGroups(realWorldSkippedFileSignals(details.Files))
	matchedIssueGroups := filterIssueGroupsForTerms(issueGroups, terms, limit)
	matchedWarningGroups := filterWarningGroupsForTerms(warningGroups, terms, limit)
	matchedSkippedGroups := filterSkippedGroupsForTerms(skippedGroups, terms, limit)
	matchedDetails := details
	matchedDetails.Issues = filterIssuesForTerms(details.Issues, terms, limit)
	examples := realWorldIssueExamples(matchedDetails, limit)
	if len(examples) == 0 && len(details.Issues) > 0 {
		examples = realWorldIssueExamples(details, min(limit, 3))
	}
	return map[string]any{
		"query":         recommendation,
		"matchedTerms":  terms,
		"issueGroups":   matchedIssueGroups,
		"exampleIssues": examples,
		"warningGroups": matchedWarningGroups,
		"skippedGroups": matchedSkippedGroups,
	}
}

func realWorldRecommendationTerms(text string) []string {
	lower := strings.ToLower(text)
	var terms []string
	for _, token := range strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_')
	}) {
		if len(token) < 4 || realWorldRecommendationStopWord(token) {
			continue
		}
		terms = appendUniqueLimit(terms, token, 20)
	}
	specials := map[string][]string{
		"funding":    {"funding.yml", "github funding"},
		"workflow":   {".github/workflows", "github workflow", "matrix", "strategy"},
		"matrix":     {".github/workflows", "github workflow", "strategy"},
		"dependabot": {"dependabot.yml", "reviewers"},
		"helm":       {"chart/templates", "templates", "helm"},
		"template":   {"templates", "chart/templates"},
		"basename":   {"watch.json", "cluster.json", "catalog"},
		"generic":    {"watch.json", "cluster.json", "catalog"},
		"schema":     {"schema", "schemaSource", "catalog"},
		"warning":    {"warning", "schemaCatalogSchemaUnavailable"},
		"skipped":    {"skipped", "skipReason", "skipClass"},
	}
	for needle, additions := range specials {
		if strings.Contains(lower, needle) {
			for _, addition := range additions {
				terms = appendUniqueLimit(terms, strings.ToLower(addition), 20)
			}
		}
	}
	if len(terms) == 0 && strings.TrimSpace(text) != "" {
		terms = append(terms, strings.ToLower(strings.TrimSpace(text)))
	}
	return terms
}

func realWorldRecommendationStopWord(token string) bool {
	switch token {
	case "with", "from", "that", "this", "when", "into", "they", "their", "would", "should", "could", "consider", "product", "recommendation", "rationale", "validation", "dollarlint":
		return true
	default:
		return false
	}
}

func filterIssuesForTerms(issues []realWorldOutputIssue, terms []string, limit int) []realWorldOutputIssue {
	var out []realWorldOutputIssue
	for _, issue := range issues {
		if realWorldTermsMatch(realWorldIssueHaystack(issue), terms) {
			out = append(out, issue)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func filterIssueGroupsForTerms(groups []realWorldIssueGroup, terms []string, limit int) []realWorldIssueGroup {
	var out []realWorldIssueGroup
	for _, group := range groups {
		haystack := strings.ToLower(strings.Join(append(append([]string{group.Repository, group.Category, group.Signal, group.SchemaSource, group.Keyword}, group.Paths...), group.Messages...), " "))
		if realWorldTermsMatch(haystack, terms) {
			out = append(out, group)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func filterWarningGroupsForTerms(groups []realWorldWarningGroup, terms []string, limit int) []realWorldWarningGroup {
	var out []realWorldWarningGroup
	for _, group := range groups {
		haystack := strings.ToLower(strings.Join(append(append([]string{group.Kind, group.Schema, group.SchemaSource}, group.Paths...), group.Messages...), " "))
		if realWorldTermsMatch(haystack, terms) {
			out = append(out, group)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func filterSkippedGroupsForTerms(groups []realWorldSkippedFileGroup, terms []string, limit int) []realWorldSkippedFileGroup {
	var out []realWorldSkippedFileGroup
	for _, group := range groups {
		haystack := strings.ToLower(strings.Join(append(append([]string{group.Class}, group.Reasons...), group.Examples...), " "))
		if realWorldTermsMatch(haystack, terms) {
			out = append(out, group)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func realWorldIssueHaystack(issue realWorldOutputIssue) string {
	return strings.ToLower(strings.Join([]string{
		issue.Path,
		issue.Schema,
		issue.SchemaSource,
		issue.Category,
		issue.Keyword,
		issue.KeywordLocation,
		issue.Property,
		issue.InstanceLocation,
		issue.Message,
		issue.Hint,
	}, " "))
}

func realWorldTermsMatch(haystack string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	for _, term := range terms {
		if term != "" && strings.Contains(haystack, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func (s *repoServer) realWorldRecommendationBacklog(args realWorldRecommendationBacklogArgs) (map[string]any, error) {
	limit := boundedRealWorldQueryLimit(args.Limit)
	minOccurrences := args.MinOccurrences
	if minOccurrences <= 0 {
		minOccurrences = 1
	}
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return nil, err
	}
	clusters := map[string]*realWorldRecommendationBacklogCluster{}
	totalRecommendations := 0
	for _, entry := range history.Entries {
		for _, recommendation := range entry.ProductRecommendations {
			if !args.IncludeNoChange && isNoChangeRecommendation(recommendation) {
				continue
			}
			totalRecommendations++
			addRealWorldRecommendationToBacklogCluster(clusters, realWorldRecommendationBacklogExample{
				EntryID:        entry.ID,
				EntryTitle:     entry.Title,
				Date:           entry.Date,
				Strength:       recommendation.Strength,
				Recommendation: recommendation.Recommendation,
				Rationale:      recommendation.Rationale,
				EntryPath:      realWorldEntryRelPath(entry),
				ArtifactPath:   entry.PersistedOutputArtifact,
			})
		}
		for _, feedback := range entry.ValidationFeedback {
			for _, recommendation := range feedback.ProductRecommendations {
				if !args.IncludeNoChange && isNoChangeRecommendation(recommendation) {
					continue
				}
				totalRecommendations++
				addRealWorldRecommendationToBacklogCluster(clusters, realWorldRecommendationBacklogExample{
					EntryID:        entry.ID,
					EntryTitle:     entry.Title,
					Date:           entry.Date,
					Repository:     feedback.Repository,
					Outcome:        feedback.Outcome,
					Strength:       recommendation.Strength,
					Recommendation: recommendation.Recommendation,
					Rationale:      recommendation.Rationale,
					EntryPath:      realWorldEntryRelPath(entry),
					ArtifactPath:   entry.PersistedOutputArtifact,
				})
			}
		}
	}
	outClusters := make([]realWorldRecommendationBacklogCluster, 0, len(clusters))
	topicFilter := strings.TrimSpace(args.Topic)
	for _, cluster := range clusters {
		if cluster.Count < minOccurrences {
			continue
		}
		if topicFilter != "" && cluster.Key != topicFilter && !strings.Contains(strings.ToLower(cluster.Title), strings.ToLower(topicFilter)) {
			continue
		}
		if limit > 0 && len(cluster.Examples) > limit {
			cluster.Examples = cluster.Examples[:limit]
		}
		cluster.SuggestedRegressionFixtures = realWorldRecommendationRegressionFixtures(*cluster)
		cluster.NextAction = realWorldRecommendationNextAction(cluster.Key)
		outClusters = append(outClusters, *cluster)
	}
	sort.Slice(outClusters, func(i, j int) bool {
		if realWorldRecommendationStrengthRank(outClusters[i].HighestStrength) != realWorldRecommendationStrengthRank(outClusters[j].HighestStrength) {
			return realWorldRecommendationStrengthRank(outClusters[i].HighestStrength) > realWorldRecommendationStrengthRank(outClusters[j].HighestStrength)
		}
		if outClusters[i].Count != outClusters[j].Count {
			return outClusters[i].Count > outClusters[j].Count
		}
		return outClusters[i].Title < outClusters[j].Title
	})
	if limit > 0 && len(outClusters) > limit {
		outClusters = outClusters[:limit]
	}
	return map[string]any{
		"ok":                     true,
		"entriesScanned":         len(history.Entries),
		"recommendationsScanned": totalRecommendations,
		"clusterCount":           len(outClusters),
		"clusters":               outClusters,
		"nextStep":               "Use real_world_artifact_query with an entryID, repository, or recommendation text to pull concrete examples for any backlog cluster.",
	}, nil
}

func addRealWorldRecommendationToBacklogCluster(clusters map[string]*realWorldRecommendationBacklogCluster, example realWorldRecommendationBacklogExample) {
	key, title := realWorldRecommendationTopic(example.Recommendation + " " + example.Rationale)
	cluster := clusters[key]
	if cluster == nil {
		cluster = &realWorldRecommendationBacklogCluster{
			Key:       key,
			Title:     title,
			Strengths: map[string]int{},
		}
		clusters[key] = cluster
	}
	cluster.Count++
	cluster.Strengths[example.Strength]++
	if realWorldRecommendationStrengthRank(example.Strength) > realWorldRecommendationStrengthRank(cluster.HighestStrength) {
		cluster.HighestStrength = example.Strength
	}
	cluster.Entries = appendUniqueLimit(cluster.Entries, example.EntryID, 12)
	cluster.Repositories = appendUniqueLimit(cluster.Repositories, example.Repository, 12)
	cluster.Examples = append(cluster.Examples, example)
}

func realWorldRecommendationTopic(text string) (string, string) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "funding"):
		return "github-funding-blank-providers", "GitHub Funding blank-provider noise"
	case strings.Contains(lower, "dependabot") && strings.Contains(lower, "review"):
		return "dependabot-reviewers-schema-drift", "Dependabot reviewers schema drift"
	case strings.Contains(lower, "workflow") && (strings.Contains(lower, "matrix") || strings.Contains(lower, "strategy")):
		return "github-workflow-strategy-drift", "GitHub Workflow strategy schema drift"
	case strings.Contains(lower, "travis") || strings.Contains(lower, "legacy ci") || strings.Contains(lower, "ci enum"):
		return "legacy-ci-enum-drift", "Legacy CI enum/schema drift"
	case strings.Contains(lower, "watch.json") || strings.Contains(lower, "cluster.json") || strings.Contains(lower, "generic basename") || strings.Contains(lower, "generic json"):
		return "schemastore-generic-basename", "Generic SchemaStore basename false positives"
	case strings.Contains(lower, "helm") || strings.Contains(lower, "templated yaml") || strings.Contains(lower, "chart template"):
		return "templated-yaml-parser-noise", "Templated YAML parser noise"
	case strings.Contains(lower, "__testfixtures__") || strings.Contains(lower, "fixture") || strings.Contains(lower, "testdata"):
		return "fixture-grouping", "Fixture and test-data grouping"
	case strings.Contains(lower, "schema compile") || strings.Contains(lower, "schema-source") || strings.Contains(lower, "schema/source") || strings.Contains(lower, "schema catalog warning"):
		return "schema-source-warning-grouping", "Schema-source warning grouping"
	case strings.Contains(lower, "skipped-file coverage") || strings.Contains(lower, "skipped coverage") || strings.Contains(lower, "skipped-file"):
		return "skipped-coverage-surfacing", "Skipped coverage surfacing"
	case strings.Contains(lower, "rust") || strings.Contains(lower, "cargo") || strings.Contains(lower, "rustfmt"):
		return "rust-config-coverage", "Rust configuration coverage"
	case strings.Contains(lower, "node_modules") || strings.Contains(lower, "local schema") || strings.Contains(lower, "dependency prep"):
		return "dependency-prep-schema-fidelity", "Dependency-prep schema fidelity"
	default:
		words := realWorldRecommendationTerms(text)
		if len(words) > 6 {
			words = words[:6]
		}
		key := slugify(strings.Join(words, " "))
		if key == "" {
			key = "uncategorized"
		}
		return key, realWorldRecommendationFallbackTitle(key)
	}
}

func realWorldRecommendationFallbackTitle(key string) string {
	words := strings.Split(strings.ReplaceAll(key, "-", " "), " ")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func realWorldRecommendationRegressionFixtures(cluster realWorldRecommendationBacklogCluster) []string {
	var fixtures []string
	for _, repo := range cluster.Repositories {
		if repo != "" {
			fixtures = appendUniqueLimit(fixtures, repo, 5)
		}
	}
	switch cluster.Key {
	case "github-funding-blank-providers":
		fixtures = appendUniqueLimit(fixtures, ".github/FUNDING.yml with blank provider keys", 6)
	case "github-workflow-strategy-drift":
		fixtures = appendUniqueLimit(fixtures, ".github/workflows/*.yml strategy with fail-fast and no matrix", 6)
	case "dependabot-reviewers-schema-drift":
		fixtures = appendUniqueLimit(fixtures, ".github/dependabot.yml reviewers", 6)
	case "legacy-ci-enum-drift":
		fixtures = appendUniqueLimit(fixtures, "legacy .travis.yml OS/compiler enums", 6)
	case "schemastore-generic-basename":
		fixtures = appendUniqueLimit(fixtures, "Redis-style src/commands/watch.json and cluster.json", 6)
	case "templated-yaml-parser-noise":
		fixtures = appendUniqueLimit(fixtures, "Helm-style chart/templates/*.yaml", 6)
	}
	return fixtures
}

func realWorldRecommendationNextAction(key string) string {
	switch key {
	case "schema-source-warning-grouping", "skipped-coverage-surfacing":
		return "Check whether this is already covered by current MCP triage output, then keep as a regression expectation."
	case "templated-yaml-parser-noise", "schemastore-generic-basename":
		return "Open implementation work with a focused fixture and verify the human CLI output, JSON, and real-world triage artifact."
	case "github-funding-blank-providers", "github-workflow-strategy-drift", "dependabot-reviewers-schema-drift", "legacy-ci-enum-drift":
		return "Add a small schema-drift fixture or targeted hint test before changing catalog behavior."
	default:
		return "Review the examples and decide whether this is a product change, documentation note, or no-change observation."
	}
}

func realWorldRecommendationStrengthRank(strength string) int {
	switch strength {
	case "high":
		return 3
	case "med":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func boundedRealWorldQueryLimit(limit int) int {
	if limit <= 0 {
		return 8
	}
	if limit > 50 {
		return 50
	}
	return limit
}
