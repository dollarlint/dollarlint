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

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldValidationDefaultConcurrency = 4
	realWorldValidationMaxConcurrency     = 16
	realWorldValidationProgressInterval   = 5 * time.Second
	realWorldValidationRunTempPrefix      = "dollarlint-real-world-validation."
)

type realWorldStartValidationArgs struct {
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
	RunID string `json:"runID"`
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

	mu          sync.Mutex
	completedAt *time.Time
	completed   int
	delivered   int
	failed      int
	resultsByID map[string]realWorldRepoValidationResult
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
	RunID          string `json:"runID"`
	CorpusDir      string `json:"corpusDir"`
	CacheDir       string `json:"cacheDir"`
	OutputArtifact string `json:"outputArtifact"`
	ManifestPath   string `json:"manifestPath"`
	OutputDir      string `json:"outputDir"`
	Total          int    `json:"total"`
	Completed      int    `json:"completed"`
	Delivered      int    `json:"delivered"`
	Failed         int    `json:"failed"`
	Running        int    `json:"running"`
	Ready          int    `json:"ready"`
	Complete       bool   `json:"complete"`
	StartedAt      string `json:"startedAt"`
	CompletedAt    string `json:"completedAt,omitempty"`
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
	run, buildResult, err := s.startRealWorldValidationRun(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if buildResult != nil && !buildResult.Succeeded {
		return structured(map[string]any{
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
		return structured(out)
	}
	return structured(map[string]any{
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
	out, err := s.realWorldWaitForValidationResult(ctx, request, run)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out["ok"] = true
	return structured(out)
}

func (s *repoServer) handleRealWorldValidationStatus(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	return structured(map[string]any{
		"ok":       true,
		"run":      run.snapshot(),
		"nextStep": realWorldValidationNextStep(run),
	})
}

func (s *repoServer) handleRealWorldFinishValidation(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldFinishValidationArgs
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
	snapshot := run.snapshot()
	if !snapshot.Complete || snapshot.Ready > 0 {
		return structured(map[string]any{
			"ok":       false,
			"message":  "validation still has running or undelivered per-repository results; call real_world_next_validation_result and keep the tool call open for progress",
			"run":      snapshot,
			"nextStep": realWorldNextValidationResult(run.ID),
		})
	}
	outputArtifact := nonEmpty(args.OutputArtifact, run.OutputArtifact)
	summary, err := realWorldMergeValidationArtifacts(run, outputArtifact)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command := run.Command
	if command == "" {
		command = realWorldManagedValidationCommand(run)
	}
	return structured(map[string]any{
		"ok":                true,
		"run":               run.snapshot(),
		"summary":           summary,
		"corpusDir":         run.CorpusDir,
		"cacheDir":          run.CacheDir,
		"outputArtifact":    outputArtifact,
		"manifestPath":      run.ManifestPath,
		"repositories":      run.Repositories,
		"dependencyPrep":    run.DependencyPrep,
		"validationCommand": command,
		"nextStep":          realWorldNextTriageOutput(run.CorpusDir, run.CacheDir, outputArtifact, run.ManifestPath, command, run.Repositories, run.DependencyPrep),
	})
}

func (s *repoServer) handleRealWorldCancelValidation(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	return structured(map[string]any{
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
	}
	if s.realWorldRuns == nil {
		s.realWorldRuns = newRealWorldRunRegistry()
	}
	s.realWorldRuns.add(run)
	run.start(s.root)
	return run, buildResult, nil
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
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-run.ctx.Done():
				run.finishRepo(realWorldCanceledValidationResult(repo))
				return
			}
			run.finishRepo(run.validateRepo(root, repo))
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

func (run *realWorldValidationRun) markDelivered() {
	run.mu.Lock()
	defer run.mu.Unlock()
	run.delivered++
}

func (run *realWorldValidationRun) snapshot() realWorldValidationSnapshot {
	run.mu.Lock()
	defer run.mu.Unlock()
	completedAt := ""
	if run.completedAt != nil {
		completedAt = run.completedAt.UTC().Format(time.RFC3339)
	}
	total := len(run.Repositories)
	return realWorldValidationSnapshot{
		RunID:          run.ID,
		CorpusDir:      run.CorpusDir,
		CacheDir:       run.CacheDir,
		OutputArtifact: run.OutputArtifact,
		ManifestPath:   run.ManifestPath,
		OutputDir:      run.OutputDir,
		Total:          total,
		Completed:      run.completed,
		Delivered:      run.delivered,
		Failed:         run.failed,
		Running:        total - run.completed,
		Ready:          len(run.results),
		Complete:       run.completed == total,
		StartedAt:      run.StartedAt.UTC().Format(time.RFC3339),
		CompletedAt:    completedAt,
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
					"run":       snapshot,
					"complete":  true,
					"message":   "validation run has no more per-repo results",
					"nextStep":  realWorldValidationNextStep(run),
					"result":    nil,
					"resultOK":  false,
					"delivered": snapshot.Delivered,
				}, nil
			}
			run.markDelivered()
			snapshot := run.snapshot()
			return map[string]any{
				"run":       snapshot,
				"complete":  snapshot.Complete && snapshot.Ready == 0,
				"result":    result,
				"resultOK":  result.Accepted,
				"nextStep":  realWorldValidationNextStep(run),
				"delivered": snapshot.Delivered,
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
			"Review each returned result, then call this tool again until nextStep asks for real_world_finish_validation.",
		},
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
	}
}

func realWorldNextFinishValidation(runID string) map[string]any {
	return map[string]any{
		"tool": "real_world_finish_validation",
		"why":  "Merge completed per-repository validation artifacts into the standard real-world JSON artifact for triage and recording.",
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
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
	if err := writeJSONFile(outputArtifact, merged); err != nil {
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
