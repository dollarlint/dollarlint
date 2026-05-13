package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldRemoteWorkflowFile = "agentic-product-testing.lock.yml"
	realWorldRemoteWorkflowName = "Agentic Product Testing"
	realWorldRemoteDefaultLimit = 10
	realWorldRemoteMaxLimit     = 50
	realWorldRemoteLinkListCap  = 10
)

type realWorldRemoteRunsArgs struct {
	Limit        int    `json:"limit"`
	Branch       string `json:"branch"`
	Status       string `json:"status"`
	Event        string `json:"event"`
	IncludeLinks bool   `json:"includeLinks"`
}

type realWorldRemoteRunArgs struct {
	RunID        string `json:"runID"`
	IncludeSteps bool   `json:"includeSteps"`
}

type realWorldRemotePullRequest struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	State     string   `json:"state"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
	MergedAt  string   `json:"mergedAt,omitempty"`
	EntryIDs  []string `json:"entryIDs,omitempty"`
}

type realWorldRemoteDiscussion struct {
	Number        int            `json:"number"`
	Title         string         `json:"title"`
	URL           string         `json:"url"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
	CommentURLs   []string       `json:"commentURLs,omitempty"`
	ResultSummary map[string]any `json:"resultSummary,omitempty"`
	EntryIDs      []string       `json:"entryIDs,omitempty"`
}

type realWorldRemoteLinks struct {
	PullRequests []realWorldRemotePullRequest `json:"pullRequests"`
	Discussions  []realWorldRemoteDiscussion  `json:"discussions"`
	EntryIDs     []string                     `json:"entryIDs,omitempty"`
	Warnings     []string                     `json:"warnings,omitempty"`
}

func (s *repoServer) handleRealWorldRemoteRuns(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldRemoteRunsArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldRemoteRuns(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) handleRealWorldRemoteRun(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldRemoteRunArgs
	_ = request.BindArguments(&args)
	out, err := s.realWorldRemoteRun(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, out)
}

func (s *repoServer) realWorldRemoteRuns(ctx context.Context, args realWorldRemoteRunsArgs) (map[string]any, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = realWorldRemoteDefaultLimit
	}
	if limit > realWorldRemoteMaxLimit {
		limit = realWorldRemoteMaxLimit
	}
	ghArgs := []string{
		"run", "list",
		"--workflow", realWorldRemoteWorkflowFile,
		"--limit", strconv.Itoa(limit),
		"--json", "databaseId,displayTitle,status,conclusion,headBranch,headSha,event,createdAt,updatedAt,url,workflowName",
	}
	if strings.TrimSpace(args.Branch) != "" {
		ghArgs = append(ghArgs, "--branch", strings.TrimSpace(args.Branch))
	}
	if strings.TrimSpace(args.Status) != "" {
		ghArgs = append(ghArgs, "--status", strings.TrimSpace(args.Status))
	}
	if strings.TrimSpace(args.Event) != "" {
		ghArgs = append(ghArgs, "--event", strings.TrimSpace(args.Event))
	}
	raw, err := commandOutput(ctx, s.root, "gh", ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w", strings.Join(ghArgs, " "), err)
	}
	var runs []ghWorkflowRun
	if err := json.Unmarshal(raw, &runs); err != nil {
		return nil, fmt.Errorf("parse gh run list: %w", err)
	}
	warnings := []string{}
	repoSlug := ""
	if args.IncludeLinks {
		var repoErr error
		repoSlug, repoErr = s.realWorldRemoteGitHubRepo(ctx)
		if repoErr != nil {
			warnings = append(warnings, repoErr.Error())
		}
		if len(runs) > realWorldRemoteLinkListCap {
			warnings = append(warnings, fmt.Sprintf("includeLinks is capped to the first %d runs to avoid excessive GitHub API calls", realWorldRemoteLinkListCap))
		}
	}
	summaries := []map[string]any{}
	for i, run := range runs {
		summary := realWorldRemoteRunSummary(run)
		if args.IncludeLinks && repoSlug != "" && i < realWorldRemoteLinkListCap {
			runID := strconv.FormatInt(run.DatabaseID, 10)
			links := s.realWorldRemoteFindLinks(ctx, repoSlug, runID, run.URL)
			summary["links"] = links
			warnings = append(warnings, links.Warnings...)
		}
		summaries = append(summaries, summary)
	}
	out := map[string]any{
		"ok":       true,
		"workflow": realWorldRemoteWorkflowName,
		"query": map[string]any{
			"limit":        limit,
			"branch":       args.Branch,
			"status":       args.Status,
			"event":        args.Event,
			"includeLinks": args.IncludeLinks,
		},
		"runs":     summaries,
		"nextStep": "Use real_world_remote_run with a runID to inspect jobs, artifacts, durable-memory PRs, Discussions, and recorded entry hints for a specific run.",
	}
	if len(warnings) > 0 {
		out["warnings"] = orderedUniqueStrings(warnings)
	}
	return out, nil
}

func (s *repoServer) realWorldRemoteRun(ctx context.Context, args realWorldRemoteRunArgs) (map[string]any, error) {
	runID, err := normalizeGitHubRunID(args.RunID)
	if err != nil {
		return nil, err
	}
	raw, err := commandOutput(ctx, s.root, "gh", "run", "view", runID, "--json", "databaseId,displayTitle,status,conclusion,headBranch,headSha,event,createdAt,updatedAt,url,workflowName,jobs")
	if err != nil {
		return nil, fmt.Errorf("gh run view %s: %w", runID, err)
	}
	var run map[string]any
	if err := json.Unmarshal(raw, &run); err != nil {
		return nil, fmt.Errorf("parse gh run view %s: %w", runID, err)
	}
	run["jobs"] = realWorldRemoteJobSummaries(run["jobs"], args.IncludeSteps)
	repoSlug, repoErr := s.realWorldRemoteGitHubRepo(ctx)
	warnings := []string{}
	if repoErr != nil {
		warnings = append(warnings, repoErr.Error())
	}
	runURL, _ := run["url"].(string)
	if runURL == "" && repoSlug != "" {
		runURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s", repoSlug, runID)
		run["url"] = runURL
	}
	artifacts := map[string]any{"totalCount": 0, "artifacts": []map[string]any{}}
	if repoSlug != "" {
		var artifactWarnings []string
		artifacts, artifactWarnings = s.realWorldRemoteArtifacts(ctx, repoSlug, runID)
		warnings = append(warnings, artifactWarnings...)
	}
	links := realWorldRemoteLinks{}
	if repoSlug != "" {
		links = s.realWorldRemoteFindLinks(ctx, repoSlug, runID, runURL)
		warnings = append(warnings, links.Warnings...)
	}
	localEntries := s.realWorldRemoteLocalEntries(links.EntryIDs)
	out := map[string]any{
		"ok":           true,
		"runID":        runID,
		"workflow":     realWorldRemoteWorkflowName,
		"run":          run,
		"artifacts":    artifacts,
		"links":        links,
		"localEntries": localEntries,
		"nextStep":     realWorldRemoteRunNextStep(links.EntryIDs, localEntries, links.PullRequests),
	}
	if len(warnings) > 0 {
		out["warnings"] = orderedUniqueStrings(warnings)
	}
	return out, nil
}

func realWorldRemoteRunSummary(run ghWorkflowRun) map[string]any {
	return map[string]any{
		"databaseId":   run.DatabaseID,
		"displayTitle": run.DisplayTitle,
		"workflowName": run.WorkflowName,
		"status":       run.Status,
		"conclusion":   run.Conclusion,
		"event":        run.Event,
		"headBranch":   run.HeadBranch,
		"headSha":      run.HeadSHA,
		"createdAt":    run.CreatedAt,
		"updatedAt":    run.UpdatedAt,
		"url":          run.URL,
	}
}

func normalizeGitHubRunID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("runID is required")
	}
	if regexp.MustCompile(`^[0-9]+$`).MatchString(raw) {
		return raw, nil
	}
	match := regexp.MustCompile(`/actions/runs/([0-9]+)`).FindStringSubmatch(raw)
	if len(match) == 2 {
		return match[1], nil
	}
	return "", fmt.Errorf("runID must be a numeric GitHub Actions run id or a run URL containing /actions/runs/<id>")
}

func (s *repoServer) realWorldRemoteGitHubRepo(ctx context.Context) (string, error) {
	if raw, err := commandOutput(ctx, s.root, "git", "remote", "get-url", "origin"); err == nil {
		if slug := githubRepoSlugFromRemote(strings.TrimSpace(string(raw))); slug != "" {
			return slug, nil
		}
	}
	raw, err := commandOutput(ctx, s.root, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", fmt.Errorf("resolve GitHub repository: %w", err)
	}
	slug := strings.TrimSpace(string(raw))
	if slug == "" || !strings.Contains(slug, "/") {
		return "", fmt.Errorf("resolve GitHub repository: gh repo view returned %q", slug)
	}
	return slug, nil
}

func githubRepoSlugFromRemote(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, ".git"))
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "git@github.com:") {
		return normalizeRepoSlug(strings.TrimPrefix(raw, "git@github.com:"))
	}
	if strings.HasPrefix(raw, "github.com:") {
		return normalizeRepoSlug(strings.TrimPrefix(raw, "github.com:"))
	}
	parsed, err := url.Parse(raw)
	if err == nil && strings.EqualFold(parsed.Host, "github.com") {
		return normalizeRepoSlug(parsed.Path)
	}
	return ""
}

func normalizeRepoSlug(raw string) string {
	raw = strings.Trim(strings.TrimSpace(strings.TrimSuffix(raw, ".git")), "/")
	parts := strings.Split(raw, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func realWorldRemoteJobSummaries(raw any, includeSteps bool) []map[string]any {
	jobs, ok := raw.([]any)
	if !ok {
		return nil
	}
	summaries := make([]map[string]any, 0, len(jobs))
	for _, item := range jobs {
		job, ok := item.(map[string]any)
		if !ok {
			continue
		}
		summary := copySelectedMapKeys(job, []string{"databaseId", "name", "status", "conclusion", "startedAt", "completedAt", "url"})
		if includeSteps {
			summary["steps"] = realWorldRemoteStepSummaries(job["steps"])
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func realWorldRemoteStepSummaries(raw any) []map[string]any {
	steps, ok := raw.([]any)
	if !ok {
		return nil
	}
	summaries := make([]map[string]any, 0, len(steps))
	for _, item := range steps {
		step, ok := item.(map[string]any)
		if !ok {
			continue
		}
		summaries = append(summaries, copySelectedMapKeys(step, []string{"number", "name", "status", "conclusion", "startedAt", "completedAt"}))
	}
	return summaries
}

func copySelectedMapKeys(source map[string]any, keys []string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := source[key]; ok {
			out[key] = value
		}
	}
	return out
}

func (s *repoServer) realWorldRemoteArtifacts(ctx context.Context, repoSlug, runID string) (map[string]any, []string) {
	raw, err := commandOutput(ctx, s.root, "gh", "api", "-X", "GET", "repos/"+repoSlug+"/actions/runs/"+runID+"/artifacts", "-f", "per_page=100")
	if err != nil {
		return map[string]any{"totalCount": 0, "artifacts": []map[string]any{}}, []string{"gh api artifacts: " + err.Error()}
	}
	var response struct {
		TotalCount int `json:"total_count"`
		Artifacts  []struct {
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			SizeInBytes        int64  `json:"size_in_bytes"`
			Expired            bool   `json:"expired"`
			CreatedAt          string `json:"created_at"`
			UpdatedAt          string `json:"updated_at"`
			ArchiveDownloadURL string `json:"archive_download_url"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return map[string]any{"totalCount": 0, "artifacts": []map[string]any{}}, []string{"parse artifacts: " + err.Error()}
	}
	artifacts := make([]map[string]any, 0, len(response.Artifacts))
	for _, artifact := range response.Artifacts {
		artifacts = append(artifacts, map[string]any{
			"id":                 artifact.ID,
			"name":               artifact.Name,
			"sizeInBytes":        artifact.SizeInBytes,
			"expired":            artifact.Expired,
			"createdAt":          artifact.CreatedAt,
			"updatedAt":          artifact.UpdatedAt,
			"archiveDownloadURL": artifact.ArchiveDownloadURL,
		})
	}
	return map[string]any{"totalCount": response.TotalCount, "artifacts": artifacts}, nil
}

func (s *repoServer) realWorldRemoteFindLinks(ctx context.Context, repoSlug, runID, runURL string) realWorldRemoteLinks {
	warnings := []string{}
	pullRequests, prWarnings := s.realWorldRemotePullRequests(ctx, repoSlug, runID, runURL)
	warnings = append(warnings, prWarnings...)
	discussions, discussionWarnings := s.realWorldRemoteDiscussions(ctx, repoSlug, runID, runURL)
	warnings = append(warnings, discussionWarnings...)
	entryIDs := []string{}
	for _, pr := range pullRequests {
		entryIDs = append(entryIDs, pr.EntryIDs...)
	}
	for _, discussion := range discussions {
		entryIDs = append(entryIDs, discussion.EntryIDs...)
	}
	return realWorldRemoteLinks{
		PullRequests: pullRequests,
		Discussions:  discussions,
		EntryIDs:     orderedUniqueStrings(entryIDs),
		Warnings:     orderedUniqueStrings(warnings),
	}
}

func (s *repoServer) realWorldRemotePullRequests(ctx context.Context, repoSlug, runID, runURL string) ([]realWorldRemotePullRequest, []string) {
	type searchItem struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		URL         string `json:"html_url"`
		State       string `json:"state"`
		UpdatedAt   string `json:"updated_at"`
		Body        string `json:"body"`
		PullRequest *struct {
			MergedAt string `json:"merged_at"`
		} `json:"pull_request"`
	}
	type searchResponse struct {
		Items []searchItem `json:"items"`
	}
	queries := orderedUniqueStrings([]string{
		fmt.Sprintf(`repo:%s is:pr "%s"`, repoSlug, runURL),
		fmt.Sprintf(`repo:%s is:pr "/actions/runs/%s"`, repoSlug, runID),
		fmt.Sprintf(`repo:%s is:pr "id: %s"`, repoSlug, runID),
	})
	warnings := []string{}
	seen := map[int]bool{}
	pullRequests := []realWorldRemotePullRequest{}
	for _, query := range queries {
		raw, err := commandOutput(ctx, s.root, "gh", "api", "-X", "GET", "search/issues", "-f", "q="+query, "-f", "per_page=10")
		if err != nil {
			warnings = append(warnings, "gh api search PRs: "+err.Error())
			continue
		}
		var response searchResponse
		if err := json.Unmarshal(raw, &response); err != nil {
			warnings = append(warnings, "parse PR search: "+err.Error())
			continue
		}
		for _, item := range response.Items {
			if item.PullRequest == nil || seen[item.Number] {
				continue
			}
			if !containsRunMarker(item.Body+"\n"+item.Title, runID, runURL) && runURL != "" {
				continue
			}
			seen[item.Number] = true
			pullRequests = append(pullRequests, realWorldRemotePullRequest{
				Number:    item.Number,
				Title:     item.Title,
				URL:       item.URL,
				State:     item.State,
				UpdatedAt: item.UpdatedAt,
				MergedAt:  item.PullRequest.MergedAt,
				EntryIDs:  extractRealWorldEntryIDs(item.Body),
			})
		}
	}
	return pullRequests, orderedUniqueStrings(warnings)
}

func (s *repoServer) realWorldRemoteDiscussions(ctx context.Context, repoSlug, runID, runURL string) ([]realWorldRemoteDiscussion, []string) {
	owner, repo, ok := strings.Cut(repoSlug, "/")
	if !ok || owner == "" || repo == "" {
		return nil, []string{"cannot query Discussions for invalid repository slug " + repoSlug}
	}
	query := `query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    discussions(first: 100, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        number
        title
        url
        body
        updatedAt
        comments(first: 50) {
          nodes { body url }
        }
      }
    }
  }
}`
	raw, err := commandOutput(ctx, s.root, "gh", "api", "graphql", "-f", "query="+query, "-f", "owner="+owner, "-f", "repo="+repo)
	if err != nil {
		return nil, []string{"gh api discussions: " + err.Error()}
	}
	var response struct {
		Data struct {
			Repository struct {
				Discussions struct {
					Nodes []struct {
						Number    int    `json:"number"`
						Title     string `json:"title"`
						URL       string `json:"url"`
						Body      string `json:"body"`
						UpdatedAt string `json:"updatedAt"`
						Comments  struct {
							Nodes []struct {
								Body string `json:"body"`
								URL  string `json:"url"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"discussions"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, []string{"parse Discussions query: " + err.Error()}
	}
	discussions := []realWorldRemoteDiscussion{}
	for _, item := range response.Data.Repository.Discussions.Nodes {
		sources := []string{item.Body}
		commentURLs := []string{}
		for _, comment := range item.Comments.Nodes {
			sources = append(sources, comment.Body)
			if containsRunMarker(comment.Body, runID, runURL) {
				commentURLs = append(commentURLs, comment.URL)
			}
		}
		combined := strings.Join(sources, "\n")
		if !containsRunMarker(combined, runID, runURL) {
			continue
		}
		discussions = append(discussions, realWorldRemoteDiscussion{
			Number:        item.Number,
			Title:         item.Title,
			URL:           item.URL,
			UpdatedAt:     item.UpdatedAt,
			CommentURLs:   commentURLs,
			ResultSummary: realWorldRemoteResultSummary(item.Body),
			EntryIDs:      extractRealWorldEntryIDs(combined),
		})
	}
	return discussions, nil
}

func containsRunMarker(text, runID, runURL string) bool {
	return (runURL != "" && strings.Contains(text, runURL)) ||
		strings.Contains(text, "/actions/runs/"+runID) ||
		strings.Contains(text, "id: "+runID)
}

func realWorldRemoteResultSummary(body string) map[string]any {
	summary := map[string]any{}
	lines := strings.Split(body, "\n")
	recommendations := []string{}
	inRecommendations := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		plain := cleanMarkdownLine(trimmed)
		lower := strings.ToLower(plain)
		switch {
		case summary["resultCounts"] == nil && strings.Contains(lower, "result counts"):
			summary["resultCounts"] = plain
		case summary["coverage"] == nil && strings.Contains(lower, "coverage"):
			summary["coverage"] = plain
		case summary["persistedArtifact"] == nil && strings.Contains(trimmed, "reports/agentic-product-testing/") && strings.Contains(trimmed, "dollarlint.json"):
			summary["persistedArtifact"] = firstRealWorldPath(trimmed)
		case summary["metadataPath"] == nil && strings.Contains(trimmed, "reports/agentic-product-testing/") && strings.Contains(trimmed, "metadata.json"):
			summary["metadataPath"] = firstRealWorldPath(trimmed)
		case summary["dollarlintCommit"] == nil && strings.Contains(lower, "dollarlint commit"):
			summary["dollarlintCommit"] = plain
		}
		if strings.HasPrefix(lower, "product recommendations") || strings.HasPrefix(lower, "## product recommendations") {
			inRecommendations = true
			continue
		}
		if inRecommendations {
			if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "<details") {
				inRecommendations = false
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				recommendations = append(recommendations, cleanMarkdownLine(strings.TrimPrefix(trimmed, "- ")))
			}
		}
	}
	if len(recommendations) > 0 {
		if len(recommendations) > 6 {
			recommendations = recommendations[:6]
		}
		summary["productRecommendations"] = recommendations
	}
	if entryIDs := extractRealWorldEntryIDs(body); len(entryIDs) > 0 {
		summary["entryIDs"] = entryIDs
	}
	return summary
}

func cleanMarkdownLine(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "`", "")
	return strings.TrimSpace(line)
}

func firstRealWorldPath(text string) string {
	matches := regexp.MustCompile("reports/agentic-product-testing/[^\\s`)]+").FindString(text)
	return strings.Trim(matches, "`.,")
}

func extractRealWorldEntryIDs(text string) []string {
	ids := []string{}
	for _, match := range regexp.MustCompile("reports/agentic-product-testing/([^/`\\s]+)/").FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			ids = append(ids, match[1])
		}
	}
	for _, match := range regexp.MustCompile("Real-world entry:\\s*`([^`]+)`").FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			ids = append(ids, match[1])
		}
	}
	return orderedUniqueStrings(ids)
}

func (s *repoServer) realWorldRemoteLocalEntries(entryIDs []string) []map[string]any {
	entryIDs = orderedUniqueStrings(entryIDs)
	if len(entryIDs) == 0 {
		return nil
	}
	historyByID := map[string]realWorldEntry{}
	if history, err := loadRealWorldHistory(s.root); err == nil {
		for _, entry := range history.Entries {
			historyByID[entry.ID] = entry
		}
	}
	entries := []map[string]any{}
	for _, entryID := range entryIDs {
		if entry, ok := historyByID[entryID]; ok {
			summary := realWorldArtifactQueryEntrySummary(s.root, entry)
			summary["presentInHistory"] = true
			entries = append(entries, summary)
			continue
		}
		metadataPath := filepath.ToSlash(filepath.Join(realWorldRunsDirRelPath, entryID, realWorldRunMetadataFileName))
		entry := map[string]any{
			"id":               entryID,
			"presentInHistory": false,
			"metadataPath":     metadataPath,
		}
		if _, err := os.Stat(filepath.Join(s.root, metadataPath)); err == nil {
			entry["metadataExists"] = true
		}
		entries = append(entries, entry)
	}
	return entries
}

func realWorldRemoteRunNextStep(entryIDs []string, localEntries []map[string]any, pullRequests []realWorldRemotePullRequest) string {
	if len(entryIDs) > 0 && len(localEntries) > 0 {
		return fmt.Sprintf("Use real_world_artifact_query with entryID %q to inspect the persisted DollarLint output, or review the linked PR/Discussion for the remote run summary.", entryIDs[0])
	}
	if len(pullRequests) > 0 {
		return "Review or merge the linked durable-memory PR so the remote result becomes available to local real-world history and artifact query tools."
	}
	return "Use the artifact download URLs or the GitHub run URL to inspect raw remote logs, then merge any durable-memory PR once available."
}

func orderedUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
