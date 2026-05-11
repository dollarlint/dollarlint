package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	actionlintCommand           = "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"
	actionlintCIWorkflowCommand = `bash -lc '
set -euo pipefail
workflows=()
while IFS= read -r workflow; do
  workflows+=("$workflow")
done < <(find .github/workflows -maxdepth 1 -type f \( -name "*.yml" -o -name "*.yaml" \) ! -name "*.lock.yml" | sort)
if [ "${#workflows[@]}" -gt 0 ]; then
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 "${workflows[@]}"
fi
lock_workflows=()
while IFS= read -r workflow; do
  lock_workflows+=("$workflow")
done < <(find .github/workflows -maxdepth 1 -type f -name "*.lock.yml" | sort)
if [ "${#lock_workflows[@]}" -gt 0 ]; then
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= "${lock_workflows[@]}"
fi
'`
	staticcheckCommand          = "go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./..."
	govulncheckCommand          = "go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./..."
	repositoryConfigLintCommand = "go run ./cmd/dollarlint validate . --exclude .goreleaser.yaml"
)

var ciJobOrder = []string{"test", "quality", "build", "docs", "goreleaser-check"}

var workflowAllMCPToolsPattern = regexp.MustCompile(`"tools"\s*:\s*\[\s*"\*"\s*\]`)

const (
	agenticWorkflowSourceRel            = ".github/workflows/agentic-product-testing.md"
	agenticWorkflowLockRel              = ".github/workflows/agentic-product-testing.lock.yml"
	agenticOutputLinkerWorkflowRel      = ".github/workflows/link-agentic-product-testing-outputs.yml"
	agenticWorkflowReadinessDescription = "Validate the Agentic Product Testing workflow before pushing, including actionlint, MCP allowlist, generated lock freshness, PR publishing credentials, and known-bad model settings."
)

func ciReadinessCommands(job string) ([]namedCommand, error) {
	if job == "" {
		job = "all"
	}
	add := func(commands *[]namedCommand, jobName string, items ...namedCommand) {
		for _, item := range items {
			item.Job = jobName
			*commands = append(*commands, item)
		}
	}
	coverageCommand := `go test -coverprofile=coverage.out ./internal/engine
go tool cover -func=coverage.out
coverage="$(go tool cover -func=coverage.out | awk '/^total:/ { sub(/%/, "", $3); print $3 }')"
awk -v coverage="$coverage" -v minimum="90.0" 'BEGIN {
  if (coverage + 0 < minimum) {
    printf "coverage %.1f%% is below required %.1f%%\n", coverage, minimum
    exit 1
  }
}'`
	gofmtCommand := `unformatted="$(find . -name '*.go' -not -path './vendor/*' -print0 | xargs -0 gofmt -l)"
if [ -n "$unformatted" ]; then
  echo "Go files need gofmt:"
  echo "$unformatted"
  exit 1
fi`
	var commands []namedCommand
	for _, jobName := range ciJobOrder {
		if job != "all" && job != jobName {
			continue
		}
		switch jobName {
		case "test":
			add(&commands, jobName,
				namedCommand{Name: "verify modules", Cmd: "go mod verify", FailureHint: "Run go mod tidy or inspect module/cache integrity before pushing."},
				namedCommand{Name: "test", Cmd: "go test ./...", FailureHint: "Run the failing package locally and add or update focused tests."},
				namedCommand{Name: "core coverage", Cmd: coverageCommand, FailureHint: "Add internal/engine coverage or adjust the CI threshold deliberately."},
			)
		case "quality":
			add(&commands, jobName,
				namedCommand{Name: "check go formatting", Cmd: gofmtCommand, FailureHint: "Run gofmt on the listed files."},
				namedCommand{Name: "vet", Cmd: "go vet ./...", FailureHint: "Fix go vet diagnostics before pushing."},
				namedCommand{Name: "staticcheck", Cmd: staticcheckCommand, FailureHint: "Fix staticcheck diagnostics such as unused helpers or ineffective code."},
				namedCommand{Name: "vulnerability scan", Cmd: govulncheckCommand, FailureHint: "Update affected dependencies or document why the finding is not reachable."},
				namedCommand{Name: "lint github actions workflows", Cmd: actionlintCIWorkflowCommand, FailureHint: "Fix actionlint diagnostics in hand-authored workflows; generated *.lock.yml files are linted with shellcheck disabled."},
				namedCommand{Name: "validate repository configs", Cmd: repositoryConfigLintCommand, FailureHint: "Fix DollarLint findings in repo config files or update the repo config intentionally."},
			)
		case "build":
			add(&commands, jobName,
				namedCommand{Name: "build", Cmd: "go build ./...", FailureHint: "Fix compile errors across packages."},
			)
		case "docs":
			add(&commands, jobName,
				namedCommand{Name: "install docs dependencies", Cmd: "npm ci", Dir: "docs", FailureHint: "Refresh docs/package-lock.json or fix docs dependency installation."},
				namedCommand{Name: "check docs formatting", Cmd: "npm run format:check", Dir: "docs", FailureHint: "Run npm run format in docs or edit the listed files."},
				namedCommand{Name: "audit docs dependencies", Cmd: "npm run audit", Dir: "docs", FailureHint: "Resolve high-severity docs dependency advisories."},
				namedCommand{Name: "build docs", Cmd: "npm run build", Dir: "docs", FailureHint: "Fix Astro/type/build diagnostics."},
			)
		case "goreleaser-check":
			add(&commands, jobName,
				namedCommand{Name: "check goreleaser config", Cmd: goreleaserCheckCommand, OptionalEnv: "GORELEASER_KEY", FailureHint: "Set GORELEASER_KEY to reproduce CI's GoReleaser Pro check locally, then fix .goreleaser.yaml diagnostics."},
			)
		}
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("unknown CI readiness job %q", job)
	}
	return commands, nil
}

func (s *repoServer) handleCIReadiness(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Job string `json:"job"`
	}
	_ = request.BindArguments(&args)
	if args.Job == "" {
		args.Job = "all"
	}
	commands, err := ciReadinessCommands(args.Job)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result, err := s.runCommandSetData(ctx, request, "ci-readiness:"+args.Job, commands)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if commandResults, ok := result["commands"].([]commandResult); ok {
		result["jobs"] = groupCommandResultsByJob(commandResults)
		result["failedSteps"] = failedStepSummaries(commandResults)
	}
	result["ciWorkflow"] = ".github/workflows/ci.yml"
	result["nextStep"] = "Fix failedSteps locally before pushing; use ci_failure_diagnose with a GitHub run id if remote CI still disagrees."
	return structured(result)
}

func groupCommandResultsByJob(results []commandResult) []map[string]any {
	byJob := map[string][]commandResult{}
	for _, result := range results {
		job := result.Job
		if job == "" {
			job = "default"
		}
		byJob[job] = append(byJob[job], result)
	}
	grouped := make([]map[string]any, 0, len(byJob))
	seen := map[string]bool{}
	for _, job := range ciJobOrder {
		if results, ok := byJob[job]; ok {
			grouped = append(grouped, commandJobSummary(job, results))
			seen[job] = true
		}
	}
	for job, results := range byJob {
		if seen[job] {
			continue
		}
		grouped = append(grouped, commandJobSummary(job, results))
	}
	return grouped
}

func commandJobSummary(job string, results []commandResult) map[string]any {
	ok := true
	for _, result := range results {
		if !result.Succeeded {
			ok = false
			break
		}
	}
	return map[string]any{"job": job, "ok": ok, "steps": results}
}

func failedStepSummaries(results []commandResult) []map[string]any {
	var failed []map[string]any
	for _, result := range results {
		if result.Succeeded {
			continue
		}
		failed = append(failed, map[string]any{
			"job":         result.Job,
			"name":        result.Name,
			"command":     result.Command,
			"exitCode":    result.ExitCode,
			"failureHint": result.FailureHint,
			"output":      result.Output,
		})
	}
	return failed
}

type readinessIssue struct {
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	Recommendation string `json:"recommendation,omitempty"`
}

func (s *repoServer) handleAgenticWorkflowReadiness(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := newProgress(ctx, s.mcp, request, 7)
	sourceRel := agenticWorkflowSourceRel
	lockRel := agenticWorkflowLockRel
	linkerRel := agenticOutputLinkerWorkflowRel
	sourcePath := filepath.Join(s.root, sourceRel)
	lockPath := filepath.Join(s.root, lockRel)
	linkerPath := filepath.Join(s.root, linkerRel)
	var issues []readinessIssue
	var checks []map[string]any

	p.step("Checking workflow files")
	source, sourceErr := os.ReadFile(sourcePath)
	lock, lockErr := os.ReadFile(lockPath)
	linker, linkerErr := os.ReadFile(linkerPath)
	if sourceErr != nil {
		issues = append(issues, readinessIssue{Severity: "error", Message: sourceErr.Error(), Recommendation: "Restore " + sourceRel + "."})
	}
	if lockErr != nil {
		issues = append(issues, readinessIssue{Severity: "error", Message: lockErr.Error(), Recommendation: "Regenerate " + lockRel + "."})
	}
	if linkerErr != nil {
		issues = append(issues, readinessIssue{Severity: "error", Message: linkerErr.Error(), Recommendation: "Restore " + linkerRel + "."})
	}
	checks = append(checks, map[string]any{"name": "workflow files", "ok": sourceErr == nil && lockErr == nil && linkerErr == nil})

	p.step("Checking real-world MCP allowlist")
	requiredTools := requiredAgenticRealWorldTools()
	missing := missingWorkflowTools(string(source), string(lock), requiredTools)
	if len(missing) > 0 {
		issues = append(issues, readinessIssue{Severity: "error", Message: "Agentic Product Testing workflow is missing real-world MCP tools: " + strings.Join(missing, ", "), Recommendation: "Add the missing tools to the workflow MCP server allowlist, or set " + toolFilterEnv + `: "real_world_*"` + ` and allow "*" for the filtered server, then regenerate the lock file.`})
	}
	checks = append(checks, map[string]any{"name": "real-world MCP allowlist", "ok": len(missing) == 0, "requiredTools": requiredTools, "missing": missing})

	p.step("Checking real-world safe outputs")
	safeOutputIssues := realWorldSafeOutputPolicyIssues(string(source), string(lock), string(linker))
	issues = append(issues, safeOutputIssues...)
	checks = append(checks, map[string]any{"name": "real-world safe outputs", "ok": len(safeOutputIssues) == 0})

	p.step("Checking PR creation credentials")
	credentialIssues, credentialCheck := s.agenticWorkflowPRCredentialReadiness(ctx)
	issues = append(issues, credentialIssues...)
	checks = append(checks, credentialCheck)

	p.step("Checking generated lock freshness")
	fresh := true
	if sourceInfo, err := os.Stat(sourcePath); err == nil {
		if lockInfo, err := os.Stat(lockPath); err == nil && sourceInfo.ModTime().After(lockInfo.ModTime().Add(time.Second)) {
			fresh = false
			issues = append(issues, readinessIssue{Severity: "warning", Message: sourceRel + " is newer than " + lockRel, Recommendation: "Regenerate the agentic workflow lock file before pushing."})
		}
	}
	checks = append(checks, map[string]any{"name": "lock freshness", "ok": fresh})

	p.step("Running actionlint on agentic workflow")
	actionlint := s.run(ctx, namedCommand{
		Job:         "agentic-workflow",
		Name:        "actionlint Agentic Product Testing workflow",
		Cmd:         actionlintCommand + " -shellcheck= " + lockRel + " " + linkerRel,
		FailureHint: "Fix actionlint diagnostics in " + lockRel + " or " + linkerRel + ", or regenerate the lock file.",
	})
	if !actionlint.Succeeded {
		issues = append(issues, readinessIssue{Severity: "error", Message: "actionlint failed for " + lockRel, Recommendation: actionlint.FailureHint})
	}
	checks = append(checks, map[string]any{"name": "actionlint", "ok": actionlint.Succeeded, "result": actionlint})

	p.step("Checking Copilot model configuration")
	modelIssues, modelChecks := s.agenticWorkflowModelReadiness(ctx, string(source), string(lock))
	issues = append(issues, modelIssues...)
	checks = append(checks, modelChecks...)

	return structured(map[string]any{
		"ok":                  !hasReadinessErrors(issues),
		"sourceWorkflow":      sourceRel,
		"generatedWorkflow":   lockRel,
		"linkerWorkflow":      linkerRel,
		"checks":              checks,
		"issues":              issues,
		"recentFailureSignal": "A recent Agentic Product Testing workflow run failed when COPILOT_MODEL was gpt-5.5; this tool flags that value because it was not accessible via Copilot chat completions.",
		"nextStep":            "Fix error-severity issues before running the Agentic Product Testing workflow. Warnings should be reviewed before push.",
	})
}

func (s *repoServer) agenticWorkflowPRCredentialReadiness(ctx context.Context) ([]readinessIssue, map[string]any) {
	secretResult := s.run(ctx, namedCommand{Name: "read gh-aw secrets", Cmd: "gh secret list --repo dollarlint/dollarlint --json name"})
	permissionResult := s.run(ctx, namedCommand{Name: "read github actions workflow permissions", Cmd: "gh api repos/dollarlint/dollarlint/actions/permissions/workflow --jq ."})
	return realWorldPRCredentialIssues(secretResult.Succeeded, secretResult.Output, permissionResult.Succeeded, permissionResult.Output, secretResult, permissionResult)
}

type ghSecretName struct {
	Name string `json:"name"`
}

type githubWorkflowPermissions struct {
	CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
	DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
}

func realWorldPRCredentialIssues(secretOK bool, secretOutput string, permissionOK bool, permissionOutput string, secretResult, permissionResult commandResult) ([]readinessIssue, map[string]any) {
	check := map[string]any{"name": "PR creation credentials"}
	var issues []readinessIssue
	hasWriteSecret := false
	if secretOK {
		secrets, err := parseGHSecretNames(secretOutput)
		if err != nil {
			check["secretWarning"] = "Could not parse gh secret list output: " + err.Error()
		} else {
			for _, secret := range secrets {
				if secret == "GH_AW_GITHUB_TOKEN" {
					hasWriteSecret = true
					break
				}
			}
		}
	} else {
		check["secretWarning"] = "Could not read repository secrets; skipping GH_AW_GITHUB_TOKEN presence check."
		check["secretResult"] = secretResult
	}

	defaultTokenCanCreatePR := false
	var permissions githubWorkflowPermissions
	if permissionOK {
		parsed, err := parseGitHubWorkflowPermissions(permissionOutput)
		if err != nil {
			check["permissionWarning"] = "Could not parse GitHub Actions workflow permissions: " + err.Error()
		} else {
			permissions = parsed
			defaultTokenCanCreatePR = permissions.CanApprovePullRequestReviews
		}
	} else {
		check["permissionWarning"] = "Could not read GitHub Actions workflow permissions; skipping GITHUB_TOKEN PR creation check."
		check["permissionResult"] = permissionResult
	}

	check["hasGH_AW_GITHUB_TOKEN"] = hasWriteSecret
	check["defaultTokenCanCreatePullRequests"] = defaultTokenCanCreatePR
	if permissions.DefaultWorkflowPermissions != "" {
		check["defaultWorkflowPermissions"] = permissions.DefaultWorkflowPermissions
	}
	if permissionOK {
		check["canApprovePullRequestReviews"] = permissions.CanApprovePullRequestReviews
	}

	if secretOK && permissionOK && !hasWriteSecret && !defaultTokenCanCreatePR {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow cannot create the required durable-memory PR with current credentials",
			Recommendation: "Set GH_AW_GITHUB_TOKEN with a fine-grained PAT that has Contents, Pull requests, Issues, and Discussions read/write, or enable the repository Actions setting that allows GitHub Actions to create and approve pull requests.",
		})
	}
	check["ok"] = len(issues) == 0
	return issues, check
}

func parseGHSecretNames(output string) ([]string, error) {
	var secrets []ghSecretName
	if err := json.Unmarshal([]byte(output), &secrets); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		names = append(names, secret.Name)
	}
	return names, nil
}

func parseGitHubWorkflowPermissions(output string) (githubWorkflowPermissions, error) {
	var permissions githubWorkflowPermissions
	if err := json.Unmarshal([]byte(output), &permissions); err != nil {
		return githubWorkflowPermissions{}, err
	}
	return permissions, nil
}

func requiredAgenticRealWorldTools() []string {
	return []string{
		"real_world_start_testing",
		"real_world_update_candidates",
		"real_world_history",
		"real_world_artifact_query",
		"real_world_prepare_corpus",
		"real_world_next_prepared_repo",
		"real_world_prepare_status",
		"real_world_cancel_prepare",
		"real_world_inspect_corpus",
		"real_world_start_validation",
		"real_world_next_validation_result",
		"real_world_record_validation_feedback",
		"real_world_validation_status",
		"real_world_finish_validation",
		"real_world_cancel_validation",
		"real_world_triage_output",
		"real_world_recommendation_backlog",
		"real_world_record_result",
	}
}

func missingWorkflowTools(source, lock string, required []string) []string {
	if hasServerSideRealWorldToolFilter(source, lock) {
		return nil
	}
	var missing []string
	for _, tool := range required {
		if !strings.Contains(source, tool) || !strings.Contains(lock, tool) {
			missing = append(missing, tool)
		}
	}
	return missing
}

func hasServerSideRealWorldToolFilter(source, lock string) bool {
	sourceHasFilter := strings.Contains(source, toolFilterEnv) && strings.Contains(source, "real_world_*")
	lockHasFilter := strings.Contains(lock, toolFilterEnv) && strings.Contains(lock, "real_world_*")
	sourceAllowsAll := strings.Contains(source, `- "*"`) || strings.Contains(source, `- '*'`) || strings.Contains(source, "- *")
	lockAllowsAll := workflowAllMCPToolsPattern.MatchString(lock) ||
		strings.Contains(lock, "dollarlint-repo(*)") ||
		strings.Contains(lock, "--allow-tool dollarlint-repo --")
	return sourceHasFilter && lockHasFilter && sourceAllowsAll && lockAllowsAll
}

func realWorldSafeOutputPolicyIssues(source, lock string, linkerSources ...string) []readinessIssue {
	var issues []readinessIssue
	linker := ""
	if len(linkerSources) > 0 {
		linker = linkerSources[0]
	}
	if !strings.Contains(source, "if-no-changes: error") || !strings.Contains(lock, `"if_no_changes":"error"`) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "real-world create_pull_request safe output is not configured to fail on missing patch changes",
			Recommendation: "Set create-pull-request.if-no-changes: error and regenerate the lock file.",
		})
	}
	hasCustomLinker := strings.Contains(source, "link-outputs") && strings.Contains(lock, "link_outputs")
	hasFollowupLinker := strings.Contains(linker, "workflow_run:") &&
		strings.Contains(linker, `workflows: ["Agentic Product Testing"]`) &&
		strings.Contains(linker, "pull-requests: write") &&
		strings.Contains(linker, "discussions: write") &&
		strings.Contains(linker, "real-world-output-links:start")
	if !hasCustomLinker && !hasFollowupLinker {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow is missing the post-safe-output PR/Discussion linker",
			Recommendation: "Configure the deterministic link-agentic-product-testing-outputs workflow, or restore a safe-outputs linker job and regenerate the lock file.",
		})
	}
	if hasFollowupLinker && (!strings.Contains(source, "PR body") || !strings.Contains(source, "workflow run URL")) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow does not require enough linking metadata in the PR body",
			Recommendation: "Require the agent to include the workflow run URL in the durable-memory PR body so the deterministic linker can find it.",
		})
	}
	if !strings.Contains(source, "patch artifact") || !strings.Contains(source, "Do not call `report_incomplete`") {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow does not explain asynchronous safe-output PR creation",
			Recommendation: "Tell the agent that create_pull_request may return a patch artifact first, that this is expected, and that report_incomplete is not appropriate when the later safe_outputs job will create the PR.",
		})
	}
	if !strings.Contains(source, "Fail incomplete Agentic Product Testing run") || !strings.Contains(source, "report_incomplete") ||
		!strings.Contains(lock, "Fail incomplete Agentic Product Testing run") || !strings.Contains(lock, "report_incomplete") {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow does not fail when the agent reports an incomplete run",
			Recommendation: "Add a source-level post-step that checks GH_AW_SAFE_OUTPUTS for report_incomplete and exits non-zero, then regenerate the lock file.",
		})
	}
	if hasCustomLinker && !strings.Contains(lock, "created_pr_url") {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "real-world linker cannot see the created PR URL in the generated workflow",
			Recommendation: "Regenerate the lock file and confirm safe_outputs exposes created_pr_url.",
		})
	}
	if strings.Contains(lock, "shell(safeoutputs:*") && (!strings.Contains(source, "\n    - printf") || !strings.Contains(lock, "shell(printf)")) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow exposes the safeoutputs CLI but printf is not allowed",
			Recommendation: "Add printf to tools.bash so the agent can pipe inline JSON payloads to safeoutputs, then regenerate the lock file.",
		})
	}
	if !strings.Contains(source, "github.event.inputs.max_repos") || !strings.Contains(source, "github.event.inputs.candidate_repos") {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow prompt does not include rendered manual dispatch inputs",
			Recommendation: "Mention github.event.inputs.max_repos and github.event.inputs.candidate_repos in the markdown prompt so manual dispatch controls the MCP repository plan.",
		})
	}
	if !strings.Contains(source, "should be merged in order to retain") {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow does not require the Discussion to explain that the PR retains the results",
			Recommendation: `Ask the agent to include a "Durable memory PR" Discussion section saying that the PR should be merged in order to retain the results, then regenerate the lock file.`,
		})
	}
	if !strings.Contains(source, "category: agentic-product-testing") || !strings.Contains(lock, `"category":"agentic-product-testing"`) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow Discussions are not configured for the Agentic Product Testing category",
			Recommendation: `Set create-discussion.category to "agentic-product-testing" and regenerate the lock file.`,
		})
	}
	if !strings.Contains(source, "@agorischek") || !strings.Contains(source, "allowed: [agorischek]") || !strings.Contains(lock, `"allowed":["agorischek"]`) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow does not require and allow the owner Discussion mention",
			Recommendation: "Ask the agent to include @agorischek in the Discussion body, allow that mention in safe-outputs.mentions, and regenerate the lock file.",
		})
	}
	if !strings.Contains(source, "allowed-files:") || !strings.Contains(source, "reports/agentic-product-testing/**") || !strings.Contains(lock, `"allowed_files":["reports/agentic-product-testing/**"]`) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow does not restrict durable-memory PRs to the managed report directory",
			Recommendation: `Set create-pull-request.allowed-files to ["reports/agentic-product-testing/**"] and regenerate the lock file.`,
		})
	}
	if !strings.Contains(source, "max-patch-size: 8192") || !strings.Contains(lock, `"max_patch_size":8192`) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow PR patch limit is too small for structured report artifacts",
			Recommendation: "Set safe-outputs.max-patch-size: 8192 and regenerate the lock file.",
		})
	}
	if exposesPackageManagerShell(source, lock) {
		issues = append(issues, readinessIssue{
			Severity:       "error",
			Message:        "Agentic Product Testing workflow exposes package-manager shell commands",
			Recommendation: "Remove package-manager commands from tools.bash; dependency prep should stay behind the real_world_* MCP flow and be recorded as skipped or needs-review when unsafe.",
		})
	}
	return issues
}

func exposesPackageManagerShell(source, lock string) bool {
	commands := []string{"npm", "pnpm", "yarn", "bun", "composer", "cargo", "pip", "pipenv", "poetry", "bundle", "gradle", "mvn", "dotnet", "terraform"}
	for _, shellCommand := range workflowSourceBashCommands(source) {
		fields := strings.Fields(shellCommand)
		if len(fields) == 0 {
			continue
		}
		first := strings.Trim(fields[0], `"'`)
		for _, command := range commands {
			if first == command || strings.HasPrefix(first, command+":") {
				return true
			}
		}
	}
	for _, command := range commands {
		if strings.Contains(lock, `"shell(`+command) || strings.Contains(lock, `'shell(`+command) {
			return true
		}
	}
	return false
}

func workflowSourceBashCommands(source string) []string {
	var commands []string
	inTools := false
	inBash := false
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !inTools {
			if trimmed == "tools:" {
				inTools = true
			}
			continue
		}
		if line == trimmed {
			inTools = false
			inBash = false
			if trimmed == "tools:" {
				inTools = true
			}
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			inBash = trimmed == "bash:"
			continue
		}
		if !inBash {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			commands = append(commands, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	return commands
}

func hasReadinessErrors(issues []readinessIssue) bool {
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

func (s *repoServer) agenticWorkflowModelReadiness(ctx context.Context, source, lock string) ([]readinessIssue, []map[string]any) {
	var issues []readinessIssue
	checks := []map[string]any{}
	localModels := collectAgenticModelValues("workflow defaults", source+"\n"+lock)
	for label, model := range localModels {
		if issue, ok := inaccessibleCopilotModelIssue(label, model); ok {
			issues = append(issues, issue)
		}
	}
	checks = append(checks, map[string]any{"name": "local model defaults", "ok": !hasReadinessErrors(issues), "models": localModels})

	variableResult := s.run(ctx, namedCommand{Name: "read github variables", Cmd: "gh variable list --repo dollarlint/dollarlint --json name,value"})
	if !variableResult.Succeeded {
		checks = append(checks, map[string]any{"name": "github model variables", "ok": true, "warning": "Could not read GitHub variables; skipping hosted variable model check.", "result": variableResult})
		return issues, checks
	}
	var variables []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(variableResult.Output), &variables); err != nil {
		checks = append(checks, map[string]any{"name": "github model variables", "ok": true, "warning": "Could not parse GitHub variable output: " + err.Error()})
		return issues, checks
	}
	var modelVars []map[string]string
	for _, variable := range variables {
		if !strings.HasPrefix(variable.Name, "GH_AW_MODEL_") {
			continue
		}
		modelVars = append(modelVars, map[string]string{"name": variable.Name, "value": variable.Value})
		if issue, ok := inaccessibleCopilotModelIssue("GitHub variable "+variable.Name, variable.Value); ok {
			issues = append(issues, issue)
		}
	}
	checks = append(checks, map[string]any{"name": "github model variables", "ok": !hasReadinessErrors(issues), "variables": modelVars})
	return issues, checks
}

func collectAgenticModelValues(label, text string) map[string]string {
	values := map[string]string{}
	pattern := regexp.MustCompile(`GH_AW_MODEL_([A-Z_]+)_COPILOT\s*\|\|\s*'([^']+)'`)
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		values[label+" "+match[1]] = match[2]
	}
	return values
}

func inaccessibleCopilotModelIssue(label, model string) (readinessIssue, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "gpt-5.5" || normalized == "openai/gpt-5.5" || normalized == "copilot/gpt-5.5" {
		return readinessIssue{
			Severity:       "error",
			Message:        label + " is set to " + model + ", which recently failed with: model \"gpt-5.5\" is not accessible via the /chat/completions endpoint",
			Recommendation: "Use a Copilot-accessible model alias such as claude-sonnet-4.6, or verify the model through a tiny workflow before running the real-world sweep.",
		}, true
	}
	return readinessIssue{}, false
}

type ghWorkflowRun struct {
	Conclusion   string `json:"conclusion"`
	CreatedAt    string `json:"createdAt"`
	DatabaseID   int64  `json:"databaseId"`
	DisplayTitle string `json:"displayTitle"`
	HeadBranch   string `json:"headBranch"`
	HeadSHA      string `json:"headSha"`
	Status       string `json:"status"`
	UpdatedAt    string `json:"updatedAt"`
	URL          string `json:"url"`
	WorkflowName string `json:"workflowName"`
}

func (s *repoServer) handleCIFailureDiagnose(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		RunID string `json:"runID"`
		Limit int    `json:"limit"`
	}
	_ = request.BindArguments(&args)
	if args.RunID != "" {
		diagnosis, err := s.diagnoseGitHubRun(ctx, args.RunID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return structured(map[string]any{"ok": diagnosis["ok"], "runs": []map[string]any{diagnosis}})
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 30 {
		limit = 30
	}
	output, err := commandOutput(ctx, s.root, "gh", "run", "list", "--limit", strconv.Itoa(limit), "--json", "databaseId,displayTitle,workflowName,status,conclusion,createdAt,updatedAt,url,headBranch,headSha")
	if err != nil {
		return mcp.NewToolResultError("gh run list: " + err.Error()), nil
	}
	var runs []ghWorkflowRun
	if err := json.Unmarshal(output, &runs); err != nil {
		return mcp.NewToolResultError("parse gh run list: " + err.Error()), nil
	}
	diagnosed := []map[string]any{}
	for _, run := range runs {
		if run.Conclusion != "failure" {
			continue
		}
		diagnosis, err := s.diagnoseGitHubRun(ctx, strconv.FormatInt(run.DatabaseID, 10))
		if err != nil {
			diagnosis = map[string]any{"ok": false, "runID": run.DatabaseID, "error": err.Error(), "run": run}
		}
		diagnosed = append(diagnosed, diagnosis)
		if len(diagnosed) >= 5 {
			break
		}
	}
	return structured(map[string]any{
		"ok":       len(diagnosed) == 0,
		"runs":     diagnosed,
		"nextStep": "Run the recommended local MCP tool/profile for each failure before pushing again.",
	})
}

func (s *repoServer) diagnoseGitHubRun(ctx context.Context, runID string) (map[string]any, error) {
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(runID) {
		return nil, fmt.Errorf("runID must be a GitHub Actions numeric run id")
	}
	meta, err := commandOutput(ctx, s.root, "gh", "run", "view", runID, "--json", "name,workflowName,conclusion,status,url,headSha,jobs")
	if err != nil {
		return nil, fmt.Errorf("gh run view %s: %w", runID, err)
	}
	var metaValue map[string]any
	if err := json.Unmarshal(meta, &metaValue); err != nil {
		return nil, fmt.Errorf("parse gh run view %s: %w", runID, err)
	}
	logOutput, err := commandOutput(ctx, s.root, "gh", "run", "view", runID, "--log-failed")
	if err != nil {
		return map[string]any{"ok": false, "runID": runID, "metadata": metaValue, "error": "gh run logs: " + err.Error()}, nil
	}
	mappings := diagnoseFailureLog(string(logOutput))
	return map[string]any{
		"ok":                     false,
		"runID":                  runID,
		"metadata":               metaValue,
		"recommendedLocalChecks": mappings,
		"logExcerpt":             failureLogExcerpt(string(logOutput), 80),
	}, nil
}

type failureMapping struct {
	Signal       string `json:"signal"`
	LocalTool    string `json:"localTool"`
	LocalCommand string `json:"localCommand"`
	Rationale    string `json:"rationale"`
}

func diagnoseFailureLog(logText string) []failureMapping {
	lower := strings.ToLower(logText)
	seen := map[string]bool{}
	var mappings []failureMapping
	appendMapping := func(mapping failureMapping) {
		key := mapping.LocalTool + "\x00" + mapping.LocalCommand
		if seen[key] {
			return
		}
		seen[key] = true
		mappings = append(mappings, mapping)
	}
	if strings.Contains(lower, "npm run format:check") || strings.Contains(lower, "prettier --check") || strings.Contains(lower, "code style issues found") {
		appendMapping(failureMapping{Signal: "docs formatting", LocalTool: "ci_readiness", LocalCommand: `{"job":"docs"}`, Rationale: "The docs CI job failed during Prettier format checking."})
	}
	if strings.Contains(lower, "staticcheck") || strings.Contains(lower, "u1000") {
		appendMapping(failureMapping{Signal: "staticcheck", LocalTool: "ci_readiness", LocalCommand: `{"job":"quality"}`, Rationale: "The quality job failed during staticcheck."})
	}
	if strings.Contains(lower, "actionlint") || strings.Contains(lower, "shellcheck") {
		appendMapping(failureMapping{Signal: "workflow lint", LocalTool: "agentic_workflow_readiness", LocalCommand: `{}`, Rationale: "Workflow shell/actionlint diagnostics should be checked before running CI."})
		appendMapping(failureMapping{Signal: "workflow lint", LocalTool: "ci_readiness", LocalCommand: `{"job":"quality"}`, Rationale: "The CI quality job also runs actionlint across workflows."})
	}
	if strings.Contains(lower, "go test ./...") || strings.Contains(lower, "coverage") && strings.Contains(lower, "below required") {
		appendMapping(failureMapping{Signal: "go tests or coverage", LocalTool: "ci_readiness", LocalCommand: `{"job":"test"}`, Rationale: "The test job failed in Go tests or coverage enforcement."})
	}
	if strings.Contains(lower, "go build ./...") {
		appendMapping(failureMapping{Signal: "go build", LocalTool: "ci_readiness", LocalCommand: `{"job":"build"}`, Rationale: "The build job failed compiling Go packages."})
	}
	if strings.Contains(lower, "dollarlint validate . --exclude .goreleaser.yaml") {
		appendMapping(failureMapping{Signal: "repository config validation", LocalTool: "ci_readiness", LocalCommand: `{"job":"quality"}`, Rationale: "The quality job failed validating repository config files with DollarLint."})
	}
	if strings.Contains(lower, "goreleaser") {
		appendMapping(failureMapping{Signal: "goreleaser", LocalTool: "ci_readiness", LocalCommand: `{"job":"goreleaser-check"}`, Rationale: "The GoReleaser check job failed."})
	}
	if strings.Contains(lower, "model \"gpt-5.5\" is not accessible") || strings.Contains(lower, "model_not_supported") {
		appendMapping(failureMapping{Signal: "unsupported agent model", LocalTool: "agentic_workflow_readiness", LocalCommand: `{}`, Rationale: "The agentic workflow model configuration is known to fail before the sweep starts."})
	}
	if len(mappings) == 0 {
		appendMapping(failureMapping{Signal: "unclassified", LocalTool: "ci_readiness", LocalCommand: `{"job":"all"}`, Rationale: "No known failure signature matched; run the full local CI readiness profile."})
	}
	return mappings
}

func failureLogExcerpt(logText string, maxLines int) []string {
	var excerpt []string
	for _, line := range strings.Split(logText, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "##[error]") ||
			strings.Contains(lower, "error") ||
			strings.Contains(lower, "failed") ||
			strings.Contains(lower, "shellcheck") ||
			strings.Contains(lower, "staticcheck") ||
			strings.Contains(lower, "prettier") {
			excerpt = append(excerpt, line)
			if len(excerpt) >= maxLines {
				break
			}
		}
	}
	if len(excerpt) == 0 {
		return lines(truncate(logText, 6000))
	}
	return excerpt
}

func commandOutput(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err != nil {
		return output.Bytes(), fmt.Errorf("%w: %s", err, truncate(output.String(), 4000))
	}
	return output.Bytes(), nil
}
