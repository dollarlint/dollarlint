package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *repoServer) addTools() {
	s.addTool("repo_status", "Summarize branch, dirty files, recent commits, tags, generated artifacts, and configured remotes.", schemaObject(nil), s.handleRepoStatus, true)
	s.addTool("project_map", "Return a concise map of important dollarlint paths, commands, and development surfaces.", schemaObject(nil), s.handleProjectMap, true)
	s.addTool("verify", "Run a named repo verification profile. Profiles: quick, full, docs, release, examples, ci.", schemaObject(map[string]any{
		"profile": enumSchema([]string{"quick", "full", "docs", "release", "examples", "ci"}, "Verification profile to run."),
	}), s.handleVerify, false)
	s.addTool("run_example", "Run dollarlint against a named example suite and return structured validation output.", schemaObject(map[string]any{
		"suite":     enumSchema([]string{"basics", "nested-configs", "schemastore", "azure", "repo-config", "all"}, "Example suite to validate."),
		"format":    enumSchema([]string{"text", "json", "sarif"}, "Output format. Defaults to text."),
		"locations": map[string]any{"type": "boolean", "description": "Include source locations. Defaults to true."},
	}), s.handleRunExample, true)
	s.addTool("diagnose_validation", "Validate files and classify common dollarlint states such as skipped files, fetch/config errors, branch error mode, and Azure pruning.", schemaObject(map[string]any{
		"include":      arrayStringSchema("File or glob patterns to validate."),
		"branchErrors": enumSchema([]string{"best", "all"}, "Temporarily override output.branchErrors."),
	}), s.handleDiagnoseValidation, true)
	s.addTool("azure_pruning_report", "Inspect an Azure ARM template and report detected resource refs, pruning config, branch error mode, and validation summary.", schemaObject(map[string]any{
		"file":         map[string]any{"type": "string", "description": "ARM template path relative to the repo root."},
		"branchErrors": enumSchema([]string{"best", "all"}, "Temporarily override output.branchErrors."),
	}), s.handleAzurePruningReport, true)
	s.addTool("release_readiness", "Run release-readiness checks without publishing anything.", schemaObject(map[string]any{
		"snapshot": map[string]any{"type": "boolean", "description": "Also run GoReleaser snapshot build. Defaults to false."},
	}), s.handleReleaseReadiness, false)
	s.addTool("release_status", "Inspect recent release-related workflow runs and tags.", schemaObject(nil), s.handleReleaseStatus, true)
	s.addTool("prepare_release", "Dry-run or trigger the Prepare Release workflow. dryRun defaults to true; dryRun=false triggers an externally visible GitHub workflow.", schemaObject(map[string]any{
		"version": map[string]any{"type": "string", "description": "Release tag to create, for example v0.1.2."},
		"dryRun":  map[string]any{"type": "boolean", "description": "When true, validate readiness without triggering GitHub Actions. Defaults to true."},
	}), s.handlePrepareRelease, false)
}

func (s *repoServer) addTool(name, description string, input map[string]any, handler server.ToolHandlerFunc, readOnly bool) {
	tool := mcp.NewToolWithRawSchema(name, description, mustRawJSON(input))
	tool.RawOutputSchema = mustRawJSON(map[string]any{"type": "object", "additionalProperties": true})
	tool.Annotations = mcp.ToolAnnotation{
		Title:           name,
		ReadOnlyHint:    mcp.ToBoolPtr(readOnly),
		IdempotentHint:  mcp.ToBoolPtr(readOnly),
		DestructiveHint: mcp.ToBoolPtr(!readOnly),
		OpenWorldHint:   mcp.ToBoolPtr(false),
	}
	s.mcp.AddTool(tool, handler)
}
