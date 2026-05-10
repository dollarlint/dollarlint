package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIReadinessCommandsMirrorCriticalCISteps(t *testing.T) {
	commands, err := ciReadinessCommands("all")
	if err != nil {
		t.Fatalf("ci readiness commands: %v", err)
	}
	assertCommand := func(job, name, contains string) {
		t.Helper()
		for _, command := range commands {
			if command.Job == job && command.Name == name {
				if !strings.Contains(command.Cmd, contains) {
					t.Fatalf("%s/%s command = %q, want containing %q", job, name, command.Cmd, contains)
				}
				return
			}
		}
		t.Fatalf("missing command %s/%s", job, name)
	}
	assertCommand("quality", "check go formatting", "gofmt -l")
	assertCommand("quality", "lint github actions workflows", "actionlint")
	assertCommand("quality", "validate repository configs", "--exclude .goreleaser.yaml")
	assertCommand("test", "core coverage", "coverage + 0 < minimum")
	assertCommand("docs", "install docs dependencies", "npm ci")
	assertCommand("goreleaser-check", "check goreleaser config", "goreleaser")
}

func TestDiagnoseFailureLogMapsRecentFailureShapes(t *testing.T) {
	logText := `
docs Check docs formatting > prettier --check .
[warn] src/pages/reference/output.astro
quality Staticcheck tools/repo-mcp/real_world.go:537:22: func is unused (U1000)
quality Lint GitHub Actions workflows shellcheck reported issue SC2016
400 model "gpt-5.5" is not accessible via the /chat/completions endpoint
`
	mappings := diagnoseFailureLog(logText)
	if !hasMapping(mappings, "ci_readiness", `{"job":"docs"}`) {
		t.Fatalf("missing docs mapping: %+v", mappings)
	}
	if !hasMapping(mappings, "ci_readiness", `{"job":"quality"}`) {
		t.Fatalf("missing quality mapping: %+v", mappings)
	}
	if !hasMapping(mappings, "agentic_workflow_readiness", `{}`) {
		t.Fatalf("missing agentic mapping: %+v", mappings)
	}
}

func TestInaccessibleCopilotModelIssueFlagsGPT55(t *testing.T) {
	issue, ok := inaccessibleCopilotModelIssue("GitHub variable GH_AW_MODEL_AGENT_COPILOT", "gpt-5.5")
	if !ok || issue.Severity != "error" || !strings.Contains(issue.Message, "gpt-5.5") {
		t.Fatalf("issue = %+v ok=%v", issue, ok)
	}
	if _, ok := inaccessibleCopilotModelIssue("default", "claude-sonnet-4.6"); ok {
		t.Fatalf("claude default should not be flagged")
	}
}

func TestAgenticWorkflowReadinessUsesCurrentWorkflowNames(t *testing.T) {
	staleScheduleWord := "week" + "ly"
	staleWorkflowName := staleScheduleWord + "-real-world-testing"
	for _, value := range []string{
		agenticWorkflowSourceRel,
		agenticWorkflowLockRel,
		agenticWorkflowReadinessDescription,
	} {
		if strings.Contains(value, staleScheduleWord) || strings.Contains(value, staleWorkflowName) {
			t.Fatalf("stale scheduled workflow reference in %q", value)
		}
	}
	if agenticWorkflowSourceRel != ".github/workflows/real-world-testing.md" {
		t.Fatalf("source workflow = %q, want real-world-testing.md", agenticWorkflowSourceRel)
	}
	if agenticWorkflowLockRel != ".github/workflows/real-world-testing.lock.yml" {
		t.Fatalf("lock workflow = %q, want real-world-testing.lock.yml", agenticWorkflowLockRel)
	}
}

func TestMissingWorkflowToolsDoesNotAcceptRealWorldWildcard(t *testing.T) {
	required := []string{"real_world_start_testing", "real_world_record_result"}
	source := "allowed:\n  - real_world_*\n"
	lock := "--allow-tool 'dollarlint-repo(real_world_*)'"
	if missing := missingWorkflowTools(source, lock, required); len(missing) != len(required) {
		t.Fatalf("missing = %v, want all required tools", missing)
	}
}

func TestMissingWorkflowToolsAcceptsServerSideRealWorldFilter(t *testing.T) {
	required := []string{"real_world_start_testing", "real_world_record_result"}
	source := `
mcp-servers:
  dollarlint-repo:
    env:
      DOLLARLINT_MCP_TOOLS: "real_world_*"
    allowed:
      - "*"
`
	lock := `
"tools": [
                  "*"
                ],
"env": {"DOLLARLINT_MCP_TOOLS": "real_world_*"}
--allow-tool dollarlint-repo --allow-tool github
`
	if missing := missingWorkflowTools(source, lock, required); len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
}

func TestMissingWorkflowToolsRequiresToolInSourceAndLock(t *testing.T) {
	required := []string{"real_world_start_testing"}
	source := "allowed:\n  - real_world_start_testing\n"
	lock := "--allow-tool 'dollarlint-repo(real_world_history)'"
	if missing := missingWorkflowTools(source, lock, required); len(missing) != 1 || missing[0] != "real_world_start_testing" {
		t.Fatalf("missing = %v, want real_world_start_testing", missing)
	}
}

func TestRealWorldSafeOutputPolicyIssuesRequirePRAndLinker(t *testing.T) {
	source := "tools:\n  bash:\n    - printf\ncreate-pull-request:\n  if-no-changes: error\njobs:\n  link-real-world-outputs:\n${{ github.event.inputs.max_repos }}\n${{ github.event.inputs.candidate_repos }}\nshould be merged in order to retain\n"
	lock := `{"create_pull_request":{"if_no_changes":"error"},"link_real_world_outputs":true,"created_pr_url":"${{ steps.outputs.url }}","shell(printf)":true,"shell(safeoutputs:*)":true,"prompt":"should be merged in order to retain"}`
	if issues := realWorldSafeOutputPolicyIssues(source, lock); len(issues) != 0 {
		t.Fatalf("issues = %+v, want none", issues)
	}

	source = "create-pull-request:\n"
	lock = `{"create_pull_request":{}}`
	if issues := realWorldSafeOutputPolicyIssues(source, lock); len(issues) != 4 {
		t.Fatalf("issues = %+v, want missing PR policy, linker, dispatch inputs, and Discussion retention text", issues)
	}
}

func TestRealWorldSafeOutputPolicyIssuesRequirePrintfForSafeoutputsCLI(t *testing.T) {
	source := "create-pull-request:\n  if-no-changes: error\njobs:\n  link-real-world-outputs:\n${{ github.event.inputs.max_repos }}\n${{ github.event.inputs.candidate_repos }}\nshould be merged in order to retain\n"
	lock := `{"create_pull_request":{"if_no_changes":"error"},"link_real_world_outputs":true,"created_pr_url":"${{ steps.outputs.url }}","shell(safeoutputs:*)":true,"prompt":"should be merged in order to retain"}`
	issues := realWorldSafeOutputPolicyIssues(source, lock)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "printf") {
		t.Fatalf("issues = %+v, want printf issue", issues)
	}
}

func TestRealWorldSafeOutputPolicyIssuesAcceptCheckedInWorkflow(t *testing.T) {
	source := readRepoFile(t, agenticWorkflowSourceRel)
	lock := readRepoFile(t, agenticWorkflowLockRel)
	if issues := realWorldSafeOutputPolicyIssues(source, lock); len(issues) != 0 {
		t.Fatalf("issues = %+v, want checked-in workflow to satisfy safe output policy", issues)
	}
}

func TestRealWorldPRCredentialIssuesRequireWritableOutputPath(t *testing.T) {
	issues, check := realWorldPRCredentialIssues(
		true,
		`[{"name":"COPILOT_GITHUB_TOKEN"}]`,
		true,
		`{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`,
		commandResult{},
		commandResult{},
	)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "durable-memory PR") {
		t.Fatalf("issues = %+v, want missing PR credential issue", issues)
	}
	if ok, _ := check["ok"].(bool); ok {
		t.Fatalf("check = %+v, want not ok", check)
	}
}

func TestRealWorldPRCredentialIssuesAcceptWritableSecretOrRepoSetting(t *testing.T) {
	issues, check := realWorldPRCredentialIssues(
		true,
		`[{"name":"GH_AW_GITHUB_TOKEN"}]`,
		true,
		`{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`,
		commandResult{},
		commandResult{},
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v, want none with GH_AW_GITHUB_TOKEN", issues)
	}
	if ok, _ := check["ok"].(bool); !ok {
		t.Fatalf("check = %+v, want ok", check)
	}

	issues, check = realWorldPRCredentialIssues(
		true,
		`[{"name":"COPILOT_GITHUB_TOKEN"}]`,
		true,
		`{"default_workflow_permissions":"read","can_approve_pull_request_reviews":true}`,
		commandResult{},
		commandResult{},
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v, want none with GitHub Actions PR setting enabled", issues)
	}
	if ok, _ := check["ok"].(bool); !ok {
		t.Fatalf("check = %+v, want ok", check)
	}
}

func hasMapping(mappings []failureMapping, tool, command string) bool {
	for _, mapping := range mappings {
		if mapping.LocalTool == tool && mapping.LocalCommand == command {
			return true
		}
	}
	return false
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
