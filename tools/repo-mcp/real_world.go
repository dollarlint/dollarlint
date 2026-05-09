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
	realWorldManifestName         = "real-world-manifest.json"
)

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

type realWorldProductRecommendation struct {
	Strength       string `json:"strength"`
	Recommendation string `json:"recommendation"`
	Rationale      string `json:"rationale"`
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
	SchemaVersion  int                   `json:"schemaVersion"`
	CreatedAt      string                `json:"createdAt"`
	Title          string                `json:"title,omitempty"`
	CorpusDir      string                `json:"corpusDir"`
	CacheDir       string                `json:"cacheDir"`
	OutputArtifact string                `json:"outputArtifact"`
	Repositories   []realWorldRepository `json:"repositories"`
}

type realWorldRecordArgs struct {
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
	return structured(map[string]any{
		"ok":                    ok,
		"message":               message,
		"title":                 args.Title,
		"dollarlintRevision":    revision,
		"workingTreeNote":       workingTreeNote,
		"historyPath":           filepath.Join(s.root, realWorldResultsRelPath),
		"entriesDir":            filepath.Join(s.root, realWorldResultsDirRelPath),
		"entryCount":            len(history.Entries),
		"repoCount":             len(tested),
		"testedRepos":           tested,
		"candidateRepositories": repositories,
		"duplicates":            duplicates,
		"recordResultContract":  realWorldRecordResultContract(),
		"rules": []string{
			"Do not create or update Markdown report files for repository memory.",
			"Record dependency preparation in dependencyPrep, including skipped or not-needed prep.",
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
		"path":          filepath.Join(s.root, realWorldResultsRelPath),
		"entriesDir":    filepath.Join(s.root, realWorldResultsDirRelPath),
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
	return structured(out)
}

func (s *repoServer) handleRealWorldPrepareCorpus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Title                 string                `json:"title"`
		Repositories          []realWorldRepository `json:"repositories"`
		Clone                 bool                  `json:"clone"`
		AllowPreviouslyTested bool                  `json:"allowPreviouslyTested"`
		OutputName            string                `json:"outputName"`
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
		return structured(map[string]any{
			"ok":         false,
			"message":    "one or more repositories were already tested; pass allowPreviouslyTested=true for an intentional rerun",
			"duplicates": duplicates,
			"nextStep":   realWorldNextChooseRepositories(),
		})
	}
	if len(repositories) == 0 {
		return mcp.NewToolResultError("repositories is required; call real_world_start_testing first if you need repository guidance"), nil
	}
	corpusDir, err := os.MkdirTemp("", "dollarlint-corpus.")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cacheDir, err := os.MkdirTemp("", "dollarlint-cache.")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	outputArtifact, err := createRealWorldOutputPath(args.Title, args.OutputName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
	return structured(map[string]any{
		"ok":                ok,
		"corpusDir":         corpusDir,
		"cacheDir":          cacheDir,
		"outputArtifact":    outputArtifact,
		"manifestPath":      manifestPath,
		"repositories":      repositories,
		"duplicates":        duplicates,
		"clone":             args.Clone,
		"cloneCommands":     cloneCommands,
		"cloneResults":      cloneResults,
		"buildCommand":      "go build -o bin/dollarlint ./cmd/dollarlint",
		"validationCommand": realWorldValidationCommand(corpusDir, cacheDir, outputArtifact, true, "warn", 1, "1ms", "1ms", nil),
		"nextStep":          realWorldNextDependencyPrep(corpusDir, cacheDir, outputArtifact, repositories),
	})
}

func (s *repoServer) handleRealWorldRunCorpus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		CorpusDir          string   `json:"corpusDir"`
		CacheDir           string   `json:"cacheDir"`
		OutputArtifact     string   `json:"outputArtifact"`
		Build              *bool    `json:"build"`
		SchemaStore        *bool    `json:"schemaStore"`
		SchemaStoreFailure string   `json:"schemaStoreFailure"`
		FetchRetries       *int     `json:"fetchRetries"`
		FetchRetryMinWait  string   `json:"fetchRetryMinWait"`
		FetchRetryMaxWait  string   `json:"fetchRetryMaxWait"`
		ExtraArgs          []string `json:"extraArgs"`
	}
	_ = request.BindArguments(&args)
	if args.CorpusDir == "" {
		return mcp.NewToolResultError("corpusDir is required; call real_world_prepare_corpus first"), nil
	}
	if args.CacheDir == "" {
		cacheDir, err := os.MkdirTemp("", "dollarlint-cache.")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		args.CacheDir = cacheDir
	}
	if args.OutputArtifact == "" {
		outputArtifact, err := createRealWorldOutputPath("", "")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		args.OutputArtifact = outputArtifact
	}
	build := true
	if args.Build != nil {
		build = *args.Build
	}
	schemaStore := true
	if args.SchemaStore != nil {
		schemaStore = *args.SchemaStore
	}
	if args.SchemaStoreFailure == "" {
		args.SchemaStoreFailure = "warn"
	}
	if args.FetchRetries == nil {
		defaultRetries := 1
		args.FetchRetries = &defaultRetries
	}
	if args.FetchRetryMinWait == "" {
		args.FetchRetryMinWait = "1ms"
	}
	if args.FetchRetryMaxWait == "" {
		args.FetchRetryMaxWait = "1ms"
	}

	p := newProgress(ctx, s.mcp, request, 2)
	var buildResult *commandResult
	if build {
		p.step("Building dollarlint CLI")
		result := s.run(ctx, namedCommand{Name: "go build", Cmd: "go build -o bin/dollarlint ./cmd/dollarlint"})
		buildResult = &result
		if !result.Succeeded {
			return structured(map[string]any{
				"ok":       false,
				"build":    result,
				"nextStep": realWorldNextFixBuild(),
			})
		}
	}
	p.step("Running real-world corpus validation")
	validationArgs := realWorldValidationArgs(args.CorpusDir, schemaStore, args.SchemaStoreFailure, *args.FetchRetries, args.FetchRetryMinWait, args.FetchRetryMaxWait, args.OutputArtifact, args.ExtraArgs)
	validationCommand := realWorldValidationCommand(args.CorpusDir, args.CacheDir, args.OutputArtifact, schemaStore, args.SchemaStoreFailure, *args.FetchRetries, args.FetchRetryMinWait, args.FetchRetryMaxWait, args.ExtraArgs)
	validation := runProcess(ctx, s.root, []string{"XDG_CACHE_HOME=" + args.CacheDir}, filepath.Join(s.root, "bin/dollarlint"), validationArgs...)
	summary, warnings, outputErr := readRealWorldOutputSummary(args.OutputArtifact)
	out := map[string]any{
		"ok":                validation.Succeeded || (validation.ExitCode == 1 && outputErr == nil),
		"build":             buildResult,
		"validation":        validation,
		"summary":           summary,
		"warnings":          warnings,
		"outputReadError":   errorString(outputErr),
		"corpusDir":         args.CorpusDir,
		"cacheDir":          args.CacheDir,
		"outputArtifact":    args.OutputArtifact,
		"validationCommand": validationCommand,
		"nextStep":          realWorldNextTriageOutput(args.CorpusDir, args.CacheDir, args.OutputArtifact, validationCommand),
	}
	if summary != nil {
		out["hasIssues"] = summary.Issues.Total > 0
	}
	return structured(out)
}

func (s *repoServer) handleRealWorldRecordResult(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldRecordArgs
	_ = request.BindArguments(&args)
	if args.Title == "" {
		return mcp.NewToolResultError("title is required"), nil
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
	return structured(map[string]any{
		"ok":                      true,
		"path":                    filepath.Join(s.root, realWorldResultsRelPath),
		"entryPath":               filepath.Join(s.root, realWorldEntryRelPath(entry)),
		"entry":                   entry,
		"persistedOutputArtifact": filepath.Join(s.root, entry.PersistedOutputArtifact),
		"replaced":                replaced,
		"entryCount":              len(history.Entries),
		"repoCount":               len(entry.Repositories),
		"testedRepos":             realWorldTestedRepos(history),
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
	manifestPath := args.ManifestPath
	if manifestPath == "" && args.Corpus != "" {
		manifestPath = filepath.Join(args.Corpus, realWorldManifestName)
	}
	if len(repositories) == 0 && manifestPath != "" {
		manifest, err := readRealWorldManifest(manifestPath)
		if err == nil {
			repositories = manifest.Repositories
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
		Repositories:           repositories,
		Result:                 result,
		Findings:               args.Findings,
		ProductRecommendations: args.ProductRecommendations,
		ProductDecisions:       args.ProductDecisions,
		FollowUp:               args.FollowUp,
	}, nil
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
	for i, recommendation := range entry.ProductRecommendations {
		if recommendation.Strength != "high" && recommendation.Strength != "med" && recommendation.Strength != "low" {
			return fmt.Errorf("productRecommendations[%d].strength must be high, med, or low", i)
		}
		if recommendation.Recommendation == "" || recommendation.Rationale == "" {
			return fmt.Errorf("productRecommendations[%d] must include recommendation and rationale", i)
		}
	}
	return nil
}

func realWorldRecordResultContract() map[string]any {
	return map[string]any{
		"required": []string{
			"title",
			"dollarlintRevision",
			"workingTreeNote",
			"corpus",
			"cacheDir",
			"command",
			"outputArtifact",
			"repositories",
			"dependencyPrep",
			"findings",
			"productRecommendations",
			"productDecisions",
			"followUp",
		},
		"dependencyPrep":          "Include every dependency-prep command that ran, failed, timed out, was narrowed, was skipped, or was not needed. Each item needs status and notes.",
		"productRecommendations":  "Use objects with strength high|med|low, recommendation, and rationale. If there is no genuine product change to consider, record an explicit no-change recommendation and use the productBehavedReasonably final-response outcome.",
		"productDecisions":        "Use for product changes or decisions made after triage. If none were made, include an explicit no-change decision.",
		"result":                  "Read automatically from outputArtifact when outputArtifact is provided.",
		"persistedOutputArtifact": "Written automatically from outputArtifact into reports/real-world-artifacts/.",
		"repositories":            "Read automatically from manifestPath when repositories is omitted and a manifest is available.",
		"finalResponseContract":   realWorldFinalResponseContract(),
	}
}

func realWorldNextChooseRepositories() map[string]any {
	return map[string]any{
		"tool": "real_world_prepare_corpus",
		"why":  "Choose fresh public repositories with diverse ecosystems, then prepare an isolated corpus.",
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
		"why":  "Create the corpus/cache/output paths, flag duplicate repositories, and optionally clone the corpus.",
		"suggestedArgs": map[string]any{
			"title":                 title,
			"repositories":          repositories,
			"clone":                 true,
			"allowPreviouslyTested": allowPreviouslyTested,
		},
	}
}

func realWorldNextDependencyPrep(corpusDir, cacheDir, outputArtifact string, repositories []realWorldRepository) map[string]any {
	return map[string]any{
		"tool": "real_world_run_corpus",
		"why":  "First inspect cloned repositories for dependency metadata and record dependencyPrep notes; then run validation.",
		"beforeCalling": []string{
			"Inspect each clone for lockfiles, local $schema refs, and node_modules/file schemas that affect validation fidelity.",
			"Run bounded, script-suppressed dependency prep when realistic.",
			"Record run/skipped/failed/not-needed prep entries for real_world_record_result.dependencyPrep.",
		},
		"suggestedArgs": map[string]any{
			"corpusDir":      corpusDir,
			"cacheDir":       cacheDir,
			"outputArtifact": outputArtifact,
		},
		"repositories": repositories,
	}
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

func realWorldNextTriageOutput(corpusDir, cacheDir, outputArtifact, command string) map[string]any {
	return map[string]any{
		"tool": "real_world_triage_output",
		"why":  "Sanity-check the JSON output, group issues/warnings by repository and signal, and draft the structured record fields before persisting.",
		"suggestedArgs": map[string]any{
			"corpusDir":      corpusDir,
			"cacheDir":       cacheDir,
			"outputArtifact": outputArtifact,
			"command":        command,
		},
	}
}

func realWorldNextRecordTriagedResult(suggestedArgs map[string]any, missingArgs []string) map[string]any {
	return map[string]any{
		"tool": "real_world_record_result",
		"why":  "Persist the structured sweep result using the triage output as the draft.",
		"beforeCalling": []string{
			"Review draftRecord from real_world_triage_output and adjust only with evidence from the JSON artifact or dependencyPrep notes.",
			"Account for dependencyPrep when interpreting missing local schemas or skipped validation.",
			"Keep productRecommendations limited to concrete product changes worth considering, or use an explicit no-change recommendation.",
			"real_world_record_result will persist the raw outputArtifact JSON into reports/real-world-artifacts/ for later per-file triage.",
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
		"message":               "Real-world result recorded. Use the structured entry as the durable source of truth.",
		"entryID":               entry.ID,
		"finalResponseContract": realWorldFinalResponseContract(),
	}
	if inGitHubAgenticWorkflow() {
		next["githubAgenticWorkflow"] = true
		next["discussion"] = "If this run is a GitHub Agentic Workflow, publish a GitHub Discussion summary from the recorded MCP entry."
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
	return os.WriteFile(path, data, 0o644)
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
		"--format", "json",
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
