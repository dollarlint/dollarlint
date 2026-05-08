package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/agorischek/dollarlint"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

type mcpServer struct {
	stdin      io.Reader
	stdout     io.Writer
	configPath *string
}

type mcpValidateArguments struct {
	Include []string `json:"include,omitempty"`
}

type mcpValidateResponse struct {
	OK      bool              `json:"ok"`
	Result  dollarlint.Result `json:"result"`
	Message string            `json:"message,omitempty"`
}

func newServeCommand(stdin io.Reader, stdout io.Writer, configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve dollarlint integrations",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newServeMCPCommand(stdin, stdout, configPath))
	return cmd
}

func newServeMCPCommand(stdin io.Reader, stdout io.Writer, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve dollarlint as an MCP server over stdio",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mcpServer := mcpServer{stdin: stdin, stdout: stdout, configPath: configPath}
			return mcpServer.serve(context.Background())
		},
	}
}

func (s mcpServer) serve(ctx context.Context) error {
	mcpServer := server.NewMCPServer(
		"dollarlint",
		version,
		server.WithInstructions("Use the validate tool to validate JSON, JSONC, JSON5, JSON Lines, YAML, and TOML files against declared JSON Schemas."),
		server.WithToolCapabilities(false),
		server.WithInputSchemaValidation(),
		server.WithOutputSchemaValidation(),
		server.WithRecovery(),
	)
	mcpServer.AddTool(validateToolDefinition(), s.handleValidateTool)
	stdio := server.NewStdioServer(mcpServer)
	stdio.SetErrorLogger(log.New(io.Discard, "", 0))
	return stdio.Listen(ctx, s.stdin, s.stdout)
}

func (s mcpServer) handleValidateTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args mcpValidateArguments
	if err := request.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid validate arguments: %v", err)), nil
	}
	result, err := s.validate(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultStructuredOnly(result), nil
}

func (s mcpServer) validate(ctx context.Context, args mcpValidateArguments) (mcpValidateResponse, error) {
	root := "."
	configPath := ""
	if s.configPath != nil {
		configPath = *s.configPath
	}
	cfg, _, err := dollarlint.LoadConfig(root, configPath)
	if err != nil {
		return mcpValidateResponse{}, err
	}
	if len(args.Include) > 0 {
		cfg.Discovery.Include = args.Include
	}
	cfg.Output.Locations = true
	result, err := dollarlint.Lint(ctx, dollarlint.Options{Root: root, Config: cfg, SourceLocations: true})
	if err != nil {
		return mcpValidateResponse{}, err
	}
	message := "validation passed"
	if result.HasIssues() {
		message = "validation issues found"
	}
	return mcpValidateResponse{OK: !result.HasIssues(), Result: result, Message: message}, nil
}

func validateToolDefinition() mcp.Tool {
	tool := mcp.NewToolWithRawSchema(
		"validate",
		"Validate JSON, JSONC, JSON5, JSON Lines, YAML, and TOML files using dollarlint config. Returns ok=false when validation issues are found.",
		mustRawJSON(validateToolInputSchema()),
	)
	tool.RawOutputSchema = mustRawJSON(validateToolOutputSchema())
	tool.Annotations = mcp.ToolAnnotation{
		Title:           "Validate files",
		ReadOnlyHint:    mcp.ToBoolPtr(true),
		IdempotentHint:  mcp.ToBoolPtr(true),
		DestructiveHint: mcp.ToBoolPtr(false),
		OpenWorldHint:   mcp.ToBoolPtr(true),
	}
	return tool
}

func validateToolInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"include": map[string]any{
				"type":        "array",
				"description": "File or glob patterns to validate, relative to server cwd. Omit for config discovery.",
				"items":       map[string]any{"type": "string"},
			},
		},
	}
}

func validateToolOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]any{
			"ok":      map[string]any{"type": "boolean"},
			"message": map[string]any{"type": "string"},
			"result":  map[string]any{"type": "object"},
		},
		"required": []string{"ok", "result"},
	}
}

func mustRawJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
