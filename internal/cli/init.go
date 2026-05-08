package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/agorischek/dollarlint"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type initOptions struct {
	output            string
	force             bool
	defaults          bool
	fetchRemote       bool
	fetchRetries      int
	schemaStore       bool
	schemaStoreStrict bool
}

type initTemplateData struct {
	SchemaStoreEnabled bool
	SchemaStoreStrict  bool
	SchemaStoreURL     string
	FetchRemote        bool
	FetchRetries       int
}

func newInitCommand(stdin io.Reader, stdout io.Writer) *cobra.Command {
	opts := initOptions{output: ".dollarlint.toml", fetchRemote: true, fetchRetries: defaultFetchRetries}
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a starter dollarlint config file",
		Long:  "Interview the user and create a starter dollarlint config file in the target directory. Existing files are not overwritten unless --force is set.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, stdin, stdout, args, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.output, "output", "o", ".dollarlint.toml", "TOML config file path to create")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite an existing config file")
	cmd.Flags().BoolVar(&opts.defaults, "defaults", false, "Skip prompts and use defaults plus provided flags")
	cmd.Flags().BoolVar(&opts.fetchRemote, "fetch-remote", true, "Allow fetching http(s) schemas in the generated config")
	cmd.Flags().IntVar(&opts.fetchRetries, "fetch-retries", defaultFetchRetries, "Retries for transient remote schema fetch failures in the generated config")
	cmd.Flags().BoolVar(&opts.schemaStore, "schema-store", false, "Enable SchemaStore catalog filename matching in the generated config")
	cmd.Flags().BoolVar(&opts.schemaStoreStrict, "schema-store-strict", false, "Fail validation when the SchemaStore catalog cannot be loaded")
	return cmd
}

func runInit(cmd *cobra.Command, stdin io.Reader, stdout io.Writer, args []string, opts initOptions) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	if shouldInterviewInit(stdin, stdout, opts) {
		if err := interviewInit(stdin, stdout, &opts); err != nil {
			return err
		}
	} else if !opts.defaults && !isInteractiveIO(stdin, stdout) {
		fmt.Fprintln(stdout, "No interactive terminal detected; using init defaults. Pass --defaults to silence this message.")
	}
	target := opts.output
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	if err := validateInitOutputPath(target); err != nil {
		return err
	}
	content, err := renderStarterConfig(opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if !opts.force {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("config %s already exists; use --force to overwrite", target)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check config %s: %w", target, err)
		}
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", target, err)
	}
	if cmd.Flags().Changed("schema-store") || opts.schemaStore {
		fmt.Fprintf(stdout, "Created %s with SchemaStore matching %s\n", target, enabledWord(opts.schemaStore))
		return nil
	}
	fmt.Fprintf(stdout, "Created %s\n", target)
	return nil
}

func shouldInterviewInit(stdin io.Reader, stdout io.Writer, opts initOptions) bool {
	return !opts.defaults && isInteractiveIO(stdin, stdout)
}

func isInteractiveIO(stdin io.Reader, stdout io.Writer) bool {
	inFile, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	outFile, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return isTerminalFile(inFile) && isTerminalFile(outFile)
}

func isTerminalFile(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func interviewInit(stdin io.Reader, stdout io.Writer, opts *initOptions) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Where should dollarlint write the config?").
				Description("Relative paths are resolved from the init target directory.").
				Value(&opts.output).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return fmt.Errorf("config path is required")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Allow remote http(s) schema fetching?").
				Description("Recommended for SchemaStore and common editor-provided schema URLs.").
				Affirmative("Yes").
				Negative("No").
				Value(&opts.fetchRemote),
			huh.NewSelect[int]().
				Title("How many times should transient schema fetches be retried?").
				Options(
					huh.NewOption("None", 0),
					huh.NewOption("2 retries (recommended)", 2),
					huh.NewOption("4 retries", 4),
				).
				Value(&opts.fetchRetries),
			huh.NewConfirm().
				Title("Enable SchemaStore filename matching?").
				Description("This lets conventional files like package.json use catalog schemas without declaring $schema.").
				Affirmative("Enable").
				Negative("Skip").
				Value(&opts.schemaStore),
			huh.NewConfirm().
				Title("Fail if the SchemaStore catalog cannot be loaded?").
				Description("Most projects should leave this off so catalog outages do not block explicit schema validation.").
				Affirmative("Strict").
				Negative("Best effort").
				Value(&opts.schemaStoreStrict),
			huh.NewConfirm().
				Title("Overwrite the config if it already exists?").
				Affirmative("Overwrite").
				Negative("Keep existing").
				Value(&opts.force),
		),
	)
	if err := form.WithInput(stdin).WithOutput(stdout).Run(); err != nil {
		return err
	}
	opts.output = strings.TrimSpace(opts.output)
	return nil
}

func enabledWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func validateInitOutputPath(target string) error {
	if strings.ToLower(filepath.Ext(target)) != ".toml" {
		return fmt.Errorf("unsupported config format %s; dollarlint config must be TOML", filepath.Ext(target))
	}
	return nil
}

func renderStarterConfig(opts initOptions) ([]byte, error) {
	data := initTemplateData{
		SchemaStoreEnabled: opts.schemaStore,
		SchemaStoreStrict:  opts.schemaStoreStrict,
		SchemaStoreURL:     dollarlint.DefaultConfig().Schema.SchemaStore.URL,
		FetchRemote:        opts.fetchRemote,
		FetchRetries:       opts.fetchRetries,
	}
	return executeInitTemplate(starterTOMLTemplate, data)
}

func executeInitTemplate(raw string, data initTemplateData) ([]byte, error) {
	tmpl, err := template.New("starter").Parse(raw)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

const starterTOMLTemplate = `# dollarlint configuration
version = 1

[discovery]
include = ["*.json", "**/*.json", "*.yaml", "**/*.yaml", "*.yml", "**/*.yml", "*.toml", "**/*.toml"]
exclude = ["node_modules", "**/node_modules/**", "dist", "**/dist/**", "build", "**/build/**", "coverage", "**/coverage/**"]

[schema]
fetchRemote = {{ .FetchRemote }}
maxDepth = 8
concurrency = 8
azureResourcePruning = true

[schema.fetch]
retries = {{ .FetchRetries }}
retryMinWait = "250ms"
retryMaxWait = "2s"

[schema.schemaStore]
enabled = {{ .SchemaStoreEnabled }}
url = "{{ .SchemaStoreURL }}"
strict = {{ .SchemaStoreStrict }}

# [[schema.associations]]
# file = "settings/*.toml"
# schema = "./schemas/settings.schema.json"

[timeouts]
fetch = "10s"
compile = "30s"

[output]
json = false
sarif = false
showSkipped = false
verbose = false
quiet = false
locations = false

# [[ignore]]
# file = "fixtures/*.json"
# keyword = "required"
# property = "legacyName"
# reason = "legacy fixture kept for compatibility"
`
