package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/agorischek/dollarlint"
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

func defaultInitOptions() initOptions {
	return initOptions{
		output:       ".dollarlint.toml",
		fetchRemote:  true,
		fetchRetries: defaultFetchRetries,
		schemaStore:  true,
	}
}

func newInitCommand(stdin io.Reader, stdout io.Writer) *cobra.Command {
	opts := defaultInitOptions()
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a starter dollarlint config file",
		Long:  "Interview the user and create a starter dollarlint config file in the target directory. Existing files are not overwritten unless --force is set.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, stdin, stdout, args, opts)
		},
	}
	cmd.Flags().StringVarP(&opts.output, "output", "o", ".dollarlint.toml", "Path to .dollarlint.toml to create")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite an existing config file")
	cmd.Flags().BoolVar(&opts.defaults, "defaults", false, "Skip prompts and use defaults plus provided flags")
	cmd.Flags().BoolVar(&opts.fetchRemote, "fetch-remote", opts.fetchRemote, "Allow fetching http(s) schemas in the generated config")
	cmd.Flags().IntVar(&opts.fetchRetries, "fetch-retries", opts.fetchRetries, "Retries for transient remote schema fetch failures in the generated config")
	cmd.Flags().BoolVar(&opts.schemaStore, "schema-store", opts.schemaStore, "Enable SchemaStore catalog filename matching in the generated config")
	cmd.Flags().BoolVar(&opts.schemaStoreStrict, "schema-store-strict", false, "Fail validation when the SchemaStore catalog cannot be loaded")
	return cmd
}

func runInit(cmd *cobra.Command, stdin io.Reader, stdout io.Writer, args []string, opts initOptions) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	interactive := isInteractiveIO(stdin, stdout)
	var promptReader *bufio.Reader
	if interactive {
		promptReader = bufio.NewReader(stdin)
	}
	if !opts.defaults && interactive {
		if err := interviewInit(promptReader, stdout, &opts); err != nil {
			return err
		}
	} else if !opts.defaults && !interactive {
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
			if !opts.defaults && interactive {
				overwrite, promptErr := promptConfirm(promptReader, stdout, "Overwrite existing .dollarlint.toml?", false)
				if promptErr != nil {
					return promptErr
				}
				if !overwrite {
					return fmt.Errorf("config %s already exists; use --force to overwrite", target)
				}
			} else {
				return fmt.Errorf("config %s already exists; use --force to overwrite", target)
			}
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

func interviewInit(reader *bufio.Reader, stdout io.Writer, opts *initOptions) error {
	fetchRemote, err := promptConfirm(reader, stdout, "Allow remote http(s) schema fetching?", opts.fetchRemote)
	if err != nil {
		return err
	}
	opts.fetchRemote = fetchRemote
	retries, err := promptNonNegativeInt(reader, stdout, "Retries for transient schema fetch failures", opts.fetchRetries)
	if err != nil {
		return err
	}
	opts.fetchRetries = retries
	schemaStore, err := promptConfirm(reader, stdout, "Enable SchemaStore filename matching?", opts.schemaStore)
	if err != nil {
		return err
	}
	opts.schemaStore = schemaStore
	if opts.schemaStore {
		strict, err := promptConfirm(reader, stdout, "Fail if the SchemaStore catalog cannot be loaded?", opts.schemaStoreStrict)
		if err != nil {
			return err
		}
		opts.schemaStoreStrict = strict
	}
	return nil
}

func promptConfirm(reader *bufio.Reader, stdout io.Writer, question string, defaultValue bool) (bool, error) {
	suffix := "[y/N]"
	if defaultValue {
		suffix = "[Y/n]"
	}
	for {
		answer, err := promptLine(reader, stdout, fmt.Sprintf("%s %s", question, suffix))
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return defaultValue, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(stdout, "Please answer y or n.")
		}
	}
}

func promptNonNegativeInt(reader *bufio.Reader, stdout io.Writer, question string, defaultValue int) (int, error) {
	for {
		answer, err := promptLine(reader, stdout, fmt.Sprintf("%s [%d]", question, defaultValue))
		if err != nil {
			return 0, err
		}
		if answer == "" {
			return defaultValue, nil
		}
		value, err := strconv.Atoi(answer)
		if err == nil && value >= 0 {
			return value, nil
		}
		fmt.Fprintln(stdout, "Please enter a non-negative integer.")
	}
}

func promptLine(reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	fmt.Fprintf(stdout, "%s: ", prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && line != "" {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func enabledWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func validateInitOutputPath(target string) error {
	if filepath.Base(target) != ".dollarlint.toml" {
		return fmt.Errorf("unsupported config file %s; dollarlint config must be named .dollarlint.toml", filepath.Base(target))
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
