package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/agorischek/dollarlint"
	"github.com/spf13/cobra"
)

const mcpProtocolVersion = "2025-11-25"

var mcpSupportedProtocolVersions = map[string]bool{
	"2024-11-05":       true,
	"2025-03-26":       true,
	"2025-06-18":       true,
	mcpProtocolVersion: true,
}

type mcpServer struct {
	stdin      io.Reader
	stdout     io.Writer
	configPath *string
}

type mcpRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpTool struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type mcpToolResult struct {
	Content           []mcpTextContent `json:"content"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpValidateArguments struct {
	Path               string   `json:"path,omitempty"`
	Config             string   `json:"config,omitempty"`
	Include            []string `json:"include,omitempty"`
	ExtendExclude      []string `json:"extend_exclude,omitempty"`
	NoDefaultExcludes  bool     `json:"no_default_excludes,omitempty"`
	NoGitIgnore        bool     `json:"no_gitignore,omitempty"`
	ForceExclude       bool     `json:"force_exclude,omitempty"`
	Schema             []string `json:"schema,omitempty"`
	SchemaStore        *bool    `json:"schema_store,omitempty"`
	SchemaStoreURL     string   `json:"schema_store_url,omitempty"`
	SchemaStoreFailure string   `json:"schema_store_failure,omitempty"`
	MaxDepth           int      `json:"max_depth,omitempty"`
	FetchRemote        *bool    `json:"fetch_remote,omitempty"`
	FetchRetries       *int     `json:"fetch_retries,omitempty"`
	FetchRetryMinWait  string   `json:"fetch_retry_min_wait,omitempty"`
	FetchRetryMaxWait  string   `json:"fetch_retry_max_wait,omitempty"`
	AllowDomain        []string `json:"allow_domain,omitempty"`
	BlockDomain        []string `json:"block_domain,omitempty"`
	FetchTimeout       string   `json:"fetch_timeout,omitempty"`
	CompileTimeout     string   `json:"compile_timeout,omitempty"`
	Locations          bool     `json:"locations,omitempty"`
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
			server := mcpServer{stdin: stdin, stdout: stdout, configPath: configPath}
			return server.serve(context.Background())
		},
	}
}

func (s mcpServer) serve(ctx context.Context) error {
	reader := bufio.NewReader(s.stdin)
	encoder := json.NewEncoder(s.stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if err != nil && errors.Is(err, io.EOF) {
				return nil
			}
			continue
		}
		var request mcpRequest
		if decodeErr := json.Unmarshal(line, &request); decodeErr != nil {
			if encodeErr := encoder.Encode(mcpResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   &mcpError{Code: -32700, Message: "Parse error"},
			}); encodeErr != nil {
				return encodeErr
			}
			continue
		}
		response, ok := s.handle(ctx, request)
		if !ok {
			continue
		}
		if encodeErr := encoder.Encode(response); encodeErr != nil {
			return encodeErr
		}
		if err != nil && errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func (s mcpServer) handle(ctx context.Context, request mcpRequest) (mcpResponse, bool) {
	if request.ID == nil {
		return mcpResponse{}, false
	}
	response := mcpResponse{JSONRPC: "2.0", ID: *request.ID}
	if request.JSONRPC != "2.0" || request.Method == "" {
		response.Error = &mcpError{Code: -32600, Message: "Invalid Request"}
		return response, true
	}
	switch request.Method {
	case "initialize":
		response.Result = s.initializeResult(request.Params)
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": []mcpTool{validateToolDefinition()}}
	case "tools/call":
		result, err := s.callTool(ctx, request.Params)
		if err != nil {
			response.Error = &mcpError{Code: -32602, Message: err.Error()}
			return response, true
		}
		response.Result = result
	default:
		response.Error = &mcpError{Code: -32601, Message: "Method not found"}
	}
	return response, true
}

func (s mcpServer) initializeResult(params json.RawMessage) map[string]any {
	protocolVersion := mcpProtocolVersion
	var initializeParams struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &initializeParams); err == nil && initializeParams.ProtocolVersion != "" {
		if mcpSupportedProtocolVersions[initializeParams.ProtocolVersion] {
			protocolVersion = initializeParams.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "dollarlint",
			"title":   "dollarlint",
			"version": version,
		},
		"instructions": "Use the validate tool to validate JSON, YAML, and TOML files against declared JSON Schemas.",
	}
}

func (s mcpServer) callTool(ctx context.Context, params json.RawMessage) (mcpToolResult, error) {
	var call mcpToolCallParams
	if err := json.Unmarshal(params, &call); err != nil {
		return mcpToolResult{}, fmt.Errorf("invalid tool call params: %w", err)
	}
	if call.Name != "validate" {
		return mcpToolResult{}, fmt.Errorf("unknown tool: %s", call.Name)
	}
	result, err := s.validate(ctx, call.Arguments)
	if err != nil {
		return textToolResult(err.Error(), true), nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return mcpToolResult{}, err
	}
	return mcpToolResult{
		Content:           []mcpTextContent{{Type: "text", Text: string(data)}},
		StructuredContent: result,
		IsError:           false,
	}, nil
}

func (s mcpServer) validate(ctx context.Context, raw json.RawMessage) (mcpValidateResponse, error) {
	args := mcpValidateArguments{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return mcpValidateResponse{}, fmt.Errorf("invalid validate arguments: %w", err)
		}
	}
	root := strings.TrimSpace(args.Path)
	if root == "" {
		root = "."
	}
	configPath := ""
	if s.configPath != nil {
		configPath = *s.configPath
	}
	if args.Config != "" {
		configPath = args.Config
	}
	cfg, _, err := dollarlint.LoadConfig(root, configPath)
	if err != nil {
		return mcpValidateResponse{}, err
	}
	if err := applyMCPValidateArguments(&cfg, args); err != nil {
		return mcpValidateResponse{}, err
	}
	result, err := dollarlint.Lint(ctx, dollarlint.Options{Root: root, Config: cfg, SourceLocations: args.Locations})
	if err != nil {
		return mcpValidateResponse{}, err
	}
	message := "validation passed"
	if result.HasIssues() {
		message = "validation issues found"
	}
	return mcpValidateResponse{OK: !result.HasIssues(), Result: result, Message: message}, nil
}

func applyMCPValidateArguments(cfg *dollarlint.Config, args mcpValidateArguments) error {
	if len(args.Include) > 0 {
		cfg.Discovery.Include = args.Include
	}
	if len(args.ExtendExclude) > 0 {
		cfg.Discovery.ExtendExclude = append(cfg.Discovery.ExtendExclude, args.ExtendExclude...)
	}
	if args.NoDefaultExcludes {
		useDefaultExcludes := false
		cfg.Discovery.UseDefaultExcludes = &useDefaultExcludes
	}
	if args.NoGitIgnore {
		respectGitIgnore := false
		cfg.Discovery.RespectGitIgnore = &respectGitIgnore
	}
	if args.ForceExclude {
		cfg.Discovery.ForceExclude = true
	}
	for _, raw := range args.Schema {
		association, err := parseAssociation(raw)
		if err != nil {
			return err
		}
		cfg.Schemas.Associations = append(cfg.Schemas.Associations, association)
	}
	if args.SchemaStore != nil {
		cfg.Schemas.Catalogs.Enabled = *args.SchemaStore
	}
	if args.SchemaStoreURL != "" {
		cfg.Schemas.Catalogs.Enabled = true
		cfg.Schemas.Catalogs.Sources = setSchemaStoreCatalogURL(cfg.Schemas.Catalogs.Sources, args.SchemaStoreURL)
	}
	if args.SchemaStoreFailure != "" {
		cfg.Schemas.Catalogs.Failure = args.SchemaStoreFailure
	}
	if args.MaxDepth > 0 {
		cfg.Schemas.MaxDepth = args.MaxDepth
	}
	if args.FetchRemote != nil {
		cfg.Schemas.Fetch.Enabled = args.FetchRemote
	}
	if args.FetchRetries != nil {
		cfg.Schemas.Fetch.Retries = args.FetchRetries
	}
	if args.FetchRetryMinWait != "" {
		if err := cfg.Schemas.Fetch.RetryMinWait.UnmarshalText([]byte(args.FetchRetryMinWait)); err != nil {
			return err
		}
	}
	if args.FetchRetryMaxWait != "" {
		if err := cfg.Schemas.Fetch.RetryMaxWait.UnmarshalText([]byte(args.FetchRetryMaxWait)); err != nil {
			return err
		}
	}
	if len(args.AllowDomain) > 0 {
		cfg.Schemas.Fetch.AllowedDomains = append(cfg.Schemas.Fetch.AllowedDomains, args.AllowDomain...)
	}
	if len(args.BlockDomain) > 0 {
		cfg.Schemas.Fetch.BlockedDomains = append(cfg.Schemas.Fetch.BlockedDomains, args.BlockDomain...)
	}
	if args.FetchTimeout != "" {
		if err := cfg.Schemas.Fetch.Timeout.UnmarshalText([]byte(args.FetchTimeout)); err != nil {
			return err
		}
	}
	if args.CompileTimeout != "" {
		if err := cfg.Schemas.Compile.Timeout.UnmarshalText([]byte(args.CompileTimeout)); err != nil {
			return err
		}
	}
	cfg.Output.Locations = cfg.Output.Locations || args.Locations
	return nil
}

func textToolResult(text string, isError bool) mcpToolResult {
	return mcpToolResult{
		Content: []mcpTextContent{{Type: "text", Text: text}},
		IsError: isError,
	}
}

func validateToolDefinition() mcpTool {
	return mcpTool{
		Name:        "validate",
		Title:       "Validate files",
		Description: "Validate JSON, YAML, and TOML files under a path against declared JSON Schemas. Returns ok=false when validation issues are found.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to validate. Defaults to the current working directory.",
					"default":     ".",
				},
				"config": map[string]any{
					"type":        "string",
					"description": "Path to a .dollarlint.toml config file.",
				},
				"include":        arrayOfStringsSchema("Discovery include glob; repeatable."),
				"extend_exclude": arrayOfStringsSchema("Additional discovery exclude glob; repeatable."),
				"no_default_excludes": map[string]any{
					"type":        "boolean",
					"description": "Disable built-in discovery excludes.",
				},
				"no_gitignore": map[string]any{
					"type":        "boolean",
					"description": "Do not apply root .gitignore patterns during discovery.",
				},
				"force_exclude": map[string]any{
					"type":        "boolean",
					"description": "Apply discovery excludes even to explicitly passed files.",
				},
				"schema": arrayOfStringsSchema("Schema association in glob=uri form; repeatable."),
				"schema_store": map[string]any{
					"type":        "boolean",
					"description": "Match conventional filenames using the SchemaStore catalog.",
				},
				"schema_store_url": map[string]any{
					"type":        "string",
					"description": "SchemaStore catalog URL or local path.",
				},
				"schema_store_failure": enumStringSchema([]string{"warn", "error", "skip"}, "SchemaStore catalog failure policy."),
				"max_depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Maximum external schema reference depth.",
				},
				"fetch_remote": map[string]any{
					"type":        "boolean",
					"description": "Allow fetching http(s) schemas.",
				},
				"fetch_retries": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Number of retries for transient remote schema fetch failures.",
				},
				"fetch_retry_min_wait": durationStringSchema("Minimum wait between remote schema fetch retries, e.g. 250ms."),
				"fetch_retry_max_wait": durationStringSchema("Maximum wait between remote schema fetch retries, e.g. 2s."),
				"allow_domain":         arrayOfStringsSchema("Allow remote schemas from this domain; repeatable."),
				"block_domain":         arrayOfStringsSchema("Block remote schemas from this domain; repeatable."),
				"fetch_timeout":        durationStringSchema("Timeout for fetching schemas, e.g. 10s."),
				"compile_timeout":      durationStringSchema("Timeout for compiling schemas, e.g. 30s."),
				"locations": map[string]any{
					"type":        "boolean",
					"description": "Include line and column locations in validation issues.",
				},
			},
		},
		OutputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"properties": map[string]any{
				"ok":      map[string]any{"type": "boolean"},
				"message": map[string]any{"type": "string"},
				"result":  map[string]any{"type": "object"},
			},
			"required": []string{"ok", "result"},
		},
		Annotations: map[string]any{
			"readOnlyHint":    true,
			"idempotentHint":  true,
			"destructiveHint": false,
		},
	}
}

func arrayOfStringsSchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

func durationStringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func enumStringSchema(values []string, description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}
