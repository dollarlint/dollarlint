package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldResultsRelPath       = "reports/real-world-results.json"
	realWorldResultsDirRelPath    = "reports/real-world-results"
	realWorldArtifactsDirRelPath  = "reports/real-world-artifacts"
	realWorldResultsSchema        = "./real-world-results.schema.json"
	realWorldEntrySchema          = "../real-world-result-entry.schema.json"
	realWorldHistorySchemaVersion = 3
	realWorldMCPContractVersion   = 7
	realWorldManifestName         = "real-world-manifest.json"
	realWorldCorpusTempPrefix     = "dollarlint-corpus."
	realWorldCacheTempPrefix      = "dollarlint-cache."
	realWorldOutputTempPrefix     = "dollarlint-"

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

type realWorldHistoryIndex struct {
	Schema        string              `json:"$schema,omitempty"`
	SchemaVersion int                 `json:"schemaVersion"`
	Entries       []realWorldEntryRef `json:"entries"`
}

type realWorldEntryRef struct {
	ID        string `json:"id"`
	Date      string `json:"date"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	RepoCount int    `json:"repoCount,omitempty"`
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
	var args struct {
		Title                 string                `json:"title"`
		Repositories          []realWorldRepository `json:"repositories"`
		AllowPreviouslyTested bool                  `json:"allowPreviouslyTested"`
	}
	_ = request.BindArguments(&args)
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	tested := realWorldTestedRepos(history)
	repositories := append([]realWorldRepository{}, args.Repositories...)
	var duplicates []realWorldRepository
	for i := range repositories {
		previous := realWorldPreviousEntries(history, repositories[i])
		if len(previous) == 0 {
			continue
		}
		repositories[i].AlreadyTested = true
		repositories[i].PreviousEntries = previous
		duplicates = append(duplicates, repositories[i])
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
		next = realWorldNextPrepareCorpus(args.Title, repositories, args.AllowPreviouslyTested)
	}
	message := "Choose fresh public repositories, then call real_world_prepare_corpus."
	if len(repositories) > 0 && ok {
		message = "Candidate repositories are ready; call real_world_prepare_corpus next."
	}
	if len(duplicates) > 0 && !args.AllowPreviouslyTested {
		message = "One or more candidate repositories were already tested; choose replacements or restart with allowPreviouslyTested=true for an intentional rerun."
		next = realWorldNextChooseRepositories()
	}
	return s.realWorldStructured(ctx, map[string]any{
		"ok":                    ok,
		"message":               message,
		"title":                 args.Title,
		"dollarlintRevision":    revision,
		"workingTreeNote":       workingTreeNote,
		"entryCount":            len(history.Entries),
		"repoCount":             len(tested),
		"testedRepos":           tested,
		"candidateRepositories": realWorldPublicRepositories(repositories),
		"duplicates":            realWorldPublicRepositories(duplicates),
		"recordResultContract":  realWorldRecordResultContract(),
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
	})
}

func (s *repoServer) handleRealWorldHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Repo           string   `json:"repo"`
		Repositories   []string `json:"repositories"`
		IncludeEntries bool     `json:"includeEntries"`
	}
	_ = request.BindArguments(&args)
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
		"schema":        history.Schema,
		"schemaVersion": history.SchemaVersion,
		"entryCount":    len(history.Entries),
		"repoCount":     len(tested),
		"testedRepos":   tested,
		"queries":       queryResults,
	}
	if args.IncludeEntries {
		out["entries"] = history.Entries
	}
	out["nextStep"] = realWorldNextChooseRepositories()
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) handleRealWorldPrepareCorpus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Title                 string                `json:"title"`
		Repositories          []realWorldRepository `json:"repositories"`
		Clone                 bool                  `json:"clone"`
		AllowPreviouslyTested bool                  `json:"allowPreviouslyTested"`
		OutputName            string                `json:"outputName"`
		Concurrency           int                   `json:"concurrency"`
		WaitForFirstResult    *bool                 `json:"waitForFirstResult"`
	}
	_ = request.BindArguments(&args)
	history, err := loadRealWorldHistory(s.root)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	repositories := append([]realWorldRepository{}, args.Repositories...)
	var duplicates []realWorldRepository
	for i := range repositories {
		previous := realWorldPreviousEntries(history, repositories[i])
		if len(previous) == 0 {
			continue
		}
		repositories[i].AlreadyTested = true
		repositories[i].PreviousEntries = previous
		duplicates = append(duplicates, repositories[i])
	}
	if len(duplicates) > 0 && !args.AllowPreviouslyTested {
		return s.realWorldStructured(ctx, map[string]any{
			"ok":         false,
			"message":    "one or more repositories were already tested; pass allowPreviouslyTested=true for an intentional rerun",
			"duplicates": duplicates,
			"nextStep":   realWorldNextChooseRepositories(),
		})
	}
	if len(repositories) == 0 {
		return mcp.NewToolResultError("repositories is required; call real_world_start_testing first if you need repository guidance"), nil
	}
	corpusDir, err := os.MkdirTemp("", realWorldCorpusTempPrefix)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cacheDir, err := os.MkdirTemp("", realWorldCacheTempPrefix)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	outputArtifact, err := createRealWorldOutputPath(args.Title, args.OutputName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
			return mcp.NewToolResultError(err.Error()), nil
		}
		wait := false
		if args.WaitForFirstResult != nil {
			wait = *args.WaitForFirstResult
		}
		if wait {
			out, err := s.realWorldWaitForPreparedRepo(ctx, request, run)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			out["ok"] = true
			out["started"] = true
			out["clone"] = true
			return s.realWorldStructured(ctx, out)
		}
		return s.realWorldStructured(ctx, map[string]any{
			"ok":               true,
			"started":          true,
			"clone":            true,
			"repositoryCount":  len(run.repositories()),
			"duplicates":       realWorldPublicRepositories(duplicates),
			"run":              run.snapshot(),
			"nextStep":         realWorldNextRunCorpusDuringPrepare(run),
			"preparedRepoStep": realWorldNextPreparedRepo(run.ID),
		})
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
		return mcp.NewToolResultError(err.Error()), nil
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
	if inspectErr != nil {
		out["dependencyPrepInspectionError"] = inspectErr.Error()
	} else {
		out["dependencyPrepInspection"] = inspection.Repositories
		out["dependencyPrep"] = inspection.DraftDependencyPrep
		out["prepSecurityPolicy"] = inspection.PrepSecurityPolicy
		out["dependencyPrepSummary"] = inspection.Summary
		out["dependencyPrepNeedsReview"] = inspection.NeedsReview
	}
	return s.realWorldStructured(ctx, out)
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
	id := args.ID
	if id == "" {
		id = slugify(date + "-" + args.Title)
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
		"persistedOutputArtifact": "Written automatically from outputArtifact into reports/real-world-artifacts/.",
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
		"tool": "real_world_prepare_corpus",
		"why":  "Choose fresh public repositories with diverse ecosystems, then start managed corpus preparation.",
		"requiredArgs": []string{
			"title",
			"repositories",
		},
		"recommendedArgs": map[string]any{
			"clone": true,
		},
	}
}

func realWorldNextPrepareCorpus(title string, repositories []realWorldRepository, allowPreviouslyTested bool) map[string]any {
	return map[string]any{
		"tool": "real_world_prepare_corpus",
		"why":  "Start managed corpus preparation, flag duplicate repositories, and begin background clone/inspection jobs.",
		"suggestedArgs": map[string]any{
			"title":                 title,
			"repositories":          realWorldPublicRepositories(repositories),
			"clone":                 true,
			"allowPreviouslyTested": allowPreviouslyTested,
		},
	}
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
			"real_world_record_result will persist the outputArtifact bundle into reports/real-world-artifacts/ for later per-file and CLI-output triage.",
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
		next["pullRequest"] = "Request create_pull_request when recorded result files changed; merging that PR is how future real-world sweeps remember tested repositories."
		next["safeOutputs"] = "If safe outputs are exposed through the safeoutputs CLI wrapper, pipe inline JSON with printf instead of writing temporary payload files; temp-file payload writes may be denied by the agent sandbox."
		next["linkOutputs"] = "After requesting create_pull_request and create_discussion, call link_real_world_outputs with the Discussion title and entryID so the final PR and Discussion cross-link each other."
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
	path := filepath.Join(root, realWorldResultsRelPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return realWorldHistory{Schema: realWorldResultsSchema, SchemaVersion: realWorldHistorySchemaVersion}, nil
	}
	if err != nil {
		return realWorldHistory{}, err
	}
	return loadSplitRealWorldHistory(root, path, data)
}

func loadSplitRealWorldHistory(root, path string, data []byte) (realWorldHistory, error) {
	var index realWorldHistoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return realWorldHistory{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if index.Schema != realWorldResultsSchema {
		return realWorldHistory{}, fmt.Errorf("parse %s: unsupported schema %q", path, index.Schema)
	}
	if index.SchemaVersion != realWorldHistorySchemaVersion {
		return realWorldHistory{}, fmt.Errorf("parse %s: unsupported schemaVersion %d", path, index.SchemaVersion)
	}
	history := realWorldHistory{Schema: index.Schema, SchemaVersion: index.SchemaVersion}
	for _, ref := range index.Entries {
		entry, err := readRealWorldEntryFile(root, ref)
		if err != nil {
			return realWorldHistory{}, err
		}
		history.Entries = append(history.Entries, entry)
	}
	return history, nil
}

func readRealWorldEntryFile(root string, ref realWorldEntryRef) (realWorldEntry, error) {
	relPath := ref.Path
	if relPath == "" {
		return realWorldEntry{}, fmt.Errorf("real-world history entry %q is missing path", ref.ID)
	}
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
	if entry.ID != ref.ID {
		return realWorldEntry{}, fmt.Errorf("parse %s: entry id %q does not match index id %q", path, entry.ID, ref.ID)
	}
	if entry.Date != ref.Date || entry.Title != ref.Title {
		return realWorldEntry{}, fmt.Errorf("parse %s: entry metadata does not match index metadata", path)
	}
	return entry, nil
}

func saveRealWorldHistory(root string, history realWorldHistory) error {
	if history.Schema == "" {
		history.Schema = realWorldResultsSchema
	}
	history.SchemaVersion = realWorldHistorySchemaVersion
	usedPaths := map[string]string{}
	index := realWorldHistoryIndex{
		Schema:        realWorldResultsSchema,
		SchemaVersion: realWorldHistorySchemaVersion,
		Entries:       make([]realWorldEntryRef, 0, len(history.Entries)),
	}
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
		index.Entries = append(index.Entries, realWorldEntryRef{
			ID:        entry.ID,
			Date:      entry.Date,
			Title:     entry.Title,
			Path:      relPath,
			RepoCount: len(entry.Repositories),
		})
	}
	return writeJSONFile(filepath.Join(root, realWorldResultsRelPath), index)
}

func realWorldEntryRelPath(entry realWorldEntry) string {
	name := slugify(entry.ID)
	if name == "" {
		name = slugify(entry.Date + "-" + entry.Title)
	}
	if name == "" {
		name = "entry"
	}
	return filepath.ToSlash(filepath.Join(realWorldResultsDirRelPath, name+".json"))
}

func realWorldArtifactRelPath(entry realWorldEntry) string {
	name := slugify(entry.ID)
	if name == "" {
		name = slugify(entry.Date + "-" + entry.Title)
	}
	if name == "" {
		name = "entry"
	}
	return filepath.ToSlash(filepath.Join(realWorldArtifactsDirRelPath, name+".dollarlint.json"))
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
		args = append(args, "--schema-store")
	}
	if failure != "" {
		args = append(args, "--schema-store-failure", failure)
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
