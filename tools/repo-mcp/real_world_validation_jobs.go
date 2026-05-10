package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dollarlint/dollarlint"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldValidationDefaultConcurrency = 4
	realWorldValidationMaxConcurrency     = 16
	realWorldValidationProgressInterval   = 5 * time.Second
	realWorldValidationRunTempPrefix      = "dollarlint-real-world-validation."
)

type realWorldStartValidationArgs struct {
	RunID              string                    `json:"runID"`
	CorpusDir          string                    `json:"corpusDir"`
	CacheDir           string                    `json:"cacheDir"`
	OutputArtifact     string                    `json:"outputArtifact"`
	ManifestPath       string                    `json:"manifestPath"`
	Repositories       []realWorldRepository     `json:"repositories"`
	DependencyPrep     []realWorldDependencyPrep `json:"dependencyPrep"`
	Build              *bool                     `json:"build"`
	SchemaStore        *bool                     `json:"schemaStore"`
	SchemaStoreFailure string                    `json:"schemaStoreFailure"`
	FetchRetries       *int                      `json:"fetchRetries"`
	FetchRetryMinWait  string                    `json:"fetchRetryMinWait"`
	FetchRetryMaxWait  string                    `json:"fetchRetryMaxWait"`
	ExtraArgs          []string                  `json:"extraArgs"`
	Concurrency        int                       `json:"concurrency"`
	WaitForFirstResult *bool                     `json:"waitForFirstResult"`
}

type realWorldNextValidationResultArgs struct {
	RunID    string                       `json:"runID"`
	Feedback *realWorldValidationFeedback `json:"feedback"`
}

type realWorldRecordValidationFeedbackArgs struct {
	RunID    string                      `json:"runID"`
	Feedback realWorldValidationFeedback `json:"feedback"`
}

type realWorldFinishValidationArgs struct {
	RunID          string `json:"runID"`
	OutputArtifact string `json:"outputArtifact"`
}

type realWorldValidationOptions struct {
	SchemaStore        bool
	SchemaStoreFailure string
	FetchRetries       int
	FetchRetryMinWait  string
	FetchRetryMaxWait  string
	ExtraArgs          []string
	Concurrency        int
}

type realWorldRunRegistry struct {
	mu   sync.Mutex
	runs map[string]*realWorldValidationRun
}

type realWorldValidationRun struct {
	ID             string
	Title          string
	CorpusDir      string
	CacheDir       string
	OutputArtifact string
	ManifestPath   string
	OutputDir      string
	Repositories   []realWorldRepository
	DependencyPrep []realWorldDependencyPrep
	Options        realWorldValidationOptions
	Command        string
	StartedAt      time.Time

	ctx    context.Context
	cancel context.CancelFunc

	results chan realWorldRepoValidationResult
	done    chan struct{}

	mu            sync.Mutex
	completedAt   *time.Time
	completed     int
	delivered     int
	failed        int
	resultsByID   map[string]realWorldRepoValidationResult
	deliveredByID map[string]realWorldRepoValidationResult
	feedbackByID  map[string]realWorldValidationFeedback
}

type realWorldRepoValidationResult struct {
	Repository     string           `json:"repository"`
	Path           string           `json:"path,omitempty"`
	OutputArtifact string           `json:"outputArtifact,omitempty"`
	CacheDir       string           `json:"cacheDir,omitempty"`
	Command        string           `json:"command,omitempty"`
	ExitCode       int              `json:"exitCode"`
	Duration       string           `json:"duration"`
	Succeeded      bool             `json:"succeeded"`
	Accepted       bool             `json:"accepted"`
	Summary        *realWorldResult `json:"summary,omitempty"`
	Warnings       int              `json:"warnings,omitempty"`
	Output         string           `json:"output,omitempty"`
	Error          string           `json:"error,omitempty"`
	StartedAt      string           `json:"startedAt"`
	FinishedAt     string           `json:"finishedAt"`
}

type realWorldValidationSnapshot struct {
	RunID                       string   `json:"runID"`
	Total                       int      `json:"total"`
	Completed                   int      `json:"completed"`
	Delivered                   int      `json:"delivered"`
	Failed                      int      `json:"failed"`
	Running                     int      `json:"running"`
	Ready                       int      `json:"ready"`
	FeedbackRecorded            int      `json:"feedbackRecorded"`
	FeedbackMissing             int      `json:"feedbackMissing"`
	FeedbackMissingRepositories []string `json:"feedbackMissingRepositories,omitempty"`
	FeedbackComplete            bool     `json:"feedbackComplete"`
	Complete                    bool     `json:"complete"`
	StartedAt                   string   `json:"startedAt"`
	CompletedAt                 string   `json:"completedAt,omitempty"`
}

type realWorldMergedOutput struct {
	Schema        string                 `json:"$schema,omitempty"`
	FormatVersion int                    `json:"formatVersion,omitempty"`
	Root          string                 `json:"root"`
	Summary       realWorldMergedSummary `json:"summary"`
	Files         []map[string]any       `json:"files"`
	Issues        []map[string]any       `json:"issues"`
	IgnoredIssues []map[string]any       `json:"ignoredIssues"`
	Warnings      []map[string]any       `json:"warnings"`
}

type realWorldMergedSummary struct {
	Discovered    int                   `json:"discovered"`
	Validated     int                   `json:"validated"`
	Skipped       int                   `json:"skipped"`
	Failed        int                   `json:"failed"`
	Issues        realWorldIssueSummary `json:"issues"`
	Ignored       int                   `json:"ignored"`
	Warnings      int                   `json:"warnings"`
	DurationNanos int64                 `json:"durationNanos"`
}

func newRealWorldRunRegistry() *realWorldRunRegistry {
	return &realWorldRunRegistry{runs: map[string]*realWorldValidationRun{}}
}

func (r *realWorldRunRegistry) add(run *realWorldValidationRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
}

func (r *realWorldRunRegistry) get(id string) (*realWorldValidationRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	return run, ok
}

func (s *repoServer) handleRealWorldStartValidation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldStartValidationArgs
	_ = request.BindArguments(&args)
	if err := realWorldRejectManualPathArgsWithRunID(args.RunID, map[string]string{
		"corpusDir":      args.CorpusDir,
		"cacheDir":       args.CacheDir,
		"outputArtifact": args.OutputArtifact,
		"manifestPath":   args.ManifestPath,
	}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.fillRealWorldStartValidationArgsFromRun(&args); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	run, buildResult, err := s.startRealWorldValidationRun(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if buildResult != nil && !buildResult.Succeeded {
		return s.realWorldStructured(ctx, map[string]any{
			"ok":       false,
			"build":    buildResult,
			"nextStep": realWorldNextFixBuild(),
		})
	}
	wait := true
	if args.WaitForFirstResult != nil {
		wait = *args.WaitForFirstResult
	}
	if wait {
		out, err := s.realWorldWaitForValidationResult(ctx, request, run)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out["ok"] = true
		out["started"] = true
		out["build"] = buildResult
		return s.realWorldStructured(ctx, out)
	}
	return s.realWorldStructured(ctx, map[string]any{
		"ok":       true,
		"started":  true,
		"build":    buildResult,
		"run":      run.snapshot(),
		"nextStep": realWorldNextValidationResult(run.ID),
	})
}

func (s *repoServer) handleRealWorldNextValidationResult(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldNextValidationResultArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required; call real_world_start_validation first"), nil
	}
	if s.realWorldRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	if args.Feedback != nil {
		if err := run.recordFeedback(*args.Feedback); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}
	if missing := run.missingFeedback(); len(missing) > 0 {
		return s.realWorldStructured(ctx, map[string]any{
			"ok":                        false,
			"message":                   "record validation feedback for the delivered repository result before waiting for another result",
			"run":                       run.snapshot(),
			"missingFeedback":           missing,
			"feedbackContract":          realWorldValidationFeedbackContract(),
			"validationFeedback":        run.validationFeedback(),
			"validationFeedbackSummary": realWorldValidationFeedbackSummary(run.validationFeedback()),
			"nextStep":                  realWorldNextValidationResultWithFeedback(run.ID, missing[0]),
		})
	}
	out, err := s.realWorldWaitForValidationResult(ctx, request, run)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out["ok"] = true
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) handleRealWorldRecordValidationFeedback(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldRecordValidationFeedbackArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required; call real_world_start_validation first"), nil
	}
	if s.realWorldRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	if err := run.recordFeedback(args.Feedback); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, map[string]any{
		"ok":                        true,
		"message":                   "validation feedback recorded",
		"run":                       run.snapshot(),
		"validationFeedback":        run.validationFeedback(),
		"validationFeedbackSummary": realWorldValidationFeedbackSummary(run.validationFeedback()),
		"nextStep":                  realWorldValidationNextStep(run),
	})
}

func (s *repoServer) handleRealWorldValidationStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldNextValidationResultArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required"), nil
	}
	if s.realWorldRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	return s.realWorldStructured(ctx, map[string]any{
		"ok":       true,
		"run":      run.snapshot(),
		"nextStep": realWorldValidationNextStep(run),
	})
}

func (s *repoServer) handleRealWorldFinishValidation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldFinishValidationArgs
	_ = request.BindArguments(&args)
	if err := realWorldRejectManualPathArgsWithRunID(args.RunID, map[string]string{
		"outputArtifact": args.OutputArtifact,
	}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required"), nil
	}
	if s.realWorldRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run.refreshFromManifest()
	snapshot := run.snapshot()
	if !snapshot.Complete || snapshot.Ready > 0 {
		return s.realWorldStructured(ctx, map[string]any{
			"ok":       false,
			"message":  "validation still has running or undelivered per-repository results; call real_world_next_validation_result and keep the tool call open for progress",
			"run":      snapshot,
			"nextStep": realWorldValidationNextStep(run),
		})
	}
	if missing := run.missingFeedback(); len(missing) > 0 {
		return s.realWorldStructured(ctx, map[string]any{
			"ok":                        false,
			"message":                   "validation has delivered repository results without recorded feedback; record feedback before finishing",
			"run":                       run.snapshot(),
			"missingFeedback":           missing,
			"feedbackContract":          realWorldValidationFeedbackContract(),
			"validationFeedback":        run.validationFeedback(),
			"validationFeedbackSummary": realWorldValidationFeedbackSummary(run.validationFeedback()),
			"nextStep":                  realWorldNextValidationResultWithFeedback(run.ID, missing[0]),
		})
	}
	outputArtifact := nonEmpty(args.OutputArtifact, run.OutputArtifact)
	summary, err := realWorldMergeValidationArtifacts(run, outputArtifact)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, map[string]any{
		"ok":                        true,
		"run":                       run.snapshot(),
		"summary":                   summary,
		"repositories":              realWorldPublicRepositories(run.Repositories),
		"dependencyPrep":            run.DependencyPrep,
		"validationFeedback":        run.validationFeedback(),
		"validationFeedbackSummary": realWorldValidationFeedbackSummary(run.validationFeedback()),
		"nextStep":                  realWorldNextTriageRun(run.ID),
	})
}

func (s *repoServer) handleRealWorldCancelValidation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldNextValidationResultArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required"), nil
	}
	if s.realWorldRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("validation run %q was not found", args.RunID)), nil
	}
	run.cancel()
	return s.realWorldStructured(ctx, map[string]any{
		"ok":      true,
		"message": "validation cancellation requested",
		"run":     run.snapshot(),
	})
}

func (s *repoServer) startRealWorldValidationRun(ctx context.Context, args realWorldStartValidationArgs) (*realWorldValidationRun, *commandResult, error) {
	if args.CorpusDir == "" {
		return nil, nil, fmt.Errorf("corpusDir is required; call real_world_prepare_corpus first")
	}
	if args.CacheDir == "" {
		cacheDir, err := os.MkdirTemp("", realWorldCacheTempPrefix)
		if err != nil {
			return nil, nil, err
		}
		args.CacheDir = cacheDir
	}
	if args.OutputArtifact == "" {
		outputArtifact, err := createRealWorldOutputPath("", "")
		if err != nil {
			return nil, nil, err
		}
		args.OutputArtifact = outputArtifact
	}
	if args.ManifestPath == "" {
		args.ManifestPath = filepath.Join(args.CorpusDir, realWorldManifestName)
	}
	repositories := append([]realWorldRepository{}, args.Repositories...)
	if len(repositories) == 0 {
		manifest, err := readRealWorldManifest(args.ManifestPath)
		if err != nil {
			return nil, nil, err
		}
		repositories = manifest.Repositories
		if len(args.DependencyPrep) == 0 {
			args.DependencyPrep = manifest.DependencyPrep
		}
	}
	if len(repositories) == 0 {
		return nil, nil, fmt.Errorf("repositories is required")
	}
	options := realWorldValidationOptionsFromArgs(args)
	build := true
	if args.Build != nil {
		build = *args.Build
	}
	var buildResult *commandResult
	if build {
		result := s.run(ctx, namedCommand{Name: "go build", Cmd: "go build -o bin/dollarlint ./cmd/dollarlint"})
		buildResult = &result
		if !result.Succeeded {
			return nil, buildResult, nil
		}
	}
	outputDir, err := os.MkdirTemp("", realWorldValidationRunTempPrefix)
	if err != nil {
		return nil, nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	runID := realWorldValidationRunID(args.CorpusDir)
	run := &realWorldValidationRun{
		ID:             runID,
		CorpusDir:      args.CorpusDir,
		CacheDir:       args.CacheDir,
		OutputArtifact: args.OutputArtifact,
		ManifestPath:   args.ManifestPath,
		OutputDir:      outputDir,
		Repositories:   repositories,
		DependencyPrep: append([]realWorldDependencyPrep{}, args.DependencyPrep...),
		Options:        options,
		Command:        realWorldManagedValidationCommandFor(args.CorpusDir, args.CacheDir, args.OutputArtifact, options),
		StartedAt:      time.Now(),
		ctx:            runCtx,
		cancel:         cancel,
		results:        make(chan realWorldRepoValidationResult, len(repositories)),
		done:           make(chan struct{}),
		resultsByID:    map[string]realWorldRepoValidationResult{},
		deliveredByID:  map[string]realWorldRepoValidationResult{},
		feedbackByID:   map[string]realWorldValidationFeedback{},
	}
	if s.realWorldRuns == nil {
		s.realWorldRuns = newRealWorldRunRegistry()
	}
	s.realWorldRuns.add(run)
	run.start(s.root)
	return run, buildResult, nil
}

func (s *repoServer) fillRealWorldStartValidationArgsFromRun(args *realWorldStartValidationArgs) error {
	if args.RunID == "" {
		return nil
	}
	if s.realWorldPrepareRuns == nil {
		return fmt.Errorf("corpus preparation run %q was not found", args.RunID)
	}
	run, ok := s.realWorldPrepareRuns.get(args.RunID)
	if !ok {
		return fmt.Errorf("corpus preparation run %q was not found", args.RunID)
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
	if len(args.Repositories) == 0 {
		args.Repositories = run.repositories()
	}
	if len(args.DependencyPrep) == 0 {
		args.DependencyPrep = run.dependencyPrep()
	}
	return nil
}

func realWorldValidationOptionsFromArgs(args realWorldStartValidationArgs) realWorldValidationOptions {
	schemaStore := true
	if args.SchemaStore != nil {
		schemaStore = *args.SchemaStore
	}
	if args.SchemaStoreFailure == "" {
		args.SchemaStoreFailure = "warn"
	}
	fetchRetries := 1
	if args.FetchRetries != nil {
		fetchRetries = *args.FetchRetries
	}
	if args.FetchRetryMinWait == "" {
		args.FetchRetryMinWait = "1ms"
	}
	if args.FetchRetryMaxWait == "" {
		args.FetchRetryMaxWait = "1ms"
	}
	concurrency := args.Concurrency
	if concurrency <= 0 {
		concurrency = realWorldValidationDefaultConcurrency
	}
	if concurrency > realWorldValidationMaxConcurrency {
		concurrency = realWorldValidationMaxConcurrency
	}
	return realWorldValidationOptions{
		SchemaStore:        schemaStore,
		SchemaStoreFailure: args.SchemaStoreFailure,
		FetchRetries:       fetchRetries,
		FetchRetryMinWait:  args.FetchRetryMinWait,
		FetchRetryMaxWait:  args.FetchRetryMaxWait,
		ExtraArgs:          append([]string{}, args.ExtraArgs...),
		Concurrency:        concurrency,
	}
}

func (run *realWorldValidationRun) start(root string) {
	sem := make(chan struct{}, run.Options.Concurrency)
	var wg sync.WaitGroup
	for _, repo := range run.Repositories {
		repo := repo
		wg.Add(1)
		go func() {
			defer wg.Done()
			started := time.Now()
			readyRepo, readyErr := run.waitForRepositoryReady(repo)
			if readyErr != nil {
				run.finishRepo(realWorldRepositoryNotReadyValidationResult(repo, readyRepo, readyErr, started))
				return
			}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-run.ctx.Done():
				run.finishRepo(realWorldCanceledValidationResult(repo))
				return
			}
			run.finishRepo(run.validateRepo(root, readyRepo))
		}()
	}
	go func() {
		wg.Wait()
		now := time.Now()
		run.mu.Lock()
		run.completedAt = &now
		run.mu.Unlock()
		close(run.results)
		close(run.done)
	}()
}

func (run *realWorldValidationRun) validateRepo(root string, repo realWorldRepository) realWorldRepoValidationResult {
	started := time.Now()
	name := nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path))
	result := realWorldRepoValidationResult{
		Repository: name,
		Path:       repo.Path,
		StartedAt:  started.UTC().Format(time.RFC3339),
	}
	readyRepo, readyErr := run.waitForRepositoryReady(repo)
	if readyErr != nil {
		result.Path = readyRepo.Path
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		result.Error = readyErr.Error()
		result.Duration = time.Since(started).Round(time.Millisecond).String()
		return result
	}
	repo = readyRepo
	result.Path = repo.Path
	if repo.Path == "" {
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		result.Error = "repository path is empty"
		result.Duration = time.Since(started).Round(time.Millisecond).String()
		return result
	}
	if info, err := os.Stat(repo.Path); err != nil || !info.IsDir() {
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Error = "repository path is not a directory"
		}
		result.Duration = time.Since(started).Round(time.Millisecond).String()
		return result
	}
	outputArtifact := filepath.Join(run.OutputDir, slugify(name)+".json")
	cacheDir := filepath.Join(run.CacheDir, "repo-"+slugify(name))
	_ = os.MkdirAll(cacheDir, 0o755)
	validationArgs := realWorldValidationArgs(repo.Path, run.Options.SchemaStore, run.Options.SchemaStoreFailure, run.Options.FetchRetries, run.Options.FetchRetryMinWait, run.Options.FetchRetryMaxWait, outputArtifact, run.Options.ExtraArgs)
	processResult := runProcess(run.ctx, root, []string{"XDG_CACHE_HOME=" + cacheDir}, filepath.Join(root, "bin/dollarlint"), validationArgs...)
	summary, warnings, outputErr := readRealWorldOutputSummary(outputArtifact)
	result.OutputArtifact = outputArtifact
	result.CacheDir = cacheDir
	result.Command = processResult.Command
	result.ExitCode = processResult.ExitCode
	result.Duration = processResult.Duration
	result.Succeeded = processResult.Succeeded
	result.Accepted = processResult.Succeeded || (processResult.ExitCode == 1 && outputErr == nil)
	result.Summary = summary
	result.Warnings = len(warnings)
	result.Output = processResult.Output
	if outputErr != nil {
		result.Error = outputErr.Error()
	}
	if errors.Is(run.ctx.Err(), context.Canceled) && result.Error == "" {
		result.Error = "validation run was canceled"
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return result
}

func (run *realWorldValidationRun) waitForRepositoryReady(repo realWorldRepository) (realWorldRepository, error) {
	if repo.Status == "cloned" && realWorldRepoPathReady(repo.Path) {
		return repo, nil
	}
	if run.ManifestPath == "" {
		return repo, nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		manifest, err := readRealWorldManifest(run.ManifestPath)
		if err == nil {
			if matched, ok := realWorldManifestRepository(manifest, repo); ok {
				repo = matched
			}
			switch repo.Status {
			case "cloned":
				if realWorldRepoPathReady(repo.Path) {
					return repo, nil
				}
			case "error", "canceled":
				return repo, fmt.Errorf("repository preparation %s: %s", repo.Status, nonEmpty(repo.Error, "clone did not produce a prepared repository"))
			}
			if !manifest.PreparationManaged || manifest.PreparationComplete {
				if realWorldRepoPathReady(repo.Path) {
					return repo, nil
				}
				return repo, fmt.Errorf("repository was not prepared before validation: status=%s path=%s", nonEmpty(repo.Status, "unknown"), repo.Path)
			}
		} else if realWorldRepoPathReady(repo.Path) {
			return repo, nil
		}
		select {
		case <-run.ctx.Done():
			return repo, fmt.Errorf("validation canceled while waiting for repository preparation")
		case <-ticker.C:
		}
	}
}

func realWorldRepoPathReady(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func realWorldManifestRepository(manifest realWorldManifest, repo realWorldRepository) (realWorldRepository, bool) {
	key := slugify(nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path)))
	for _, candidate := range manifest.Repositories {
		candidateKey := slugify(nonEmpty(candidate.Name, repoNameFromURL(candidate.CloneURL), filepath.Base(candidate.Path)))
		if candidateKey != "" && candidateKey == key {
			return candidate, true
		}
		if repo.CloneURL != "" && candidate.CloneURL == repo.CloneURL {
			return candidate, true
		}
		if repo.Path != "" && candidate.Path == repo.Path {
			return candidate, true
		}
	}
	return realWorldRepository{}, false
}

func (run *realWorldValidationRun) finishRepo(result realWorldRepoValidationResult) {
	key := slugify(result.Repository)
	run.mu.Lock()
	run.completed++
	if !result.Accepted {
		run.failed++
	}
	run.resultsByID[key] = result
	run.mu.Unlock()
	run.results <- result
}

func realWorldCanceledValidationResult(repo realWorldRepository) realWorldRepoValidationResult {
	now := time.Now().UTC().Format(time.RFC3339)
	name := nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path))
	return realWorldRepoValidationResult{
		Repository: name,
		Path:       repo.Path,
		StartedAt:  now,
		FinishedAt: now,
		Error:      "validation run was canceled before this repository started",
		Duration:   "0s",
	}
}

func realWorldRepositoryNotReadyValidationResult(original, ready realWorldRepository, err error, started time.Time) realWorldRepoValidationResult {
	now := time.Now().UTC().Format(time.RFC3339)
	name := nonEmpty(ready.Name, original.Name, repoNameFromURL(nonEmpty(ready.CloneURL, original.CloneURL)), filepath.Base(nonEmpty(ready.Path, original.Path)))
	return realWorldRepoValidationResult{
		Repository: name,
		Path:       nonEmpty(ready.Path, original.Path),
		StartedAt:  started.UTC().Format(time.RFC3339),
		FinishedAt: now,
		Error:      err.Error(),
		Duration:   time.Since(started).Round(time.Millisecond).String(),
	}
}

func (run *realWorldValidationRun) markDelivered(result realWorldRepoValidationResult) {
	key := slugify(result.Repository)
	run.mu.Lock()
	defer run.mu.Unlock()
	run.delivered++
	run.deliveredByID[key] = result
}

func (run *realWorldValidationRun) snapshot() realWorldValidationSnapshot {
	run.mu.Lock()
	defer run.mu.Unlock()
	completedAt := ""
	if run.completedAt != nil {
		completedAt = run.completedAt.UTC().Format(time.RFC3339)
	}
	total := len(run.Repositories)
	missingFeedback := run.missingFeedbackLocked()
	return realWorldValidationSnapshot{
		RunID:                       run.ID,
		Total:                       total,
		Completed:                   run.completed,
		Delivered:                   run.delivered,
		Failed:                      run.failed,
		Running:                     total - run.completed,
		Ready:                       len(run.results),
		FeedbackRecorded:            len(run.feedbackByID),
		FeedbackMissing:             len(missingFeedback),
		FeedbackMissingRepositories: missingFeedback,
		FeedbackComplete:            len(missingFeedback) == 0,
		Complete:                    run.completed == total,
		StartedAt:                   run.StartedAt.UTC().Format(time.RFC3339),
		CompletedAt:                 completedAt,
	}
}

func (run *realWorldValidationRun) recordFeedback(feedback realWorldValidationFeedback) error {
	if err := validateRealWorldValidationFeedback(feedback); err != nil {
		return err
	}
	key := slugify(feedback.Repository)
	run.mu.Lock()
	defer run.mu.Unlock()
	result, ok := run.deliveredByID[key]
	if !ok {
		if _, exists := run.resultsByID[key]; exists {
			return fmt.Errorf("validation feedback for %q cannot be recorded until that repository result has been delivered", feedback.Repository)
		}
		return fmt.Errorf("validation feedback references unknown or undelivered repository %q", feedback.Repository)
	}
	feedback.Repository = result.Repository
	run.feedbackByID[key] = feedback
	return nil
}

func (run *realWorldValidationRun) missingFeedback() []string {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.missingFeedbackLocked()
}

func (run *realWorldValidationRun) missingFeedbackLocked() []string {
	missing := make([]string, 0, len(run.deliveredByID))
	for key, result := range run.deliveredByID {
		if _, ok := run.feedbackByID[key]; !ok {
			missing = append(missing, result.Repository)
		}
	}
	sort.Strings(missing)
	return missing
}

func (run *realWorldValidationRun) validationFeedback() []realWorldValidationFeedback {
	run.mu.Lock()
	defer run.mu.Unlock()
	feedback := make([]realWorldValidationFeedback, 0, len(run.feedbackByID))
	for _, item := range run.feedbackByID {
		feedback = append(feedback, item)
	}
	sort.Slice(feedback, func(i, j int) bool {
		return feedback[i].Repository < feedback[j].Repository
	})
	return feedback
}

func (run *realWorldValidationRun) refreshFromManifest() {
	if run.ManifestPath == "" {
		return
	}
	manifest, err := readRealWorldManifest(run.ManifestPath)
	if err != nil {
		return
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if len(manifest.Repositories) > 0 {
		run.Repositories = manifest.Repositories
	}
	if len(manifest.DependencyPrep) > 0 {
		run.DependencyPrep = manifest.DependencyPrep
	}
}

func (s *repoServer) realWorldWaitForValidationResult(ctx context.Context, request mcp.CallToolRequest, run *realWorldValidationRun) (map[string]any, error) {
	p := newProgress(ctx, s.mcp, request, len(run.Repositories))
	snapshot := run.snapshot()
	p.report(snapshot.Completed, snapshot.Total, realWorldValidationProgressMessage(run, snapshot))
	ticker := time.NewTicker(realWorldValidationProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case result, ok := <-run.results:
			if !ok {
				snapshot := run.snapshot()
				return map[string]any{
					"run":                       snapshot,
					"complete":                  true,
					"message":                   "validation run has no more per-repo results",
					"feedbackContract":          realWorldValidationFeedbackContract(),
					"validationFeedback":        run.validationFeedback(),
					"validationFeedbackSummary": realWorldValidationFeedbackSummary(run.validationFeedback()),
					"nextStep":                  realWorldValidationNextStep(run),
					"result":                    nil,
					"resultOK":                  false,
					"delivered":                 snapshot.Delivered,
				}, nil
			}
			run.markDelivered(result)
			snapshot := run.snapshot()
			return map[string]any{
				"run":                       snapshot,
				"complete":                  snapshot.Complete && snapshot.Ready == 0,
				"result":                    realWorldPublicValidationResult(run, result),
				"resultOK":                  result.Accepted,
				"feedbackContract":          realWorldValidationFeedbackContract(),
				"validationFeedback":        run.validationFeedback(),
				"validationFeedbackSummary": realWorldValidationFeedbackSummary(run.validationFeedback()),
				"nextStep":                  realWorldValidationNextStep(run),
				"delivered":                 snapshot.Delivered,
			}, nil
		case <-ticker.C:
			snapshot := run.snapshot()
			p.report(snapshot.Completed, snapshot.Total, realWorldValidationProgressMessage(run, snapshot))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func realWorldValidationProgressMessage(run *realWorldValidationRun, snapshot realWorldValidationSnapshot) string {
	return fmt.Sprintf("Validation run %s: %d/%d repositories complete, %d running, %d result(s) ready, %d failed.",
		run.ID, snapshot.Completed, snapshot.Total, snapshot.Running, snapshot.Ready, snapshot.Failed)
}

func realWorldValidationNextStep(run *realWorldValidationRun) map[string]any {
	snapshot := run.snapshot()
	if len(snapshot.FeedbackMissingRepositories) > 0 {
		return realWorldNextValidationResultWithFeedback(run.ID, snapshot.FeedbackMissingRepositories[0])
	}
	if snapshot.Complete && snapshot.Ready == 0 {
		return realWorldNextFinishValidation(run.ID)
	}
	return realWorldNextValidationResult(run.ID)
}

func realWorldNextValidationResult(runID string) map[string]any {
	return map[string]any{
		"tool": "real_world_next_validation_result",
		"why":  "Wait with MCP progress notifications until the next per-repository validation result is ready.",
		"beforeCalling": []string{
			"Do not poll with shell sleep loops.",
			"Keep this tool call open while validation is running; it will send progress notifications and return the next completed repository result.",
			"Review each returned result as a developer experience: correctness, clarity, noise, skipped coverage, and whether DollarLint helped the user decide what to do next.",
			"Use the result.evidence bundle before deciding; raw count strings alone are rejected as feedback.",
			"Call this tool again with evidence-backed feedback for that result until nextStep asks for real_world_finish_validation.",
		},
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
	}
}

func realWorldNextValidationResultWithFeedback(runID, repository string) map[string]any {
	return map[string]any{
		"tool": "real_world_next_validation_result",
		"why":  "Record structured feedback for the previously delivered repository result, then wait with progress notifications for the next result.",
		"beforeCalling": []string{
			"Do not poll with shell sleep loops.",
			"Review the delivered repository result like a developer trying DollarLint, including correctness and ergonomics.",
			"Record whether the result revealed a product signal, behaved reasonably with evidence, or was blocked.",
			"Write qualitative evidence in findings, caveats, notes, or productRecommendations; raw count strings alone are rejected.",
			"Pass feedback in this call so the tool can keep waiting for the next completed repository result.",
			"If you only need to record feedback without waiting, call real_world_record_validation_feedback instead.",
		},
		"feedbackContract": realWorldValidationFeedbackContract(),
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
		"feedbackTemplate": map[string]any{
			"repository": repository,
			"outcome":    "choose behaved-reasonably, product-signal, or blocked",
			"findings": []string{
				"Summarize concrete qualitative evidence from this repository result before deciding.",
			},
			"notes": "Explain the developer experience, including why any findings are or are not actionable product feedback.",
		},
	}
}

func realWorldNextFinishValidation(runID string) map[string]any {
	return map[string]any{
		"tool": "real_world_finish_validation",
		"why":  "Merge completed per-repository validation artifacts into the standard real-world JSON artifact for triage and recording.",
		"beforeCalling": []string{
			"Only finish after every delivered repository result has validationFeedback recorded.",
		},
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
	}
}

func realWorldValidationFeedbackContract() map[string]any {
	return map[string]any{
		"required": []string{
			"repository",
			"outcome",
		},
		"assessmentPerspective":  realWorldDeveloperExperienceGuidance(),
		"outcome":                "Use behaved-reasonably when DollarLint handled the repo sensibly for a developer, product-signal when the result suggests a correctness or ergonomics product change to consider, or blocked when the repo result could not be interpreted.",
		"evidence":               "Use cliPreview, exampleIssues, skippedGroups, source excerpts, and concrete qualitative findings or caveats so the final answer can be reconstructed after context compaction. Raw counts and artifact references are rejected; notes-only boilerplate is rejected; behaved-reasonably feedback must include findings or caveats.",
		"productRecommendations": "Only include concrete product changes worth considering. Each recommendation needs strength high|med|low, recommendation, and rationale. Ergonomic improvements count when they would make the tool more useful or less confusing.",
		"finalUse":               "real_world_finish_validation and real_world_triage_output carry this ledger into the draft record; the final user response should use it instead of relying on conversation memory.",
	}
}

func realWorldValidationFeedbackSummary(feedback []realWorldValidationFeedback) map[string]any {
	counts := map[string]int{
		realWorldFeedbackBehavedReasonably: 0,
		realWorldFeedbackProductSignal:     0,
		realWorldFeedbackBlocked:           0,
	}
	recommendations := 0
	for _, item := range feedback {
		counts[item.Outcome]++
		recommendations += len(item.ProductRecommendations)
	}
	return map[string]any{
		"total":                  len(feedback),
		"behavedReasonably":      counts[realWorldFeedbackBehavedReasonably],
		"productSignals":         counts[realWorldFeedbackProductSignal],
		"blocked":                counts[realWorldFeedbackBlocked],
		"productRecommendations": recommendations,
		"developerExperience":    "Feedback assesses DollarLint's developer experience, including correctness, ergonomics, clarity, noise, and coverage caveats.",
	}
}

func realWorldMergeValidationArtifacts(run *realWorldValidationRun, outputArtifact string) (*realWorldResult, error) {
	run.mu.Lock()
	results := make([]realWorldRepoValidationResult, 0, len(run.resultsByID))
	for _, result := range run.resultsByID {
		results = append(results, result)
	}
	run.mu.Unlock()
	sort.Slice(results, func(i, j int) bool {
		return results[i].Repository < results[j].Repository
	})
	merged := realWorldMergedOutput{
		Root:          run.CorpusDir,
		FormatVersion: 1,
		Files:         []map[string]any{},
		Issues:        []map[string]any{},
		IgnoredIssues: []map[string]any{},
		Warnings:      []map[string]any{},
	}
	for _, result := range results {
		if !result.Accepted || result.OutputArtifact == "" {
			merged.Warnings = append(merged.Warnings, map[string]any{
				"kind":    "realWorldRepoValidationFailed",
				"path":    result.Repository,
				"message": nonEmpty(result.Error, result.Output, "repository validation did not produce a usable JSON artifact"),
			})
			merged.Summary.Warnings++
			continue
		}
		payload, err := readRealWorldRawOutput(result.OutputArtifact)
		if err != nil {
			return nil, err
		}
		if payload.Schema != "" {
			merged.Schema = payload.Schema
		}
		if payload.FormatVersion != 0 {
			merged.FormatVersion = payload.FormatVersion
		}
		merged.Summary.add(payload.Summary)
		for _, file := range payload.Files {
			merged.Files = append(merged.Files, realWorldPrefixOutputPath(file, result.Repository, result.Path))
		}
		for _, issue := range payload.Issues {
			merged.Issues = append(merged.Issues, realWorldPrefixOutputPath(issue, result.Repository, result.Path))
		}
		for _, issue := range payload.IgnoredIssues {
			merged.IgnoredIssues = append(merged.IgnoredIssues, realWorldPrefixOutputPath(issue, result.Repository, result.Path))
		}
		for _, warning := range payload.Warnings {
			merged.Warnings = append(merged.Warnings, realWorldPrefixOutputPath(warning, result.Repository, result.Path))
		}
	}
	if err := writeRealWorldBundleOutput(outputArtifact, merged); err != nil {
		return nil, err
	}
	return &realWorldResult{
		Discovered: merged.Summary.Discovered,
		Validated:  merged.Summary.Validated,
		Skipped:    merged.Summary.Skipped,
		Failed:     merged.Summary.Failed,
		Issues:     merged.Summary.Issues,
		Ignored:    merged.Summary.Ignored,
		Warnings:   merged.Summary.Warnings,
		Duration:   &realWorldDurationInfo{Nanos: merged.Summary.DurationNanos},
	}, nil
}

func writeRealWorldBundleOutput(path string, merged realWorldMergedOutput) error {
	bundle, err := dollarlint.FormatBundle(realWorldMergedOutputToResult(merged), dollarlint.OutputConfig{ShowSkipped: true, Locations: true})
	if err != nil {
		return err
	}
	if len(bundle) == 0 || bundle[len(bundle)-1] != '\n' {
		bundle = append(bundle, '\n')
	}
	return os.WriteFile(path, bundle, 0o644)
}

func realWorldMergedOutputToResult(merged realWorldMergedOutput) dollarlint.Result {
	result := dollarlint.Result{
		Root: merged.Root,
		Summary: dollarlint.Summary{
			Discovered: merged.Summary.Discovered,
			Validated:  merged.Summary.Validated,
			Skipped:    merged.Summary.Skipped,
			Failed:     merged.Summary.Failed,
			Issues: dollarlint.IssueSummary{
				Total:      merged.Summary.Issues.Total,
				Parsing:    merged.Summary.Issues.Parsing,
				Validation: merged.Summary.Issues.Validation,
				Schema:     merged.Summary.Issues.Schema,
				Coverage:   merged.Summary.Issues.Coverage,
			},
			Ignored:       merged.Summary.Ignored,
			Warnings:      merged.Summary.Warnings,
			DurationNanos: merged.Summary.DurationNanos,
		},
	}
	for _, file := range merged.Files {
		path := realWorldMapString(file, "path")
		result.Files = append(result.Files, dollarlint.FileResult{
			Path:           path,
			RelativePath:   path,
			Format:         realWorldMapString(file, "format"),
			Schema:         realWorldMapString(file, "schema"),
			SchemaSource:   realWorldMapString(file, "schemaSource"),
			Status:         realWorldMapString(file, "status"),
			Issues:         realWorldMapInt(file, "issues"),
			Ignored:        realWorldMapInt(file, "ignored"),
			Message:        realWorldMapString(file, "message"),
			SkipReason:     realWorldMapString(file, "skipReason"),
			SkipClass:      realWorldMapString(file, "skipClass"),
			SkipImportance: realWorldMapString(file, "skipImportance"),
			SkipDetail:     realWorldMapString(file, "skipDetail"),
		})
	}
	for _, issue := range merged.Issues {
		result.Issues = append(result.Issues, realWorldMapIssue(issue, false))
	}
	for _, issue := range merged.IgnoredIssues {
		result.Issues = append(result.Issues, realWorldMapIssue(issue, true))
	}
	for _, warning := range merged.Warnings {
		result.Warnings = append(result.Warnings, dollarlint.Warning{
			Kind:         realWorldMapString(warning, "kind"),
			Source:       realWorldMapString(warning, "source"),
			Path:         realWorldMapString(warning, "path"),
			Schema:       realWorldMapString(warning, "schema"),
			SchemaSource: realWorldMapString(warning, "schemaSource"),
			Message:      realWorldMapString(warning, "message"),
			Hint:         realWorldMapString(warning, "hint"),
		})
	}
	return result
}

func realWorldMapIssue(raw map[string]any, ignored bool) dollarlint.Issue {
	path := realWorldMapString(raw, "path")
	return dollarlint.Issue{
		File:             path,
		RelativePath:     path,
		Schema:           realWorldMapString(raw, "schema"),
		SchemaSource:     realWorldMapString(raw, "schemaSource"),
		Keyword:          realWorldMapString(raw, "keyword"),
		KeywordLocation:  realWorldMapString(raw, "keywordLocation"),
		Property:         realWorldMapString(raw, "property"),
		InstanceLocation: realWorldMapString(raw, "instanceLocation"),
		Line:             realWorldMapInt(raw, "line"),
		Column:           realWorldMapInt(raw, "column"),
		Message:          realWorldMapString(raw, "message"),
		Hint:             realWorldMapString(raw, "hint"),
		Ignored:          ignored,
		IgnoreReason:     realWorldMapString(raw, "ignoreReason"),
	}
}

func (s *realWorldMergedSummary) add(other realWorldMergedSummary) {
	s.Discovered += other.Discovered
	s.Validated += other.Validated
	s.Skipped += other.Skipped
	s.Failed += other.Failed
	s.Issues.Total += other.Issues.Total
	s.Issues.Parsing += other.Issues.Parsing
	s.Issues.Validation += other.Issues.Validation
	s.Issues.Schema += other.Issues.Schema
	s.Issues.Coverage += other.Issues.Coverage
	s.Ignored += other.Ignored
	s.Warnings += other.Warnings
	s.DurationNanos += other.DurationNanos
}

func readRealWorldRawOutput(path string) (realWorldMergedOutput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return realWorldMergedOutput{}, err
	}
	data, _, _, err = realWorldUnwrapBundle(data)
	if err != nil {
		return realWorldMergedOutput{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var payload realWorldMergedOutput
	if err := json.Unmarshal(data, &payload); err != nil {
		return realWorldMergedOutput{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return payload, nil
}

func realWorldPrefixOutputPath(item map[string]any, repoName, repoPath string) map[string]any {
	out := make(map[string]any, len(item))
	for key, value := range item {
		out[key] = value
	}
	raw, ok := out["path"].(string)
	if !ok || raw == "" {
		return out
	}
	rel := filepath.ToSlash(raw)
	if filepath.IsAbs(raw) {
		if repoPath != "" {
			if maybeRel, err := filepath.Rel(repoPath, raw); err == nil && maybeRel != "." && maybeRel != ".." && !strings.HasPrefix(maybeRel, ".."+string(filepath.Separator)) {
				rel = filepath.ToSlash(maybeRel)
			}
		}
	}
	if rel == "." || rel == "" {
		rel = filepath.Base(raw)
	}
	repo := slugify(repoName)
	if repo == "" {
		repo = repoName
	}
	out["path"] = filepath.ToSlash(filepath.Join(repo, rel))
	return out
}

func realWorldValidationRunID(corpusDir string) string {
	base := slugify(filepath.Base(corpusDir))
	if base == "" {
		base = "validation"
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

func realWorldManagedValidationCommand(run *realWorldValidationRun) string {
	return realWorldManagedValidationCommandFor(run.CorpusDir, run.CacheDir, run.OutputArtifact, run.Options)
}

func realWorldManagedValidationCommandFor(corpusDir, cacheDir, outputArtifact string, options realWorldValidationOptions) string {
	return fmt.Sprintf("real_world_start_validation corpusDir=%s cacheDir=%s outputArtifact=%s concurrency=%d schemaStore=%t schemaStoreFailure=%s",
		shellQuote(corpusDir), shellQuote(cacheDir), shellQuote(outputArtifact), options.Concurrency, options.SchemaStore, shellQuote(options.SchemaStoreFailure))
}
