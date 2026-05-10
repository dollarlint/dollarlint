package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dollarlint/dollarlint"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *repoServer) handleRepoStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := newProgress(ctx, s.mcp, request, 4)
	p.step("Reading git status")
	status := s.output(ctx, "git status --short --branch --untracked-files=all")
	p.step("Reading recent commits")
	commits := s.output(ctx, "git log --oneline --decorate -8")
	p.step("Reading tags and remotes")
	tags := s.output(ctx, "git tag --sort=-creatordate | head -10")
	remotes := s.output(ctx, "git remote -v")
	p.step("Checking generated artifacts")
	artifacts := existingPaths(s.root, []string{"coverage.out", "coverage.html", "dist", "docs/dist", "bin/dollarlint", "dollarlint"})
	return structured(map[string]any{
		"root":               s.root,
		"status":             strings.TrimSpace(status),
		"recentCommits":      lines(commits),
		"recentTags":         lines(tags),
		"remotes":            lines(remotes),
		"generatedArtifacts": artifacts,
		"clean":              !hasDirtyStatus(status),
	})
}

func (s *repoServer) handleProjectMap(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return structured(map[string]any{
		"entrypoints": map[string]string{
			"cli":        "cmd/dollarlint/main.go",
			"public_mcp": "internal/cli/mcp.go",
			"repo_mcp":   "tools/repo-mcp/main.go",
		},
		"core": []string{
			"internal/engine/validate.go",
			"internal/engine/schema_cache.go",
			"internal/engine/config.go",
			"internal/engine/azure_pruning.go",
			"internal/engine/output.go",
			"internal/engine/sarif.go",
		},
		"docs":      "docs",
		"examples":  []string{"examples/basics", "examples/nested-configs", "examples/schemastore", "examples/azure"},
		"release":   []string{".goreleaser.yaml", ".github/workflows/prepare-release.yml", ".github/workflows/release.yml"},
		"keyChecks": []string{"ci_readiness", "agentic_workflow_readiness", "go test ./...", "go vet ./...", "npm run build in docs", goreleaserCheckCommand},
	})
}

func (s *repoServer) handleVerify(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Profile string `json:"profile"`
	}
	_ = request.BindArguments(&args)
	if args.Profile == "" {
		args.Profile = "quick"
	}
	commands, err := verifyCommands(args.Profile)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.runCommandSet(ctx, request, args.Profile, commands)
}

func (s *repoServer) handleRunExample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Suite     string `json:"suite"`
		Format    string `json:"format"`
		Locations *bool  `json:"locations"`
	}
	_ = request.BindArguments(&args)
	if args.Suite == "" {
		args.Suite = "basics"
	}
	if args.Format == "" {
		args.Format = "text"
	}
	locations := true
	if args.Locations != nil {
		locations = *args.Locations
	}
	commands, err := exampleCommands(args.Suite, args.Format, locations)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.runCommandSet(ctx, request, "examples:"+args.Suite, commands)
}

func (s *repoServer) handleDiagnoseValidation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Include      []string `json:"include"`
		BranchErrors string   `json:"branchErrors"`
	}
	_ = request.BindArguments(&args)
	p := newProgress(ctx, s.mcp, request, 3)
	p.step("Loading config")
	cfg, configPath, err := dollarlint.LoadConfig(s.root, "")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(args.Include) > 0 {
		cfg.Discovery.Include = args.Include
	}
	if args.BranchErrors != "" {
		cfg.Output.BranchErrors = args.BranchErrors
	}
	cfg.Output.Locations = true
	overlay := func(config *dollarlint.Config) error {
		if len(args.Include) > 0 {
			config.Discovery.Include = args.Include
		}
		if args.BranchErrors != "" {
			config.Output.BranchErrors = args.BranchErrors
		}
		config.Output.Locations = true
		return nil
	}
	p.step("Running validation")
	result, err := dollarlint.Lint(ctx, dollarlint.Options{Root: s.root, Config: cfg, ConfigPath: configPath, ConfigOverlay: overlay, SourceLocations: true})
	if err != nil {
		return structured(map[string]any{"ok": false, "error": err.Error(), "classification": classifyError(err.Error()), "configPath": configPath})
	}
	p.step("Classifying result")
	return structured(map[string]any{
		"ok":             !result.HasIssues(),
		"configPath":     configPath,
		"summary":        result.Summary,
		"warnings":       result.Warnings,
		"classification": classifyResult(result),
		"files":          result.Files,
	})
}

func (s *repoServer) handleAzurePruningReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		File         string `json:"file"`
		BranchErrors string `json:"branchErrors"`
	}
	_ = request.BindArguments(&args)
	if args.File == "" {
		return mcp.NewToolResultError("file is required"), nil
	}
	p := newProgress(ctx, s.mcp, request, 4)
	p.step("Reading ARM template")
	rel, err := cleanRelativePath(args.File)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	abs := filepath.Join(s.root, rel)
	data, err := os.ReadFile(abs)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("parse ARM template JSON: %v", err)), nil
	}
	p.step("Collecting resource types")
	refs := collectARMRefs(document)
	p.step("Running focused validation")
	cfg, configPath, err := dollarlint.LoadConfig(s.root, "")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cfg.Discovery.Include = []string{rel}
	cfg.Output.Locations = true
	if args.BranchErrors != "" {
		cfg.Output.BranchErrors = args.BranchErrors
	}
	overlay := func(config *dollarlint.Config) error {
		config.Discovery.Include = []string{rel}
		config.Output.Locations = true
		if args.BranchErrors != "" {
			config.Output.BranchErrors = args.BranchErrors
		}
		return nil
	}
	result, lintErr := dollarlint.Lint(ctx, dollarlint.Options{Root: s.root, Config: cfg, ConfigPath: configPath, ConfigOverlay: overlay, SourceLocations: true})
	p.step("Building report")
	report := map[string]any{
		"file":             rel,
		"configPath":       configPath,
		"pruningEnabled":   boolValue(cfg.Schemas.Optimizations.Enabled) && boolValue(cfg.Schemas.Optimizations.Azure.PruneResources),
		"branchErrors":     cfg.Output.BranchErrors,
		"resourceRefs":     refs,
		"resourceRefCount": len(refs),
	}
	if lintErr != nil {
		report["validationError"] = lintErr.Error()
		report["classification"] = classifyError(lintErr.Error())
	} else {
		report["validationSummary"] = result.Summary
		report["issues"] = resultIssueDigest(result, 12)
	}
	return structured(report)
}
