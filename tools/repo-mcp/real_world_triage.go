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
	RunID                  string                           `json:"runID"`
	Title                  string                           `json:"title"`
	CorpusDir              string                           `json:"corpusDir"`
	CacheDir               string                           `json:"cacheDir"`
	Command                string                           `json:"command"`
	OutputArtifact         string                           `json:"outputArtifact"`
	ManifestPath           string                           `json:"manifestPath"`
	Repositories           []realWorldRepository            `json:"repositories"`
	DependencyPrep         []realWorldDependencyPrep        `json:"dependencyPrep"`
	ValidationFeedback     []realWorldValidationFeedback    `json:"validationFeedback"`
	ProductRecommendations []realWorldProductRecommendation `json:"productRecommendations"`
	ProductDecisions       []string                         `json:"productDecisions"`
	FollowUp               []string                         `json:"followUp"`
}

type realWorldOutputDetails struct {
	Root     string                 `json:"root,omitempty"`
	Summary  *realWorldResult       `json:"summary,omitempty"`
	Issues   []realWorldOutputIssue `json:"issues,omitempty"`
	Warnings []map[string]any       `json:"warnings,omitempty"`
	Files    []realWorldOutputFile  `json:"files,omitempty"`
	Styled   *realWorldStyledOutput `json:"styled,omitempty"`
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
	Path           string `json:"path"`
	Format         string `json:"format,omitempty"`
	Schema         string `json:"schema,omitempty"`
	SchemaSource   string `json:"schemaSource,omitempty"`
	Status         string `json:"status"`
	Issues         int    `json:"issues,omitempty"`
	Ignored        int    `json:"ignored,omitempty"`
	Message        string `json:"message,omitempty"`
	SkipReason     string `json:"skipReason,omitempty"`
	SkipClass      string `json:"skipClass,omitempty"`
	SkipImportance string `json:"skipImportance,omitempty"`
	SkipDetail     string `json:"skipDetail,omitempty"`
}

type realWorldTriage struct {
	OutputArtifact            string                           `json:"outputArtifact"`
	Summary                   *realWorldResult                 `json:"summary,omitempty"`
	PerRepository             []realWorldRepositoryTriage      `json:"perRepository"`
	IssueGroups               []realWorldIssueGroup            `json:"issueGroups"`
	WarningGroups             []realWorldWarningGroup          `json:"warningGroups"`
	ValidationFeedback        []realWorldValidationFeedback    `json:"validationFeedback,omitempty"`
	ValidationFeedbackSummary map[string]any                   `json:"validationFeedbackSummary,omitempty"`
	Findings                  []string                         `json:"findings"`
	ProductRecommendations    []realWorldProductRecommendation `json:"productRecommendations"`
	ProductDecisions          []string                         `json:"productDecisions"`
	FollowUp                  []string                         `json:"followUp"`
	DiscussionPacket          map[string]any                   `json:"discussionPacket,omitempty"`
	FinalResponseContract     map[string]any                   `json:"finalResponseContract"`
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

type realWorldStyledOutput struct {
	Plain     string         `json:"plain,omitempty"`
	ANSI      string         `json:"ansi,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}

type realWorldIssueExample struct {
	Path             string                  `json:"path"`
	Category         string                  `json:"category"`
	Keyword          string                  `json:"keyword,omitempty"`
	Schema           string                  `json:"schema,omitempty"`
	SchemaSource     string                  `json:"schemaSource,omitempty"`
	InstanceLocation string                  `json:"instanceLocation,omitempty"`
	KeywordLocation  string                  `json:"keywordLocation,omitempty"`
	Property         string                  `json:"property,omitempty"`
	Line             int                     `json:"line,omitempty"`
	Column           int                     `json:"column,omitempty"`
	MessagePreview   string                  `json:"messagePreview"`
	MessageLength    int                     `json:"messageLength"`
	MessageTruncated bool                    `json:"messageTruncated"`
	ProductSignal    bool                    `json:"productSignal"`
	UXSignals        []string                `json:"uxSignals,omitempty"`
	SourceExcerpt    *realWorldSourceExcerpt `json:"sourceExcerpt,omitempty"`
}

type realWorldSourceExcerpt struct {
	Path      string                `json:"path"`
	StartLine int                   `json:"startLine"`
	Lines     []realWorldSourceLine `json:"lines"`
}

type realWorldSourceLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type realWorldSkippedFileSignal struct {
	Path          string `json:"path"`
	Format        string `json:"format,omitempty"`
	Class         string `json:"class"`
	Reason        string `json:"reason"`
	ProductSignal bool   `json:"productSignal"`
}

type realWorldSkippedFileGroup struct {
	Class         string   `json:"class"`
	Count         int      `json:"count"`
	ProductSignal bool     `json:"productSignal"`
	Reasons       []string `json:"reasons,omitempty"`
	Examples      []string `json:"examples,omitempty"`
}

type realWorldRepoIdentity struct {
	Name     string
	Path     string
	CloneURL string
}

func (s *repoServer) handleRealWorldTriageOutput(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldTriageArgs
	_ = request.BindArguments(&args)
	if err := realWorldRejectManualPathArgsWithRunID(args.RunID, map[string]string{
		"corpusDir":      args.CorpusDir,
		"cacheDir":       args.CacheDir,
		"command":        args.Command,
		"outputArtifact": args.OutputArtifact,
		"manifestPath":   args.ManifestPath,
	}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if args.RunID != "" {
		if s.realWorldRuns == nil {
			return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
		}
		run, ok := s.realWorldRuns.get(args.RunID)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
		}
		run.refreshFromManifest()
		if args.Title == "" {
			args.Title = run.Title
		}
		if args.CorpusDir == "" {
			args.CorpusDir = run.CorpusDir
		}
		if args.CacheDir == "" {
			args.CacheDir = run.CacheDir
		}
		if args.OutputArtifact == "" {
			args.OutputArtifact = run.OutputArtifact
		}
		if args.ManifestPath == "" {
			args.ManifestPath = run.ManifestPath
		}
		if args.Command == "" {
			args.Command = nonEmpty(run.Command, realWorldManagedValidationCommand(run))
		}
		if len(args.Repositories) == 0 {
			args.Repositories = append([]realWorldRepository{}, run.Repositories...)
		}
		if len(args.DependencyPrep) == 0 {
			args.DependencyPrep = append([]realWorldDependencyPrep{}, run.DependencyPrep...)
		}
		if len(args.ValidationFeedback) == 0 {
			args.ValidationFeedback = run.validationFeedback()
		}
	}

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
			if len(args.DependencyPrep) == 0 {
				args.DependencyPrep = manifest.DependencyPrep
			}
		}
	}
	if args.OutputArtifact == "" {
		return mcp.NewToolResultError("outputArtifact is required; call real_world_finish_validation first or pass manifestPath"), nil
	}
	if args.Command == "" && args.CorpusDir != "" && args.CacheDir != "" {
		args.Command = realWorldValidationCommand(args.CorpusDir, args.CacheDir, args.OutputArtifact, true, "warn", 1, "1ms", "1ms", nil)
	}
	for i, feedback := range args.ValidationFeedback {
		if err := validateRealWorldValidationFeedback(feedback); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("validationFeedback[%d]: %v", i, err)), nil
		}
	}

	triage, err := triageRealWorldOutput(args.OutputArtifact, repositories)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	applyRealWorldValidationFeedback(&triage, args.ValidationFeedback)
	if len(args.ProductRecommendations) > 0 {
		triage.ProductRecommendations = args.ProductRecommendations
	}
	if len(args.ProductDecisions) > 0 {
		triage.ProductDecisions = args.ProductDecisions
	}
	if len(args.FollowUp) > 0 {
		triage.FollowUp = args.FollowUp
	}
	if triage.DiscussionPacket != nil {
		triage.DiscussionPacket["productRecommendations"] = triage.ProductRecommendations
		if triage.ValidationFeedbackSummary != nil {
			triage.DiscussionPacket["validationFeedbackSummary"] = triage.ValidationFeedbackSummary
		}
	}

	draftRecord := map[string]any{
		"title":                  args.Title,
		"dependencyPrep":         args.DependencyPrep,
		"validationFeedback":     args.ValidationFeedback,
		"findings":               triage.Findings,
		"productRecommendations": triage.ProductRecommendations,
		"productDecisions":       triage.ProductDecisions,
		"followUp":               triage.FollowUp,
	}
	if args.RunID != "" {
		draftRecord["runID"] = args.RunID
	} else {
		draftRecord["corpus"] = args.CorpusDir
		draftRecord["cacheDir"] = args.CacheDir
		draftRecord["command"] = args.Command
		draftRecord["outputArtifact"] = args.OutputArtifact
		draftRecord["manifestPath"] = manifestPath
		draftRecord["repositories"] = repositories
	}
	missingArgs := realWorldTriageDraftRecordMissing(args, repositories)
	out := map[string]any{
		"ok":                        true,
		"summary":                   triage.Summary,
		"perRepository":             realWorldPublicRepositoryTriage(triage.PerRepository),
		"issueGroups":               triage.IssueGroups,
		"warningGroups":             triage.WarningGroups,
		"discussionPacket":          triage.DiscussionPacket,
		"validationFeedback":        triage.ValidationFeedback,
		"validationFeedbackSummary": triage.ValidationFeedbackSummary,
		"draftRecord":               draftRecord,
		"draftRecordMissing":        missingArgs,
		"finalResponseContract":     triage.FinalResponseContract,
		"nextStep":                  realWorldNextRecordTriagedResult(draftRecord, missingArgs),
	}
	if args.RunID == "" {
		out["outputArtifact"] = args.OutputArtifact
		out["manifestPath"] = manifestPath
	}
	return s.realWorldStructured(ctx, out)
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
	discussionPacket := realWorldDiscussionPacket(details, perRepo, issueGroups, warningGroups, productRecommendations)
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
		DiscussionPacket:       discussionPacket,
		FinalResponseContract:  realWorldFinalResponseContract(),
	}, nil
}

func applyRealWorldValidationFeedback(triage *realWorldTriage, feedback []realWorldValidationFeedback) {
	if len(feedback) == 0 {
		return
	}
	triage.ValidationFeedback = append([]realWorldValidationFeedback{}, feedback...)
	triage.ValidationFeedbackSummary = realWorldValidationFeedbackSummary(feedback)
	triage.Findings = append([]string{realWorldValidationFeedbackFinding(feedback)}, triage.Findings...)
	var feedbackRecommendations []realWorldProductRecommendation
	var feedbackDecisions []string
	var feedbackFollowUp []string
	for _, item := range feedback {
		feedbackRecommendations = append(feedbackRecommendations, item.ProductRecommendations...)
		feedbackDecisions = append(feedbackDecisions, item.ProductDecisions...)
		feedbackFollowUp = append(feedbackFollowUp, item.FollowUp...)
	}
	if len(feedbackRecommendations) > 0 {
		triage.ProductRecommendations = appendNonNoChangeRecommendations(triage.ProductRecommendations, feedbackRecommendations)
	}
	if len(feedbackDecisions) > 0 {
		triage.ProductDecisions = append(triage.ProductDecisions, feedbackDecisions...)
	}
	if len(feedbackFollowUp) > 0 {
		triage.FollowUp = append(triage.FollowUp, feedbackFollowUp...)
	}
}

func realWorldValidationFeedbackFinding(feedback []realWorldValidationFeedback) string {
	summary := realWorldValidationFeedbackSummary(feedback)
	return fmt.Sprintf("Agent validation feedback ledger: %d repo(s) behaved reasonably, %d repo(s) produced product signals, and %d repo(s) were blocked or uninterpretable.",
		summary["behavedReasonably"], summary["productSignals"], summary["blocked"])
}

func realWorldDiscussionPacket(details realWorldOutputDetails, perRepo []realWorldRepositoryTriage, issueGroups []realWorldIssueGroup, warningGroups []realWorldWarningGroup, recommendations []realWorldProductRecommendation) map[string]any {
	skippedSignals := realWorldSkippedFileSignals(details.Files)
	skippedGroups := realWorldSkippedFileGroups(skippedSignals)
	issueExamples := realWorldIssueExamples(details, 5)
	return map[string]any{
		"purpose":                "Team-facing product discussion brief generated from the real-world validation artifact.",
		"summary":                details.Summary,
		"repositories":           realWorldDiscussionRepositories(perRepo),
		"productRecommendations": recommendations,
		"exampleIssues":          issueExamples,
		"skippedGroups":          skippedGroups,
		"topIssueGroups":         firstIssueGroups(issueGroups, 5),
		"topWarningGroups":       firstWarningGroups(warningGroups, 5),
		"cliPreview":             realWorldCLIPreview(details.Styled),
		"uxSignals":              realWorldUXSignals(details, issueExamples, skippedGroups),
		"positiveEvidence":       realWorldPositiveEvidence(perRepo),
	}
}

func realWorldDiscussionRepositories(perRepo []realWorldRepositoryTriage) []map[string]string {
	out := []map[string]string{}
	for _, repo := range perRepo {
		url := realWorldRepositoryWebURL(repo)
		if url == "" {
			continue
		}
		name := nonEmpty(repo.Repository, repoNameFromURL(url))
		item := map[string]string{
			"name":     name,
			"url":      url,
			"markdown": fmt.Sprintf("[%s](%s)", escapeMarkdownLinkLabel(name), url),
		}
		out = append(out, item)
	}
	return out
}

func realWorldRepositoryWebURL(repo realWorldRepositoryTriage) string {
	cloneURL := strings.TrimSpace(repo.CloneURL)
	if cloneURL != "" {
		if strings.HasPrefix(cloneURL, "git@github.com:") {
			path := strings.TrimSuffix(strings.TrimPrefix(cloneURL, "git@github.com:"), ".git")
			path = strings.Trim(path, "/")
			if path != "" {
				return "https://github.com/" + path
			}
		}
		if strings.HasPrefix(cloneURL, "https://github.com/") || strings.HasPrefix(cloneURL, "http://github.com/") {
			return strings.TrimSuffix(strings.TrimRight(cloneURL, "/"), ".git")
		}
		if strings.HasPrefix(cloneURL, "https://") || strings.HasPrefix(cloneURL, "http://") {
			return strings.TrimSuffix(strings.TrimRight(cloneURL, "/"), ".git")
		}
	}
	key := normalizeRepoQuery(repo.Repository)
	if strings.Count(key, "/") == 1 {
		return "https://github.com/" + key
	}
	return ""
}

func escapeMarkdownLinkLabel(label string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)
	return replacer.Replace(label)
}

func firstIssueGroups(groups []realWorldIssueGroup, limit int) []realWorldIssueGroup {
	if len(groups) <= limit {
		return groups
	}
	return groups[:limit]
}

func firstWarningGroups(groups []realWorldWarningGroup, limit int) []realWorldWarningGroup {
	if len(groups) <= limit {
		return groups
	}
	return groups[:limit]
}

func realWorldPositiveEvidence(perRepo []realWorldRepositoryTriage) []string {
	var out []string
	for _, repo := range perRepo {
		if repo.Discovered == 0 || repo.IssueCount > 0 || repo.WarningCount > 0 || repo.Failed > 0 {
			continue
		}
		note := fmt.Sprintf("%s validated %d/%d discovered file(s) without issues or warnings", repo.Repository, repo.Validated, repo.Discovered)
		if repo.Skipped > 0 {
			note += fmt.Sprintf(" (%d skipped)", repo.Skipped)
		}
		out = append(out, note)
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func appendNonNoChangeRecommendations(existing, additional []realWorldProductRecommendation) []realWorldProductRecommendation {
	out := make([]realWorldProductRecommendation, 0, len(existing)+len(additional))
	for _, recommendation := range existing {
		if isNoChangeRecommendation(recommendation) {
			continue
		}
		out = append(out, recommendation)
	}
	out = append(out, additional...)
	return out
}

func isNoChangeRecommendation(recommendation realWorldProductRecommendation) bool {
	return strings.HasPrefix(strings.ToLower(recommendation.Recommendation), "no product change")
}

func readRealWorldOutputDetails(path string) (realWorldOutputDetails, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return realWorldOutputDetails{}, err
	}
	data, styled, _, err := realWorldUnwrapBundle(data)
	if err != nil {
		return realWorldOutputDetails{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var payload struct {
		Root    string `json:"root"`
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
		Root:     payload.Root,
		Summary:  summary,
		Issues:   payload.Issues,
		Warnings: payload.Warnings,
		Files:    payload.Files,
		Styled:   styled,
	}, nil
}

func realWorldUnwrapBundle(data []byte) ([]byte, *realWorldStyledOutput, bool, error) {
	var probe struct {
		JSON   json.RawMessage        `json:"json"`
		Styled *realWorldStyledOutput `json:"styled"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, nil, false, err
	}
	if len(probe.JSON) == 0 {
		return data, nil, false, nil
	}
	return probe.JSON, probe.Styled, true, nil
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

func realWorldValidationEvidence(repository, outputArtifact string) (map[string]any, error) {
	details, err := readRealWorldOutputDetails(outputArtifact)
	if err != nil {
		return nil, err
	}
	if err := validateRealWorldOutputDetails(outputArtifact, details); err != nil {
		return nil, err
	}
	statusCounts := map[string]int{}
	var skippedExamples []realWorldOutputFile
	var failedExamples []realWorldOutputFile
	for _, file := range details.Files {
		statusCounts[file.Status]++
		switch file.Status {
		case "skipped":
			if len(skippedExamples) < 8 {
				skippedExamples = append(skippedExamples, file)
			}
		case "failed", "error":
			if len(failedExamples) < 8 {
				failedExamples = append(failedExamples, file)
			}
		}
	}
	issueGroups := realWorldGroupRepositoryIssues(repository, details.Issues)
	warningGroups := realWorldGroupWarnings(details.Warnings)
	issueExamples := realWorldIssueExamples(details, 6)
	skippedSignals := realWorldSkippedFileSignals(details.Files)
	skippedGroups := realWorldSkippedFileGroups(skippedSignals)
	if len(issueGroups) > 6 {
		issueGroups = issueGroups[:6]
	}
	if len(warningGroups) > 6 {
		warningGroups = warningGroups[:6]
	}
	cliPreview := realWorldCLIPreview(details.Styled)
	return map[string]any{
		"summary":              details.Summary,
		"statusCounts":         statusCounts,
		"skippedCoverageRatio": realWorldSkippedCoverageRatio(details.Summary),
		"skippedExamples":      skippedExamples,
		"skippedSignals":       skippedSignals,
		"skippedGroups":        skippedGroups,
		"failedExamples":       failedExamples,
		"topIssueGroups":       issueGroups,
		"exampleIssues":        issueExamples,
		"topWarningGroups":     warningGroups,
		"cliPreview":           cliPreview,
		"uxSignals":            realWorldUXSignals(details, issueExamples, skippedGroups),
		"outcomeHint":          realWorldEvidenceOutcomeHint(details.Summary, issueGroups, warningGroups, skippedGroups, issueExamples),
		"assessmentChecklist": []string{
			"Did DollarLint parse and classify files in a way a developer would understand?",
			"Does cliPreview.plain look like a useful terminal experience for a real user?",
			"Are skipped files and warnings clear enough to judge coverage?",
			"Are repeated issues grouped enough to reveal the next action?",
			"Are failures expected fixtures, repository-specific unsupported syntax, or product behavior worth improving?",
		},
	}, nil
}

func realWorldSkippedCoverageRatio(summary *realWorldResult) float64 {
	if summary == nil || summary.Discovered == 0 {
		return 0
	}
	return float64(summary.Skipped) / float64(summary.Discovered)
}

func realWorldIssueExamples(details realWorldOutputDetails, limit int) []realWorldIssueExample {
	if limit <= 0 {
		return nil
	}
	out := make([]realWorldIssueExample, 0, min(len(details.Issues), limit))
	for _, issue := range details.Issues {
		if len(out) >= limit {
			break
		}
		preview := truncate(issue.Message, 700)
		example := realWorldIssueExample{
			Path:             issue.Path,
			Category:         issue.Category,
			Keyword:          issue.Keyword,
			Schema:           issue.Schema,
			SchemaSource:     issue.SchemaSource,
			InstanceLocation: issue.InstanceLocation,
			KeywordLocation:  issue.KeywordLocation,
			Property:         issue.Property,
			Line:             issue.Line,
			Column:           issue.Column,
			MessagePreview:   preview,
			MessageLength:    len(issue.Message),
			MessageTruncated: len(strings.TrimSpace(issue.Message)) > len(preview),
			ProductSignal:    realWorldIssueIsProductSignal(issue),
			UXSignals:        realWorldIssueUXSignals(issue),
			SourceExcerpt:    realWorldSourceExcerptForIssue(details.Root, issue),
		}
		out = append(out, example)
	}
	return out
}

func realWorldIssueUXSignals(issue realWorldOutputIssue) []string {
	var signals []string
	if len(issue.Message) > 2000 {
		signals = append(signals, "message-over-2000-chars")
	}
	if strings.Contains(strings.ToLower(issue.Message), "value must be one of") && strings.Count(issue.Message, "'") > 20 {
		signals = append(signals, "large-enum-message")
	}
	if strings.HasPrefix(issue.SchemaSource, "catalog:") {
		signals = append(signals, "catalog-backed-schema")
	}
	if issue.InstanceLocation != "" {
		signals = append(signals, "has-instance-location")
	}
	return signals
}

func realWorldSourceExcerptForIssue(root string, issue realWorldOutputIssue) *realWorldSourceExcerpt {
	if root == "" || issue.Path == "" || issue.Line <= 0 || filepath.IsAbs(issue.Path) {
		return nil
	}
	cleanRoot := filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(issue.Path)))
	rel, err := filepath.Rel(cleanRoot, candidate)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	data, err := os.ReadFile(candidate)
	if err != nil || len(data) > 2*1024*1024 {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if issue.Line > len(lines) {
		return nil
	}
	start := max(1, issue.Line-1)
	end := min(len(lines), issue.Line+1)
	excerpt := realWorldSourceExcerpt{
		Path:      issue.Path,
		StartLine: start,
		Lines:     make([]realWorldSourceLine, 0, end-start+1),
	}
	for line := start; line <= end; line++ {
		excerpt.Lines = append(excerpt.Lines, realWorldSourceLine{Number: line, Text: truncate(lines[line-1], 240)})
	}
	return &excerpt
}

func realWorldSkippedFileSignals(files []realWorldOutputFile) []realWorldSkippedFileSignal {
	var out []realWorldSkippedFileSignal
	for _, file := range files {
		if file.Status != "skipped" {
			continue
		}
		class, reason, productSignal := realWorldClassifySkippedFile(file)
		out = append(out, realWorldSkippedFileSignal{
			Path:          file.Path,
			Format:        file.Format,
			Class:         class,
			Reason:        reason,
			ProductSignal: productSignal,
		})
	}
	return out
}

func realWorldClassifySkippedFile(file realWorldOutputFile) (string, string, bool) {
	if file.SkipClass != "" {
		productSignal := file.SkipImportance == "high" ||
			file.SkipClass == "unsupported-config" ||
			file.SkipClass == "repo-management-config" ||
			file.SkipClass == "external-catalog"
		reason := nonEmpty(file.SkipDetail, file.SkipReason, "DollarLint classified this skipped file.")
		return file.SkipClass, reason, productSignal
	}
	path := strings.ToLower(filepath.ToSlash(file.Path))
	base := filepath.Base(path)
	switch {
	case strings.HasSuffix(base, "-lock.json") || base == "package-lock.json" || base == "composer.lock" || base == "cargo.lock" || strings.Contains(base, "lock"):
		return "lockfile", "Generated or dependency lockfile; often reasonable to skip unless lockfile validation is a product goal.", false
	case strings.Contains(path, "/.cargo/") || base == ".clippy.toml" || base == "deny.toml" || base == "release.toml":
		return "tooling-config", "Common hand-authored tooling configuration without a matched schema.", true
	case base == ".rubocop.yml" || base == ".rubocop_todo.yml" || base == ".coveralls.yml":
		return "tooling-config", "Common Ruby tooling/service configuration without a matched schema.", true
	case base == "atmos.yaml" || base == "atmos.yml":
		return "infrastructure-config", "Common infrastructure/project configuration without a matched schema.", true
	case strings.Contains(path, "/.github/settings.yml") || strings.Contains(path, "/.github/settings.yaml") || base == "readme.yaml":
		return "repo-management-config", "Repository-management configuration without a matched schema.", true
	case strings.Contains(path, "/locale/") || strings.Contains(path, "/locales/") || strings.Contains(path, "/i18n/"):
		return "locale-data", "Locale or translation data; usually lower priority than project configuration.", false
	case strings.Contains(path, "/test") || strings.Contains(path, "/spec") || strings.Contains(path, "/fixture") || strings.Contains(path, "/benchmark"):
		return "test-or-fixture-data", "Test, fixture, or benchmark data; often reasonable to skip unless users expect general data validation.", false
	case strings.Contains(path, "/data/"):
		return "application-data", "Application data without a matched schema; may be reasonable to skip for a config-focused linter.", false
	default:
		return "unknown-schema-less", "No matched schema; review whether this is user-authored configuration or arbitrary data.", false
	}
}

func realWorldSkippedFileGroups(signals []realWorldSkippedFileSignal) []realWorldSkippedFileGroup {
	groups := map[string]*realWorldSkippedFileGroup{}
	for _, signal := range signals {
		group := groups[signal.Class]
		if group == nil {
			group = &realWorldSkippedFileGroup{Class: signal.Class}
			groups[signal.Class] = group
		}
		group.Count++
		group.ProductSignal = group.ProductSignal || signal.ProductSignal
		group.Reasons = appendUniqueLimit(group.Reasons, signal.Reason, 3)
		group.Examples = appendUniqueLimit(group.Examples, signal.Path, 8)
	}
	out := make([]realWorldSkippedFileGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProductSignal != out[j].ProductSignal {
			return out[i].ProductSignal
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Class < out[j].Class
	})
	return out
}

func realWorldCLIPreview(styled *realWorldStyledOutput) map[string]any {
	if styled == nil {
		return nil
	}
	plain := truncate(styled.Plain, 6000)
	ansi := truncate(styled.ANSI, 6000)
	return map[string]any{
		"plain":          plain,
		"ansi":           ansi,
		"plainTruncated": len(strings.TrimSpace(styled.Plain)) > len(plain),
		"ansiTruncated":  len(strings.TrimSpace(styled.ANSI)) > len(ansi),
		"options":        styled.Options,
	}
}

func realWorldUXSignals(details realWorldOutputDetails, issueExamples []realWorldIssueExample, skippedGroups []realWorldSkippedFileGroup) []string {
	var signals []string
	for _, issue := range issueExamples {
		for _, signal := range issue.UXSignals {
			signals = appendUniqueLimit(signals, signal, 12)
		}
	}
	for _, group := range skippedGroups {
		if group.ProductSignal {
			signals = appendUniqueLimit(signals, "product-relevant-skipped-"+group.Class, 12)
		}
	}
	if details.Styled != nil && strings.Contains(details.Styled.Plain, "... truncated ...") {
		signals = appendUniqueLimit(signals, "cli-preview-was-truncated", 12)
	}
	if details.Summary != nil && details.Summary.Discovered > 0 && realWorldSkippedCoverageRatio(details.Summary) >= 0.25 {
		signals = appendUniqueLimit(signals, "notable-skipped-coverage", 12)
	}
	return signals
}

func realWorldEvidenceOutcomeHint(summary *realWorldResult, issueGroups []realWorldIssueGroup, warningGroups []realWorldWarningGroup, skippedGroups []realWorldSkippedFileGroup, issueExamples []realWorldIssueExample) map[string]any {
	hint := "behaved-reasonably"
	var reasons []string
	if summary == nil {
		return map[string]any{"outcome": "blocked", "reasons": []string{"No JSON summary was available."}}
	}
	if summary.Discovered == 0 {
		hint = realWorldFeedbackBlocked
		reasons = append(reasons, "DollarLint discovered zero files, so the repository result has no usable coverage.")
	}
	if summary.Failed > 0 || summary.Issues.Parsing > 0 {
		hint = "product-signal-or-blocked"
		reasons = append(reasons, "Parsing failures or failed files need qualitative review before the result is trusted.")
	}
	if summary.Discovered > 0 && realWorldSkippedCoverageRatio(summary) >= 0.7 {
		if hint == "behaved-reasonably" {
			hint = realWorldFeedbackProductSignal
		}
		reasons = append(reasons, "At least 70% of discovered files were skipped, so coverage confidence may be unclear.")
	}
	if summary.Issues.Total >= 50 {
		if hint == "behaved-reasonably" {
			hint = realWorldFeedbackProductSignal
		}
		reasons = append(reasons, "The repository produced many issues, so grouping/actionability needs review.")
	}
	if len(warningGroups) > 0 {
		if hint == "behaved-reasonably" {
			hint = realWorldFeedbackProductSignal
		}
		reasons = append(reasons, "Warnings were emitted and may affect developer confidence.")
	}
	for _, group := range skippedGroups {
		if group.ProductSignal {
			if hint == "behaved-reasonably" {
				hint = realWorldFeedbackProductSignal
			}
			reasons = append(reasons, "Skipped files include common hand-authored "+group.Class+" files, which may be a coverage product signal.")
			break
		}
	}
	for _, issue := range issueExamples {
		if containsString(issue.UXSignals, "large-enum-message") {
			if hint == "behaved-reasonably" {
				hint = realWorldFeedbackProductSignal
			}
			reasons = append(reasons, "At least one issue has a very large enum message; review CLI actionability, not just correctness.")
			break
		}
	}
	for _, group := range issueGroups {
		if group.ProductSignal {
			if hint == "behaved-reasonably" {
				hint = realWorldFeedbackProductSignal
			}
			reasons = append(reasons, "At least one top issue group is classified as a product signal.")
			break
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "No obvious parser, coverage, warning, or high-volume issue signal was detected.")
	}
	return map[string]any{"outcome": hint, "reasons": reasons}
}

func realWorldGroupRepositoryIssues(repository string, issues []realWorldOutputIssue) []realWorldIssueGroup {
	groups := map[string]*realWorldIssueGroup{}
	for _, issue := range issues {
		signal := realWorldIssueSignal(issue)
		key := strings.Join([]string{issue.Category, signal, issue.SchemaSource, issue.Keyword}, "\x00")
		group := groups[key]
		if group == nil {
			group = &realWorldIssueGroup{
				Repository:    repository,
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
		return strings.Join([]string{out[i].Signal, out[i].Category, out[i].SchemaSource, out[i].Keyword}, "\x00") <
			strings.Join([]string{out[j].Signal, out[j].Category, out[j].SchemaSource, out[j].Keyword}, "\x00")
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
		"assessmentPerspective":    realWorldDeveloperExperienceGuidance(),
		"required":                 "The agent's final message to the user must choose exactly one of these outcomes: productChangesToConsider or productBehavedReasonably.",
		"productChangesToConsider": "Use this when the sweep found concrete correctness or ergonomics changes worth considering; list each recommendation with strength and rationale grounded in the MCP triage/record result.",
		"productBehavedReasonably": "Use this only when no genuine product change is recommended after assessing the developer experience; say the product behaved reasonably and briefly explain the observed evidence and caveats.",
		"doNot": []string{
			"Do not end with only raw counts or a generic run summary.",
			"Do not say the product behaved reasonably just because the command produced structured JSON.",
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
	if len(args.ValidationFeedback) == 0 {
		missing = append(missing, "validationFeedback")
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

func realWorldMapInt(value map[string]any, key string) int {
	raw, ok := value[key]
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	default:
		return 0
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
