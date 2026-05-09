package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dollarlint/dollarlint"
	"github.com/spf13/cobra"
)

var errIssues = errors.New("issues found")
var version = "dev"

const defaultFetchRetries = 2

const (
	outputFormatText  = "text"
	outputFormatJSON  = "json"
	outputFormatSARIF = "sarif"
)

type validateOptions struct {
	configPath        *string
	format            string
	outputPath        string
	showSkipped       bool
	verbose           bool
	quiet             bool
	locations         bool
	includes          []string
	excludes          []string
	noDefaultExcludes bool
	noGitIgnore       bool
	forceExclude      bool
	associations      []string
	schemaStore       bool
	schemaStoreURL    string
	catalogFailure    string
	maxDepth          int
	fetchRemote       bool
	noSchemaCache     bool
	fetchRetries      int
	fetchRetryMinWait string
	fetchRetryMaxWait string
	allowedDomains    []string
	blockedDomains    []string
	fetchTimeout      string
	compileTimeout    string
}

func Execute(args []string, stdout, stderr io.Writer) int {
	return ExecuteWithIO(args, os.Stdin, stdout, stderr)
}

func ExecuteWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	configureNoColor()
	cmd := NewRootCommandWithIO(stdin, stdout)
	cmd.SetArgs(args)
	cmd.SetErr(stderr)
	cmd.SetOut(stdout)
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, errIssues) {
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}

func NewRootCommand(stdout io.Writer) *cobra.Command {
	return NewRootCommandWithIO(os.Stdin, stdout)
}

func NewRootCommandWithIO(stdin io.Reader, stdout io.Writer) *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:           "dollarlint",
		Short:         "Validate JSON-family, YAML, and TOML files against their declared JSON Schemas",
		Long:          "dollarlint validates JSON, JSONC, JSON5, JSON Lines, YAML, and TOML files against their declared JSON Schemas.\n\nRun `dollarlint validate [path]` to validate files.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to a dollarlint config file")
	cmd.AddCommand(newValidateCommand(stdout, &configPath))
	cmd.AddCommand(newInitCommand(stdin, stdout))
	cmd.AddCommand(newServeCommand(stdin, stdout, &configPath))
	cmd.AddCommand(newVersionCommand(stdout))
	return cmd
}

func newValidateCommand(stdout io.Writer, configPath *string) *cobra.Command {
	opts := validateOptions{configPath: configPath, fetchRemote: true, fetchRetries: defaultFetchRetries}
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate files against their declared JSON Schemas",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(cmd, stdout, args, &opts)
		},
	}
	addValidateFlags(cmd, &opts)
	return cmd
}

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the dollarlint version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(stdout, "dollarlint version %s\n", version)
		},
	}
}

func addValidateFlags(cmd *cobra.Command, opts *validateOptions) {
	cmd.Flags().StringVar(&opts.format, "format", outputFormatText, "Output format: text, json, or sarif")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "", "Write output to a file instead of stdout")
	cmd.Flags().BoolVar(&opts.showSkipped, "show-skipped", false, "Show files skipped because they do not declare a schema")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "Show expanded issue metadata in text output")
	cmd.Flags().BoolVar(&opts.quiet, "quiet", false, "Use terse text output")
	cmd.Flags().BoolVar(&opts.locations, "locations", false, "Include line and column locations in text and JSON output")
	cmd.Flags().StringArrayVar(&opts.includes, "include", nil, "Glob to include during discovery; repeatable")
	cmd.Flags().StringArrayVar(&opts.excludes, "exclude", nil, "Additional discovery exclude glob; repeatable")
	cmd.Flags().BoolVar(&opts.noDefaultExcludes, "no-default-excludes", false, "Disable built-in discovery excludes for common generated and dependency directories")
	cmd.Flags().BoolVar(&opts.noGitIgnore, "no-gitignore", false, "Do not apply .gitignore patterns during discovery")
	cmd.Flags().BoolVar(&opts.forceExclude, "force-exclude", false, "Apply discovery excludes even to explicitly passed files")
	cmd.Flags().StringArrayVar(&opts.associations, "schema", nil, "Associate a file glob with a schema as glob=uri; repeatable")
	cmd.Flags().BoolVar(&opts.schemaStore, "schema-store", false, "Match conventional filenames using the SchemaStore catalog")
	cmd.Flags().StringVar(&opts.schemaStoreURL, "schema-store-url", "", "SchemaStore catalog URL or local path")
	cmd.Flags().StringVar(&opts.catalogFailure, "schema-store-failure", "", "SchemaStore catalog failure policy: warn, error, or skip")
	cmd.Flags().IntVar(&opts.maxDepth, "max-depth", 0, "Maximum external schema reference depth")
	cmd.Flags().BoolVar(&opts.fetchRemote, "fetch-remote", true, "Allow fetching http(s) schemas")
	cmd.Flags().BoolVar(&opts.noSchemaCache, "no-schema-cache", false, "Disable disk cache for remote schemas and catalogs")
	cmd.Flags().IntVar(&opts.fetchRetries, "fetch-retries", defaultFetchRetries, "Number of retries for transient remote schema fetch failures")
	cmd.Flags().StringVar(&opts.fetchRetryMinWait, "fetch-retry-min-wait", "", "Minimum wait between remote schema fetch retries, e.g. 250ms")
	cmd.Flags().StringVar(&opts.fetchRetryMaxWait, "fetch-retry-max-wait", "", "Maximum wait between remote schema fetch retries, e.g. 2s")
	cmd.Flags().StringArrayVar(&opts.allowedDomains, "allow-domain", nil, "Allow remote schemas from this domain; repeatable")
	cmd.Flags().StringArrayVar(&opts.blockedDomains, "block-domain", nil, "Block remote schemas from this domain; repeatable")
	cmd.Flags().StringVar(&opts.fetchTimeout, "fetch-timeout", "", "Timeout for fetching schemas, e.g. 10s")
	cmd.Flags().StringVar(&opts.compileTimeout, "compile-timeout", "", "Timeout for compiling schemas, e.g. 30s")
	cmd.Flags().Lookup("fetch-remote").NoOptDefVal = "true"
}

func runValidate(cmd *cobra.Command, stdout io.Writer, args []string, opts *validateOptions) error {
	startedAt := time.Now()
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	if err := validateExplicitTarget(root); err != nil {
		return err
	}
	cfg, configPath, err := dollarlint.LoadConfig(root, *opts.configPath)
	if err != nil {
		return err
	}
	overlay := func(config *dollarlint.Config) error {
		return applyValidateConfigOptions(cmd, opts, config)
	}
	if err := overlay(&cfg); err != nil {
		return err
	}
	format, err := validateOutputFormat(opts.format)
	if err != nil {
		return err
	}
	result, err := dollarlint.Lint(context.Background(), dollarlint.Options{
		Root:            root,
		Config:          cfg,
		ConfigPath:      configPath,
		ExplicitConfig:  *opts.configPath != "",
		ConfigOverlay:   overlay,
		SourceLocations: format == outputFormatSARIF,
		StartedAt:       startedAt,
	})
	if err != nil {
		return err
	}
	data, err := formatValidateResult(result, cfg.Output, format)
	if err != nil {
		return err
	}
	if err := writeValidateOutput(stdout, opts.outputPath, data); err != nil {
		return err
	}
	if result.HasIssues() {
		return errIssues
	}
	return nil
}

func applyValidateConfigOptions(cmd *cobra.Command, opts *validateOptions, cfg *dollarlint.Config) error {
	if len(opts.includes) > 0 {
		cfg.Discovery.Include = opts.includes
	}
	if len(opts.excludes) > 0 {
		cfg.Discovery.Exclude = append(cfg.Discovery.Exclude, opts.excludes...)
	}
	if opts.noDefaultExcludes {
		useDefaultExcludes := false
		cfg.Discovery.UseDefaultExcludes = &useDefaultExcludes
	}
	if opts.noGitIgnore {
		respectGitIgnore := false
		cfg.Discovery.RespectGitIgnore = &respectGitIgnore
	}
	if opts.forceExclude {
		cfg.Discovery.ForceExclude = true
	}
	for _, raw := range opts.associations {
		association, err := parseAssociation(raw)
		if err != nil {
			return err
		}
		cfg.Schemas.Associations = append(cfg.Schemas.Associations, association)
	}
	if cmd.Flags().Changed("schema-store") {
		cfg.Schemas.Catalogs.Enabled = opts.schemaStore
	}
	if opts.schemaStoreURL != "" {
		cfg.Schemas.Catalogs.Enabled = true
		cfg.Schemas.Catalogs.Sources = setSchemaStoreCatalogURL(cfg.Schemas.Catalogs.Sources, opts.schemaStoreURL)
	}
	if opts.catalogFailure != "" {
		cfg.Schemas.Catalogs.Failure = opts.catalogFailure
	}
	if cmd.Flags().Changed("max-depth") {
		cfg.Schemas.MaxDepth = opts.maxDepth
	}
	if cmd.Flags().Changed("fetch-remote") {
		cfg.Schemas.Fetch.Enabled = &opts.fetchRemote
	}
	if opts.noSchemaCache {
		cache := false
		cfg.Schemas.Fetch.Cache = &cache
	}
	if cmd.Flags().Changed("fetch-retries") {
		cfg.Schemas.Fetch.Retries = &opts.fetchRetries
	}
	if opts.fetchRetryMinWait != "" {
		if err := cfg.Schemas.Fetch.RetryMinWait.UnmarshalText([]byte(opts.fetchRetryMinWait)); err != nil {
			return err
		}
	}
	if opts.fetchRetryMaxWait != "" {
		if err := cfg.Schemas.Fetch.RetryMaxWait.UnmarshalText([]byte(opts.fetchRetryMaxWait)); err != nil {
			return err
		}
	}
	if len(opts.allowedDomains) > 0 {
		cfg.Schemas.Fetch.AllowedDomains = append(cfg.Schemas.Fetch.AllowedDomains, opts.allowedDomains...)
	}
	if len(opts.blockedDomains) > 0 {
		cfg.Schemas.Fetch.BlockedDomains = append(cfg.Schemas.Fetch.BlockedDomains, opts.blockedDomains...)
	}
	if opts.fetchTimeout != "" {
		if err := cfg.Schemas.Fetch.Timeout.UnmarshalText([]byte(opts.fetchTimeout)); err != nil {
			return err
		}
	}
	if opts.compileTimeout != "" {
		if err := cfg.Schemas.Compile.Timeout.UnmarshalText([]byte(opts.compileTimeout)); err != nil {
			return err
		}
	}
	cfg.Output.ShowSkipped = cfg.Output.ShowSkipped || opts.showSkipped
	cfg.Output.Verbose = cfg.Output.Verbose || opts.verbose
	cfg.Output.Quiet = cfg.Output.Quiet || opts.quiet
	cfg.Output.Locations = cfg.Output.Locations || opts.locations
	return nil
}

func validateOutputFormat(format string) (string, error) {
	switch strings.ToLower(format) {
	case "", outputFormatText:
		return outputFormatText, nil
	case outputFormatJSON:
		return outputFormatJSON, nil
	case outputFormatSARIF:
		return outputFormatSARIF, nil
	default:
		return "", fmt.Errorf("unknown output format %q (expected text, json, or sarif)", format)
	}
}

func validateExplicitTarget(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if info.IsDir() {
		return nil
	}
	if _, err := dollarlintFileFormat(root); err != nil {
		return err
	}
	return nil
}

func dollarlintFileFormat(path string) (string, error) {
	switch strings.ToLower(filepathExt(path)) {
	case ".json":
		return "json", nil
	case ".jsonc":
		return "jsonc", nil
	case ".json5":
		return "json5", nil
	case ".jsonl", ".ndjson":
		return "jsonl", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	default:
		return "", fmt.Errorf("unsupported explicit file %s; expected .json, .jsonc, .json5, .jsonl, .ndjson, .yaml, .yml, or .toml", path)
	}
}

func filepathExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		switch path[i] {
		case '.':
			return path[i:]
		case '/', '\\':
			return ""
		}
	}
	return ""
}

func formatValidateResult(result dollarlint.Result, output dollarlint.OutputConfig, format string) ([]byte, error) {
	switch format {
	case outputFormatJSON:
		return dollarlint.FormatJSON(result)
	case outputFormatSARIF:
		return dollarlint.FormatSARIF(result)
	default:
		return []byte(dollarlint.FormatText(result, output)), nil
	}
}

func writeValidateOutput(stdout io.Writer, outputPath string, data []byte) error {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	if outputPath == "" || outputPath == "-" {
		_, err := stdout.Write(data)
		return err
	}
	return os.WriteFile(outputPath, data, 0o644)
}

func setSchemaStoreCatalogURL(sources []dollarlint.CatalogSource, catalogURL string) []dollarlint.CatalogSource {
	enabled := true
	for i := range sources {
		if sources[i].Name == "schemastore" || sources[i].Format == "schemastore" {
			sources[i].Name = "schemastore"
			sources[i].Format = "schemastore"
			sources[i].URL = catalogURL
			sources[i].Path = ""
			sources[i].Enabled = &enabled
			return sources
		}
	}
	return append(sources, dollarlint.CatalogSource{
		Name:    "schemastore",
		Format:  "schemastore",
		URL:     catalogURL,
		Enabled: &enabled,
	})
}

func parseAssociation(raw string) (dollarlint.SchemaAssociation, error) {
	glob, schema, ok := strings.Cut(raw, "=")
	if !ok || strings.TrimSpace(glob) == "" || strings.TrimSpace(schema) == "" {
		return dollarlint.SchemaAssociation{}, fmt.Errorf("schema association must be glob=uri")
	}
	return dollarlint.SchemaAssociation{File: strings.TrimSpace(glob), Schema: strings.TrimSpace(schema)}, nil
}
