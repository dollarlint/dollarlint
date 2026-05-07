package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/agorischek/dollarlint"
	"github.com/spf13/cobra"
)

var errIssues = errors.New("issues found")
var version = "dev"

func Execute(args []string, stdout, stderr io.Writer) int {
	cmd := NewRootCommand(stdout)
	cmd.SetArgs(args)
	cmd.SetErr(stderr)
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
	var configPath string
	var jsonOutput bool
	var sarifOutput bool
	var showSkipped bool
	var verbose bool
	var quiet bool
	var locations bool
	var includes []string
	var excludes []string
	var associations []string
	var maxDepth int
	var fetchRemote bool
	var fetchTimeout string
	var compileTimeout string

	cmd := &cobra.Command{
		Use:           "dollarlint [path]",
		Short:         "Validate JSON, YAML, and TOML files against their declared JSON Schemas",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			cfg, _, err := dollarlint.LoadConfig(root, configPath)
			if err != nil {
				return err
			}
			if len(includes) > 0 {
				cfg.Discovery.Include = includes
			}
			if len(excludes) > 0 {
				cfg.Discovery.Exclude = append(cfg.Discovery.Exclude, excludes...)
			}
			for _, raw := range associations {
				association, err := parseAssociation(raw)
				if err != nil {
					return err
				}
				cfg.Schema.Associations = append(cfg.Schema.Associations, association)
			}
			if maxDepth > 0 {
				cfg.Schema.MaxDepth = maxDepth
			}
			if cmd.Flags().Changed("fetch-remote") {
				cfg.Schema.FetchRemote = &fetchRemote
			}
			if fetchTimeout != "" {
				if err := cfg.Timeouts.Fetch.UnmarshalText([]byte(fetchTimeout)); err != nil {
					return err
				}
			}
			if compileTimeout != "" {
				if err := cfg.Timeouts.Compile.UnmarshalText([]byte(compileTimeout)); err != nil {
					return err
				}
			}
			cfg.Output.JSON = cfg.Output.JSON || jsonOutput
			cfg.Output.SARIF = cfg.Output.SARIF || sarifOutput
			cfg.Output.ShowSkipped = cfg.Output.ShowSkipped || showSkipped
			cfg.Output.Verbose = cfg.Output.Verbose || verbose
			cfg.Output.Quiet = cfg.Output.Quiet || quiet
			cfg.Output.Locations = cfg.Output.Locations || locations
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
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a dollarlint config file")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Write machine-readable JSON output")
	cmd.Flags().BoolVar(&sarifOutput, "sarif", false, "Write SARIF 2.1.0 output")
	cmd.Flags().BoolVar(&showSkipped, "show-skipped", false, "Show files skipped because they do not declare a schema")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show expanded issue metadata in text output")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Use terse text output")
	cmd.Flags().BoolVar(&locations, "locations", false, "Include line and column locations in text and JSON output")
	cmd.Flags().StringArrayVar(&includes, "include", nil, "Glob to include during discovery; repeatable")
	cmd.Flags().StringArrayVar(&excludes, "exclude", nil, "Glob to exclude during discovery; repeatable")
	cmd.Flags().StringArrayVar(&associations, "schema", nil, "Associate a file glob with a schema as glob=uri; repeatable")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "Maximum external schema reference depth")
	cmd.Flags().BoolVar(&fetchRemote, "fetch-remote", true, "Allow fetching http(s) schemas")
	cmd.Flags().StringVar(&fetchTimeout, "fetch-timeout", "", "Timeout for fetching schemas, e.g. 10s")
	cmd.Flags().StringVar(&compileTimeout, "compile-timeout", "", "Timeout for compiling schemas, e.g. 30s")
	cmd.Flags().Lookup("fetch-remote").NoOptDefVal = "true"
	return cmd
}

func parseAssociation(raw string) (dollarlint.SchemaAssociation, error) {
	glob, schema, ok := strings.Cut(raw, "=")
	if !ok || strings.TrimSpace(glob) == "" || strings.TrimSpace(schema) == "" {
		return dollarlint.SchemaAssociation{}, fmt.Errorf("schema association must be glob=uri")
	}
	return dollarlint.SchemaAssociation{File: strings.TrimSpace(glob), Schema: strings.TrimSpace(schema)}, nil
}
