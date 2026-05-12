package main

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	githubapi "github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldRunsDirRelPath           = "reports/agentic-product-testing"
	realWorldRunMetadataFileName      = "metadata.json"
	realWorldRunArtifactFileName      = "dollarlint.json"
	realWorldEntrySchema              = "../metadata.schema.json"
	realWorldHistorySchemaVersion     = 4
	realWorldMCPContractVersion       = 11
	realWorldManifestName             = "real-world-manifest.json"
	realWorldCorpusTempPrefix         = "dollarlint-corpus."
	realWorldCacheTempPrefix          = "dollarlint-cache."
	realWorldOutputTempPrefix         = "dollarlint-"
	realWorldCandidateSetPrefix       = "dollarlint-candidate-set-"
	realWorldDefaultHistoryLimit      = 25
	realWorldMaxHistoryLimit          = 200
	realWorldEntryIDSuffixBytes       = 8
	realWorldDefaultCandidateTarget   = 10
	realWorldMaxCandidateTarget       = 2000
	realWorldDefaultDiscoveryMinStars = 25
	realWorldMaxDiscoveryPerQuery     = 100
	realWorldGitHubAPIDefaultBaseURL  = "https://api.github.com"

	realWorldFeedbackBehavedReasonably = "behaved-reasonably"
	realWorldFeedbackProductSignal     = "product-signal"
	realWorldFeedbackBlocked           = "blocked"
)

func (s *repoServer) handleRealWorldCapabilities(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.realWorldStructured(ctx, map[string]any{
		"ok":           true,
		"capabilities": s.realWorldMCPContract(ctx),
		"nextStep":     realWorldNextChooseRepositories(),
	})
}

func (s *repoServer) realWorldStructured(ctx context.Context, value map[string]any) (*mcp.CallToolResult, error) {
	if value == nil {
		value = map[string]any{}
	}
	value["realWorldMCP"] = s.realWorldMCPContract(ctx)
	return structured(value)
}

func (s *repoServer) realWorldMCPContract(ctx context.Context) map[string]any {
	revision := strings.TrimSpace(s.output(ctx, "git rev-parse HEAD"))
	shortRevision := strings.TrimSpace(s.output(ctx, "git rev-parse --short HEAD"))
	status := s.output(ctx, "git status --short")
	return map[string]any{
		"server":           serverName,
		"toolNamespace":    "dollarlint_repo",
		"contractVersion":  realWorldMCPContractVersion,
		"managedFlow":      true,
		"revision":         revision,
		"shortRevision":    shortRevision,
		"workingTreeDirty": hasDirtyStatus(status),
		"staleIfMissing":   "If this object is missing or contractVersion is lower, restart the MCP server before running real-world testing.",
		"happyPath": []string{
			"Use runID-based real_world_* tools in nextStep order.",
			"When no explicit candidate repositories are provided, use real_world_discover_candidates instead of manual GitHub searching or hand-picked repo lists.",
			"Use candidateSetID and real_world_update_candidates diffs instead of resubmitting large repository lists after duplicate checks.",
			"Do not call legacy/path-based runners during managed real-world testing.",
			"Keep long-running prep/validation tool calls open for progress notifications; do not poll with shell sleep loops.",
			"Record qualitative developer-experience feedback for every delivered repository result.",
		},
		"analysisTools": []string{
			"real_world_artifact_query",
			"real_world_recommendation_backlog",
		},
	}
}

func realWorldRejectManualPathArgsWithRunID(runID string, fields map[string]string) error {
	if runID == "" {
		return nil
	}
	for name, value := range fields {
		if strings.TrimSpace(value) != "" {
			return fmt.Errorf("omit %s when runID is provided; the managed MCP run carries filesystem paths internally", name)
		}
	}
	return nil
}

type realWorldHistory struct {
	Schema        string           `json:"$schema,omitempty"`
	SchemaVersion int              `json:"schemaVersion"`
	Entries       []realWorldEntry `json:"entries"`
}

type realWorldEntryFile struct {
	Schema        string `json:"$schema,omitempty"`
	SchemaVersion int    `json:"schemaVersion"`
	realWorldEntry
}

type realWorldEntry struct {
	ID                      string                           `json:"id"`
	Date                    string                           `json:"date"`
	Title                   string                           `json:"title"`
	DollarLintRevision      string                           `json:"dollarlintRevision"`
	WorkingTreeNote         string                           `json:"workingTreeNote,omitempty"`
	Corpus                  string                           `json:"corpus"`
	CacheDir                string                           `json:"cacheDir,omitempty"`
	Command                 string                           `json:"command"`
	OutputArtifact          string                           `json:"outputArtifact"`
	PersistedOutputArtifact string                           `json:"persistedOutputArtifact,omitempty"`
	DependencyPrep          []realWorldDependencyPrep        `json:"dependencyPrep,omitempty"`
	ValidationFeedback      []realWorldValidationFeedback    `json:"validationFeedback,omitempty"`
	Repositories            []realWorldRepository            `json:"repositories"`
	Result                  *realWorldResult                 `json:"result,omitempty"`
	BeforeResult            *realWorldResult                 `json:"beforeResult,omitempty"`
	Findings                []string                         `json:"findings,omitempty"`
	ProductRecommendations  []realWorldProductRecommendation `json:"productRecommendations,omitempty"`
	ProductDecisions        []string                         `json:"productDecisions,omitempty"`
	FollowUp                []string                         `json:"followUp,omitempty"`
}

type realWorldDependencyPrep struct {
	Repository string `json:"repository,omitempty"`
	Command    string `json:"command,omitempty"`
	Status     string `json:"status,omitempty"`
	Notes      string `json:"notes,omitempty"`
	Error      string `json:"error,omitempty"`
	Output     string `json:"output,omitempty"`
}

type realWorldCleanupResult struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type realWorldProductRecommendation struct {
	Strength       string `json:"strength"`
	Recommendation string `json:"recommendation"`
	Rationale      string `json:"rationale"`
}

type realWorldValidationFeedback struct {
	Repository             string                           `json:"repository"`
	Outcome                string                           `json:"outcome"`
	Findings               []string                         `json:"findings,omitempty"`
	ProductRecommendations []realWorldProductRecommendation `json:"productRecommendations,omitempty"`
	ProductDecisions       []string                         `json:"productDecisions,omitempty"`
	Caveats                []string                         `json:"caveats,omitempty"`
	FollowUp               []string                         `json:"followUp,omitempty"`
	Notes                  string                           `json:"notes,omitempty"`
}

type realWorldRepository struct {
	Name            string   `json:"name"`
	Ecosystem       string   `json:"ecosystem,omitempty"`
	CloneURL        string   `json:"cloneURL"`
	Commit          string   `json:"commit,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	Path            string   `json:"path,omitempty"`
	Status          string   `json:"status,omitempty"`
	Error           string   `json:"error,omitempty"`
	AlreadyTested   bool     `json:"alreadyTested,omitempty"`
	PreviousEntries []string `json:"previousEntries,omitempty"`
}

type realWorldStartTestingArgs struct {
	Title                 string                `json:"title"`
	Repositories          []realWorldRepository `json:"repositories"`
	TargetCount           int                   `json:"targetCount"`
	AllowPreviouslyTested bool                  `json:"allowPreviouslyTested"`
	IncludeTestedRepos    bool                  `json:"includeTestedRepos"`
	TestedRepoLimit       int                   `json:"testedRepoLimit"`
}

type realWorldDiscoverCandidatesArgs struct {
	Title                 string   `json:"title"`
	TargetCount           int      `json:"targetCount"`
	AllowPreviouslyTested bool     `json:"allowPreviouslyTested"`
	Ecosystems            []string `json:"ecosystems"`
	MinStars              int      `json:"minStars"`
	PerQueryLimit         int      `json:"perQueryLimit"`
}

type realWorldHistoryArgs struct {
	Repo               string   `json:"repo"`
	Repositories       []string `json:"repositories"`
	IncludeEntries     bool     `json:"includeEntries"`
	IncludeTestedRepos bool     `json:"includeTestedRepos"`
	Limit              int      `json:"limit"`
}

type realWorldUpdateCandidatesArgs struct {
	CandidateSetID        string                 `json:"candidateSetID"`
	Diff                  realWorldCandidateDiff `json:"diff"`
	ExpectedCount         int                    `json:"expectedCount"`
	AllowPreviouslyTested *bool                  `json:"allowPreviouslyTested"`
	Title                 string                 `json:"title"`
}

type realWorldPrepareCorpusArgs struct {
	Title                 string                 `json:"title"`
	Repositories          []realWorldRepository  `json:"repositories"`
	CandidateSetID        string                 `json:"candidateSetID"`
	CandidateDiff         realWorldCandidateDiff `json:"candidateDiff"`
	ExpectedCount         int                    `json:"expectedCount"`
	Clone                 bool                   `json:"clone"`
	AllowPreviouslyTested *bool                  `json:"allowPreviouslyTested"`
	OutputName            string                 `json:"outputName"`
	Concurrency           int                    `json:"concurrency"`
	WaitForFirstResult    *bool                  `json:"waitForFirstResult"`
}

type realWorldCandidateSet struct {
	SchemaVersion         int                   `json:"schemaVersion"`
	ID                    string                `json:"id"`
	Title                 string                `json:"title,omitempty"`
	AllowPreviouslyTested bool                  `json:"allowPreviouslyTested,omitempty"`
	CreatedAt             string                `json:"createdAt"`
	UpdatedAt             string                `json:"updatedAt"`
	Repositories          []realWorldRepository `json:"repositories"`
}

type realWorldCandidateDiff struct {
	Add     []realWorldRepository           `json:"add,omitempty"`
	Remove  []string                        `json:"remove,omitempty"`
	Replace []realWorldCandidateReplacement `json:"replace,omitempty"`
}

type realWorldCandidateReplacement struct {
	Match      string              `json:"match"`
	Repository realWorldRepository `json:"repository"`
}

type realWorldCandidateDiffResult struct {
	Added    []realWorldRepository `json:"added,omitempty"`
	Removed  []realWorldRepository `json:"removed,omitempty"`
	Replaced []map[string]any      `json:"replaced,omitempty"`
}

type realWorldResult struct {
	Discovered int                    `json:"discovered"`
	Validated  int                    `json:"validated"`
	Skipped    int                    `json:"skipped"`
	Failed     int                    `json:"failed"`
	Issues     realWorldIssueSummary  `json:"issues"`
	Ignored    int                    `json:"ignored,omitempty"`
	Warnings   int                    `json:"warnings"`
	Duration   *realWorldDurationInfo `json:"duration,omitempty"`
}

type realWorldDurationInfo struct {
	Nanos int64 `json:"nanos,omitempty"`
}

type realWorldIssueSummary struct {
	Total      int `json:"total"`
	Parsing    int `json:"parsing"`
	Validation int `json:"validation"`
	Schema     int `json:"schema"`
	Coverage   int `json:"coverage"`
}

type realWorldTestedRepo struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	CloneURL     string   `json:"cloneURL,omitempty"`
	Ecosystems   []string `json:"ecosystems,omitempty"`
	Entries      []string `json:"entries"`
	Commits      []string `json:"commits,omitempty"`
	LastTested   string   `json:"lastTested"`
	TestCount    int      `json:"testCount"`
	LatestCommit string   `json:"latestCommit,omitempty"`
}

type realWorldCandidateSearchSpec struct {
	Ecosystem string `json:"ecosystem"`
	Query     string `json:"query"`
}

type realWorldManifest struct {
	SchemaVersion             int                           `json:"schemaVersion"`
	CreatedAt                 string                        `json:"createdAt"`
	Title                     string                        `json:"title,omitempty"`
	CorpusDir                 string                        `json:"corpusDir"`
	CacheDir                  string                        `json:"cacheDir"`
	OutputArtifact            string                        `json:"outputArtifact"`
	PreparationRunID          string                        `json:"preparationRunID,omitempty"`
	PreparationStartedAt      string                        `json:"preparationStartedAt,omitempty"`
	PreparationCompletedAt    string                        `json:"preparationCompletedAt,omitempty"`
	PreparationComplete       bool                          `json:"preparationComplete,omitempty"`
	PreparationManaged        bool                          `json:"preparationManaged,omitempty"`
	DependencyPrep            []realWorldDependencyPrep     `json:"dependencyPrep,omitempty"`
	DependencyPrepInspection  []realWorldDependencyPrepScan `json:"dependencyPrepInspection,omitempty"`
	DependencyPrepSummary     string                        `json:"dependencyPrepSummary,omitempty"`
	DependencyPrepNeedsReview bool                          `json:"dependencyPrepNeedsReview,omitempty"`
	PrepSecurityPolicy        map[string]any                `json:"prepSecurityPolicy,omitempty"`
	Repositories              []realWorldRepository         `json:"repositories"`
}

type realWorldRecordArgs struct {
	RunID                  string                           `json:"runID"`
	ID                     string                           `json:"id"`
	Date                   string                           `json:"date"`
	Title                  string                           `json:"title"`
	DollarLintRevision     string                           `json:"dollarlintRevision"`
	WorkingTreeNote        string                           `json:"workingTreeNote"`
	Corpus                 string                           `json:"corpus"`
	CacheDir               string                           `json:"cacheDir"`
	Command                string                           `json:"command"`
	OutputArtifact         string                           `json:"outputArtifact"`
	ManifestPath           string                           `json:"manifestPath"`
	Repositories           []realWorldRepository            `json:"repositories"`
	DependencyPrep         []realWorldDependencyPrep        `json:"dependencyPrep"`
	ValidationFeedback     []realWorldValidationFeedback    `json:"validationFeedback"`
	Findings               []string                         `json:"findings"`
	ProductRecommendations []realWorldProductRecommendation `json:"productRecommendations"`
	ProductDecisions       []string                         `json:"productDecisions"`
	FollowUp               []string                         `json:"followUp"`
	Replace                bool                             `json:"replace"`
}

func (s *repoServer) handleRealWorldStartTesting(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldStartTestingArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldStartTesting(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) realWorldStartTesting(ctx context.Context, args realWorldStartTestingArgs) (map[string]any, error) {
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return nil, err
	}
	targetCount := boundedRealWorldCandidateTarget(args.TargetCount)
	tested := realWorldTestedRepos(history)
	repositories, duplicates := realWorldAnnotateCandidateRepositories(history, args.Repositories)
	var candidateSet *realWorldCandidateSet
	if len(repositories) > 0 {
		set := newRealWorldCandidateSet(args.Title, args.AllowPreviouslyTested, repositories)
		if err := saveRealWorldCandidateSet(set); err != nil {
			return nil, err
		}
		candidateSet = &set
	}
	status := s.output(ctx, "git status --short")
	workingTreeNote := "clean working tree"
	if hasDirtyStatus(status) {
		workingTreeNote = "dirty working tree: " + strings.Join(lines(status), "; ")
	}
	revision := strings.TrimSpace(s.output(ctx, "git rev-parse HEAD"))
	ok := len(duplicates) == 0 || args.AllowPreviouslyTested
	next := realWorldNextChooseRepositories()
	if len(repositories) > 0 && ok {
		next = realWorldNextPrepareCorpus(args.Title, repositories, realWorldCandidateSetID(candidateSet), args.AllowPreviouslyTested)
	}
	message := "Candidate discovery is required before corpus preparation."
	if len(repositories) > 0 && ok {
		message = "Candidate repositories are ready; call real_world_prepare_corpus next."
	}
	if len(duplicates) > 0 && !args.AllowPreviouslyTested {
		ok = false
		message = "One or more candidate repositories were already tested; apply a candidate diff with replacements or restart with allowPreviouslyTested=true for an intentional rerun."
		next = realWorldNextUpdateCandidates(realWorldCandidateSetID(candidateSet), duplicates)
	}
	if len(repositories) == 0 {
		ok = false
		message = "No candidate repositories were provided. Call real_world_discover_candidates with the target count; do not search GitHub or hand-pick repositories manually."
		next = realWorldNextDiscoverCandidates(args.Title, targetCount, args.AllowPreviouslyTested)
	}
	out := map[string]any{
		"ok":                    ok,
		"message":               message,
		"title":                 args.Title,
		"targetCount":           targetCount,
		"dollarlintRevision":    revision,
		"workingTreeNote":       workingTreeNote,
		"entryCount":            len(history.Entries),
		"repoCount":             len(tested),
		"candidateRepositories": realWorldPublicRepositories(repositories),
		"duplicates":            realWorldPublicRepositories(duplicates),
		"rules": []string{
			"Do not create or update Markdown report files for repository memory.",
			"Do not wait for every repository to clone before starting validation; managed validation can begin from the manifest while corpus preparation continues.",
			"Record dependency preparation in dependencyPrep, including skipped or not-needed prep.",
			"Do not run dependency lifecycle scripts, postinstall hooks, package-manager plugins, or repository install scripts during dependency prep.",
			"Use managed per-repository validation tools for long runs; do not monitor validation with shell sleep loops.",
			"Record structured validationFeedback for each returned per-repository result before finishing validation.",
			"Assess each repository like a developer trying DollarLint: judge correctness, clarity, noise, skipped coverage, and whether the output helps decide what to do next.",
			"Record recommendations in productRecommendations with strength, recommendation, and rationale.",
			"Record product changes or decisions in productDecisions; use a no-change note when nothing changed.",
			"After recording, the final user message must either recommend product changes to consider or state that the product behaved reasonably.",
		},
		"nextStep": next,
	}
	if args.IncludeTestedRepos {
		testedRepos, truncated := limitedRealWorldTestedRepos(tested, args.TestedRepoLimit)
		out["testedRepos"] = testedRepos
		out["testedReposLimit"] = boundedRealWorldHistoryLimit(args.TestedRepoLimit)
		out["testedReposTruncated"] = truncated
	} else {
		out["testedReposOmitted"] = "Default responses omit the full tested repository index to keep MCP output small. Pass candidate repositories to this tool for duplicate checks, or call real_world_history with specific repositories."
	}
	if candidateSet != nil {
		out["candidateSetID"] = candidateSet.ID
		out["candidateSet"] = realWorldCandidateSetSummary(*candidateSet, duplicates, 0)
	}
	return out, nil
}

func (s *repoServer) handleRealWorldDiscoverCandidates(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldDiscoverCandidatesArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldDiscoverCandidates(ctx, request, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) realWorldDiscoverCandidates(ctx context.Context, request mcp.CallToolRequest, args realWorldDiscoverCandidatesArgs) (map[string]any, error) {
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return nil, err
	}
	targetCount := boundedRealWorldCandidateTarget(args.TargetCount)
	title := strings.TrimSpace(args.Title)
	if title == "" {
		title = fmt.Sprintf("%d-repo real-world sweep", targetCount)
	}
	specs := realWorldCandidateDiscoverySpecs(args.Ecosystems, args.MinStars)
	perQueryLimit := boundedRealWorldDiscoveryPerQuery(args.PerQueryLimit, targetCount, len(specs))
	p := newProgress(ctx, s.mcp, request, len(specs)+1)

	groups := make([][]realWorldRepository, 0, len(specs))
	searchResults := make([]map[string]any, 0, len(specs))
	var firstSearchErr error
	for _, spec := range specs {
		p.step("Searching GitHub repositories for " + spec.Ecosystem)
		repositories, err := realWorldSearchGitHubRepositories(ctx, s.root, spec, perQueryLimit)
		result := map[string]any{
			"ecosystem": spec.Ecosystem,
			"query":     spec.Query,
			"limit":     perQueryLimit,
			"returned":  len(repositories),
		}
		if err != nil {
			result["error"] = err.Error()
			if firstSearchErr == nil {
				firstSearchErr = err
			}
		} else {
			groups = append(groups, repositories)
		}
		searchResults = append(searchResults, result)
	}

	selected, omittedAlreadyTested, omittedCandidateDuplicates := realWorldSelectDiscoveredCandidates(history, groups, targetCount, args.AllowPreviouslyTested)
	if len(selected) == 0 && firstSearchErr != nil {
		return nil, fmt.Errorf("discover real-world candidates through GitHub API: %w", firstSearchErr)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("discover real-world candidates: GitHub search returned no usable repositories")
	}
	set := newRealWorldCandidateSet(title, args.AllowPreviouslyTested, selected)
	if err := saveRealWorldCandidateSet(set); err != nil {
		return nil, err
	}
	p.step(fmt.Sprintf("Saved candidate set with %d repositories", len(selected)))

	message := fmt.Sprintf("Discovered %d real-world candidate repositories mechanically through GitHub search and saved a candidate set.", len(selected))
	if len(selected) < targetCount {
		message = fmt.Sprintf("Discovered %d real-world candidate repositories for a target of %d; proceed with the saved candidate set or call this tool again with broader filters.", len(selected), targetCount)
	}
	next := realWorldNextPrepareCorpus(title, selected, set.ID, args.AllowPreviouslyTested)
	out := map[string]any{
		"ok":                             true,
		"message":                        message,
		"title":                          title,
		"targetCount":                    targetCount,
		"selectedCount":                  len(selected),
		"candidateSetID":                 set.ID,
		"candidateSet":                   realWorldCandidateSetSummary(set, nil, len(selected)),
		"candidatePreview":               realWorldRepositoryPreview(selected, 25),
		"candidatePreviewLimit":          25,
		"omittedAlreadyTestedCount":      len(omittedAlreadyTested),
		"omittedCandidateDuplicateCount": omittedCandidateDuplicates,
		"allowPreviouslyTested":          args.AllowPreviouslyTested,
		"searchPlan":                     searchResults,
		"nextStep":                       next,
		"selectionRules": []string{
			"Candidate discovery is MCP-managed; do not run shell GitHub searches or manually replace this list unless a later MCP duplicate check asks for a small diff.",
			"Use candidateSetID with real_world_prepare_corpus rather than resubmitting the full candidate list.",
		},
	}
	if len(omittedAlreadyTested) > 0 {
		out["omittedAlreadyTestedPreview"] = realWorldRepositoryPreview(omittedAlreadyTested, 25)
	}
	return out, nil
}

func (s *repoServer) handleRealWorldUpdateCandidates(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldUpdateCandidatesArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldUpdateCandidates(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) realWorldUpdateCandidates(ctx context.Context, args realWorldUpdateCandidatesArgs) (map[string]any, error) {
	if strings.TrimSpace(args.CandidateSetID) == "" {
		return nil, fmt.Errorf("candidateSetID is required; call real_world_start_testing first")
	}
	set, err := loadRealWorldCandidateSet(args.CandidateSetID)
	if err != nil {
		return nil, err
	}
	if args.Title != "" {
		set.Title = args.Title
	}
	if args.AllowPreviouslyTested != nil {
		set.AllowPreviouslyTested = *args.AllowPreviouslyTested
	}
	updated, diffResult, err := applyRealWorldCandidateDiff(set.Repositories, args.Diff)
	if err != nil {
		return nil, err
	}
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return nil, err
	}
	set.Repositories, _ = realWorldAnnotateCandidateRepositories(history, updated)
	set.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := saveRealWorldCandidateSet(set); err != nil {
		return nil, err
	}
	_, duplicates := realWorldAnnotateCandidateRepositories(history, set.Repositories)
	countOK := args.ExpectedCount <= 0 || len(set.Repositories) == args.ExpectedCount
	ok := countOK && (len(duplicates) == 0 || set.AllowPreviouslyTested)
	message := "Candidate set updated; call real_world_prepare_corpus with candidateSetID next."
	next := realWorldNextPrepareCorpus(set.Title, set.Repositories, set.ID, set.AllowPreviouslyTested)
	if len(duplicates) > 0 && !set.AllowPreviouslyTested {
		message = "Candidate set still contains previously tested repositories; apply another small diff or set allowPreviouslyTested for an intentional rerun."
		next = realWorldNextUpdateCandidates(set.ID, duplicates)
	} else if !countOK {
		message = fmt.Sprintf("Candidate set has %d repositories, expected %d; apply another diff before preparing the corpus.", len(set.Repositories), args.ExpectedCount)
		next = realWorldNextUpdateCandidates(set.ID, nil)
	}
	return map[string]any{
		"ok":                    ok,
		"message":               message,
		"candidateSetID":        set.ID,
		"candidateSet":          realWorldCandidateSetSummary(set, duplicates, args.ExpectedCount),
		"candidateRepositories": realWorldPublicRepositories(set.Repositories),
		"duplicates":            realWorldPublicRepositories(duplicates),
		"diff":                  diffResult,
		"nextStep":              next,
	}, nil
}

func (s *repoServer) handleRealWorldHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldHistoryArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldHistory(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) realWorldHistory(args realWorldHistoryArgs) (map[string]any, error) {
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return nil, err
	}
	tested := realWorldTestedRepos(history)
	queries := append([]string{}, args.Repositories...)
	if args.Repo != "" {
		queries = append([]string{args.Repo}, queries...)
	}
	queryResults := make([]map[string]any, 0, len(queries))
	for _, query := range queries {
		matches := realWorldRepoMatches(history, query)
		queryResults = append(queryResults, map[string]any{
			"query":         query,
			"alreadyTested": len(matches) > 0,
			"matches":       matches,
		})
	}
	out := map[string]any{
		"metadataSchema": realWorldEntrySchema,
		"schemaVersion":  history.SchemaVersion,
		"layout":         realWorldRunsDirRelPath + "/<run-id>/" + realWorldRunMetadataFileName,
		"entryCount":     len(history.Entries),
		"repoCount":      len(tested),
		"queries":        queryResults,
	}
	if args.IncludeTestedRepos {
		testedRepos, truncated := limitedRealWorldTestedRepos(tested, args.Limit)
		out["testedRepos"] = testedRepos
		out["testedReposLimit"] = boundedRealWorldHistoryLimit(args.Limit)
		out["testedReposTruncated"] = truncated
	} else {
		out["testedReposOmitted"] = "Default history responses omit the full tested repository index. Query specific repositories with repo/repositories, or pass includeTestedRepos=true with limit for a capped sample."
	}
	if args.IncludeEntries {
		out["entries"] = history.Entries
	}
	out["nextStep"] = realWorldNextChooseRepositories()
	return out, nil
}

func (s *repoServer) handleRealWorldPrepareCorpus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldPrepareCorpusArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldPrepareCorpus(ctx, request, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) realWorldPrepareCorpus(ctx context.Context, request mcp.CallToolRequest, args realWorldPrepareCorpusArgs) (map[string]any, error) {
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return nil, err
	}
	allowPreviouslyTested := args.AllowPreviouslyTested != nil && *args.AllowPreviouslyTested
	var candidateSet *realWorldCandidateSet
	repositories := append([]realWorldRepository{}, args.Repositories...)
	if strings.TrimSpace(args.CandidateSetID) != "" {
		if len(args.Repositories) > 0 {
			return nil, fmt.Errorf("omit repositories when candidateSetID is provided; use candidateDiff for small changes")
		}
		set, err := loadRealWorldCandidateSet(args.CandidateSetID)
		if err != nil {
			return nil, err
		}
		if args.Title != "" {
			set.Title = args.Title
		}
		if args.AllowPreviouslyTested != nil {
			set.AllowPreviouslyTested = *args.AllowPreviouslyTested
		}
		if !realWorldCandidateDiffEmpty(args.CandidateDiff) {
			updated, _, err := applyRealWorldCandidateDiff(set.Repositories, args.CandidateDiff)
			if err != nil {
				return nil, err
			}
			set.Repositories = updated
		}
		set.Repositories, _ = realWorldAnnotateCandidateRepositories(history, set.Repositories)
		set.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := saveRealWorldCandidateSet(set); err != nil {
			return nil, err
		}
		candidateSet = &set
		repositories = set.Repositories
		if args.Title == "" {
			args.Title = set.Title
		}
		allowPreviouslyTested = set.AllowPreviouslyTested
	} else if !realWorldCandidateDiffEmpty(args.CandidateDiff) {
		return nil, fmt.Errorf("candidateDiff requires candidateSetID")
	}
	repositories, duplicates := realWorldAnnotateCandidateRepositories(history, repositories)
	countOK := args.ExpectedCount <= 0 || len(repositories) == args.ExpectedCount
	if len(duplicates) > 0 && !allowPreviouslyTested {
		out := map[string]any{
			"ok":         false,
			"message":    "one or more repositories were already tested; apply a candidate diff or pass allowPreviouslyTested=true for an intentional rerun",
			"duplicates": duplicates,
			"nextStep":   realWorldNextChooseRepositories(),
		}
		if candidateSet != nil {
			candidateSet.Repositories = repositories
			out["candidateSetID"] = candidateSet.ID
			out["candidateSet"] = realWorldCandidateSetSummary(*candidateSet, duplicates, args.ExpectedCount)
			out["nextStep"] = realWorldNextUpdateCandidates(candidateSet.ID, duplicates)
		}
		return out, nil
	}
	if !countOK {
		out := map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("candidate set has %d repositories, expected %d; apply another diff before preparing the corpus", len(repositories), args.ExpectedCount),
		}
		if candidateSet != nil {
			candidateSet.Repositories = repositories
			out["candidateSetID"] = candidateSet.ID
			out["candidateSet"] = realWorldCandidateSetSummary(*candidateSet, duplicates, args.ExpectedCount)
			out["nextStep"] = realWorldNextUpdateCandidates(candidateSet.ID, nil)
		} else {
			out["nextStep"] = realWorldNextChooseRepositories()
		}
		return out, nil
	}
	if len(repositories) == 0 {
		return nil, fmt.Errorf("repositories or candidateSetID is required; call real_world_start_testing first if you need repository guidance")
	}
	corpusDir, err := os.MkdirTemp("", realWorldCorpusTempPrefix)
	if err != nil {
		return nil, err
	}
	cacheDir, err := os.MkdirTemp("", realWorldCacheTempPrefix)
	if err != nil {
		return nil, err
	}
	outputArtifact, err := createRealWorldOutputPath(args.Title, args.OutputName)
	if err != nil {
		return nil, err
	}

	if args.Clone {
		run, err := s.startRealWorldPrepareRun(realWorldStartPrepareArgs{
			Title:          args.Title,
			CorpusDir:      corpusDir,
			CacheDir:       cacheDir,
			OutputArtifact: outputArtifact,
			Repositories:   repositories,
			Concurrency:    args.Concurrency,
		})
		if err != nil {
			return nil, err
		}
		wait := false
		if args.WaitForFirstResult != nil {
			wait = *args.WaitForFirstResult
		}
		if wait {
			out, err := s.realWorldWaitForPreparedRepo(ctx, request, run)
			if err != nil {
				return nil, err
			}
			out["ok"] = true
			out["started"] = true
			out["clone"] = true
			if candidateSet != nil {
				out["candidateSetID"] = candidateSet.ID
				out["candidateSet"] = realWorldCandidateSetSummary(*candidateSet, duplicates, args.ExpectedCount)
			}
			return out, nil
		}
		out := map[string]any{
			"ok":               true,
			"started":          true,
			"clone":            true,
			"repositoryCount":  len(run.repositories()),
			"duplicates":       realWorldPublicRepositories(duplicates),
			"run":              run.snapshot(),
			"nextStep":         realWorldNextRunCorpusDuringPrepare(run),
			"preparedRepoStep": realWorldNextPreparedRepo(run.ID),
		}
		if candidateSet != nil {
			out["candidateSetID"] = candidateSet.ID
			out["candidateSet"] = realWorldCandidateSetSummary(*candidateSet, duplicates, args.ExpectedCount)
		}
		return out, nil
	}

	cloneCommands := make([]string, 0, len(repositories))
	cloneResults := make([]commandResult, 0, len(repositories))
	ok := true
	for i := range repositories {
		targetName := slugify(nonEmpty(repositories[i].Name, repoNameFromURL(repositories[i].CloneURL)))
		if targetName == "" {
			targetName = fmt.Sprintf("repo-%d", i+1)
		}
		target := filepath.Join(corpusDir, targetName)
		repositories[i].Path = target
		repositories[i].Status = "pending"
		cloneCommands = append(cloneCommands, fmt.Sprintf("git clone --depth 1 --quiet %s %s", shellQuote(repositories[i].CloneURL), shellQuote(target)))
		if !args.Clone {
			continue
		}
		result := runProcess(ctx, s.root, nil, "git", "clone", "--depth", "1", "--quiet", repositories[i].CloneURL, target)
		cloneResults = append(cloneResults, result)
		if !result.Succeeded {
			ok = false
			repositories[i].Status = "error"
			repositories[i].Error = result.Output
			continue
		}
		repositories[i].Status = "cloned"
		commit := runProcess(ctx, target, nil, "git", "rev-parse", "HEAD")
		if commit.Succeeded {
			repositories[i].Commit = strings.TrimSpace(commit.Output)
		}
	}

	manifest := realWorldManifest{
		SchemaVersion:  1,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		Title:          args.Title,
		CorpusDir:      corpusDir,
		CacheDir:       cacheDir,
		OutputArtifact: outputArtifact,
		Repositories:   repositories,
	}
	manifestPath := filepath.Join(corpusDir, realWorldManifestName)
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		return nil, err
	}
	inspection, inspectErr := realWorldInspectCorpus(realWorldInspectArgs{
		CorpusDir:      corpusDir,
		CacheDir:       cacheDir,
		OutputArtifact: outputArtifact,
		ManifestPath:   manifestPath,
		Repositories:   repositories,
	})
	nextStep := realWorldNextInspectCorpus("", corpusDir, manifestPath)
	if inspectErr == nil {
		nextStep = realWorldNextRunCorpus("", corpusDir, cacheDir, outputArtifact, manifestPath, inspection.DraftDependencyPrep)
	}
	out := map[string]any{
		"ok":                ok,
		"corpusDir":         corpusDir,
		"cacheDir":          cacheDir,
		"outputArtifact":    outputArtifact,
		"manifestPath":      manifestPath,
		"repositories":      realWorldPublicRepositories(repositories),
		"duplicates":        realWorldPublicRepositories(duplicates),
		"clone":             args.Clone,
		"cloneCommands":     cloneCommands,
		"cloneResults":      cloneResults,
		"buildCommand":      "go build -o bin/dollarlint ./cmd/dollarlint",
		"validationCommand": realWorldValidationCommand(corpusDir, cacheDir, outputArtifact, true, "warn", 1, "1ms", "1ms", nil),
		"nextStep":          nextStep,
	}
	if candidateSet != nil {
		out["candidateSetID"] = candidateSet.ID
		out["candidateSet"] = realWorldCandidateSetSummary(*candidateSet, duplicates, args.ExpectedCount)
	}
	if inspectErr != nil {
		out["dependencyPrepInspectionError"] = inspectErr.Error()
	} else {
		out["dependencyPrepInspection"] = inspection.Repositories
		out["dependencyPrep"] = inspection.DraftDependencyPrep
		out["prepSecurityPolicy"] = inspection.PrepSecurityPolicy
		out["dependencyPrepSummary"] = inspection.Summary
		out["dependencyPrepNeedsReview"] = inspection.NeedsReview
	}
	return out, nil
}

func (s *repoServer) handleRealWorldRecordResult(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldRecordArgs
	_ = request.BindArguments(&args)
	if err := realWorldRejectManualPathArgsWithRunID(args.RunID, map[string]string{
		"corpus":         args.Corpus,
		"cacheDir":       args.CacheDir,
		"command":        args.Command,
		"outputArtifact": args.OutputArtifact,
		"manifestPath":   args.ManifestPath,
	}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if args.Title == "" && args.RunID == "" {
		return mcp.NewToolResultError("title is required unless runID is provided"), nil
	}
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	entry, err := s.realWorldEntryFromArgs(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateRealWorldEntryForRecord(entry); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	persistedOutputArtifact, err := persistRealWorldOutputArtifact(s.root, entry)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	entry.PersistedOutputArtifact = persistedOutputArtifact
	replaced := false
	for i := range history.Entries {
		if history.Entries[i].ID != entry.ID {
			continue
		}
		if !args.Replace {
			return mcp.NewToolResultError(fmt.Sprintf("entry %q already exists; pass replace=true to update it", entry.ID)), nil
		}
		history.Entries[i] = entry
		replaced = true
		break
	}
	if !replaced {
		history.Entries = append(history.Entries, entry)
	}
	if err := saveRealWorldHistory(s.root, history); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cleanup := realWorldCleanupRecordTemps(entry)
	if args.RunID != "" && s.realWorldRuns != nil {
		if run, ok := s.realWorldRuns.get(args.RunID); ok {
			cleanup = append(cleanup, realWorldCleanupTempDir("validationOutputDir", run.OutputDir, realWorldValidationRunTempPrefix))
		}
	}
	cleanup = append(cleanup, realWorldCleanupTempFile("outputArtifact", entry.OutputArtifact, realWorldOutputTempPrefix))
	return s.realWorldStructured(ctx, map[string]any{
		"ok":                      true,
		"entryPath":               realWorldEntryRelPath(entry),
		"entry":                   realWorldPublicEntry(entry),
		"persistedOutputArtifact": entry.PersistedOutputArtifact,
		"replaced":                replaced,
		"entryCount":              len(history.Entries),
		"repoCount":               len(entry.Repositories),
		"testedRepos":             realWorldTestedRepos(history),
		"cleanup":                 realWorldPublicCleanup(cleanup),
		"cleanupOK":               realWorldCleanupOK(cleanup),
		"nextStep":                realWorldNextAfterRecord(entry),
	})
}

func (s *repoServer) realWorldEntryFromArgs(args realWorldRecordArgs) (realWorldEntry, error) {
	date := args.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	revision := args.DollarLintRevision
	if revision == "" {
		revision = strings.TrimSpace(s.output(context.Background(), "git rev-parse HEAD"))
	}
	workingTreeNote := args.WorkingTreeNote
	if workingTreeNote == "" {
		status := s.output(context.Background(), "git status --short")
		if hasDirtyStatus(status) {
			workingTreeNote = "dirty working tree: " + strings.Join(lines(status), "; ")
		} else {
			workingTreeNote = "clean working tree"
		}
	}
	repositories := append([]realWorldRepository{}, args.Repositories...)
	if args.RunID != "" {
		if s.realWorldRuns == nil {
			return realWorldEntry{}, fmt.Errorf("validation run %q was not found", args.RunID)
		}
		run, ok := s.realWorldRuns.get(args.RunID)
		if !ok {
			return realWorldEntry{}, fmt.Errorf("validation run %q was not found", args.RunID)
		}
		run.refreshFromManifest()
		if args.Title == "" {
			args.Title = run.Title
		}
		if args.Corpus == "" {
			args.Corpus = run.CorpusDir
		}
		if args.CacheDir == "" {
			args.CacheDir = run.CacheDir
		}
		if args.Command == "" {
			args.Command = nonEmpty(run.Command, realWorldManagedValidationCommand(run))
		}
		if args.OutputArtifact == "" {
			args.OutputArtifact = run.OutputArtifact
		}
		if args.ManifestPath == "" {
			args.ManifestPath = run.ManifestPath
		}
		if len(repositories) == 0 {
			repositories = append([]realWorldRepository{}, run.Repositories...)
		}
		if len(args.DependencyPrep) == 0 {
			args.DependencyPrep = append([]realWorldDependencyPrep{}, run.DependencyPrep...)
		}
		if len(args.ValidationFeedback) == 0 {
			args.ValidationFeedback = run.validationFeedback()
		}
		if err := realWorldEnsureManagedOutputArtifact(run); err != nil {
			return realWorldEntry{}, err
		}
		args.OutputArtifact = run.OutputArtifact
	}
	manifestPath := args.ManifestPath
	if manifestPath == "" && args.Corpus != "" {
		manifestPath = filepath.Join(args.Corpus, realWorldManifestName)
	}
	if manifestPath != "" {
		manifest, err := readRealWorldManifest(manifestPath)
		if err == nil {
			if len(repositories) == 0 {
				repositories = manifest.Repositories
			}
			if len(args.DependencyPrep) == 0 {
				args.DependencyPrep = manifest.DependencyPrep
			}
			if args.Corpus == "" {
				args.Corpus = manifest.CorpusDir
			}
			if args.CacheDir == "" {
				args.CacheDir = manifest.CacheDir
			}
			if args.OutputArtifact == "" {
				args.OutputArtifact = manifest.OutputArtifact
			}
		}
	}
	var result *realWorldResult
	if args.OutputArtifact != "" {
		summary, _, err := readRealWorldOutputSummary(args.OutputArtifact)
		if err != nil {
			return realWorldEntry{}, err
		}
		result = summary
	}
	command := args.Command
	if command == "" && args.Corpus != "" && args.CacheDir != "" && args.OutputArtifact != "" {
		command = realWorldValidationCommand(args.Corpus, args.CacheDir, args.OutputArtifact, true, "warn", 1, "1ms", "1ms", nil)
	}
	id := args.ID
	if id == "" {
		var err error
		id, err = newRealWorldEntryID(date, args)
		if err != nil {
			return realWorldEntry{}, err
		}
	}
	return realWorldEntry{
		ID:                     id,
		Date:                   date,
		Title:                  args.Title,
		DollarLintRevision:     revision,
		WorkingTreeNote:        workingTreeNote,
		Corpus:                 args.Corpus,
		CacheDir:               args.CacheDir,
		Command:                command,
		OutputArtifact:         args.OutputArtifact,
		DependencyPrep:         args.DependencyPrep,
		ValidationFeedback:     args.ValidationFeedback,
		Repositories:           repositories,
		Result:                 result,
		Findings:               args.Findings,
		ProductRecommendations: args.ProductRecommendations,
		ProductDecisions:       args.ProductDecisions,
		FollowUp:               args.FollowUp,
	}, nil
}

func realWorldCleanupRecordTemps(entry realWorldEntry) []realWorldCleanupResult {
	var results []realWorldCleanupResult
	seen := map[string]bool{}
	for _, item := range []struct {
		kind   string
		path   string
		prefix string
	}{
		{kind: "corpusDir", path: entry.Corpus, prefix: realWorldCorpusTempPrefix},
		{kind: "cacheDir", path: entry.CacheDir, prefix: realWorldCacheTempPrefix},
	} {
		clean := filepath.Clean(item.path)
		if item.path != "" && seen[clean] {
			results = append(results, realWorldCleanupResult{
				Kind:   item.kind,
				Path:   item.path,
				Status: "skipped",
				Reason: "same path was already considered for cleanup",
			})
			continue
		}
		if item.path != "" {
			seen[clean] = true
		}
		results = append(results, realWorldCleanupTempDir(item.kind, item.path, item.prefix))
	}
	return results
}

func realWorldCleanupTempDir(kind, path, prefix string) realWorldCleanupResult {
	result := realWorldCleanupResult{Kind: kind, Path: path}
	ok, reason := realWorldManagedTempDir(path, prefix)
	if !ok {
		result.Status = "skipped"
		result.Reason = reason
		return result
	}
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		result.Status = "missing"
		return result
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.Status = "skipped"
		result.Reason = "managed temp path is a symlink"
		return result
	}
	if !info.IsDir() {
		result.Status = "skipped"
		result.Reason = "managed temp path is not a directory"
		return result
	}
	if err := os.RemoveAll(clean); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}
	result.Status = "removed"
	return result
}

func realWorldCleanupTempFile(kind, path, prefix string) realWorldCleanupResult {
	result := realWorldCleanupResult{Kind: kind, Path: path}
	ok, reason := realWorldManagedTempPath(path, prefix)
	if !ok {
		result.Status = "skipped"
		result.Reason = reason
		return result
	}
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		result.Status = "missing"
		return result
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.Status = "skipped"
		result.Reason = "managed temp path is a symlink"
		return result
	}
	if info.IsDir() {
		result.Status = "skipped"
		result.Reason = "managed temp path is a directory"
		return result
	}
	if err := os.Remove(clean); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}
	result.Status = "removed"
	return result
}

func realWorldManagedTempDir(path, prefix string) (bool, string) {
	return realWorldManagedTempPath(path, prefix)
}

func realWorldManagedTempPath(path, prefix string) (bool, string) {
	if path == "" {
		return false, "path is empty"
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return false, "path is not absolute"
	}
	tempDir := filepath.Clean(os.TempDir())
	rel, err := filepath.Rel(tempDir, clean)
	if err != nil {
		return false, err.Error()
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, "path is outside the OS temp directory"
	}
	if strings.Contains(rel, string(filepath.Separator)) {
		return false, "path is not a direct child of the OS temp directory"
	}
	if !strings.HasPrefix(filepath.Base(clean), prefix) {
		return false, fmt.Sprintf("path does not use managed temp prefix %q", prefix)
	}
	return true, ""
}

func realWorldCleanupOK(results []realWorldCleanupResult) bool {
	for _, result := range results {
		if result.Status == "failed" {
			return false
		}
	}
	return true
}

func realWorldPublicRepositories(repositories []realWorldRepository) []realWorldRepository {
	out := append([]realWorldRepository{}, repositories...)
	for i := range out {
		out[i].Path = ""
	}
	return out
}

func realWorldPublicCleanup(results []realWorldCleanupResult) []realWorldCleanupResult {
	out := append([]realWorldCleanupResult{}, results...)
	for i := range out {
		out[i].Path = ""
	}
	return out
}

func realWorldPublicEntry(entry realWorldEntry) map[string]any {
	return map[string]any{
		"id":                        entry.ID,
		"date":                      entry.Date,
		"title":                     entry.Title,
		"dollarlintRevision":        entry.DollarLintRevision,
		"workingTreeNote":           entry.WorkingTreeNote,
		"repositoryCount":           len(entry.Repositories),
		"result":                    entry.Result,
		"findings":                  entry.Findings,
		"productRecommendations":    entry.ProductRecommendations,
		"productDecisions":          entry.ProductDecisions,
		"followUp":                  entry.FollowUp,
		"validationFeedbackSummary": realWorldValidationFeedbackSummary(entry.ValidationFeedback),
	}
}

func realWorldPublicInspection(scans []realWorldDependencyPrepScan) []realWorldDependencyPrepScan {
	out := append([]realWorldDependencyPrepScan{}, scans...)
	for i := range out {
		out[i].Path = ""
	}
	return out
}

func realWorldPublicPrepareResult(result realWorldRepoPrepareResult) map[string]any {
	record := result.RepositoryRecord
	record.Path = ""
	out := map[string]any{
		"repository":       result.Repository,
		"cloneURL":         result.CloneURL,
		"commit":           result.Commit,
		"status":           result.Status,
		"succeeded":        result.Succeeded,
		"duration":         result.Duration,
		"output":           result.Output,
		"error":            result.Error,
		"startedAt":        result.StartedAt,
		"finishedAt":       result.FinishedAt,
		"repositoryRecord": record,
	}
	if result.DependencyPrepInspection != nil {
		scan := *result.DependencyPrepInspection
		scan.Path = ""
		out["dependencyPrepInspection"] = scan
	}
	if result.DependencyPrep != nil {
		out["dependencyPrep"] = result.DependencyPrep
	}
	if result.PrepSecurityPolicy != nil {
		out["prepSecurityPolicy"] = result.PrepSecurityPolicy
	}
	return out
}

func realWorldPublicValidationResult(run *realWorldValidationRun, result realWorldRepoValidationResult) map[string]any {
	out := map[string]any{
		"repository": result.Repository,
		"exitCode":   result.ExitCode,
		"duration":   result.Duration,
		"succeeded":  result.Succeeded,
		"accepted":   result.Accepted,
		"summary":    result.Summary,
		"warnings":   result.Warnings,
		"output":     result.Output,
		"error":      result.Error,
		"startedAt":  result.StartedAt,
		"finishedAt": result.FinishedAt,
	}
	if artifactPath := realWorldPublicValidationArtifactPath(run, result.OutputArtifact); artifactPath != "" {
		out["fullArtifactPath"] = artifactPath
		if evidence, err := realWorldValidationEvidence(result.Repository, result.OutputArtifact); err == nil {
			out["evidence"] = evidence
		} else {
			out["evidenceReadError"] = err.Error()
		}
	}
	out["feedbackInstructions"] = []string{
		"Use the evidence bundle, especially cliPreview, exampleIssues, skippedGroups, and source excerpts, to assess DollarLint like a developer using the tool.",
		"Feedback must explain correctness, ergonomics, coverage, and actionability in words; raw counts alone are rejected.",
		"Choose product-signal for product changes worth considering, behaved-reasonably only with concrete evidence, or blocked when the repo result is uninterpretable.",
	}
	return out
}

func realWorldPublicValidationArtifactPath(run *realWorldValidationRun, outputArtifact string) string {
	if run == nil || outputArtifact == "" {
		return ""
	}
	if ok, _ := realWorldManagedTempDir(run.OutputDir, realWorldValidationRunTempPrefix); !ok {
		return ""
	}
	cleanDir := filepath.Clean(run.OutputDir)
	cleanArtifact := filepath.Clean(outputArtifact)
	rel, err := filepath.Rel(cleanDir, cleanArtifact)
	if err != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return ""
	}
	return cleanArtifact
}

func realWorldPublicRepositoryTriage(items []realWorldRepositoryTriage) []realWorldRepositoryTriage {
	out := append([]realWorldRepositoryTriage{}, items...)
	for i := range out {
		out[i].Path = ""
	}
	return out
}

func validateRealWorldEntryForRecord(entry realWorldEntry) error {
	var missing []string
	if entry.Title == "" {
		missing = append(missing, "title")
	}
	if entry.DollarLintRevision == "" {
		missing = append(missing, "dollarlintRevision")
	}
	if entry.WorkingTreeNote == "" {
		missing = append(missing, "workingTreeNote")
	}
	if entry.Corpus == "" {
		missing = append(missing, "corpus")
	}
	if entry.CacheDir == "" {
		missing = append(missing, "cacheDir")
	}
	if entry.Command == "" {
		missing = append(missing, "command")
	}
	if entry.OutputArtifact == "" {
		missing = append(missing, "outputArtifact")
	}
	if len(entry.Repositories) == 0 {
		missing = append(missing, "repositories")
	}
	if len(entry.DependencyPrep) == 0 {
		missing = append(missing, "dependencyPrep")
	}
	if len(entry.ValidationFeedback) == 0 {
		missing = append(missing, "validationFeedback")
	}
	if len(entry.Findings) == 0 {
		missing = append(missing, "findings")
	}
	if len(entry.ProductRecommendations) == 0 {
		missing = append(missing, "productRecommendations")
	}
	if len(entry.ProductDecisions) == 0 {
		missing = append(missing, "productDecisions")
	}
	if len(entry.FollowUp) == 0 {
		missing = append(missing, "followUp")
	}
	if len(missing) > 0 {
		return fmt.Errorf("real-world result is incomplete; provide %s", strings.Join(missing, ", "))
	}
	for i, prep := range entry.DependencyPrep {
		if prep.Status == "" || prep.Notes == "" {
			return fmt.Errorf("dependencyPrep[%d] must include status and notes", i)
		}
	}
	for i, feedback := range entry.ValidationFeedback {
		if err := validateRealWorldValidationFeedback(feedback); err != nil {
			return fmt.Errorf("validationFeedback[%d]: %w", i, err)
		}
	}
	for i, recommendation := range entry.ProductRecommendations {
		if err := validateRealWorldProductRecommendation(recommendation); err != nil {
			return fmt.Errorf("productRecommendations[%d]: %w", i, err)
		}
	}
	if realWorldFeedbackHasProductSignal(entry.ValidationFeedback) && !realWorldHasActionableRecommendation(entry.ProductRecommendations) {
		return fmt.Errorf("productRecommendations must include at least one actionable non-no-change recommendation because validationFeedback contains product-signal outcomes")
	}
	return nil
}

func validateRealWorldValidationFeedback(feedback realWorldValidationFeedback) error {
	if feedback.Repository == "" {
		return fmt.Errorf("repository is required")
	}
	switch feedback.Outcome {
	case realWorldFeedbackBehavedReasonably, realWorldFeedbackProductSignal, realWorldFeedbackBlocked:
	default:
		return fmt.Errorf("outcome must be %s, %s, or %s", realWorldFeedbackBehavedReasonably, realWorldFeedbackProductSignal, realWorldFeedbackBlocked)
	}
	if len(feedback.Findings) == 0 &&
		len(feedback.ProductRecommendations) == 0 &&
		len(feedback.ProductDecisions) == 0 &&
		len(feedback.Caveats) == 0 &&
		len(feedback.FollowUp) == 0 &&
		feedback.Notes == "" {
		return fmt.Errorf("include findings, notes, caveats, follow-up, or product recommendations")
	}
	if feedback.Outcome == realWorldFeedbackProductSignal && len(feedback.Findings) == 0 && len(feedback.ProductRecommendations) == 0 {
		return fmt.Errorf("product-signal feedback must include at least one finding or product recommendation")
	}
	if feedback.Outcome == realWorldFeedbackBehavedReasonably && len(feedback.Findings) == 0 && len(feedback.Caveats) == 0 {
		return fmt.Errorf("behaved-reasonably feedback must include at least one finding or caveat explaining the developer experience and why no product change is warranted")
	}
	if !realWorldFeedbackHasQualitativeEvidence(feedback) {
		return fmt.Errorf("feedback must include qualitative developer-experience evidence; raw counts or artifact references alone are not enough")
	}
	for i, recommendation := range feedback.ProductRecommendations {
		if err := validateRealWorldProductRecommendation(recommendation); err != nil {
			return fmt.Errorf("productRecommendations[%d]: %w", i, err)
		}
	}
	return nil
}

func realWorldFeedbackHasProductSignal(feedback []realWorldValidationFeedback) bool {
	for _, item := range feedback {
		if item.Outcome == realWorldFeedbackProductSignal {
			return true
		}
	}
	return false
}

func realWorldHasActionableRecommendation(recommendations []realWorldProductRecommendation) bool {
	for _, recommendation := range recommendations {
		if !isNoChangeRecommendation(recommendation) {
			return true
		}
	}
	return false
}

func realWorldFeedbackHasQualitativeEvidence(feedback realWorldValidationFeedback) bool {
	for _, text := range feedback.Findings {
		if realWorldQualitativeEvidenceText(text) {
			return true
		}
	}
	for _, text := range feedback.Caveats {
		if realWorldQualitativeEvidenceText(text) {
			return true
		}
	}
	for _, text := range feedback.FollowUp {
		if realWorldQualitativeEvidenceText(text) {
			return true
		}
	}
	if realWorldQualitativeEvidenceText(feedback.Notes) {
		return true
	}
	for _, recommendation := range feedback.ProductRecommendations {
		if realWorldQualitativeEvidenceText(recommendation.Recommendation) && realWorldQualitativeEvidenceText(recommendation.Rationale) {
			return true
		}
	}
	return false
}

func realWorldQualitativeEvidenceText(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 40 {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "see merged artifact") || strings.Contains(lower, "see artifact") {
		return false
	}
	fields := strings.Fields(text)
	if len(fields) < 7 {
		return false
	}
	metricTokens := 0
	for _, field := range fields {
		field = strings.Trim(field, ".,;:()[]{}")
		if strings.Contains(field, "=") {
			metricTokens++
		}
	}
	return metricTokens < len(fields)
}

func validateRealWorldProductRecommendation(recommendation realWorldProductRecommendation) error {
	if recommendation.Strength != "high" && recommendation.Strength != "med" && recommendation.Strength != "low" {
		return fmt.Errorf("strength must be high, med, or low")
	}
	if recommendation.Recommendation == "" || recommendation.Rationale == "" {
		return fmt.Errorf("must include recommendation and rationale")
	}
	return nil
}

func realWorldRecordResultContract() map[string]any {
	return map[string]any{
		"required": []string{
			"dollarlintRevision",
			"workingTreeNote",
			"validationFeedback",
			"findings",
			"productRecommendations",
			"productDecisions",
			"followUp",
		},
		"managedFlow":             "Pass runID from real_world_start_validation/real_world_finish_validation; the MCP server fills corpus, cacheDir, command, outputArtifact, repositories, dependencyPrep, and validationFeedback from managed run state.",
		"manualFlowExtraRequired": []string{"title", "corpus", "cacheDir", "command", "outputArtifact", "repositories", "dependencyPrep"},
		"dependencyPrep":          "Include every dependency-prep command that ran, failed, timed out, was narrowed, was skipped, or was not needed. Each item needs status and notes.",
		"validationFeedback":      "Include the structured per-repository developer-experience feedback recorded while reviewing managed validation results. Each item needs repository, outcome behaved-reasonably|product-signal|blocked, and evidence.",
		"assessmentPerspective":   realWorldDeveloperExperienceGuidance(),
		"productRecommendations":  "Use objects with strength high|med|low, recommendation, and rationale. If there is no genuine product change to consider, record an explicit no-change recommendation and use the productBehavedReasonably final-response outcome.",
		"productDecisions":        "Use for product changes or decisions made after triage. If none were made, include an explicit no-change decision.",
		"result":                  "Read automatically from outputArtifact when outputArtifact is provided.",
		"persistedOutputArtifact": "Written automatically from outputArtifact into the run directory as " + realWorldRunArtifactFileName + ".",
		"cleanup":                 "After recording succeeds, managed temp corpus/cache dirs are removed automatically. Non-temp paths are skipped.",
		"repositories":            "Read automatically from manifestPath when repositories is omitted and a manifest is available.",
		"finalResponseContract":   realWorldFinalResponseContract(),
	}
}

func realWorldDeveloperExperienceGuidance() map[string]any {
	return map[string]any{
		"perspective": "Assess the result like a developer trying DollarLint on a real repository, not like a schema oracle. Judge whether the tool was correct, clear, actionable, appropriately quiet, and honest about coverage.",
		"lookFor": []string{
			"Correctness: crashes, wrong schemas, misleading validation, parsing files the tool should understand, or treating generated/templated inputs in a confusing way.",
			"Ergonomics: noisy repeated findings, warnings that require too much interpretation, unclear skipped-file coverage, missing grouping, or output that leaves the user unsure what to do next.",
			"Coverage: large skipped-file counts, missing dependency-local schemas, blocked repositories, or any caveat that limits confidence in the result.",
			"Good behavior: focused findings, clear caveats, expected invalid fixture handling, and no obvious product improvement after checking representative issues and warnings.",
		},
		"outcomeBar": map[string]string{
			realWorldFeedbackProductSignal:     "Use when the developer experience suggests a product change, even if DollarLint technically produced valid output.",
			realWorldFeedbackBehavedReasonably: "Use only after noting concrete evidence that the result was understandable, expected, and did not suggest a product improvement.",
			realWorldFeedbackBlocked:           "Use when checkout, dependency prep, tool failure, or missing environment support makes the repository result uninterpretable.",
		},
		"goodFeedbackExamples": []map[string]any{
			{
				"outcome":        realWorldFeedbackProductSignal,
				"finding":        "Helm chart templates produced many YAML parse errors outside obvious invalid fixtures.",
				"recommendation": "Consider detecting or specially classifying templated YAML so users are not left with raw parser errors for common Helm files.",
			},
			{
				"outcome":        realWorldFeedbackProductSignal,
				"finding":        "Most discovered files were skipped and the result did not make the skipped reasons easy to scan by repository.",
				"recommendation": "Surface skipped-file coverage by repository and reason in triage/final output.",
			},
			{
				"outcome":        realWorldFeedbackProductSignal,
				"finding":        "The same catalog schema failed to compile across many files, creating repeated warnings with one underlying cause.",
				"recommendation": "Group repeated catalog-schema warnings by schema/source and explain validation impact once.",
			},
			{
				"outcome": realWorldFeedbackBehavedReasonably,
				"finding": "The only failures were intentionally invalid test fixtures under testdata, and normal repo configuration files validated or skipped with clear reasons.",
				"caveat":  "Coverage was small, so this repo alone should not be used as broad product evidence.",
			},
			{
				"outcome":  realWorldFeedbackBlocked,
				"finding":  "Checkout failed before validation because git-lfs was missing.",
				"followUp": "Install git-lfs and rerun before drawing a DollarLint product conclusion.",
			},
		},
		"antiPatterns": []string{
			"Do not mark a repo behaved-reasonably with only 'accepted' or 'see merged artifact'.",
			"Do not treat a technically valid validation error as automatically good product UX.",
			"Do not ignore skipped coverage, repeated warnings, or confusing classifications just because the command exited with structured JSON.",
		},
	}
}

func realWorldNextChooseRepositories() map[string]any {
	return map[string]any{
		"tool": "real_world_start_testing",
		"why":  "Start the managed real-world testing flow with literal candidate repositories when provided; otherwise request MCP-managed candidate discovery.",
		"requiredArgs": []string{
			"title",
		},
		"optionalArgs": []string{
			"repositories",
			"targetCount",
			"allowPreviouslyTested",
		},
	}
}

func realWorldNextDiscoverCandidates(title string, targetCount int, allowPreviouslyTested bool) map[string]any {
	return map[string]any{
		"tool": "real_world_discover_candidates",
		"why":  "Let the MCP server mechanically discover a broad public candidate set, de-duplicate it against real-world history, and save it for preparation.",
		"requiredArgs": []string{
			"title",
			"targetCount",
		},
		"suggestedArgs": map[string]any{
			"title":                 title,
			"targetCount":           boundedRealWorldCandidateTarget(targetCount),
			"allowPreviouslyTested": allowPreviouslyTested,
		},
		"rules": []string{
			"Do not run gh search, gh repo list, or ad hoc web searches for candidate selection.",
			"Use the returned candidateSetID with real_world_prepare_corpus.",
		},
	}
}

func realWorldNextPrepareCorpus(title string, repositories []realWorldRepository, candidateSetID string, allowPreviouslyTested bool) map[string]any {
	step := map[string]any{
		"tool": "real_world_prepare_corpus",
		"why":  "Start managed corpus preparation, flag duplicate repositories, and begin background clone/inspection jobs.",
		"suggestedArgs": map[string]any{
			"title":                 title,
			"clone":                 true,
			"allowPreviouslyTested": allowPreviouslyTested,
		},
	}
	args := step["suggestedArgs"].(map[string]any)
	if candidateSetID != "" {
		args["candidateSetID"] = candidateSetID
		args["expectedCount"] = len(repositories)
	} else {
		args["repositories"] = realWorldPublicRepositories(repositories)
	}
	return step
}

func realWorldNextUpdateCandidates(candidateSetID string, duplicates []realWorldRepository) map[string]any {
	step := map[string]any{
		"tool": "real_world_update_candidates",
		"why":  "Apply a small add/remove/replace diff to the stored candidate set instead of resubmitting the full repository list.",
		"requiredArgs": []string{
			"candidateSetID",
			"diff",
		},
		"diffShape": map[string]any{
			"replace": []map[string]any{
				{
					"match":      "existing repo name, clone URL, or owner/repo",
					"repository": map[string]any{"name": "replacement", "ecosystem": "ecosystem", "cloneURL": "https://github.com/owner/repo.git"},
				},
			},
			"remove": []string{"repo to remove"},
			"add":    []map[string]any{{"name": "repo to add", "ecosystem": "ecosystem", "cloneURL": "https://github.com/owner/repo.git"}},
		},
	}
	if candidateSetID != "" {
		step["suggestedArgs"] = map[string]any{
			"candidateSetID": candidateSetID,
		}
	}
	if len(duplicates) > 0 {
		var replace []map[string]any
		for _, repo := range duplicates {
			replace = append(replace, map[string]any{
				"match": nonEmpty(repo.CloneURL, repo.Name),
				"repository": map[string]any{
					"name":     "choose-fresh-replacement",
					"cloneURL": "https://github.com/owner/repo.git",
				},
			})
		}
		step["duplicateReplacementsNeeded"] = replace
	}
	return step
}

func realWorldNextInspectCorpus(runID, corpusDir, manifestPath string) map[string]any {
	step := map[string]any{
		"tool": "real_world_inspect_corpus",
		"why":  "Scan cloned repositories for lockfiles and local schema refs before deciding whether dependency prep affects validation fidelity.",
	}
	if runID != "" {
		step["suggestedArgs"] = map[string]any{"runID": runID}
		return step
	}
	step["suggestedArgs"] = map[string]any{
		"corpusDir":    corpusDir,
		"manifestPath": manifestPath,
	}
	return step
}

func realWorldNextRunCorpus(runID, corpusDir, cacheDir, outputArtifact, manifestPath string, dependencyPrep []realWorldDependencyPrep) map[string]any {
	step := map[string]any{
		"tool": "real_world_start_validation",
		"why":  "Start managed per-repository validation jobs. If corpus preparation is still running, validation waits inside the tool for each repository to become ready.",
		"beforeCalling": []string{
			"Use dependencyPrep entries from real_world_inspect_corpus or the managed preparation manifest as the first-pass notes.",
			"Never run dependency lifecycle scripts, postinstall hooks, package-manager plugins, or repository install scripts.",
			"If any entry has status needs-review, run a bounded prep command only when lifecycle scripts are disabled; otherwise replace it with a skipped/not-needed note before recording.",
			"Do not wait for all clones to finish before starting validation when you have a managed manifest.",
			"Do not start shell sleep loops to monitor validation.",
			"Keep real_world_start_validation or real_world_next_validation_result open while waiting; those tools send progress notifications and return completed per-repo results.",
			"After each returned result, send validationFeedback for that repository in the next real_world_next_validation_result call.",
			"Pass dependencyPrep into real_world_start_validation so it is carried forward to triage and record_result.",
		},
		"prepSecurityPolicy": realWorldDependencyPrepSecurityPolicy(),
	}
	if runID != "" {
		step["suggestedArgs"] = map[string]any{
			"runID":              runID,
			"build":              true,
			"concurrency":        realWorldValidationDefaultConcurrency,
			"waitForFirstResult": true,
		}
		return step
	}
	step["suggestedArgs"] = map[string]any{
		"corpusDir":          corpusDir,
		"cacheDir":           cacheDir,
		"outputArtifact":     outputArtifact,
		"manifestPath":       manifestPath,
		"dependencyPrep":     dependencyPrep,
		"build":              true,
		"concurrency":        realWorldValidationDefaultConcurrency,
		"waitForFirstResult": true,
	}
	return step
}

func realWorldNextFixBuild() map[string]any {
	return map[string]any{
		"tool": "verify",
		"why":  "The CLI build failed before validation; inspect the build output and fix or report the blocker before recording a sweep.",
		"suggestedArgs": map[string]any{
			"profile": "quick",
		},
	}
}

func realWorldNextTriageRun(runID string) map[string]any {
	return map[string]any{
		"tool": "real_world_triage_output",
		"why":  "Sanity-check the managed validation output, group issues/warnings by repository and signal, and draft the structured record fields before persisting.",
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
	}
}

func realWorldNextRecordTriagedResult(suggestedArgs map[string]any, missingArgs []string) map[string]any {
	return map[string]any{
		"tool": "real_world_record_result",
		"why":  "Persist the structured sweep result using the triage output as the draft.",
		"beforeCalling": []string{
			"Review draftRecord from real_world_triage_output and adjust only with evidence from the JSON artifact or dependencyPrep notes.",
			"Keep validationFeedback from the managed validation run intact; it is the durable ledger for compaction-safe final recommendations.",
			"Account for dependencyPrep when interpreting missing local schemas or skipped validation.",
			"Keep productRecommendations limited to concrete product changes worth considering, or use an explicit no-change recommendation.",
			"real_world_record_result will persist the outputArtifact bundle into the run directory for later per-file and CLI-output triage.",
			"real_world_record_result will clean managed temp corpus/cache dirs after the structured result is saved.",
			"Do not create or update Markdown report files.",
		},
		"requiredArgs":          realWorldRecordResultContract()["required"],
		"missingArgs":           missingArgs,
		"suggestedArgs":         suggestedArgs,
		"finalResponseContract": realWorldFinalResponseContract(),
	}
}

func realWorldNextAfterRecord(entry realWorldEntry) map[string]any {
	next := map[string]any{
		"message":               "Real-world result recorded and managed temp corpus/cache cleanup attempted. Use the structured entry as the durable source of truth.",
		"entryID":               entry.ID,
		"finalResponseContract": realWorldFinalResponseContract(),
	}
	if inGitHubAgenticWorkflow() {
		next["githubAgenticWorkflow"] = true
		next["discussion"] = "Publish a GitHub Discussion summary from the recorded MCP entry, including a Durable memory PR section that says the PR should be merged in order to retain the results."
		next["pullRequest"] = "Request create_pull_request when recorded result files changed; include the workflow run URL in the PR body because a deterministic follow-up workflow uses it to cross-link the PR and Discussion."
		next["safeOutputs"] = "If safe outputs are exposed through the safeoutputs CLI wrapper, pipe inline JSON with printf instead of writing temporary payload files; temp-file payload writes may be denied by the agent sandbox."
		next["outputLinking"] = "No extra linker tool call is needed. The follow-up GitHub workflow cross-links the PR and Discussion after this run completes."
	}
	return next
}

func inGitHubAgenticWorkflow() bool {
	return os.Getenv("GITHUB_AW") == "true" ||
		os.Getenv("GH_AW") == "true" ||
		os.Getenv("GH_AW_WORKFLOW_ID") != "" ||
		os.Getenv("GH_AW_CURRENT_WORKFLOW_REF") != "" ||
		os.Getenv("GH_AW_CALLER_WORKFLOW_ID") != ""
}

func loadRealWorldHistory(root string) (realWorldHistory, error) {
	dir := filepath.Join(root, realWorldRunsDirRelPath)
	items, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return realWorldHistory{Schema: realWorldEntrySchema, SchemaVersion: realWorldHistorySchemaVersion}, nil
	}
	if err != nil {
		return realWorldHistory{}, err
	}
	history := realWorldHistory{Schema: realWorldEntrySchema, SchemaVersion: realWorldHistorySchemaVersion}
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(realWorldRunsDirRelPath, item.Name(), realWorldRunMetadataFileName))
		entry, err := readRealWorldEntryFile(root, relPath)
		if err != nil {
			return realWorldHistory{}, err
		}
		history.Entries = append(history.Entries, entry)
	}
	sort.Slice(history.Entries, func(i, j int) bool {
		left := history.Entries[i].Date + "\x00" + history.Entries[i].ID
		right := history.Entries[j].Date + "\x00" + history.Entries[j].ID
		return left < right
	})
	return history, nil
}

func readRealWorldEntryFile(root, relPath string) (realWorldEntry, error) {
	clean, err := cleanRelativePath(relPath)
	if err != nil {
		return realWorldEntry{}, fmt.Errorf("invalid real-world result path %q: %w", relPath, err)
	}
	path := filepath.Join(root, clean)
	data, err := os.ReadFile(path)
	if err != nil {
		return realWorldEntry{}, err
	}
	var file realWorldEntryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return realWorldEntry{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if file.Schema != realWorldEntrySchema {
		return realWorldEntry{}, fmt.Errorf("parse %s: unsupported schema %q", path, file.Schema)
	}
	if file.SchemaVersion != realWorldHistorySchemaVersion {
		return realWorldEntry{}, fmt.Errorf("parse %s: unsupported schemaVersion %d", path, file.SchemaVersion)
	}
	entry := file.realWorldEntry
	if entry.ID == "" || entry.Date == "" || entry.Title == "" {
		return realWorldEntry{}, fmt.Errorf("parse %s: entry is missing id, date, or title", path)
	}
	return entry, nil
}

func saveRealWorldHistory(root string, history realWorldHistory) error {
	if history.Schema == "" {
		history.Schema = realWorldEntrySchema
	}
	history.SchemaVersion = realWorldHistorySchemaVersion
	usedPaths := map[string]string{}
	for _, entry := range history.Entries {
		if entry.ID == "" {
			return fmt.Errorf("real-world history entry is missing id")
		}
		relPath := realWorldEntryRelPath(entry)
		if previousID := usedPaths[relPath]; previousID != "" {
			return fmt.Errorf("real-world history entries %q and %q map to %s", previousID, entry.ID, relPath)
		}
		usedPaths[relPath] = entry.ID
		file := realWorldEntryFile{
			Schema:         realWorldEntrySchema,
			SchemaVersion:  realWorldHistorySchemaVersion,
			realWorldEntry: entry,
		}
		if err := writeJSONFile(filepath.Join(root, relPath), file); err != nil {
			return err
		}
	}
	return nil
}

func realWorldEntryRelPath(entry realWorldEntry) string {
	name := slugify(entry.ID)
	if name == "" {
		name = slugify(entry.Date + "-" + entry.Title)
	}
	if name == "" {
		name = "entry"
	}
	return filepath.ToSlash(filepath.Join(realWorldRunsDirRelPath, name, realWorldRunMetadataFileName))
}

func realWorldArtifactRelPath(entry realWorldEntry) string {
	name := slugify(entry.ID)
	if name == "" {
		name = slugify(entry.Date + "-" + entry.Title)
	}
	if name == "" {
		name = "entry"
	}
	return filepath.ToSlash(filepath.Join(realWorldRunsDirRelPath, name, realWorldRunArtifactFileName))
}

var realWorldRandomBytes = cryptorand.Read

func newRealWorldEntryID(date string, args realWorldRecordArgs) (string, error) {
	base := slugify(date)
	if base == "" {
		base = time.Now().Format("2006-01-02")
	}
	suffix, ok := realWorldStableEntryIDSuffix(args)
	if !ok {
		var err error
		suffix, err = realWorldRandomHexID(realWorldEntryIDSuffixBytes)
		if err != nil {
			return "", fmt.Errorf("generate real-world entry id: %w", err)
		}
	}
	return base + "-" + suffix, nil
}

func realWorldStableEntryIDSuffix(args realWorldRecordArgs) (string, bool) {
	var parts []string
	appendPart := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	appendPart("outputArtifact", args.OutputArtifact)
	appendPart("manifestPath", args.ManifestPath)
	appendPart("corpus", args.Corpus)
	appendPart("cacheDir", args.CacheDir)
	appendPart("command", args.Command)
	for _, repo := range args.Repositories {
		appendPart("repository", strings.Join([]string{repo.Name, repo.Ecosystem, repo.CloneURL}, "|"))
	}
	if len(parts) == 0 {
		appendPart("runID", args.RunID)
	}
	if len(parts) == 0 {
		return "", false
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", sum[:])[:realWorldEntryIDSuffixBytes*2], true
}

func realWorldRandomHexID(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	n, err := realWorldRandomBytes(data)
	if err != nil {
		return "", err
	}
	if n != len(data) {
		return "", fmt.Errorf("random source returned %d bytes, want %d", n, len(data))
	}
	return fmt.Sprintf("%x", data), nil
}

func persistRealWorldOutputArtifact(root string, entry realWorldEntry) (string, error) {
	if entry.OutputArtifact == "" {
		return "", nil
	}
	data, err := os.ReadFile(entry.OutputArtifact)
	if err != nil {
		return "", fmt.Errorf("read output artifact %s: %w", entry.OutputArtifact, err)
	}
	if !json.Valid(data) {
		return "", fmt.Errorf("read output artifact %s: invalid JSON", entry.OutputArtifact)
	}
	relPath := realWorldArtifactRelPath(entry)
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return relPath, nil
}

func readRealWorldManifest(path string) (realWorldManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return realWorldManifest{}, err
	}
	var manifest realWorldManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return realWorldManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return manifest, nil
}

func readRealWorldOutputSummary(path string) (*realWorldResult, []map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	data, _, _, err = realWorldUnwrapBundle(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var payload struct {
		Summary struct {
			Discovered    int                   `json:"discovered"`
			Validated     int                   `json:"validated"`
			Skipped       int                   `json:"skipped"`
			Failed        int                   `json:"failed"`
			Issues        realWorldIssueSummary `json:"issues"`
			Ignored       int                   `json:"ignored"`
			Warnings      int                   `json:"warnings"`
			DurationNanos int64                 `json:"durationNanos"`
		} `json:"summary"`
		Warnings []map[string]any `json:"warnings"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	result := &realWorldResult{
		Discovered: payload.Summary.Discovered,
		Validated:  payload.Summary.Validated,
		Skipped:    payload.Summary.Skipped,
		Failed:     payload.Summary.Failed,
		Issues:     payload.Summary.Issues,
		Ignored:    payload.Summary.Ignored,
		Warnings:   payload.Summary.Warnings,
	}
	if payload.Summary.DurationNanos > 0 {
		result.Duration = &realWorldDurationInfo{Nanos: payload.Summary.DurationNanos}
	}
	return result, payload.Warnings, nil
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func realWorldTestedRepos(history realWorldHistory) []realWorldTestedRepo {
	index := map[string]*realWorldTestedRepo{}
	for _, entry := range history.Entries {
		for _, repo := range entry.Repositories {
			key := normalizedRepoKey(repo)
			if key == "" {
				continue
			}
			tested := index[key]
			if tested == nil {
				tested = &realWorldTestedRepo{Key: key, Name: repo.Name, CloneURL: repo.CloneURL}
				index[key] = tested
			}
			tested.TestCount++
			tested.Entries = appendUnique(tested.Entries, entry.ID)
			tested.Ecosystems = appendUnique(tested.Ecosystems, repo.Ecosystem)
			tested.Commits = appendUnique(tested.Commits, repo.Commit)
			if entry.Date >= tested.LastTested {
				tested.LastTested = entry.Date
				tested.LatestCommit = repo.Commit
				if repo.Name != "" {
					tested.Name = repo.Name
				}
				if repo.CloneURL != "" {
					tested.CloneURL = repo.CloneURL
				}
			}
		}
	}
	repos := make([]realWorldTestedRepo, 0, len(index))
	for _, repo := range index {
		sort.Strings(repo.Ecosystems)
		sort.Strings(repo.Entries)
		sort.Strings(repo.Commits)
		repos = append(repos, *repo)
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Key < repos[j].Key
	})
	return repos
}

func limitedRealWorldTestedRepos(repos []realWorldTestedRepo, limit int) ([]realWorldTestedRepo, bool) {
	limit = boundedRealWorldHistoryLimit(limit)
	if len(repos) <= limit {
		return repos, false
	}
	return repos[:limit], true
}

func boundedRealWorldHistoryLimit(limit int) int {
	if limit <= 0 {
		return realWorldDefaultHistoryLimit
	}
	if limit > realWorldMaxHistoryLimit {
		return realWorldMaxHistoryLimit
	}
	return limit
}

func realWorldRepoMatches(history realWorldHistory, query string) []realWorldTestedRepo {
	query = normalizeRepoQuery(query)
	if query == "" {
		return nil
	}
	var matches []realWorldTestedRepo
	for _, repo := range realWorldTestedRepos(history) {
		fields := []string{repo.Key, normalizeRepoQuery(repo.Name), normalizeRepoQuery(repo.CloneURL)}
		for _, field := range fields {
			if field == query || strings.Contains(field, query) || strings.Contains(query, field) {
				matches = append(matches, repo)
				break
			}
		}
	}
	return matches
}

func realWorldPreviousEntries(history realWorldHistory, repo realWorldRepository) []string {
	key := normalizedRepoKey(repo)
	if key == "" {
		return nil
	}
	var entries []string
	for _, entry := range history.Entries {
		for _, existing := range entry.Repositories {
			if normalizedRepoKey(existing) == key {
				entries = appendUnique(entries, entry.ID)
			}
		}
	}
	sort.Strings(entries)
	return entries
}

func realWorldAnnotateCandidateRepositories(history realWorldHistory, repositories []realWorldRepository) ([]realWorldRepository, []realWorldRepository) {
	out := make([]realWorldRepository, len(repositories))
	var duplicates []realWorldRepository
	for i := range repositories {
		out[i] = realWorldCleanCandidateRepository(repositories[i])
		previous := realWorldPreviousEntries(history, out[i])
		if len(previous) == 0 {
			continue
		}
		out[i].AlreadyTested = true
		out[i].PreviousEntries = previous
		duplicates = append(duplicates, out[i])
	}
	return out, duplicates
}

func realWorldCleanCandidateRepository(repo realWorldRepository) realWorldRepository {
	repo.Path = ""
	repo.Status = ""
	repo.Error = ""
	repo.AlreadyTested = false
	repo.PreviousEntries = nil
	return repo
}

func boundedRealWorldCandidateTarget(target int) int {
	if target <= 0 {
		return realWorldDefaultCandidateTarget
	}
	if target > realWorldMaxCandidateTarget {
		return realWorldMaxCandidateTarget
	}
	return target
}

func boundedRealWorldDiscoveryPerQuery(perQueryLimit, targetCount, specCount int) int {
	if perQueryLimit > 0 {
		if perQueryLimit > realWorldMaxDiscoveryPerQuery {
			return realWorldMaxDiscoveryPerQuery
		}
		return perQueryLimit
	}
	if specCount <= 0 {
		return realWorldMaxDiscoveryPerQuery
	}
	limit := targetCount/specCount + 20
	if targetCount%specCount != 0 {
		limit++
	}
	if limit < 10 {
		return 10
	}
	if limit > realWorldMaxDiscoveryPerQuery {
		return realWorldMaxDiscoveryPerQuery
	}
	return limit
}

func realWorldCandidateDiscoverySpecs(ecosystems []string, minStars int) []realWorldCandidateSearchSpec {
	if minStars <= 0 {
		minStars = realWorldDefaultDiscoveryMinStars
	}
	if len(ecosystems) == 0 {
		ecosystems = []string{
			"TypeScript",
			"JavaScript",
			"Python",
			"Go",
			"Rust",
			"Java",
			"Kotlin",
			"Ruby",
			"PHP",
			"C#",
			"C++",
			"C",
			"Swift",
			"Dart",
			"Shell",
			"HTML",
			"CSS",
			"Vue",
			"Svelte",
			"Scala",
			"Elixir",
			"Clojure",
			"Haskell",
			"Lua",
			"R",
			"Julia",
			"HCL",
		}
	}
	cutoff := time.Now().UTC().AddDate(-2, 0, 0).Format("2006-01-02")
	specs := make([]realWorldCandidateSearchSpec, 0, len(ecosystems))
	seen := map[string]bool{}
	for _, ecosystem := range ecosystems {
		ecosystem = strings.TrimSpace(ecosystem)
		if ecosystem == "" {
			continue
		}
		key := strings.ToLower(ecosystem)
		if seen[key] {
			continue
		}
		seen[key] = true
		specs = append(specs, realWorldCandidateSearchSpec{
			Ecosystem: ecosystem,
			Query:     fmt.Sprintf("language:%q stars:>=%d pushed:>%s archived:false fork:false", ecosystem, minStars, cutoff),
		})
	}
	return specs
}

func realWorldSearchGitHubRepositories(ctx context.Context, root string, spec realWorldCandidateSearchSpec, limit int) ([]realWorldRepository, error) {
	_ = root
	client, err := realWorldGitHubClient()
	if err != nil {
		return nil, fmt.Errorf("configure GitHub API client: %w", err)
	}
	result, _, err := client.Search.Repositories(ctx, spec.Query, &githubapi.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: githubapi.ListOptions{
			PerPage: limit,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("search GitHub repositories for %s: %w", spec.Ecosystem, err)
	}
	repositories := make([]realWorldRepository, 0, len(result.Repositories))
	for _, item := range result.Repositories {
		repo, ok := realWorldRepositoryFromGitHubSearch(item, spec.Ecosystem)
		if !ok {
			continue
		}
		repositories = append(repositories, repo)
	}
	return repositories, nil
}

var realWorldGitHubHTTPClient = &http.Client{Timeout: 30 * time.Second}

func realWorldGitHubClient() (*githubapi.Client, error) {
	baseURL := realWorldGitHubAPIBaseURL()
	client := githubapi.NewClient(realWorldGitHubHTTPClient)
	client.UserAgent = "dollarlint-repo-mcp"
	if strings.TrimRight(baseURL, "/") != realWorldGitHubAPIDefaultBaseURL {
		parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
		if err != nil {
			return nil, fmt.Errorf("parse GitHub API URL %q: %w", baseURL, err)
		}
		client.BaseURL = parsed
	}
	if token := realWorldGitHubToken(); token != "" {
		client = client.WithAuthToken(token)
		client.UserAgent = "dollarlint-repo-mcp"
	}
	return client, nil
}

func realWorldGitHubAPIBaseURL() string {
	for _, name := range []string{"DOLLARLINT_GITHUB_API_URL", "GITHUB_API_URL"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return realWorldGitHubAPIDefaultBaseURL
}

func realWorldGitHubToken() string {
	for _, name := range []string{
		"GITHUB_TOKEN",
		"GH_TOKEN",
		"GITHUB_PERSONAL_ACCESS_TOKEN",
		"GITHUB_MCP_SERVER_TOKEN",
		"GH_AW_GITHUB_MCP_SERVER_TOKEN",
		"GH_AW_GITHUB_TOKEN",
	} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func realWorldRepositoryFromGitHubSearch(item *githubapi.Repository, fallbackEcosystem string) (realWorldRepository, bool) {
	if item == nil || item.GetArchived() || item.GetFork() || strings.TrimSpace(item.GetFullName()) == "" {
		return realWorldRepository{}, false
	}
	cloneURL := strings.TrimSpace(item.GetCloneURL())
	if cloneURL == "" && strings.TrimSpace(item.GetHTMLURL()) != "" {
		cloneURL = strings.TrimRight(item.GetHTMLURL(), "/") + ".git"
	}
	if cloneURL == "" {
		return realWorldRepository{}, false
	}
	name := slugify(strings.ReplaceAll(item.GetFullName(), "/", "-"))
	if name == "" {
		name = slugify(repoNameFromURL(cloneURL))
	}
	ecosystem := strings.TrimSpace(item.GetLanguage())
	if ecosystem == "" {
		ecosystem = fallbackEcosystem
	}
	return realWorldRepository{
		Name:      name,
		Ecosystem: ecosystem,
		CloneURL:  cloneURL,
	}, true
}

func realWorldSelectDiscoveredCandidates(history realWorldHistory, groups [][]realWorldRepository, targetCount int, allowPreviouslyTested bool) ([]realWorldRepository, []realWorldRepository, int) {
	targetCount = boundedRealWorldCandidateTarget(targetCount)
	indexes := make([]int, len(groups))
	seen := map[string]bool{}
	var selected []realWorldRepository
	var omittedAlreadyTested []realWorldRepository
	omittedCandidateDuplicates := 0
	for len(selected) < targetCount {
		progress := false
		for groupIndex := range groups {
			if len(selected) >= targetCount {
				break
			}
			if indexes[groupIndex] >= len(groups[groupIndex]) {
				continue
			}
			repo := realWorldCleanCandidateRepository(groups[groupIndex][indexes[groupIndex]])
			indexes[groupIndex]++
			progress = true
			key := normalizedRepoKey(repo)
			if key == "" || seen[key] {
				omittedCandidateDuplicates++
				continue
			}
			seen[key] = true
			previous := realWorldPreviousEntries(history, repo)
			if len(previous) > 0 {
				repo.AlreadyTested = true
				repo.PreviousEntries = previous
				if !allowPreviouslyTested {
					omittedAlreadyTested = append(omittedAlreadyTested, repo)
					continue
				}
			}
			selected = append(selected, repo)
		}
		if !progress {
			break
		}
	}
	return selected, omittedAlreadyTested, omittedCandidateDuplicates
}

func realWorldRepositoryPreview(repositories []realWorldRepository, limit int) []realWorldRepository {
	if limit <= 0 || len(repositories) <= limit {
		return realWorldPublicRepositories(repositories)
	}
	return realWorldPublicRepositories(repositories[:limit])
}

func newRealWorldCandidateSet(title string, allowPreviouslyTested bool, repositories []realWorldRepository) realWorldCandidateSet {
	now := time.Now().UTC().Format(time.RFC3339)
	return realWorldCandidateSet{
		SchemaVersion:         1,
		ID:                    newRealWorldCandidateSetID(title),
		Title:                 title,
		AllowPreviouslyTested: allowPreviouslyTested,
		CreatedAt:             now,
		UpdatedAt:             now,
		Repositories:          append([]realWorldRepository{}, repositories...),
	}
}

func newRealWorldCandidateSetID(title string) string {
	base := slugify(title)
	if base == "" {
		base = "candidate-set"
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UTC().UnixNano())
}

func realWorldCandidateSetID(set *realWorldCandidateSet) string {
	if set == nil {
		return ""
	}
	return set.ID
}

func realWorldCandidateSetPath(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("candidateSetID is required")
	}
	if slugify(id) != id {
		return "", fmt.Errorf("invalid candidateSetID %q", id)
	}
	return filepath.Join(os.TempDir(), realWorldCandidateSetPrefix+id+".json"), nil
}

func loadRealWorldCandidateSet(id string) (realWorldCandidateSet, error) {
	path, err := realWorldCandidateSetPath(id)
	if err != nil {
		return realWorldCandidateSet{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return realWorldCandidateSet{}, fmt.Errorf("candidate set %q was not found; restart with real_world_start_testing", id)
		}
		return realWorldCandidateSet{}, err
	}
	var set realWorldCandidateSet
	if err := json.Unmarshal(data, &set); err != nil {
		return realWorldCandidateSet{}, fmt.Errorf("parse candidate set %q: %w", id, err)
	}
	if set.SchemaVersion != 1 {
		return realWorldCandidateSet{}, fmt.Errorf("candidate set %q has unsupported schemaVersion %d", id, set.SchemaVersion)
	}
	if set.ID != id {
		return realWorldCandidateSet{}, fmt.Errorf("candidate set %q contains mismatched id %q", id, set.ID)
	}
	return set, nil
}

func saveRealWorldCandidateSet(set realWorldCandidateSet) error {
	if set.ID == "" {
		return fmt.Errorf("candidate set is missing id")
	}
	set.SchemaVersion = 1
	if set.CreatedAt == "" {
		set.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	set.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	path, err := realWorldCandidateSetPath(set.ID)
	if err != nil {
		return err
	}
	return writeJSONFile(path, set)
}

func realWorldCandidateDiffEmpty(diff realWorldCandidateDiff) bool {
	return len(diff.Add) == 0 && len(diff.Remove) == 0 && len(diff.Replace) == 0
}

func applyRealWorldCandidateDiff(repositories []realWorldRepository, diff realWorldCandidateDiff) ([]realWorldRepository, realWorldCandidateDiffResult, error) {
	out := append([]realWorldRepository{}, repositories...)
	result := realWorldCandidateDiffResult{}
	for _, query := range diff.Remove {
		index := realWorldCandidateIndex(out, query)
		if index < 0 {
			return nil, result, fmt.Errorf("candidate diff remove %q did not match a repository", query)
		}
		removed := out[index]
		result.Removed = append(result.Removed, removed)
		out = append(out[:index], out[index+1:]...)
	}
	for _, replacement := range diff.Replace {
		index := realWorldCandidateIndex(out, replacement.Match)
		if index < 0 {
			return nil, result, fmt.Errorf("candidate diff replace %q did not match a repository", replacement.Match)
		}
		before := out[index]
		after := realWorldCleanCandidateRepository(replacement.Repository)
		if err := validateRealWorldDiffRepository(after, "replace "+replacement.Match); err != nil {
			return nil, result, err
		}
		out[index] = after
		result.Replaced = append(result.Replaced, map[string]any{
			"match":   replacement.Match,
			"removed": before,
			"added":   after,
		})
	}
	for _, repo := range diff.Add {
		clean := realWorldCleanCandidateRepository(repo)
		if err := validateRealWorldDiffRepository(clean, "add"); err != nil {
			return nil, result, err
		}
		out = append(out, clean)
		result.Added = append(result.Added, clean)
	}
	return out, result, nil
}

func validateRealWorldDiffRepository(repo realWorldRepository, operation string) error {
	if strings.TrimSpace(repo.CloneURL) == "" {
		return fmt.Errorf("candidate diff %s repository.cloneURL is required", operation)
	}
	return nil
}

func realWorldCandidateIndex(repositories []realWorldRepository, query string) int {
	query = normalizeRepoQuery(query)
	if query == "" {
		return -1
	}
	querySlug := slugify(query)
	for i, repo := range repositories {
		fields := []string{
			normalizeRepoQuery(repo.Name),
			normalizeRepoQuery(repo.CloneURL),
			normalizeRepoQuery(repoNameFromURL(repo.CloneURL)),
			slugify(repo.Name),
			slugify(repoNameFromURL(repo.CloneURL)),
		}
		for _, field := range fields {
			if field == "" {
				continue
			}
			if field == query || (querySlug != "" && field == querySlug) || (len(query) >= 4 && (strings.Contains(field, query) || strings.Contains(query, field))) {
				return i
			}
		}
	}
	return -1
}

func realWorldCandidateSetSummary(set realWorldCandidateSet, duplicates []realWorldRepository, expectedCount int) map[string]any {
	out := map[string]any{
		"id":                    set.ID,
		"title":                 set.Title,
		"repositoryCount":       len(set.Repositories),
		"allowPreviouslyTested": set.AllowPreviouslyTested,
		"createdAt":             set.CreatedAt,
		"updatedAt":             set.UpdatedAt,
		"ready":                 len(duplicates) == 0 || set.AllowPreviouslyTested,
	}
	if expectedCount > 0 {
		out["expectedCount"] = expectedCount
		out["countMatchesExpected"] = len(set.Repositories) == expectedCount
	}
	if len(duplicates) > 0 {
		out["duplicates"] = realWorldPublicRepositories(duplicates)
	}
	return out
}

func normalizedRepoKey(repo realWorldRepository) string {
	if repo.CloneURL != "" {
		return normalizeRepoQuery(repo.CloneURL)
	}
	return normalizeRepoQuery(repo.Name)
}

func normalizeRepoQuery(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "git@github.com:")
	value = strings.TrimPrefix(value, "https://github.com/")
	value = strings.TrimPrefix(value, "http://github.com/")
	value = strings.TrimSuffix(value, ".git")
	value = strings.Trim(value, "/")
	return value
}

func realWorldValidationArgs(corpusDir string, schemaStore bool, failure string, fetchRetries int, minWait, maxWait, outputArtifact string, extra []string) []string {
	args := []string{"validate", corpusDir}
	if schemaStore {
		args = append(args, "--catalogs")
	}
	if failure != "" {
		args = append(args, "--catalog-failure", failure)
	}
	args = append(args,
		"--fetch-retries", fmt.Sprint(fetchRetries),
		"--fetch-retry-min-wait", minWait,
		"--fetch-retry-max-wait", maxWait,
		"--format", "bundle",
		"--locations",
		"--show-skipped",
		"--output", outputArtifact,
	)
	args = append(args, extra...)
	return args
}

func realWorldValidationCommand(corpusDir, cacheDir, outputArtifact string, schemaStore bool, failure string, fetchRetries int, minWait, maxWait string, extra []string) string {
	args := realWorldValidationArgs(corpusDir, schemaStore, failure, fetchRetries, minWait, maxWait, outputArtifact, extra)
	parts := []string{"XDG_CACHE_HOME=" + shellQuote(cacheDir), "bin/dollarlint"}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func createRealWorldOutputPath(title, outputName string) (string, error) {
	if outputName != "" {
		if filepath.IsAbs(outputName) {
			return outputName, nil
		}
		return filepath.Join(os.TempDir(), filepath.Base(outputName)), nil
	}
	prefix := "dollarlint-corpus"
	if slug := slugify(title); slug != "" {
		prefix = "dollarlint-" + slug
	}
	file, err := os.CreateTemp("", prefix+"-*.json")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func runProcess(ctx context.Context, dir string, env []string, name string, args ...string) commandResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return commandResult{
		Name:      filepath.Base(name),
		Command:   commandDisplay(env, name, args),
		ExitCode:  exitCode,
		Duration:  time.Since(start).Round(time.Millisecond).String(),
		Output:    truncate(output.String(), 12000),
		Succeeded: err == nil,
	}
}

func commandDisplay(env []string, name string, args []string) string {
	var parts []string
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			parts = append(parts, key+"="+shellQuote(value))
		} else {
			parts = append(parts, shellQuote(item))
		}
	}
	parts = append(parts, shellQuote(name))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func repoNameFromURL(url string) string {
	url = strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	if url == "" {
		return ""
	}
	parts := strings.FieldsFunc(url, func(r rune) bool {
		return r == '/' || r == ':'
	})
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func slugify(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
