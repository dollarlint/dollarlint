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
	realWorldResultsRelPath = "reports/real-world-results.json"
	realWorldManifestName   = "real-world-manifest.json"
)

type realWorldHistory struct {
	SchemaVersion int              `json:"schemaVersion"`
	Entries       []realWorldEntry `json:"entries"`
}

type realWorldEntry struct {
	ID                 string                `json:"id"`
	Date               string                `json:"date"`
	Title              string                `json:"title"`
	DollarLintRevision string                `json:"dollarlintRevision"`
	WorkingTreeNote    string                `json:"workingTreeNote,omitempty"`
	Corpus             string                `json:"corpus"`
	CacheDir           string                `json:"cacheDir,omitempty"`
	Command            string                `json:"command"`
	OutputArtifact     string                `json:"outputArtifact"`
	Repositories       []realWorldRepository `json:"repositories"`
	Result             *realWorldResult      `json:"result,omitempty"`
	BeforeResult       *realWorldResult      `json:"beforeResult,omitempty"`
	Findings           []string              `json:"findings,omitempty"`
	ProductDecisions   []string              `json:"productDecisions,omitempty"`
	FollowUp           []string              `json:"followUp,omitempty"`
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
	ID                 string                `json:"id"`
	Date               string                `json:"date"`
	Title              string                `json:"title"`
	DollarLintRevision string                `json:"dollarlintRevision"`
	WorkingTreeNote    string                `json:"workingTreeNote"`
	Corpus             string                `json:"corpus"`
	CacheDir           string                `json:"cacheDir"`
	Command            string                `json:"command"`
	OutputArtifact     string                `json:"outputArtifact"`
	ManifestPath       string                `json:"manifestPath"`
	Repositories       []realWorldRepository `json:"repositories"`
	Findings           []string              `json:"findings"`
	ProductDecisions   []string              `json:"productDecisions"`
	FollowUp           []string              `json:"followUp"`
	Replace            bool                  `json:"replace"`
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
		"schemaVersion": history.SchemaVersion,
		"entryCount":    len(history.Entries),
		"repoCount":     len(tested),
		"testedRepos":   tested,
		"queries":       queryResults,
	}
	if args.IncludeEntries {
		out["entries"] = history.Entries
	}
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
		})
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
		return mcp.NewToolResultError("corpusDir is required"), nil
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
			return structured(map[string]any{"ok": false, "build": result})
		}
	}
	p.step("Running real-world corpus validation")
	validationArgs := realWorldValidationArgs(args.CorpusDir, schemaStore, args.SchemaStoreFailure, *args.FetchRetries, args.FetchRetryMinWait, args.FetchRetryMaxWait, args.OutputArtifact, args.ExtraArgs)
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
		"validationCommand": realWorldValidationCommand(args.CorpusDir, args.CacheDir, args.OutputArtifact, schemaStore, args.SchemaStoreFailure, *args.FetchRetries, args.FetchRetryMinWait, args.FetchRetryMaxWait, args.ExtraArgs),
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
		"ok":          true,
		"path":        filepath.Join(s.root, realWorldResultsRelPath),
		"entry":       entry,
		"replaced":    replaced,
		"entryCount":  len(history.Entries),
		"repoCount":   len(entry.Repositories),
		"testedRepos": realWorldTestedRepos(history),
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
		ID:                 id,
		Date:               date,
		Title:              args.Title,
		DollarLintRevision: revision,
		WorkingTreeNote:    workingTreeNote,
		Corpus:             args.Corpus,
		CacheDir:           args.CacheDir,
		Command:            command,
		OutputArtifact:     args.OutputArtifact,
		Repositories:       repositories,
		Result:             result,
		Findings:           args.Findings,
		ProductDecisions:   args.ProductDecisions,
		FollowUp:           args.FollowUp,
	}, nil
}

func loadRealWorldHistory(root string) (realWorldHistory, error) {
	path := filepath.Join(root, realWorldResultsRelPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return realWorldHistory{SchemaVersion: 1}, nil
	}
	if err != nil {
		return realWorldHistory{}, err
	}
	var history realWorldHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return realWorldHistory{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if history.SchemaVersion == 0 {
		history.SchemaVersion = 1
	}
	return history, nil
}

func saveRealWorldHistory(root string, history realWorldHistory) error {
	if history.SchemaVersion == 0 {
		history.SchemaVersion = 1
	}
	path := filepath.Join(root, realWorldResultsRelPath)
	return writeJSONFile(path, history)
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
