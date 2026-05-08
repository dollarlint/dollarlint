package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/agorischek/dollarlint"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type initOptions struct {
	output             string
	force              bool
	defaults           bool
	fetchRemote        bool
	fetchRetries       int
	schemaStore        bool
	schemaStoreFailure string
	schemaStoreStrict  bool
}

type initTemplateData struct {
	SchemaStoreEnabled bool
	SchemaStoreFailure string
	SchemaStoreStrict  bool
	SchemaStoreURL     string
	FetchRemote        bool
	FetchRetries       int
}

func defaultInitOptions() initOptions {
	return initOptions{
		output:             ".dollarlint.toml",
		fetchRemote:        true,
		fetchRetries:       defaultFetchRetries,
		schemaStore:        true,
		schemaStoreFailure: dollarlint.SchemaStoreFailureWarn,
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
	cmd.Flags().StringVar(&opts.schemaStoreFailure, "schema-store-failure", opts.schemaStoreFailure, "SchemaStore catalog failure policy in the generated config: warn, error, or skip")
	cmd.Flags().BoolVar(&opts.schemaStoreStrict, "schema-store-strict", false, "Fail validation when the SchemaStore catalog cannot be loaded")
	return cmd
}

func runInit(cmd *cobra.Command, stdin io.Reader, stdout io.Writer, args []string, opts initOptions) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	interactive := isInteractiveIO(stdin, stdout)
	if !opts.defaults && interactive {
		if err := interviewInit(asReadCloser(stdin), asWriteCloser(stdout), &opts); err != nil {
			return err
		}
	} else if !opts.defaults && !interactive {
		fmt.Fprintln(stdout, "No interactive terminal detected; using init defaults. Pass --defaults to silence this message.")
	}
	if err := normalizeInitOptions(&opts); err != nil {
		return err
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
				overwrite, promptErr := promptConfirm(asReadCloser(stdin), asWriteCloser(stdout), "Overwrite existing .dollarlint.toml?", false)
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

type initPrompter interface {
	Confirm(question string, defaultValue bool) (bool, error)
	NonNegativeInt(question string, defaultValue int) (int, error)
	SchemaStoreFailure(defaultValue string) (string, error)
}

type promptuiPrompter struct {
	stdin  io.ReadCloser
	stdout io.WriteCloser
}

func (p promptuiPrompter) Confirm(question string, defaultValue bool) (bool, error) {
	return promptConfirm(p.stdin, p.stdout, question, defaultValue)
}

func (p promptuiPrompter) NonNegativeInt(question string, defaultValue int) (int, error) {
	return promptNonNegativeInt(p.stdin, p.stdout, question, defaultValue)
}

func (p promptuiPrompter) SchemaStoreFailure(defaultValue string) (string, error) {
	return promptSchemaStoreFailure(p.stdin, p.stdout, defaultValue)
}

func interviewInit(stdin io.ReadCloser, stdout io.WriteCloser, opts *initOptions) error {
	return interviewInitWithPrompter(promptuiPrompter{stdin: stdin, stdout: stdout}, opts)
}

func interviewInitWithPrompter(prompter initPrompter, opts *initOptions) error {
	fetchRemote, err := prompter.Confirm("Allow remote http(s) schema fetching?", opts.fetchRemote)
	if err != nil {
		return err
	}
	opts.fetchRemote = fetchRemote
	retries, err := prompter.NonNegativeInt("Retries for transient schema fetch failures", opts.fetchRetries)
	if err != nil {
		return err
	}
	opts.fetchRetries = retries
	schemaStore, err := prompter.Confirm("Enable SchemaStore filename matching?", opts.schemaStore)
	if err != nil {
		return err
	}
	opts.schemaStore = schemaStore
	if opts.schemaStore {
		failure, err := prompter.SchemaStoreFailure(opts.schemaStoreFailure)
		if err != nil {
			return err
		}
		opts.schemaStoreFailure = failure
	}
	return nil
}

func promptConfirm(stdin io.ReadCloser, stdout io.WriteCloser, question string, defaultValue bool) (bool, error) {
	prompt := promptui.Prompt{
		Label:   question,
		Default: "n",
		Stdin:   stdin,
		Stdout:  stdout,
		Validate: func(input string) error {
			switch strings.ToLower(strings.TrimSpace(input)) {
			case "", "y", "yes", "n", "no":
				return nil
			default:
				return fmt.Errorf("please answer y or n")
			}
		},
	}
	if defaultValue {
		prompt.Default = "y"
	}
	answer, err := prompt.Run()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "":
		return defaultValue, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func promptNonNegativeInt(stdin io.ReadCloser, stdout io.WriteCloser, question string, defaultValue int) (int, error) {
	prompt := promptui.Prompt{
		Label:     question,
		Default:   strconv.Itoa(defaultValue),
		AllowEdit: true,
		Stdin:     stdin,
		Stdout:    stdout,
		Validate: func(input string) error {
			value, err := strconv.Atoi(strings.TrimSpace(input))
			if err != nil || value < 0 {
				return fmt.Errorf("please enter a non-negative integer")
			}
			return nil
		},
	}
	answer, err := prompt.Run()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(answer))
}

func promptSchemaStoreFailure(stdin io.ReadCloser, stdout io.WriteCloser, defaultValue string) (string, error) {
	items := []string{
		dollarlint.SchemaStoreFailureWarn,
		dollarlint.SchemaStoreFailureError,
		dollarlint.SchemaStoreFailureSkip,
	}
	cursor := 0
	for i, item := range items {
		if item == defaultValue {
			cursor = i
			break
		}
	}
	prompt := promptui.Select{
		Label:     "SchemaStore catalog failure policy",
		Items:     items,
		CursorPos: cursor,
		Size:      len(items),
		Stdin:     stdin,
		Stdout:    stdout,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}",
			Active:   "> {{ . }}",
			Inactive: "  {{ . }}",
			Selected: "{{ . }}",
			Details:  "warn: continue with a warning\nerror: fail the run\nskip: silently skip SchemaStore inference",
		},
	}
	_, value, err := prompt.Run()
	return value, err
}

type readCloser struct {
	io.Reader
}

func (readCloser) Close() error {
	return nil
}

type writeCloser struct {
	io.Writer
}

func (writeCloser) Close() error {
	return nil
}

func asReadCloser(reader io.Reader) io.ReadCloser {
	if closer, ok := reader.(io.ReadCloser); ok {
		return closer
	}
	return readCloser{Reader: reader}
}

func asWriteCloser(writer io.Writer) io.WriteCloser {
	if closer, ok := writer.(io.WriteCloser); ok {
		return closer
	}
	return writeCloser{Writer: writer}
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

func normalizeInitOptions(opts *initOptions) error {
	if opts.schemaStoreStrict {
		opts.schemaStoreFailure = dollarlint.SchemaStoreFailureError
	}
	switch opts.schemaStoreFailure {
	case dollarlint.SchemaStoreFailureWarn, dollarlint.SchemaStoreFailureError, dollarlint.SchemaStoreFailureSkip:
		return nil
	default:
		return fmt.Errorf("unsupported schema-store-failure %q; expected warn, error, or skip", opts.schemaStoreFailure)
	}
}

func renderStarterConfig(opts initOptions) ([]byte, error) {
	if err := normalizeInitOptions(&opts); err != nil {
		return nil, err
	}
	data := initTemplateData{
		SchemaStoreEnabled: opts.schemaStore,
		SchemaStoreFailure: opts.schemaStoreFailure,
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
failure = "{{ .SchemaStoreFailure }}"
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
