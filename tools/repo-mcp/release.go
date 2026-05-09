package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

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
	p := newProgress(ctx, s.mcp, request, 3)
	p.step("Reading local tags")
	tags := s.output(ctx, "git tag --sort=-creatordate | head -12")
	p.step("Reading release workflows")
	prepare := s.output(ctx, "gh run list --workflow prepare-release.yml --limit 5")
	release := s.output(ctx, "gh run list --workflow release.yml --limit 5")
	p.step("Reading GitHub releases")
	releases := s.output(ctx, "gh release list --limit 8")
	return structured(map[string]any{
		"tags":                lines(tags),
		"prepareReleaseRuns":  lines(prepare),
		"releaseRuns":         lines(release),
		"githubReleases":      lines(releases),
		"prepareReleaseFile":  ".github/workflows/prepare-release.yml",
		"releaseWorkflowFile": ".github/workflows/release.yml",
	})
}

func (s *repoServer) handlePrepareRelease(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Version string `json:"version"`
		DryRun  *bool  `json:"dryRun"`
	}
	_ = request.BindArguments(&args)
	if err := validateVersion(args.Version); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
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
		result["wouldRun"] = fmt.Sprintf("gh workflow run prepare-release.yml -f version=%s", args.Version)
		return structured(result)
	}
	commands = append(commands, namedCommand{Name: "trigger prepare-release", Cmd: fmt.Sprintf("gh workflow run prepare-release.yml -f version=%s", shellQuote(args.Version))})
	result, err := s.runCommandSetData(ctx, request, "prepare-release", commands)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result["dryRun"] = false
	result["nextStep"] = "Use release_status to watch Prepare Release create the tag and the Release workflow publish artifacts."
	return structured(result)
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
