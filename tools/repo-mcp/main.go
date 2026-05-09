package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dollarlint/dollarlint"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverName = "dollarlint-repo"

type repoServer struct {
	root string
	mcp  *server.MCPServer
}

type commandResult struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exitCode"`
	Duration  string `json:"duration"`
	Output    string `json:"output,omitempty"`
	Succeeded bool   `json:"succeeded"`
}

func main() {
	root, err := findRepoRoot()
	if err != nil {
		log.Fatal(err)
	}
	rs := &repoServer{root: root}
	rs.mcp = server.NewMCPServer(
		serverName,
		"0.1.0",
		server.WithInstructions("Repo-only maintenance tools for the dollarlint checkout. These tools run curated verification, release, example, and Azure diagnostics workflows for Codex sessions working in this repository."),
		server.WithToolCapabilities(false),
		server.WithInputSchemaValidation(),
		server.WithOutputSchemaValidation(),
		server.WithRecovery(),
	)
	rs.addTools()
	stdio := server.NewStdioServer(rs.mcp)
	stdio.SetErrorLogger(log.New(io.Discard, "", 0))
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func (s *repoServer) addTools() {
	s.addTool("repo_status", "Summarize branch, dirty files, recent commits, tags, generated artifacts, and configured remotes.", schemaObject(nil), s.handleRepoStatus, true)
	s.addTool("project_map", "Return a concise map of important dollarlint paths, commands, and development surfaces.", schemaObject(nil), s.handleProjectMap, true)
	s.addTool("verify", "Run a named repo verification profile. Profiles: quick, full, docs, release, examples, ci.", schemaObject(map[string]any{
		"profile": enumSchema([]string{"quick", "full", "docs", "release", "examples", "ci"}, "Verification profile to run."),
	}), s.handleVerify, false)
	s.addTool("run_example", "Run dollarlint against a named example suite and return structured validation output.", schemaObject(map[string]any{
		"suite":     enumSchema([]string{"basics", "schemastore", "azure", "repo-config", "all"}, "Example suite to validate."),
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
		"examples":  []string{"examples/basics", "examples/schemastore", "examples/azure"},
		"release":   []string{".goreleaser.yaml", ".github/workflows/prepare-release.yml", ".github/workflows/release.yml"},
		"keyChecks": []string{"go test ./...", "go vet ./...", "npm run build in docs", "go run github.com/goreleaser/goreleaser/v2@latest check"},
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
	p.step("Running validation")
	result, err := dollarlint.Lint(ctx, dollarlint.Options{Root: s.root, Config: cfg, SourceLocations: true})
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
	result, lintErr := dollarlint.Lint(ctx, dollarlint.Options{Root: s.root, Config: cfg, SourceLocations: true})
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
		{Name: "goreleaser check", Cmd: "go run github.com/goreleaser/goreleaser/v2@latest check"},
		{Name: "remote tags", Cmd: "git ls-remote --tags origin 'refs/tags/v*' | tail -10"},
	}
	if args.Snapshot {
		commands = append(commands, namedCommand{Name: "goreleaser snapshot", Cmd: "go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish"})
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
		{Name: "goreleaser check", Cmd: "go run github.com/goreleaser/goreleaser/v2@latest check"},
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

type namedCommand struct {
	Name string
	Cmd  string
	Dir  string
}

func verifyCommands(profile string) ([]namedCommand, error) {
	switch profile {
	case "quick":
		return []namedCommand{{Name: "go test", Cmd: "go test ./..."}, {Name: "go vet", Cmd: "go vet ./..."}}, nil
	case "docs":
		return []namedCommand{
			{Name: "docs format", Cmd: "npm run format:check", Dir: "docs"},
			{Name: "docs audit", Cmd: "npm run audit", Dir: "docs"},
			{Name: "docs build", Cmd: "npm run build", Dir: "docs"},
		}, nil
	case "release":
		return []namedCommand{
			{Name: "goreleaser check", Cmd: "go run github.com/goreleaser/goreleaser/v2@latest check"},
			{Name: "snapshot", Cmd: "go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish"},
		}, nil
	case "examples":
		return exampleCommands("all", "text", true)
	case "ci", "full":
		return []namedCommand{
			{Name: "go mod verify", Cmd: "go mod verify"},
			{Name: "go test", Cmd: "go test ./..."},
			{Name: "engine coverage", Cmd: "go test -coverprofile=coverage.out ./internal/engine && go tool cover -func=coverage.out | tail -n 1"},
			{Name: "go vet", Cmd: "go vet ./..."},
			{Name: "staticcheck", Cmd: "go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./..."},
			{Name: "govulncheck", Cmd: "go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./..."},
			{Name: "actionlint", Cmd: "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"},
			{Name: "go build", Cmd: "go build ./..."},
			{Name: "docs format", Cmd: "npm run format:check", Dir: "docs"},
			{Name: "docs audit", Cmd: "npm run audit", Dir: "docs"},
			{Name: "docs build", Cmd: "npm run build", Dir: "docs"},
			{Name: "goreleaser check", Cmd: "go run github.com/goreleaser/goreleaser/v2@latest check"},
			{Name: "diff check", Cmd: "git diff --check"},
		}, nil
	default:
		return nil, fmt.Errorf("unknown verify profile %q", profile)
	}
}

func exampleCommands(suite, format string, locations bool) ([]namedCommand, error) {
	base := "go run ./cmd/dollarlint validate"
	flags := []string{}
	if locations {
		flags = append(flags, "--locations")
	}
	if format != "" && format != "text" {
		flags = append(flags, "--format", format)
	}
	args := strings.Join(flags, " ")
	command := func(path string) namedCommand {
		cmd := strings.TrimSpace(base + " " + path + " " + args)
		return namedCommand{Name: path, Cmd: cmd}
	}
	switch suite {
	case "basics":
		return []namedCommand{command("./examples/basics")}, nil
	case "schemastore":
		return []namedCommand{command("./examples/schemastore")}, nil
	case "azure":
		return []namedCommand{command("./examples/azure")}, nil
	case "repo-config":
		return []namedCommand{command(". --include .dollarlint.toml")}, nil
	case "all":
		return []namedCommand{command("./examples/basics"), command("./examples/schemastore"), command("./examples/azure"), command(". --include .dollarlint.toml")}, nil
	default:
		return nil, fmt.Errorf("unknown example suite %q", suite)
	}
}

func (s *repoServer) runCommandSet(ctx context.Context, request mcp.CallToolRequest, profile string, commands []namedCommand) (*mcp.CallToolResult, error) {
	data, err := s.runCommandSetData(ctx, request, profile, commands)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return structured(data)
}

func (s *repoServer) runCommandSetData(ctx context.Context, request mcp.CallToolRequest, profile string, commands []namedCommand) (map[string]any, error) {
	p := newProgress(ctx, s.mcp, request, len(commands))
	results := make([]commandResult, 0, len(commands))
	ok := true
	for _, command := range commands {
		p.step("Running " + command.Name)
		result := s.run(ctx, command)
		results = append(results, result)
		if !result.Succeeded {
			ok = false
		}
	}
	return map[string]any{
		"profile":  profile,
		"ok":       ok,
		"commands": results,
	}, nil
}

func (s *repoServer) run(ctx context.Context, command namedCommand) commandResult {
	start := time.Now()
	dir := s.root
	if command.Dir != "" {
		dir = filepath.Join(s.root, command.Dir)
	}
	cmd := exec.CommandContext(ctx, "/bin/zsh", "-lc", command.Cmd)
	cmd.Dir = dir
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
		Name:      command.Name,
		Command:   command.Cmd,
		ExitCode:  exitCode,
		Duration:  time.Since(start).Round(time.Millisecond).String(),
		Output:    truncate(output.String(), 12000),
		Succeeded: err == nil,
	}
}

func (s *repoServer) output(ctx context.Context, cmd string) string {
	return s.run(ctx, namedCommand{Name: cmd, Cmd: cmd}).Output
}

type progress struct {
	ctx    context.Context
	server *server.MCPServer
	token  any
	total  int
	done   int
}

func newProgress(ctx context.Context, srv *server.MCPServer, request mcp.CallToolRequest, total int) *progress {
	var token any
	if request.Params.Meta != nil {
		token = request.Params.Meta.ProgressToken
	}
	return &progress{ctx: ctx, server: srv, token: token, total: total}
}

func (p *progress) step(message string) {
	p.done++
	if p.token == nil || p.server == nil {
		return
	}
	_ = p.server.SendNotificationToClient(p.ctx, "notifications/progress", map[string]any{
		"progressToken": p.token,
		"progress":      p.done,
		"total":         p.total,
		"message":       message,
	})
}

func structured(value map[string]any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultStructuredOnly(value), nil
}

func findRepoRoot() (string, error) {
	if root := os.Getenv("DOLLARLINT_REPO_ROOT"); root != "" {
		return filepath.Abs(root)
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("find repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func schemaObject(properties map[string]any) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
}

func enumSchema(values []string, description string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

func arrayStringSchema(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "string"}}
}

func mustRawJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func lines(text string) []string {
	raw := strings.Split(strings.TrimSpace(text), "\n")
	if len(raw) == 1 && raw[0] == "" {
		return nil
	}
	return raw
}

func existingPaths(root string, paths []string) []string {
	var existing []string
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			existing = append(existing, path)
		}
	}
	return existing
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func hasDirtyStatus(status string) bool {
	for _, line := range lines(status) {
		if strings.HasPrefix(line, "## ") {
			continue
		}
		return true
	}
	return false
}

func truncate(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return text[:max] + "\n... truncated ..."
}

func cleanRelativePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative to the repo root")
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("path must stay inside the repo")
	}
	return clean, nil
}

func classifyError(message string) string {
	switch {
	case strings.Contains(message, "compile schema"):
		return "schema_compile"
	case strings.Contains(message, "fetch") || strings.Contains(message, "http"):
		return "schema_fetch"
	case strings.Contains(message, "catalog"):
		return "catalog"
	case strings.Contains(message, "config"):
		return "config"
	default:
		return "operational"
	}
}

func classifyResult(result dollarlint.Result) map[string]any {
	counts := map[string]int{}
	for _, file := range result.Files {
		counts[string(file.Status)]++
	}
	return map[string]any{
		"statuses":  counts,
		"hasIssues": result.HasIssues(),
		"warnings":  len(result.Warnings),
	}
}

func resultIssueDigest(result dollarlint.Result, limit int) []map[string]any {
	var digest []map[string]any
	for _, issue := range result.Issues {
		digest = append(digest, map[string]any{
			"file":             issue.RelativePath,
			"instanceLocation": issue.InstanceLocation,
			"keyword":          issue.Keyword,
			"message":          issue.Message,
			"line":             issue.Line,
			"column":           issue.Column,
			"property":         issue.Property,
		})
		if len(digest) >= limit {
			return digest
		}
	}
	return digest
}

type armRef struct {
	API        string `json:"apiVersion"`
	Type       string `json:"type"`
	Provider   string `json:"provider"`
	Definition string `json:"definition"`
}

func collectARMRefs(template any) []armRef {
	seen := map[string]bool{}
	refs := collectARMRefsFromTemplate(template, seen, nil)
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Type < refs[j].Type
	})
	return refs
}

func collectARMRefsFromTemplate(template any, seen map[string]bool, refs []armRef) []armRef {
	object, ok := template.(map[string]any)
	if !ok {
		return refs
	}
	resources, ok := object["resources"].([]any)
	if !ok {
		return refs
	}
	for _, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := armRefFromResource(resource)
		if ok && !seen[ref.API+"/"+ref.Type] {
			seen[ref.API+"/"+ref.Type] = true
			refs = append(refs, ref)
		}
		if properties, ok := resource["properties"].(map[string]any); ok {
			refs = collectARMRefsFromTemplate(properties["template"], seen, refs)
		}
	}
	return refs
}

func armRefFromResource(resource map[string]any) (armRef, bool) {
	rawType, ok := resource["type"].(string)
	if !ok || strings.TrimSpace(rawType) == "" || strings.HasPrefix(strings.TrimSpace(rawType), "[") {
		return armRef{}, false
	}
	apiVersion, ok := resource["apiVersion"].(string)
	if !ok || strings.TrimSpace(apiVersion) == "" || strings.HasPrefix(strings.TrimSpace(apiVersion), "[") {
		return armRef{}, false
	}
	parts := strings.Split(strings.TrimSpace(rawType), "/")
	if len(parts) < 2 || parts[0] == "" {
		return armRef{}, false
	}
	return armRef{
		API:        strings.TrimSpace(apiVersion),
		Type:       strings.TrimSpace(rawType),
		Provider:   parts[0],
		Definition: strings.Join(parts[1:], "_"),
	}, true
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
