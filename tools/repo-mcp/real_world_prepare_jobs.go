package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldPrepareDefaultConcurrency = 4
	realWorldPrepareMaxConcurrency     = 12
	realWorldPrepareProgressInterval   = 5 * time.Second
)

type realWorldStartPrepareArgs struct {
	Title          string
	CorpusDir      string
	CacheDir       string
	OutputArtifact string
	Repositories   []realWorldRepository
	Concurrency    int
}

type realWorldNextPreparedRepoArgs struct {
	RunID string `json:"runID"`
}

type realWorldPrepareRegistry struct {
	mu   sync.Mutex
	runs map[string]*realWorldPrepareRun
}

type realWorldPrepareRun struct {
	ID             string
	Title          string
	CorpusDir      string
	CacheDir       string
	OutputArtifact string
	ManifestPath   string
	Repositories   []realWorldRepository
	Concurrency    int
	StartedAt      time.Time

	ctx    context.Context
	cancel context.CancelFunc

	results chan realWorldRepoPrepareResult
	done    chan struct{}

	mu             sync.Mutex
	completedAt    *time.Time
	completed      int
	delivered      int
	failed         int
	resultsByID    map[string]realWorldRepoPrepareResult
	inspectionByID map[string]realWorldDependencyPrepScan
	prepByID       map[string]realWorldDependencyPrep
}

type realWorldRepoPrepareResult struct {
	Repository               string                       `json:"repository"`
	CloneURL                 string                       `json:"cloneURL,omitempty"`
	Path                     string                       `json:"path,omitempty"`
	Commit                   string                       `json:"commit,omitempty"`
	Status                   string                       `json:"status"`
	Succeeded                bool                         `json:"succeeded"`
	Command                  string                       `json:"command,omitempty"`
	Duration                 string                       `json:"duration"`
	Output                   string                       `json:"output,omitempty"`
	Error                    string                       `json:"error,omitempty"`
	StartedAt                string                       `json:"startedAt"`
	FinishedAt               string                       `json:"finishedAt"`
	RepositoryRecord         realWorldRepository          `json:"repositoryRecord"`
	DependencyPrepInspection *realWorldDependencyPrepScan `json:"dependencyPrepInspection,omitempty"`
	DependencyPrep           *realWorldDependencyPrep     `json:"dependencyPrep,omitempty"`
	PrepSecurityPolicy       map[string]any               `json:"prepSecurityPolicy,omitempty"`
}

type realWorldPrepareSnapshot struct {
	RunID          string `json:"runID"`
	CorpusDir      string `json:"corpusDir"`
	CacheDir       string `json:"cacheDir"`
	OutputArtifact string `json:"outputArtifact"`
	ManifestPath   string `json:"manifestPath"`
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

func newRealWorldPrepareRegistry() *realWorldPrepareRegistry {
	return &realWorldPrepareRegistry{runs: map[string]*realWorldPrepareRun{}}
}

func (r *realWorldPrepareRegistry) add(run *realWorldPrepareRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
}

func (r *realWorldPrepareRegistry) get(id string) (*realWorldPrepareRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	return run, ok
}

func (s *repoServer) handleRealWorldNextPreparedRepo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldNextPreparedRepoArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required; call real_world_prepare_corpus first"), nil
	}
	if s.realWorldPrepareRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldPrepareRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
	}
	out, err := s.realWorldWaitForPreparedRepo(ctx, request, run)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out["ok"] = true
	return structured(out)
}

func (s *repoServer) handleRealWorldPrepareStatus(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldNextPreparedRepoArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required"), nil
	}
	if s.realWorldPrepareRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldPrepareRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
	}
	return structured(map[string]any{
		"ok":                 true,
		"run":                run.snapshot(),
		"dependencyPrep":     run.dependencyPrep(),
		"inspection":         run.inspection(),
		"nextStep":           realWorldPrepareNextStep(run),
		"validationStep":     realWorldNextRunCorpusDuringPrepare(run),
		"prepSecurityPolicy": realWorldDependencyPrepSecurityPolicy(),
	})
}

func (s *repoServer) handleRealWorldCancelPrepare(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldNextPreparedRepoArgs
	_ = request.BindArguments(&args)
	if args.RunID == "" {
		return mcp.NewToolResultError("runID is required"), nil
	}
	if s.realWorldPrepareRuns == nil {
		return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
	}
	run, ok := s.realWorldPrepareRuns.get(args.RunID)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
	}
	run.cancel()
	return structured(map[string]any{
		"ok":      true,
		"message": "corpus preparation cancellation requested",
		"run":     run.snapshot(),
	})
}

func (s *repoServer) startRealWorldPrepareRun(args realWorldStartPrepareArgs) (*realWorldPrepareRun, error) {
	if args.CorpusDir == "" {
		return nil, fmt.Errorf("corpusDir is required")
	}
	if args.CacheDir == "" {
		return nil, fmt.Errorf("cacheDir is required")
	}
	if args.OutputArtifact == "" {
		return nil, fmt.Errorf("outputArtifact is required")
	}
	repositories := realWorldPrepareRepositoryPaths(args.CorpusDir, args.Repositories)
	concurrency := args.Concurrency
	if concurrency <= 0 {
		concurrency = realWorldPrepareDefaultConcurrency
	}
	if concurrency > realWorldPrepareMaxConcurrency {
		concurrency = realWorldPrepareMaxConcurrency
	}
	runCtx, cancel := context.WithCancel(context.Background())
	run := &realWorldPrepareRun{
		ID:             realWorldPrepareRunID(args.CorpusDir),
		Title:          args.Title,
		CorpusDir:      args.CorpusDir,
		CacheDir:       args.CacheDir,
		OutputArtifact: args.OutputArtifact,
		ManifestPath:   filepath.Join(args.CorpusDir, realWorldManifestName),
		Repositories:   repositories,
		Concurrency:    concurrency,
		StartedAt:      time.Now(),
		ctx:            runCtx,
		cancel:         cancel,
		results:        make(chan realWorldRepoPrepareResult, len(repositories)),
		done:           make(chan struct{}),
		resultsByID:    map[string]realWorldRepoPrepareResult{},
		inspectionByID: map[string]realWorldDependencyPrepScan{},
		prepByID:       map[string]realWorldDependencyPrep{},
	}
	if err := run.writeManifest(); err != nil {
		cancel()
		return nil, err
	}
	if s.realWorldPrepareRuns == nil {
		s.realWorldPrepareRuns = newRealWorldPrepareRegistry()
	}
	s.realWorldPrepareRuns.add(run)
	run.start(s.root)
	return run, nil
}

func realWorldPrepareRepositoryPaths(corpusDir string, repositories []realWorldRepository) []realWorldRepository {
	out := append([]realWorldRepository{}, repositories...)
	used := map[string]int{}
	for i := range out {
		targetName := slugify(nonEmpty(out[i].Name, repoNameFromURL(out[i].CloneURL)))
		if targetName == "" {
			targetName = fmt.Sprintf("repo-%d", i+1)
		}
		if n := used[targetName]; n > 0 {
			used[targetName] = n + 1
			targetName = fmt.Sprintf("%s-%d", targetName, n+1)
		} else {
			used[targetName] = 1
		}
		out[i].Path = filepath.Join(corpusDir, targetName)
		if out[i].Status == "" {
			out[i].Status = "pending"
		}
	}
	return out
}

func (run *realWorldPrepareRun) start(root string) {
	sem := make(chan struct{}, run.Concurrency)
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
				run.finishRepo(realWorldCanceledPrepareResult(repo))
				return
			}
			run.finishRepo(run.prepareRepo(root, repo))
		}()
	}
	go func() {
		wg.Wait()
		now := time.Now()
		run.mu.Lock()
		run.completedAt = &now
		_ = run.writeManifestLocked()
		run.mu.Unlock()
		close(run.results)
		close(run.done)
	}()
}

func (run *realWorldPrepareRun) prepareRepo(root string, repo realWorldRepository) realWorldRepoPrepareResult {
	started := time.Now()
	name := nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path))
	result := realWorldRepoPrepareResult{
		Repository:       name,
		CloneURL:         repo.CloneURL,
		Path:             repo.Path,
		Status:           "cloning",
		StartedAt:        started.UTC().Format(time.RFC3339),
		RepositoryRecord: repo,
	}
	processResult := runProcess(run.ctx, root, nil, "git", "clone", "--depth", "1", "--quiet", repo.CloneURL, repo.Path)
	result.Command = processResult.Command
	result.Duration = processResult.Duration
	result.Output = processResult.Output
	if !processResult.Succeeded {
		result.Status = "error"
		result.Error = nonEmpty(processResult.Output, "git clone failed")
		if run.ctx.Err() != nil {
			result.Status = "canceled"
			result.Error = "corpus preparation was canceled"
		}
		result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		repo.Status = result.Status
		repo.Error = result.Error
		result.RepositoryRecord = repo
		return result
	}
	repo.Status = "cloned"
	result.Status = "cloned"
	result.Succeeded = true
	commit := runProcess(run.ctx, repo.Path, nil, "git", "rev-parse", "HEAD")
	if commit.Succeeded {
		repo.Commit = strings.TrimSpace(commit.Output)
		result.Commit = repo.Commit
	}
	scan := realWorldInspectRepository(repo, realWorldInspectDefaultMaxMatches)
	prepItems := realWorldDraftDependencyPrep([]realWorldDependencyPrepScan{scan})
	result.DependencyPrepInspection = &scan
	if len(prepItems) > 0 {
		result.DependencyPrep = &prepItems[0]
	}
	result.RepositoryRecord = repo
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return result
}

func realWorldCanceledPrepareResult(repo realWorldRepository) realWorldRepoPrepareResult {
	now := time.Now().UTC().Format(time.RFC3339)
	name := nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path))
	repo.Status = "canceled"
	repo.Error = "corpus preparation was canceled before this repository started"
	return realWorldRepoPrepareResult{
		Repository:       name,
		CloneURL:         repo.CloneURL,
		Path:             repo.Path,
		Status:           "canceled",
		Error:            repo.Error,
		Duration:         "0s",
		StartedAt:        now,
		FinishedAt:       now,
		RepositoryRecord: repo,
	}
}

func (run *realWorldPrepareRun) finishRepo(result realWorldRepoPrepareResult) {
	key := slugify(result.Repository)
	run.mu.Lock()
	run.completed++
	if !result.Succeeded {
		run.failed++
	}
	run.resultsByID[key] = result
	for i := range run.Repositories {
		if slugify(nonEmpty(run.Repositories[i].Name, repoNameFromURL(run.Repositories[i].CloneURL), filepath.Base(run.Repositories[i].Path))) == key {
			run.Repositories[i] = result.RepositoryRecord
			break
		}
	}
	if result.DependencyPrepInspection != nil {
		run.inspectionByID[key] = *result.DependencyPrepInspection
	}
	if result.DependencyPrep != nil {
		run.prepByID[key] = *result.DependencyPrep
	}
	_ = run.writeManifestLocked()
	run.mu.Unlock()
	run.results <- result
}

func (run *realWorldPrepareRun) markDelivered() {
	run.mu.Lock()
	defer run.mu.Unlock()
	run.delivered++
}

func (run *realWorldPrepareRun) snapshot() realWorldPrepareSnapshot {
	run.mu.Lock()
	defer run.mu.Unlock()
	completedAt := ""
	if run.completedAt != nil {
		completedAt = run.completedAt.UTC().Format(time.RFC3339)
	}
	total := len(run.Repositories)
	return realWorldPrepareSnapshot{
		RunID:          run.ID,
		CorpusDir:      run.CorpusDir,
		CacheDir:       run.CacheDir,
		OutputArtifact: run.OutputArtifact,
		ManifestPath:   run.ManifestPath,
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

func (run *realWorldPrepareRun) writeManifest() error {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.writeManifestLocked()
}

func (run *realWorldPrepareRun) writeManifestLocked() error {
	inspection := run.inspectionLocked()
	dependencyPrep := run.dependencyPrepLocked()
	summary, needsReview := realWorldInspectionSummary(inspection)
	manifest := realWorldManifest{
		SchemaVersion:             1,
		CreatedAt:                 run.StartedAt.UTC().Format(time.RFC3339),
		Title:                     run.Title,
		CorpusDir:                 run.CorpusDir,
		CacheDir:                  run.CacheDir,
		OutputArtifact:            run.OutputArtifact,
		PreparationRunID:          run.ID,
		PreparationStartedAt:      run.StartedAt.UTC().Format(time.RFC3339),
		PreparationManaged:        true,
		PreparationComplete:       run.completed == len(run.Repositories),
		DependencyPrep:            dependencyPrep,
		DependencyPrepInspection:  inspection,
		DependencyPrepSummary:     summary,
		DependencyPrepNeedsReview: needsReview,
		PrepSecurityPolicy:        realWorldDependencyPrepSecurityPolicy(),
		Repositories:              append([]realWorldRepository{}, run.Repositories...),
	}
	if run.completedAt != nil {
		manifest.PreparationCompletedAt = run.completedAt.UTC().Format(time.RFC3339)
	}
	return writeJSONFile(run.ManifestPath, manifest)
}

func (run *realWorldPrepareRun) dependencyPrep() []realWorldDependencyPrep {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.dependencyPrepLocked()
}

func (run *realWorldPrepareRun) dependencyPrepLocked() []realWorldDependencyPrep {
	prep := make([]realWorldDependencyPrep, 0, len(run.prepByID))
	for _, item := range run.prepByID {
		prep = append(prep, item)
	}
	sort.Slice(prep, func(i, j int) bool {
		return prep[i].Repository < prep[j].Repository
	})
	return prep
}

func (run *realWorldPrepareRun) inspection() []realWorldDependencyPrepScan {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.inspectionLocked()
}

func (run *realWorldPrepareRun) inspectionLocked() []realWorldDependencyPrepScan {
	inspection := make([]realWorldDependencyPrepScan, 0, len(run.inspectionByID))
	for _, item := range run.inspectionByID {
		inspection = append(inspection, item)
	}
	sort.Slice(inspection, func(i, j int) bool {
		return inspection[i].Repository < inspection[j].Repository
	})
	return inspection
}

func (run *realWorldPrepareRun) cloneCommands() []string {
	commands := make([]string, 0, len(run.Repositories))
	for _, repo := range run.Repositories {
		commands = append(commands, fmt.Sprintf("git clone --depth 1 --quiet %s %s", shellQuote(repo.CloneURL), shellQuote(repo.Path)))
	}
	return commands
}

func (s *repoServer) realWorldWaitForPreparedRepo(ctx context.Context, request mcp.CallToolRequest, run *realWorldPrepareRun) (map[string]any, error) {
	p := newProgress(ctx, s.mcp, request, len(run.Repositories))
	snapshot := run.snapshot()
	p.report(snapshot.Completed, snapshot.Total, realWorldPrepareProgressMessage(run, snapshot))
	ticker := time.NewTicker(realWorldPrepareProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case result, ok := <-run.results:
			if !ok {
				snapshot := run.snapshot()
				return map[string]any{
					"run":                snapshot,
					"complete":           true,
					"message":            "corpus preparation has no more per-repo results",
					"result":             nil,
					"resultOK":           false,
					"delivered":          snapshot.Delivered,
					"dependencyPrep":     run.dependencyPrep(),
					"inspection":         run.inspection(),
					"nextStep":           realWorldPrepareNextStep(run),
					"validationStep":     realWorldNextRunCorpusDuringPrepare(run),
					"prepSecurityPolicy": realWorldDependencyPrepSecurityPolicy(),
				}, nil
			}
			run.markDelivered()
			snapshot := run.snapshot()
			return map[string]any{
				"run":                snapshot,
				"complete":           snapshot.Complete && snapshot.Ready == 0,
				"result":             result,
				"resultOK":           result.Succeeded,
				"delivered":          snapshot.Delivered,
				"dependencyPrep":     run.dependencyPrep(),
				"inspection":         run.inspection(),
				"nextStep":           realWorldPrepareNextStep(run),
				"validationStep":     realWorldNextRunCorpusDuringPrepare(run),
				"prepSecurityPolicy": realWorldDependencyPrepSecurityPolicy(),
			}, nil
		case <-ticker.C:
			snapshot := run.snapshot()
			p.report(snapshot.Completed, snapshot.Total, realWorldPrepareProgressMessage(run, snapshot))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func realWorldPrepareProgressMessage(run *realWorldPrepareRun, snapshot realWorldPrepareSnapshot) string {
	return fmt.Sprintf("Corpus preparation %s: %d/%d repositories prepared, %d running, %d result(s) ready, %d failed.",
		run.ID, snapshot.Completed, snapshot.Total, snapshot.Running, snapshot.Ready, snapshot.Failed)
}

func realWorldPrepareNextStep(run *realWorldPrepareRun) map[string]any {
	snapshot := run.snapshot()
	if snapshot.Complete && snapshot.Ready == 0 {
		return realWorldNextRunCorpusDuringPrepare(run)
	}
	return realWorldNextPreparedRepo(run.ID)
}

func realWorldNextPreparedRepo(runID string) map[string]any {
	return map[string]any{
		"tool": "real_world_next_prepared_repo",
		"why":  "Wait with MCP progress notifications until another repository is cloned and dependency-prep inspected.",
		"beforeCalling": []string{
			"Do not poll with shell sleep loops.",
			"Use this only when you want to inspect corpus-prep details directly; validation can already start from the manifest while preparation continues.",
		},
		"suggestedArgs": map[string]any{
			"runID": runID,
		},
	}
}

func realWorldNextRunCorpusDuringPrepare(run *realWorldPrepareRun) map[string]any {
	step := realWorldNextRunCorpus(run.CorpusDir, run.CacheDir, run.OutputArtifact, run.ManifestPath, run.dependencyPrep())
	step["why"] = "Start managed validation now; validation will wait inside the tool for repositories that are still being cloned and will read final dependency-prep notes from the manifest before finishing."
	step["duringPreparation"] = true
	step["beforeCalling"] = []string{
		"You do not need to wait for every clone before starting validation.",
		"Pass manifestPath and omit repositories/dependencyPrep unless you intentionally want to narrow the corpus.",
		"Do not poll corpus setup with shell sleep loops; validation and prep tools send progress notifications.",
	}
	return step
}

func realWorldPrepareRunID(corpusDir string) string {
	base := slugify(filepath.Base(corpusDir))
	if base == "" {
		base = "prepare"
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}
