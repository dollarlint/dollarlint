package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type commandResult struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exitCode"`
	Duration  string `json:"duration"`
	Output    string `json:"output,omitempty"`
	Succeeded bool   `json:"succeeded"`
}

type namedCommand struct {
	Name string
	Cmd  string
	Dir  string
}

const goreleaserSnapshotCheckCommand = "go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish"

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
			{Name: "goreleaser snapshot", Cmd: goreleaserSnapshotCheckCommand},
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
			{Name: "goreleaser snapshot", Cmd: goreleaserSnapshotCheckCommand},
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
