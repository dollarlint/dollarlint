package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type commandResult struct {
	Job         string `json:"job,omitempty"`
	Name        string `json:"name"`
	Command     string `json:"command"`
	ExitCode    int    `json:"exitCode"`
	Duration    string `json:"duration"`
	Output      string `json:"output,omitempty"`
	Succeeded   bool   `json:"succeeded"`
	Skipped     bool   `json:"skipped,omitempty"`
	SkipReason  string `json:"skipReason,omitempty"`
	FailureHint string `json:"failureHint,omitempty"`
}

type namedCommand struct {
	Job         string
	Name        string
	Cmd         string
	Dir         string
	FailureHint string
	OptionalEnv string
}

const goreleaserCheckCommand = "go run github.com/goreleaser/goreleaser/v2@latest check"
const goreleaserSnapshotCheckCommand = "go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish"

func verifyCommands(profile string) ([]namedCommand, error) {
	switch profile {
	case "quick":
		return []namedCommand{{Name: "go test", Cmd: "go test ./..."}, {Name: "go vet", Cmd: "go vet ./..."}}, nil
	case "docs":
		return ciReadinessCommands("docs")
	case "release":
		return []namedCommand{
			{Name: "goreleaser snapshot", Cmd: goreleaserSnapshotCheckCommand},
		}, nil
	case "examples":
		return exampleCommands("all", "text", true)
	case "ci", "full":
		return ciReadinessCommands("all")
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
	case "nested-configs":
		return []namedCommand{command("./examples/nested-configs")}, nil
	case "azure":
		return []namedCommand{command("./examples/azure")}, nil
	case "repo-config":
		return []namedCommand{command(". --include .dollarlint.toml")}, nil
	case "all":
		return []namedCommand{command("./examples/basics"), command("./examples/nested-configs"), command("./examples/schemastore"), command("./examples/azure"), command(". --include .dollarlint.toml")}, nil
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
	failed := make([]commandResult, 0)
	ok := true
	for _, command := range commands {
		p.step("Running " + command.Name)
		result := s.run(ctx, command)
		results = append(results, result)
		if !result.Succeeded {
			ok = false
			failed = append(failed, result)
		}
	}
	return map[string]any{
		"profile":        profile,
		"ok":             ok,
		"commands":       results,
		"failedCommands": failed,
	}, nil
}

func (s *repoServer) run(ctx context.Context, command namedCommand) commandResult {
	start := time.Now()
	if command.OptionalEnv != "" && os.Getenv(command.OptionalEnv) == "" {
		reason := command.OptionalEnv + " is not set"
		return commandResult{
			Job:         command.Job,
			Name:        command.Name,
			Command:     command.Cmd,
			ExitCode:    0,
			Duration:    time.Since(start).Round(time.Millisecond).String(),
			Output:      reason,
			Succeeded:   true,
			Skipped:     true,
			SkipReason:  reason,
			FailureHint: command.FailureHint,
		}
	}
	dir := s.root
	if command.Dir != "" {
		dir = filepath.Join(s.root, command.Dir)
	}
	shell, shellArgs := repoCommandShell()
	cmdArgs := append(append([]string{}, shellArgs...), command.Cmd)
	cmd := exec.CommandContext(ctx, shell, cmdArgs...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	outputText := output.String()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if strings.TrimSpace(outputText) == "" {
			outputText = err.Error()
		}
	}
	return commandResult{
		Job:         command.Job,
		Name:        command.Name,
		Command:     command.Cmd,
		ExitCode:    exitCode,
		Duration:    time.Since(start).Round(time.Millisecond).String(),
		Output:      truncate(outputText, 12000),
		Succeeded:   err == nil,
		FailureHint: command.FailureHint,
	}
}

func (s *repoServer) output(ctx context.Context, cmd string) string {
	return s.run(ctx, namedCommand{Name: cmd, Cmd: cmd}).Output
}

func repoCommandShell() (string, []string) {
	return repoCommandShellWith(strings.TrimSpace(os.Getenv("DOLLARLINT_MCP_SHELL")), commandExists)
}

func repoCommandShellWith(preferred string, exists func(string) bool) (string, []string) {
	candidates := []string{}
	if preferred != "" {
		candidates = append(candidates, preferred)
	}
	candidates = append(candidates, "/bin/zsh", "/bin/bash", "/bin/sh")
	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate, repoCommandShellArgs(candidate)
		}
	}
	return "/bin/sh", []string{"-c"}
}

func repoCommandShellArgs(shell string) []string {
	switch filepath.Base(shell) {
	case "zsh":
		return []string{"-lc"}
	default:
		return []string{"-c"}
	}
}

func commandExists(name string) bool {
	if filepath.IsAbs(name) {
		info, err := os.Stat(name)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(name)
	return err == nil
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
	p.report(p.done, p.total, message)
}

func (p *progress) report(done, total int, message string) {
	if p.token == nil || p.server == nil {
		return
	}
	_ = p.server.SendNotificationToClient(p.ctx, "notifications/progress", map[string]any{
		"progressToken": p.token,
		"progress":      done,
		"total":         total,
		"message":       message,
	})
}
