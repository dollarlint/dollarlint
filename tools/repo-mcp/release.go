package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	releaseWorkflowID          = "release.yml"
	prepareReleaseWorkflowID   = "prepare-release.yml"
	releaseWorkflowPath        = ".github/workflows/release.yml"
	prepareReleaseWorkflowPath = ".github/workflows/prepare-release.yml"
	wingetBranchPrefix         = "dollarlint-"
	wingetAzureProjectBaseURL  = "https://dev.azure.com/shine-oss/8b78618a-7973-49d8-9174-4360829d979b"
)

type releaseStartArgs struct {
	Version string `json:"version"`
	DryRun  *bool  `json:"dryRun"`
}

type releaseWatchArgs struct {
	Version             string `json:"version"`
	RunID               string `json:"runID"`
	AutoDispatchRelease *bool  `json:"autoDispatchRelease"`
	TimeoutSeconds      int    `json:"timeoutSeconds"`
}

type releaseWingetWatchArgs struct {
	Version        string `json:"version"`
	PRNumber       int    `json:"prNumber"`
	BuildID        string `json:"buildID"`
	BuildURL       string `json:"buildURL"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	PollSeconds    int    `json:"pollSeconds"`
	IncludeLogs    bool   `json:"includeLogs"`
}

type releaseWorkflowRun struct {
	DatabaseID   int64  `json:"databaseId"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HeadBranch   string `json:"headBranch"`
	Event        string `json:"event"`
	DisplayTitle string `json:"displayTitle"`
	WorkflowName string `json:"workflowName"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	URL          string `json:"url"`
	Jobs         []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"jobs,omitempty"`
}

type ghRelease struct {
	TagName      string `json:"tagName"`
	URL          string `json:"url"`
	IsDraft      bool   `json:"isDraft"`
	IsPrerelease bool   `json:"isPrerelease"`
	PublishedAt  string `json:"publishedAt"`
}

type ghWingetPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	Body        string `json:"body,omitempty"`
}

type ghWingetPRDetails struct {
	Number           int    `json:"number"`
	Title            string `json:"title"`
	State            string `json:"state"`
	URL              string `json:"url"`
	IsDraft          bool   `json:"isDraft"`
	HeadRefName      string `json:"headRefName"`
	Body             string `json:"body"`
	MergeStateStatus string `json:"mergeStateStatus"`
	ReviewDecision   string `json:"reviewDecision"`
	Labels           []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Comments []ghPRComment `json:"comments"`
}

type ghPRComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type azureBuildRef struct {
	BuildID string `json:"buildID"`
	APIBase string `json:"apiBase"`
	WebURL  string `json:"webURL"`
}

type azureBuild struct {
	ID          int    `json:"id"`
	BuildNumber string `json:"buildNumber"`
	Status      string `json:"status"`
	Result      string `json:"result"`
	QueueTime   string `json:"queueTime"`
	StartTime   string `json:"startTime"`
	FinishTime  string `json:"finishTime"`
	Links       struct {
		Web struct {
			Href string `json:"href"`
		} `json:"web"`
	} `json:"_links"`
}

type azureTimeline struct {
	Records       []azureTimelineRecord `json:"records"`
	LastChangedOn string                `json:"lastChangedOn"`
	ChangeID      int                   `json:"changeId"`
}

type azureTimelineRecord struct {
	ID               string `json:"id"`
	ParentID         string `json:"parentId"`
	Type             string `json:"type"`
	Name             string `json:"name"`
	State            string `json:"state"`
	Result           string `json:"result"`
	StartTime        string `json:"startTime"`
	FinishTime       string `json:"finishTime"`
	CurrentOperation string `json:"currentOperation"`
	PercentComplete  *int   `json:"percentComplete"`
	ErrorCount       int    `json:"errorCount"`
	WarningCount     int    `json:"warningCount"`
	Log              *struct {
		ID  int    `json:"id"`
		URL string `json:"url"`
	} `json:"log"`
}

func (s *repoServer) handleReleaseReadiness(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Snapshot bool `json:"snapshot"`
	}
	_ = request.BindArguments(&args)
	commands := []namedCommand{
		{Name: "working tree", Cmd: "git diff --quiet && git diff --cached --quiet"},
		{Name: "go test", Cmd: "go test ./..."},
		{Name: "go vet", Cmd: "go vet ./..."},
		{Name: "actionlint", Cmd: "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"},
		{Name: "docs build", Cmd: "npm run build", Dir: "docs"},
		{Name: "goreleaser snapshot", Cmd: goreleaserSnapshotCheckCommand},
		{Name: "remote tags", Cmd: "git ls-remote --tags origin 'refs/tags/v*' | tail -10"},
	}
	return s.runCommandSet(ctx, request, "release-readiness", commands)
}

func (s *repoServer) handleReleaseStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Version string `json:"version"`
	}
	_ = request.BindArguments(&args)
	p := newProgress(ctx, s.mcp, request, 4)
	p.step("Reading release tags")
	status, err := s.releaseStatus(ctx, args.Version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p.step("Reading Prepare Release runs")
	prepareRuns, err := s.releaseWorkflowRuns(ctx, "prepare-release.yml", 10)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p.step("Reading Release runs")
	releaseRuns, err := s.releaseWorkflowRuns(ctx, "release.yml", 10)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p.step("Reading WinGet PR")
	pr, prErr := s.wingetPR(ctx, status["version"].(string), false)
	status["prepareReleaseRuns"] = prepareRuns
	status["releaseRuns"] = releaseRuns
	if prErr == nil && pr != nil {
		status["wingetPR"] = pr
		status["wingetPRCheck"] = releaseWingetPRCheck(*pr, status["version"].(string))
	} else if prErr != nil {
		status["wingetPRError"] = prErr.Error()
	}
	return structured(status)
}

func (s *repoServer) handlePrepareRelease(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args releaseStartArgs
	_ = request.BindArguments(&args)
	version, err := s.releaseVersionOrNext(ctx, args.Version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args.Version = version
	dryRun := true
	if args.DryRun != nil {
		dryRun = *args.DryRun
	}
	commands := []namedCommand{
		{Name: "working tree", Cmd: "git diff --quiet && git diff --cached --quiet"},
		{Name: "remote tag check", Cmd: fmt.Sprintf("test -z \"$(git ls-remote --tags origin refs/tags/%s)\"", shellQuote(args.Version))},
		{Name: "goreleaser snapshot", Cmd: goreleaserSnapshotCheckCommand},
	}
	if dryRun {
		result, err := s.runCommandSetData(ctx, request, "prepare-release-dry-run", commands)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result["dryRun"] = true
		result["version"] = args.Version
		result["wouldRun"] = fmt.Sprintf("gh workflow run prepare-release.yml --ref main -f version=%s", args.Version)
		result["nextStep"] = releaseNextStep("prepare_release", args.Version, "", false)
		return structured(result)
	}
	commands = append(commands, namedCommand{Name: "trigger prepare-release", Cmd: fmt.Sprintf("gh workflow run prepare-release.yml --ref main -f version=%s", shellQuote(args.Version))})
	result, err := s.runCommandSetData(ctx, request, "prepare-release", commands)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result["dryRun"] = false
	result["version"] = args.Version
	result["nextStep"] = releaseNextStep("release_watch", args.Version, "", true)
	return structured(result)
}

func (s *repoServer) handleReleaseStart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args releaseStartArgs
	_ = request.BindArguments(&args)
	version, err := s.releaseVersionOrNext(ctx, args.Version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	dryRun := true
	if args.DryRun != nil {
		dryRun = *args.DryRun
	}
	status, err := s.releaseStatus(ctx, version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	status["dryRun"] = dryRun
	if dryRun {
		status["nextStep"] = releaseNextStep("release_start", version, "", false)
		return structured(status)
	}
	if status["githubRelease"] != nil {
		status["ok"] = false
		status["message"] = "GitHub release already exists; use release_status to inspect it instead of starting another release."
		return structured(status)
	}
	remoteTagExists, _ := status["remoteTagExists"].(bool)
	if remoteTagExists {
		if existingRun, _ := s.releaseRunForVersion(ctx, version); existingRun != nil && existingRun.Status != "completed" {
			status["triggered"] = "existing-release-run"
			status["run"] = existingRun
			status["nextStep"] = releaseNextStep("release_watch", version, strconv.FormatInt(existingRun.DatabaseID, 10), true)
			return structured(status)
		}
		run, err := s.triggerReleaseWorkflow(ctx, releaseWorkflowID, "main", map[string]string{"version": version})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		status["triggered"] = "release"
		status["run"] = run
		status["nextStep"] = releaseNextStep("release_watch", version, strconv.FormatInt(run.DatabaseID, 10), true)
		return structured(status)
	}
	run, err := s.triggerReleaseWorkflow(ctx, prepareReleaseWorkflowID, "main", map[string]string{"version": version})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	status["triggered"] = "prepare-release"
	status["run"] = run
	status["nextStep"] = releaseNextStep("release_watch", version, strconv.FormatInt(run.DatabaseID, 10), true)
	status["message"] = "Prepare Release was triggered. Keep release_watch open; it will dispatch the Release workflow after the tag exists."
	return structured(status)
}

func (s *repoServer) handleReleaseWatch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args releaseWatchArgs
	_ = request.BindArguments(&args)
	version, err := s.releaseVersionOrNext(ctx, args.Version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeout := time.Duration(args.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	if timeout > 2*time.Hour {
		timeout = 2 * time.Hour
	}
	autoDispatch := true
	if args.AutoDispatchRelease != nil {
		autoDispatch = *args.AutoDispatchRelease
	}
	deadline := time.Now().Add(timeout)
	p := newProgress(ctx, s.mcp, request, 100)
	runID := strings.TrimSpace(args.RunID)
	var releaseRunID string
	var autoDispatched bool
	for {
		status, err := s.releaseStatus(ctx, version)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if status["githubRelease"] != nil {
			pr, prErr := s.wingetPR(ctx, version, true)
			if prErr == nil && pr != nil {
				check := releaseWingetPRCheck(*pr, version)
				status["wingetPR"] = pr
				status["wingetPRCheck"] = check
				status["ok"] = check["ok"]
				status["message"] = "Release exists and WinGet PR was inspected. Keep release_winget_watch open for Microsoft validation instead of polling Azure manually."
				status["nextStep"] = releaseWingetWatchNextStep(version, pr.Number, nil, releaseWingetWatchArgs{})
				return structured(status)
			}
			status["ok"] = prErr == nil
			if prErr != nil {
				status["wingetPRError"] = prErr.Error()
				status["message"] = "Release exists, but no WinGet PR could be inspected yet."
			}
			return structured(status)
		}
		if runID != "" {
			run, err := s.releaseRun(ctx, runID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			status["watchedRun"] = run
			p.report(releaseProgressPercent(run.Status, run.Conclusion, autoDispatched), 100, releaseProgressMessage(version, run))
			if run.Status == "completed" && run.Conclusion != "success" {
				failedLogs, _ := commandOutput(ctx, s.root, "gh", "run", "view", runID, "--log-failed")
				status["ok"] = false
				status["failedLogs"] = truncate(string(failedLogs), 12000)
				status["message"] = "Watched release workflow failed."
				return structured(status)
			}
			if run.Status == "completed" && run.Conclusion == "success" {
				runID = ""
			}
		}
		remoteTagExists, _ := status["remoteTagExists"].(bool)
		if remoteTagExists && autoDispatch && !autoDispatched && releaseRunID == "" {
			if existingRun, _ := s.releaseRunForVersion(ctx, version); existingRun != nil && existingRun.Status != "completed" {
				releaseRunID = strconv.FormatInt(existingRun.DatabaseID, 10)
				runID = releaseRunID
				p.report(35, 100, "Release tag exists; watching existing Release workflow for "+version)
				continue
			}
			run, err := s.triggerReleaseWorkflow(ctx, releaseWorkflowID, "main", map[string]string{"version": version})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			releaseRunID = strconv.FormatInt(run.DatabaseID, 10)
			runID = releaseRunID
			autoDispatched = true
			status["autoDispatchedReleaseRun"] = run
			p.report(35, 100, "Release tag exists; dispatched Release workflow for "+version)
		}
		if !remoteTagExists {
			p.report(10, 100, "Waiting for Prepare Release to create tag "+version)
		} else if runID == "" {
			p.report(45, 100, "Waiting for GitHub release and WinGet validation for "+version)
		}
		if time.Now().After(deadline) {
			status["ok"] = false
			status["message"] = "Timed out waiting for release completion."
			status["nextStep"] = releaseNextStep("release_watch", version, runID, autoDispatch)
			return structured(status)
		}
		select {
		case <-ctx.Done():
			return mcp.NewToolResultError(ctx.Err().Error()), nil
		case <-time.After(10 * time.Second):
		}
	}
}

func (s *repoServer) handleReleaseWingetPR(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Version     string `json:"version"`
		IncludeBody bool   `json:"includeBody"`
	}
	_ = request.BindArguments(&args)
	version, err := s.releaseVersionOrLatest(ctx, args.Version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	pr, err := s.wingetPR(ctx, version, args.IncludeBody)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out := map[string]any{
		"ok":      true,
		"version": version,
		"branch":  releaseWingetBranch(version),
		"pr":      pr,
		"check":   releaseWingetPRCheck(*pr, version),
	}
	return structured(out)
}

func (s *repoServer) handleReleaseWingetWatch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args releaseWingetWatchArgs
	_ = request.BindArguments(&args)
	version, err := s.releaseVersionOrLatest(ctx, args.Version)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	timeout := time.Duration(args.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Minute
	}
	if timeout > 4*time.Hour {
		timeout = 4 * time.Hour
	}
	poll := time.Duration(args.PollSeconds) * time.Second
	if poll <= 0 {
		poll = 30 * time.Second
	}
	if poll < 5*time.Second {
		poll = 5 * time.Second
	}
	if poll > 2*time.Minute {
		poll = 2 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	p := newProgress(ctx, s.mcp, request, 100)
	var prNumber int
	var prDetails *ghWingetPRDetails
	if args.PRNumber > 0 {
		prNumber = args.PRNumber
	}
	var buildRef *azureBuildRef
	if strings.TrimSpace(args.BuildURL) != "" || strings.TrimSpace(args.BuildID) != "" {
		ref, err := azureBuildRefFromInput(nonEmpty(args.BuildURL, args.BuildID))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		buildRef = &ref
	}
	for {
		if buildRef == nil {
			if prNumber == 0 {
				pr, err := s.wingetPR(ctx, version, false)
				if err == nil && pr != nil {
					prNumber = pr.Number
				} else {
					p.report(10, 100, "Waiting for Microsoft WinGet PR for "+version)
				}
			}
			if prNumber != 0 {
				details, err := s.wingetPRDetails(ctx, prNumber)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				prDetails = details
				if ref, ok := latestWingetValidationBuild(details.Comments); ok {
					buildRef = &ref
					p.report(25, 100, fmt.Sprintf("Found Microsoft validation build %s for PR #%d", ref.BuildID, prNumber))
				} else {
					p.report(20, 100, fmt.Sprintf("Waiting for wingetbot validation build comment on PR #%d", prNumber))
				}
			}
			if buildRef == nil {
				if time.Now().After(deadline) {
					return structured(map[string]any{
						"ok":       false,
						"version":  version,
						"prNumber": prNumber,
						"message":  "Timed out waiting for wingetbot to publish a Microsoft validation build.",
						"nextStep": releaseWingetWatchNextStep(version, prNumber, nil, args),
					})
				}
				if err := sleepOrDone(ctx, poll); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				continue
			}
		}
		build, err := azureBuildStatus(ctx, *buildRef)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		timeline, timelineErr := azureBuildTimeline(ctx, *buildRef)
		result := releaseWingetValidationSnapshot(version, prDetails, *buildRef, build, timeline)
		if timelineErr != nil {
			result["timelineError"] = timelineErr.Error()
		}
		p.report(releaseWingetValidationProgress(build, timeline), 100, releaseWingetValidationProgressMessage(build, timeline))
		if strings.EqualFold(build.Status, "completed") {
			ok := strings.EqualFold(build.Result, "succeeded")
			result["ok"] = ok
			if ok {
				result["message"] = "Microsoft WinGet validation completed successfully."
			} else {
				result["message"] = "Microsoft WinGet validation completed with result: " + nonEmpty(build.Result, "unknown")
			}
			if !ok || args.IncludeLogs {
				result["logs"] = azureRelevantLogs(ctx, timeline)
			}
			return structured(result)
		}
		if time.Now().After(deadline) {
			result["ok"] = false
			result["message"] = "Timed out waiting for Microsoft WinGet validation to complete."
			result["nextStep"] = releaseWingetWatchNextStep(version, prNumber, buildRef, args)
			return structured(result)
		}
		if err := sleepOrDone(ctx, poll); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}
}

func validateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version is required")
	}
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`).MatchString(version) {
		return fmt.Errorf("version must look like v0.1.2")
	}
	return nil
}

func shellQuote(value string) string {
	return strconv.Quote(value)
}

func (s *repoServer) releaseVersionOrNext(ctx context.Context, version string) (string, error) {
	version = strings.TrimSpace(version)
	if version != "" {
		return version, validateVersion(version)
	}
	tags, err := s.localReleaseTags(ctx)
	if err != nil {
		return "", err
	}
	if len(tags) > 0 {
		if release, _ := s.githubRelease(ctx, tags[0]); release == nil {
			return tags[0], nil
		}
	}
	next, err := nextPatchReleaseTag(tags)
	if err != nil {
		return "", err
	}
	return next, nil
}

func (s *repoServer) releaseVersionOrLatest(ctx context.Context, version string) (string, error) {
	version = strings.TrimSpace(version)
	if version != "" {
		return version, validateVersion(version)
	}
	tags, err := s.localReleaseTags(ctx)
	if err != nil {
		return "", err
	}
	if len(tags) == 0 {
		return "v0.1.0", nil
	}
	return tags[0], nil
}

func (s *repoServer) releaseStatus(ctx context.Context, version string) (map[string]any, error) {
	version, err := s.releaseVersionOrNext(ctx, version)
	if err != nil {
		return nil, err
	}
	tags, err := s.localReleaseTags(ctx)
	if err != nil {
		return nil, err
	}
	next, _ := nextPatchReleaseTag(tags)
	localTagExists := containsString(tags, version)
	remoteTagExists, err := s.remoteTagExists(ctx, version)
	if err != nil {
		return nil, err
	}
	ghRelease, _ := s.githubRelease(ctx, version)
	return map[string]any{
		"ok":                  true,
		"version":             version,
		"packageVersion":      strings.TrimPrefix(version, "v"),
		"wingetBranch":        releaseWingetBranch(version),
		"latestLocalTag":      firstString(tags),
		"suggestedNextTag":    next,
		"localTagExists":      localTagExists,
		"remoteTagExists":     remoteTagExists,
		"githubRelease":       ghRelease,
		"releaseWorkflowFile": releaseWorkflowPath,
		"prepareReleaseFile":  prepareReleaseWorkflowPath,
	}, nil
}

func (s *repoServer) localReleaseTags(ctx context.Context) ([]string, error) {
	raw, err := commandOutput(ctx, s.root, "git", "tag", "--sort=-v:refname")
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, line := range lines(string(raw)) {
		tag := strings.TrimSpace(line)
		if regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(tag) {
			tags = append(tags, tag)
		}
	}
	sort.SliceStable(tags, func(i, j int) bool { return compareReleaseTags(tags[i], tags[j]) > 0 })
	return tags, nil
}

func (s *repoServer) remoteTagExists(ctx context.Context, version string) (bool, error) {
	raw, err := commandOutput(ctx, s.root, "git", "ls-remote", "--tags", "origin", "refs/tags/"+version)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) != "", nil
}

func (s *repoServer) githubRelease(ctx context.Context, version string) (*ghRelease, error) {
	raw, err := commandOutput(ctx, s.root, "gh", "release", "view", version, "--repo", "dollarlint/dollarlint", "--json", "tagName,url,isDraft,isPrerelease,publishedAt")
	if err != nil {
		return nil, err
	}
	var release ghRelease
	if err := json.Unmarshal(raw, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *repoServer) releaseWorkflowRuns(ctx context.Context, workflow string, limit int) ([]releaseWorkflowRun, error) {
	if limit <= 0 {
		limit = 10
	}
	raw, err := commandOutput(ctx, s.root, "gh", "run", "list", "--workflow", workflow, "--limit", fmt.Sprint(limit), "--json", "databaseId,status,conclusion,headBranch,event,displayTitle,createdAt,updatedAt,url,workflowName")
	if err != nil {
		return nil, err
	}
	var runs []releaseWorkflowRun
	if err := json.Unmarshal(raw, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *repoServer) releaseRun(ctx context.Context, runID string) (releaseWorkflowRun, error) {
	raw, err := commandOutput(ctx, s.root, "gh", "run", "view", runID, "--json", "databaseId,status,conclusion,headBranch,event,displayTitle,createdAt,updatedAt,url,workflowName,jobs")
	if err != nil {
		return releaseWorkflowRun{}, err
	}
	var run releaseWorkflowRun
	if err := json.Unmarshal(raw, &run); err != nil {
		return releaseWorkflowRun{}, err
	}
	return run, nil
}

func (s *repoServer) releaseRunForVersion(ctx context.Context, version string) (*releaseWorkflowRun, error) {
	runs, err := s.releaseWorkflowRuns(ctx, releaseWorkflowID, 20)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.HeadBranch == version {
			return &run, nil
		}
	}
	return nil, nil
}

func (s *repoServer) triggerReleaseWorkflow(ctx context.Context, workflowFile, ref string, fields map[string]string) (releaseWorkflowRun, error) {
	startedAt := time.Now().UTC().Add(-5 * time.Second)
	args := []string{"workflow", "run", workflowFile, "--ref", ref}
	for key, value := range fields {
		args = append(args, "-f", key+"="+value)
	}
	if _, err := commandOutput(ctx, s.root, "gh", args...); err != nil {
		return releaseWorkflowRun{}, err
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := s.releaseWorkflowRuns(ctx, workflowFile, 10)
		if err != nil {
			return releaseWorkflowRun{}, err
		}
		for _, run := range runs {
			createdAt, _ := time.Parse(time.RFC3339, run.CreatedAt)
			if run.Event == "workflow_dispatch" && !createdAt.Before(startedAt) {
				return run, nil
			}
		}
		select {
		case <-ctx.Done():
			return releaseWorkflowRun{}, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return releaseWorkflowRun{}, fmt.Errorf("workflow %s was triggered, but no workflow_dispatch run appeared within 90s", workflowFile)
}

func (s *repoServer) wingetPR(ctx context.Context, version string, includeBody bool) (*ghWingetPR, error) {
	fields := "number,title,state,url,isDraft,headRefName"
	if includeBody {
		fields += ",body"
	}
	raw, err := commandOutput(ctx, s.root, "gh", "pr", "list", "--repo", "microsoft/winget-pkgs", "--head", releaseWingetBranch(version), "--state", "all", "--json", fields, "--jq", ".[0]")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil, fmt.Errorf("no WinGet PR found for head %s", releaseWingetBranch(version))
	}
	var pr ghWingetPR
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (s *repoServer) wingetPRDetails(ctx context.Context, number int) (*ghWingetPRDetails, error) {
	raw, err := commandOutput(ctx, s.root, "gh", "pr", "view", strconv.Itoa(number), "--repo", "microsoft/winget-pkgs", "--json", "number,title,state,url,isDraft,headRefName,body,comments,labels,mergeStateStatus,reviewDecision")
	if err != nil {
		return nil, err
	}
	var pr ghWingetPRDetails
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func latestWingetValidationBuild(comments []ghPRComment) (azureBuildRef, bool) {
	for i := len(comments) - 1; i >= 0; i-- {
		comment := comments[i]
		if !strings.EqualFold(comment.Author.Login, "wingetbot") {
			continue
		}
		if ref, ok := azureBuildRefFromText(comment.Body); ok {
			return ref, true
		}
	}
	for i := len(comments) - 1; i >= 0; i-- {
		if ref, ok := azureBuildRefFromText(comments[i].Body); ok {
			return ref, true
		}
	}
	return azureBuildRef{}, false
}

func azureBuildRefFromText(text string) (azureBuildRef, bool) {
	match := regexp.MustCompile(`https://dev\.azure\.com/[^\s)]+buildId=\d+[^\s)]*`).FindString(text)
	if match == "" {
		return azureBuildRef{}, false
	}
	ref, err := azureBuildRefFromInput(match)
	return ref, err == nil
}

func azureBuildRefFromInput(input string) (azureBuildRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return azureBuildRef{}, fmt.Errorf("azure build id or URL is required")
	}
	if regexp.MustCompile(`^\d+$`).MatchString(input) {
		return azureBuildRef{
			BuildID: input,
			APIBase: wingetAzureProjectBaseURL,
			WebURL:  wingetAzureProjectBaseURL + "/_build/results?buildId=" + input,
		}, nil
	}
	parsed, err := url.Parse(input)
	if err != nil {
		return azureBuildRef{}, err
	}
	buildID := parsed.Query().Get("buildId")
	if buildID == "" {
		return azureBuildRef{}, fmt.Errorf("azure build URL is missing buildId")
	}
	marker := "/_build/"
	idx := strings.Index(parsed.Path, marker)
	if idx == -1 {
		return azureBuildRef{}, fmt.Errorf("azure build URL must contain /_build/")
	}
	base := parsed.Scheme + "://" + parsed.Host + parsed.Path[:idx]
	return azureBuildRef{BuildID: buildID, APIBase: base, WebURL: input}, nil
}

func azureBuildStatus(ctx context.Context, ref azureBuildRef) (azureBuild, error) {
	var build azureBuild
	err := azureGetJSON(ctx, ref.APIBase+"/_apis/build/builds/"+ref.BuildID+"?api-version=7.1-preview.7", &build)
	if build.Links.Web.Href == "" {
		build.Links.Web.Href = ref.WebURL
	}
	return build, err
}

func azureBuildTimeline(ctx context.Context, ref azureBuildRef) (*azureTimeline, error) {
	var timeline azureTimeline
	err := azureGetJSON(ctx, ref.APIBase+"/_apis/build/builds/"+ref.BuildID+"/timeline?api-version=7.1-preview.2", &timeline)
	if err != nil {
		return nil, err
	}
	return &timeline, nil
}

func azureGetJSON(ctx context.Context, target string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4000))
		return fmt.Errorf("GET %s: %s: %s", target, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func azureGetText(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4000))
		return "", fmt.Errorf("GET %s: %s: %s", target, resp.Status, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20000))
	return string(body), err
}

func releaseWingetValidationSnapshot(version string, pr *ghWingetPRDetails, ref azureBuildRef, build azureBuild, timeline *azureTimeline) map[string]any {
	result := map[string]any{
		"ok":      false,
		"version": version,
		"build": map[string]any{
			"id":          build.ID,
			"buildID":     ref.BuildID,
			"buildNumber": build.BuildNumber,
			"status":      build.Status,
			"result":      build.Result,
			"queueTime":   build.QueueTime,
			"startTime":   build.StartTime,
			"finishTime":  build.FinishTime,
			"url":         nonEmpty(build.Links.Web.Href, ref.WebURL),
		},
	}
	if pr != nil {
		result["pr"] = map[string]any{
			"number":           pr.Number,
			"title":            pr.Title,
			"state":            pr.State,
			"isDraft":          pr.IsDraft,
			"url":              pr.URL,
			"mergeStateStatus": pr.MergeStateStatus,
			"reviewDecision":   pr.ReviewDecision,
			"labels":           wingetPRLabelNames(pr.Labels),
		}
		result["prCheck"] = releaseWingetPRCheck(ghWingetPR{
			Number:      pr.Number,
			Title:       pr.Title,
			State:       pr.State,
			URL:         pr.URL,
			IsDraft:     pr.IsDraft,
			HeadRefName: pr.HeadRefName,
			Body:        pr.Body,
		}, version)
	}
	if timeline != nil {
		result["timeline"] = map[string]any{
			"lastChangedOn": timeline.LastChangedOn,
			"changeID":      timeline.ChangeID,
			"active":        azureRecordSummaries(azureTimelineRecordsByState(timeline, "inProgress")),
			"failed":        azureRecordSummaries(azureTimelineRecordsByResult(timeline, "failed", "canceled")),
			"completed":     azureRecordSummaries(azureTimelineCompletedMilestones(timeline)),
		}
	}
	return result
}

func wingetPRLabelNames(labels []struct {
	Name string `json:"name"`
}) []string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return names
}

func azureTimelineRecordsByState(timeline *azureTimeline, state string) []azureTimelineRecord {
	if timeline == nil {
		return nil
	}
	var records []azureTimelineRecord
	for _, record := range timeline.Records {
		if strings.EqualFold(record.State, state) {
			records = append(records, record)
		}
	}
	return records
}

func azureTimelineRecordsByResult(timeline *azureTimeline, results ...string) []azureTimelineRecord {
	if timeline == nil {
		return nil
	}
	var records []azureTimelineRecord
	for _, record := range timeline.Records {
		for _, result := range results {
			if strings.EqualFold(record.Result, result) {
				records = append(records, record)
				break
			}
		}
	}
	return records
}

func azureTimelineCompletedMilestones(timeline *azureTimeline) []azureTimelineRecord {
	if timeline == nil {
		return nil
	}
	var records []azureTimelineRecord
	for _, record := range timeline.Records {
		if !strings.EqualFold(record.State, "completed") || !strings.EqualFold(record.Result, "succeeded") {
			continue
		}
		if record.Type == "Stage" || record.Type == "Phase" || record.Type == "Job" {
			records = append(records, record)
		}
	}
	if len(records) > 12 {
		return records[len(records)-12:]
	}
	return records
}

func azureRecordSummaries(records []azureTimelineRecord) []map[string]any {
	summaries := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := map[string]any{
			"type":       record.Type,
			"name":       record.Name,
			"state":      record.State,
			"result":     record.Result,
			"startTime":  record.StartTime,
			"finishTime": record.FinishTime,
		}
		if record.CurrentOperation != "" {
			item["currentOperation"] = record.CurrentOperation
		}
		if record.PercentComplete != nil {
			item["percentComplete"] = *record.PercentComplete
		}
		if record.ErrorCount != 0 {
			item["errorCount"] = record.ErrorCount
		}
		if record.WarningCount != 0 {
			item["warningCount"] = record.WarningCount
		}
		if record.Log != nil {
			item["logURL"] = record.Log.URL
		}
		summaries = append(summaries, item)
	}
	return summaries
}

func azureRelevantLogs(ctx context.Context, timeline *azureTimeline) map[string]string {
	logs := map[string]string{}
	for _, record := range azureTimelineRecordsByResult(timeline, "failed", "canceled") {
		if record.Log == nil || record.Log.URL == "" {
			continue
		}
		text, err := azureGetText(ctx, record.Log.URL)
		if err != nil {
			logs[record.Name] = err.Error()
			continue
		}
		logs[record.Name] = truncate(text, 12000)
	}
	return logs
}

func releaseWingetValidationProgress(build azureBuild, timeline *azureTimeline) int {
	if strings.EqualFold(build.Status, "completed") {
		return 100
	}
	for _, record := range azureTimelineRecordsByState(timeline, "inProgress") {
		switch record.Name {
		case "Post Validation":
			return 90
		case "Installer Validation", "Installation Validation", "Installer Metadata Validation":
			return 75
		case "Catalog Content Verification":
			return 65
		case "Manifest Content Validation":
			return 55
		}
	}
	if strings.EqualFold(build.Status, "inProgress") {
		return 40
	}
	return 15
}

func releaseWingetValidationProgressMessage(build azureBuild, timeline *azureTimeline) string {
	active := azureTimelineRecordsByState(timeline, "inProgress")
	if len(active) == 0 {
		return fmt.Sprintf("Microsoft WinGet validation build %d is %s", build.ID, nonEmpty(build.Status, "pending"))
	}
	var names []string
	for _, record := range active {
		if record.Type == "Task" || record.Type == "Phase" {
			names = append(names, record.Name)
		}
	}
	if len(names) == 0 {
		for _, record := range active {
			names = append(names, record.Name)
		}
	}
	return fmt.Sprintf("Microsoft WinGet validation build %d is %s: %s", build.ID, nonEmpty(build.Status, "pending"), strings.Join(names, " / "))
}

func releaseWingetWatchNextStep(version string, prNumber int, ref *azureBuildRef, args releaseWingetWatchArgs) map[string]any {
	nextArgs := map[string]any{"version": version}
	if prNumber != 0 {
		nextArgs["prNumber"] = prNumber
	}
	if ref != nil {
		nextArgs["buildID"] = ref.BuildID
	}
	if args.TimeoutSeconds != 0 {
		nextArgs["timeoutSeconds"] = args.TimeoutSeconds
	}
	if args.PollSeconds != 0 {
		nextArgs["pollSeconds"] = args.PollSeconds
	}
	if args.IncludeLogs {
		nextArgs["includeLogs"] = true
	}
	return map[string]any{
		"tool":          "release_winget_watch",
		"suggestedArgs": nextArgs,
	}
}

func sleepOrDone(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

func releaseWingetPRCheck(pr ghWingetPR, version string) map[string]any {
	body := pr.Body
	expected := []string{
		"Updates `DollarLint.DollarLint` to version `" + strings.TrimPrefix(version, "v") + "`.",
		"Automated Windows validation:",
		"- [x] Signed the [Contributor License Agreement]",
		"- [x] This PR only modifies one (1) manifest",
		"- [x] Tested manifest with `winget install --manifest <path>`",
	}
	var missing []string
	for _, value := range expected {
		if body != "" && !strings.Contains(body, value) {
			missing = append(missing, value)
		}
	}
	ok := pr.State == "OPEN" && !pr.IsDraft && (body == "" || len(missing) == 0)
	return map[string]any{
		"ok":                 ok,
		"readyForReview":     !pr.IsDraft,
		"state":              pr.State,
		"bodyChecked":        body != "",
		"missingBodySignals": missing,
	}
}

func releaseNextStep(tool, version, runID string, autoDispatch bool) map[string]any {
	args := map[string]any{"version": version}
	if runID != "" {
		args["runID"] = runID
	}
	if tool == "release_watch" {
		args["autoDispatchRelease"] = autoDispatch
	}
	return map[string]any{
		"tool":          tool,
		"suggestedArgs": args,
	}
}

func releaseWingetBranch(version string) string {
	return wingetBranchPrefix + strings.TrimPrefix(version, "v")
}

func nextPatchReleaseTag(tags []string) (string, error) {
	if len(tags) == 0 {
		return "v0.1.0", nil
	}
	major, minor, patch, ok := parseReleaseTag(tags[0])
	if !ok {
		return "", fmt.Errorf("latest release tag %q is not a simple vMAJOR.MINOR.PATCH tag", tags[0])
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch+1), nil
}

func compareReleaseTags(a, b string) int {
	amajor, aminor, apatch, aok := parseReleaseTag(a)
	bmajor, bminor, bpatch, bok := parseReleaseTag(b)
	if !aok || !bok {
		return strings.Compare(a, b)
	}
	if amajor != bmajor {
		return amajor - bmajor
	}
	if aminor != bminor {
		return aminor - bminor
	}
	return apatch - bpatch
}

func parseReleaseTag(tag string) (int, int, int, bool) {
	match := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`).FindStringSubmatch(strings.TrimSpace(tag))
	if match == nil {
		return 0, 0, 0, false
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return major, minor, patch, true
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func releaseProgressPercent(status, conclusion string, autoDispatched bool) int {
	if status == "completed" && conclusion == "success" {
		return 80
	}
	if status == "completed" {
		return 100
	}
	if autoDispatched {
		return 60
	}
	return 25
}

func releaseProgressMessage(version string, run releaseWorkflowRun) string {
	message := fmt.Sprintf("Watching %s for %s: %s", run.WorkflowName, version, run.Status)
	if run.Conclusion != "" {
		message += " / " + run.Conclusion
	}
	if len(run.Jobs) > 0 {
		var jobStates []string
		for _, job := range run.Jobs {
			jobStates = append(jobStates, strings.TrimSpace(job.Name+"="+nonEmpty(job.Conclusion, job.Status)))
		}
		message += " (" + strings.Join(jobStates, ", ") + ")"
	}
	return message
}
