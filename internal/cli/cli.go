package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agorischek/dollarlint"
	"github.com/spf13/cobra"
)

var errIssues = errors.New("issues found")
var version = "dev"

const defaultFetchRetries = 2

type validateOptions struct {
	configPath         *string
	jsonOutput         bool
	sarifOutput        bool
	showSkipped        bool
	verbose            bool
	quiet              bool
	locations          bool
	includes           []string
	excludes           []string
	associations       []string
	schemaStore        bool
	schemaStoreURL     string
	schemaStoreStrict  bool
	schemaStoreFailure string
	maxDepth           int
	fetchRemote        bool
	fetchRetries       int
	fetchRetryMinWait  string
	fetchRetryMaxWait  string
	allowedDomains     []string
	blockedDomains     []string
	fetchTimeout       string
	compileTimeout     string
}

func Execute(args []string, stdout, stderr io.Writer) int {
	return ExecuteWithIO(args, os.Stdin, stdout, stderr)
}

func ExecuteWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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
		Short:         "Validate JSON, YAML, and TOML files against their declared JSON Schemas",
		Long:          "dollarlint validates JSON, YAML, and TOML files against their declared JSON Schemas.\n\nRun `dollarlint validate [path]` to validate files.",
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
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "Write machine-readable JSON output")
	cmd.Flags().BoolVar(&opts.sarifOutput, "sarif", false, "Write SARIF 2.1.0 output")
	cmd.Flags().BoolVar(&opts.showSkipped, "show-skipped", false, "Show files skipped because they do not declare a schema")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "Show expanded issue metadata in text output")
	cmd.Flags().BoolVar(&opts.quiet, "quiet", false, "Use terse text output")
	cmd.Flags().BoolVar(&opts.locations, "locations", false, "Include line and column locations in text and JSON output")
	cmd.Flags().StringArrayVar(&opts.includes, "include", nil, "Glob to include during discovery; repeatable")
	cmd.Flags().StringArrayVar(&opts.excludes, "exclude", nil, "Glob to exclude during discovery; repeatable")
	cmd.Flags().StringArrayVar(&opts.associations, "schema", nil, "Associate a file glob with a schema as glob=uri; repeatable")
	cmd.Flags().BoolVar(&opts.schemaStore, "schema-store", false, "Match conventional filenames using the SchemaStore catalog")
	cmd.Flags().StringVar(&opts.schemaStoreURL, "schema-store-url", "", "SchemaStore catalog URL or local path")
	cmd.Flags().StringVar(&opts.schemaStoreFailure, "schema-store-failure", "", "SchemaStore catalog failure policy: warn, error, or skip")
	cmd.Flags().BoolVar(&opts.schemaStoreStrict, "schema-store-strict", false, "Fail when the SchemaStore catalog cannot be loaded")
	cmd.Flags().IntVar(&opts.maxDepth, "max-depth", 0, "Maximum external schema reference depth")
	cmd.Flags().BoolVar(&opts.fetchRemote, "fetch-remote", true, "Allow fetching http(s) schemas")
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
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	cfg, _, err := dollarlint.LoadConfig(root, *opts.configPath)
	if err != nil {
		return err
	}
	if len(opts.includes) > 0 {
		cfg.Discovery.Include = opts.includes
	}
	if len(opts.excludes) > 0 {
		cfg.Discovery.Exclude = append(cfg.Discovery.Exclude, opts.excludes...)
	}
	for _, raw := range opts.associations {
		association, err := parseAssociation(raw)
		if err != nil {
			return err
		}
		cfg.Schema.Associations = append(cfg.Schema.Associations, association)
	}
	if cmd.Flags().Changed("schema-store") {
		cfg.Schema.SchemaStore.Enabled = opts.schemaStore
	}
	if opts.schemaStoreURL != "" {
		cfg.Schema.SchemaStore.Enabled = true
		cfg.Schema.SchemaStore.URL = opts.schemaStoreURL
		cfg.Schema.SchemaStoreCatalogURL = opts.schemaStoreURL
	}
	if opts.schemaStoreFailure != "" {
		cfg.Schema.SchemaStore.Failure = opts.schemaStoreFailure
	}
	if cmd.Flags().Changed("schema-store-strict") {
		cfg.Schema.SchemaStore.Strict = opts.schemaStoreStrict
	}
	if opts.maxDepth > 0 {
		cfg.Schema.MaxDepth = opts.maxDepth
	}
	if cmd.Flags().Changed("fetch-remote") {
		cfg.Schema.FetchRemote = &opts.fetchRemote
	}
	if cmd.Flags().Changed("fetch-retries") {
		cfg.Schema.Fetch.Retries = &opts.fetchRetries
	}
	if opts.fetchRetryMinWait != "" {
		if err := cfg.Schema.Fetch.RetryMinWait.UnmarshalText([]byte(opts.fetchRetryMinWait)); err != nil {
			return err
		}
	}
	if opts.fetchRetryMaxWait != "" {
		if err := cfg.Schema.Fetch.RetryMaxWait.UnmarshalText([]byte(opts.fetchRetryMaxWait)); err != nil {
			return err
		}
	}
	if len(opts.allowedDomains) > 0 {
		cfg.Schema.AllowedDomains = append(cfg.Schema.AllowedDomains, opts.allowedDomains...)
	}
	if len(opts.blockedDomains) > 0 {
		cfg.Schema.BlockedDomains = append(cfg.Schema.BlockedDomains, opts.blockedDomains...)
	}
	if opts.fetchTimeout != "" {
		if err := cfg.Timeouts.Fetch.UnmarshalText([]byte(opts.fetchTimeout)); err != nil {
			return err
		}
	}
	if opts.compileTimeout != "" {
		if err := cfg.Timeouts.Compile.UnmarshalText([]byte(opts.compileTimeout)); err != nil {
			return err
		}
	}
	cfg.Output.JSON = cfg.Output.JSON || opts.jsonOutput
	cfg.Output.SARIF = cfg.Output.SARIF || opts.sarifOutput
	cfg.Output.ShowSkipped = cfg.Output.ShowSkipped || opts.showSkipped
	cfg.Output.Verbose = cfg.Output.Verbose || opts.verbose
	cfg.Output.Quiet = cfg.Output.Quiet || opts.quiet
	cfg.Output.Locations = cfg.Output.Locations || opts.locations
	result, err := dollarlint.Lint(context.Background(), dollarlint.Options{Root: root, Config: cfg})
	if err != nil {
		return err
	}
	if cfg.Output.SARIF {
		data, err := dollarlint.FormatSARIF(result)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(data))
	} else if cfg.Output.JSON {
		data, err := dollarlint.FormatJSON(result)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(data))
	} else {
		fmt.Fprint(stdout, dollarlint.FormatText(result, cfg.Output))
	}
	if result.HasIssues() {
		return errIssues
	}
	return nil
}

func parseAssociation(raw string) (dollarlint.SchemaAssociation, error) {
	glob, schema, ok := strings.Cut(raw, "=")
	if !ok || strings.TrimSpace(glob) == "" || strings.TrimSpace(schema) == "" {
		return dollarlint.SchemaAssociation{}, fmt.Errorf("schema association must be glob=uri")
	}
	return dollarlint.SchemaAssociation{File: strings.TrimSpace(glob), Schema: strings.TrimSpace(schema)}, nil
}
