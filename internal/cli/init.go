package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dollarlint/dollarlint"
	"github.com/orochaa/go-clack/prompts"
	"github.com/spf13/cobra"
)

type initOptions struct {
	output         string
	force          bool
	defaults       bool
	comments       bool
	fetchRemote    bool
	fetchRetries   int
	catalogs       bool
	catalogFailure string
}

func defaultInitOptions() initOptions {
	return initOptions{
		output:         ".dollarlint.toml",
		fetchRemote:    true,
		fetchRetries:   defaultFetchRetries,
		catalogs:       true,
		catalogFailure: dollarlint.CatalogFailureWarn,
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
	cmd.Flags().BoolVar(&opts.comments, "comments", false, "Add inline comments explaining each generated option")
	cmd.Flags().BoolVar(&opts.fetchRemote, "fetch-remote", opts.fetchRemote, "Allow fetching http(s) schemas in the generated config")
	cmd.Flags().IntVar(&opts.fetchRetries, "fetch-retries", opts.fetchRetries, "Retries for transient remote schema fetch failures in the generated config")
	cmd.Flags().BoolVar(&opts.catalogs, "catalogs", opts.catalogs, "Enable catalog filename matching in the generated config")
	cmd.Flags().StringVar(&opts.catalogFailure, "catalog-failure", opts.catalogFailure, "Catalog failure policy in the generated config: warn, error, or skip")
	return cmd
}

func runInit(cmd *cobra.Command, stdin io.Reader, stdout io.Writer, args []string, opts initOptions) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	interactive := isInteractiveIO(stdin, stdout)
	var prompter initPrompter
	if interactive {
		prompter = clackPrompter{stdin: stdin.(*os.File), stdout: stdout.(*os.File)}
	}
	target := opts.output
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	if err := validateInitOutputPath(target); err != nil {
		return err
	}
	if err := confirmInitTarget(target, opts, interactive, prompter); err != nil {
		return err
	}
	if !opts.defaults && interactive {
		if err := interviewInitWithPrompter(prompter, &opts); err != nil {
			return err
		}
	} else if !opts.defaults && !interactive {
		fmt.Fprintln(stdout, "No interactive terminal detected; using init defaults. Pass --defaults to silence this message.")
	}
	if err := normalizeInitOptions(&opts); err != nil {
		return err
	}
	content, err := renderStarterConfig(opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", target, err)
	}
	fmt.Fprintf(stdout, "Created %s\n", target)
	fmt.Fprintln(stdout, "Run dollarlint validate . to check your files.")
	return nil
}

func confirmInitTarget(target string, opts initOptions, interactive bool, prompter initPrompter) error {
	if opts.force {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		if !opts.defaults && interactive {
			overwrite, promptErr := prompter.Confirm("Overwrite existing .dollarlint.toml?", false)
			if promptErr != nil {
				return promptErr
			}
			if overwrite {
				return nil
			}
		}
		return fmt.Errorf("config %s already exists; use --force to overwrite", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config %s: %w", target, err)
	}
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
	CatalogFailure(defaultValue string) (string, error)
}

type clackPrompter struct {
	stdin  *os.File
	stdout *os.File
}

func (p clackPrompter) Confirm(question string, defaultValue bool) (bool, error) {
	return promptConfirm(p.stdin, p.stdout, question, defaultValue)
}

func (p clackPrompter) NonNegativeInt(question string, defaultValue int) (int, error) {
	return promptNonNegativeInt(p.stdin, p.stdout, question, defaultValue)
}

func (p clackPrompter) CatalogFailure(defaultValue string) (string, error) {
	return promptCatalogFailure(p.stdin, p.stdout, defaultValue)
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
	catalogs, err := prompter.Confirm("Enable catalog filename matching?", opts.catalogs)
	if err != nil {
		return err
	}
	opts.catalogs = catalogs
	if opts.catalogs {
		failure, err := prompter.CatalogFailure(opts.catalogFailure)
		if err != nil {
			return err
		}
		opts.catalogFailure = failure
	}
	return nil
}

func promptConfirm(stdin, stdout *os.File, question string, defaultValue bool) (bool, error) {
	return prompts.Confirm(prompts.ConfirmParams{
		Context:      context.Background(),
		Input:        stdin,
		Output:       stdout,
		Message:      question,
		InitialValue: defaultValue,
		Active:       "Yes",
		Inactive:     "No",
	})
}

func promptNonNegativeInt(stdin, stdout *os.File, question string, defaultValue int) (int, error) {
	answer, err := prompts.Text(prompts.TextParams{
		Context:      context.Background(),
		Input:        stdin,
		Output:       stdout,
		Message:      question,
		InitialValue: strconv.Itoa(defaultValue),
		Required:     true,
		Validate: func(input string) error {
			value, err := strconv.Atoi(strings.TrimSpace(input))
			if err != nil || value < 0 {
				return fmt.Errorf("please enter a non-negative integer")
			}
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(answer))
}

func promptCatalogFailure(stdin, stdout *os.File, defaultValue string) (string, error) {
	return prompts.Select(prompts.SelectParams[string]{
		Context:      context.Background(),
		Input:        stdin,
		Output:       stdout,
		Message:      "Catalog failure policy",
		InitialValue: defaultValue,
		Options: []*prompts.SelectOption[string]{
			{Label: "warn", Value: dollarlint.CatalogFailureWarn, Hint: "continue with a warning"},
			{Label: "error", Value: dollarlint.CatalogFailureError, Hint: "fail the run"},
			{Label: "skip", Value: dollarlint.CatalogFailureSkip, Hint: "silently skip catalog inference"},
		},
		Required: true,
	})
}

func validateInitOutputPath(target string) error {
	if filepath.Base(target) != ".dollarlint.toml" {
		return fmt.Errorf("unsupported config file %s; dollarlint config must be named .dollarlint.toml", filepath.Base(target))
	}
	return nil
}

func normalizeInitOptions(opts *initOptions) error {
	if opts.fetchRetries < 0 {
		return fmt.Errorf("fetch-retries must be >= 0")
	}
	switch opts.catalogFailure {
	case dollarlint.CatalogFailureWarn, dollarlint.CatalogFailureError, dollarlint.CatalogFailureSkip:
		return nil
	default:
		return fmt.Errorf("unsupported catalog-failure %q; expected warn, error, or skip", opts.catalogFailure)
	}
}

func renderStarterConfig(opts initOptions) ([]byte, error) {
	if err := normalizeInitOptions(&opts); err != nil {
		return nil, err
	}
	return renderInitTOMLDocument(starterConfigDocument(opts), opts.comments), nil
}

type initTOMLEntryKind int

const (
	initTOMLBlank initTOMLEntryKind = iota
	initTOMLComment
	initTOMLTable
	initTOMLArrayTable
	initTOMLKey
	initTOMLCommentedKey
)

type initTOMLDocument []initTOMLEntry

type initTOMLEntry struct {
	Kind    initTOMLEntryKind
	Name    string
	Key     string
	Value   initTOMLValue
	Text    string
	Comment string
}

type initTOMLValue struct {
	kind    initTOMLValueKind
	boolVal bool
	intVal  int
	strVal  string
	strsVal []string
}

type initTOMLValueKind int

const (
	initTOMLBool initTOMLValueKind = iota
	initTOMLInt
	initTOMLStringKind
	initTOMLStringArray
)

func tomlBool(value bool) initTOMLValue {
	return initTOMLValue{kind: initTOMLBool, boolVal: value}
}

func tomlInt(value int) initTOMLValue {
	return initTOMLValue{kind: initTOMLInt, intVal: value}
}

func tomlString(value string) initTOMLValue {
	return initTOMLValue{kind: initTOMLStringKind, strVal: value}
}

func tomlStringArray(values ...string) initTOMLValue {
	return initTOMLValue{kind: initTOMLStringArray, strsVal: append([]string(nil), values...)}
}

func starterConfigDocument(opts initOptions) initTOMLDocument {
	schemaStoreURL := dollarlint.DefaultConfig().Schemas.Catalogs.Sources[0].URL
	return initTOMLDocument{
		comment("dollarlint configuration"),
		key("version", tomlInt(1), "Config schema version."),
		blank(),
		table("configs"),
		key("mode", tomlString("single"), `Use "single" for one config per run, or "nearest" to let nested .dollarlint.toml files apply to their own subtrees.`),
		blank(),
		table("discovery"),
		key("exclude", tomlStringArray(), "Project-specific file globs to skip during discovery."),
		key("useDefaultExcludes", tomlBool(true), "Skip built-in dependency, generated, cache, build, temp, and VCS directories."),
		key("respectGitIgnore", tomlBool(true), "Apply patterns from .gitignore files during discovery."),
		key("forceExclude", tomlBool(false), "Apply excludes even when a file path is passed explicitly."),
		key("followSymlinks", tomlBool(false), "Traverse symlinked directories during discovery."),
		blank(),
		table("parsing.json"),
		key("mode", tomlString("auto"), `Parse .json as strict JSON first, then allow JSONC-style comments and trailing commas when needed.`),
		blank(),
		table("schemas"),
		key("maxDepth", tomlInt(8), "Maximum depth for resolving nested external schema references."),
		key("concurrency", tomlInt(8), "Number of concurrent schema validation workers."),
		key("requireCoverage", tomlBool(false), "Report discovered files that have no schema association."),
		blank(),
		table("schemas.optimizations"),
		key("enabled", tomlBool(true), "Enable schema-specific performance and signal optimizations."),
		blank(),
		table("schemas.optimizations.azure"),
		key("pruneResources", tomlBool(true), "Prune irrelevant Azure ARM resource definitions after the resource type is known."),
		blank(),
		table("schemas.fetch"),
		key("enabled", tomlBool(opts.fetchRemote), "Allow fetching HTTP(S) schemas and catalogs."),
		key("cache", tomlBool(true), "Cache fetched remote schemas and catalogs on disk."),
		key("timeout", tomlString("10s"), "Maximum time to wait for each remote schema or catalog request."),
		key("retries", tomlInt(opts.fetchRetries), "Retry transient remote fetch failures this many times."),
		key("retryMinWait", tomlString("250ms"), "Initial wait before retrying a transient remote fetch failure."),
		key("retryMaxWait", tomlString("2s"), "Maximum wait between remote fetch retries."),
		key("allowedDomains", tomlStringArray(), "When non-empty, only fetch remote schemas from these domains."),
		key("blockedDomains", tomlStringArray(), "Never fetch remote schemas from these domains."),
		blank(),
		table("schemas.compile"),
		key("timeout", tomlString("30s"), "Maximum time to spend compiling a schema."),
		blank(),
		table("schemas.catalogs"),
		key("enabled", tomlBool(opts.catalogs), "Infer schemas for conventional config filenames from configured catalogs."),
		key("failure", tomlString(opts.catalogFailure), `Use "warn", "error", or "skip" when catalog loading fails.`),
		key("match", tomlString("auto"), `Use "auto" for high-confidence catalog matches and skip noisy low-confidence matches.`),
		blank(),
		arrayTable("schemas.catalogs.sources"),
		key("name", tomlString("schemastore"), "Unique name for this catalog source."),
		key("format", tomlString("schemastore"), "Catalog format understood by DollarLint."),
		key("url", tomlString(schemaStoreURL), "Remote SchemaStore catalog URL."),
		key("enabled", tomlBool(true), "Enable this catalog source."),
		blank(),
		arrayTable("schemas.catalogs.sources"),
		key("name", tomlString("rubyschema"), "Unique name for this catalog source."),
		key("format", tomlString("rubyschema"), "Catalog format understood by DollarLint."),
		key("enabled", tomlBool(true), "Enable this catalog source."),
		blank(),
		commentedTable("schemas.associations"),
		commentedKey("file", tomlString("settings/*.toml"), "File glob to match."),
		commentedKey("schema", tomlString("./schemas/settings.schema.json"), "Schema URI or local path to use for matching files."),
		blank(),
		table("output"),
		key("showSkipped", tomlBool(false), "Show files skipped because no schema was associated."),
		key("verbose", tomlBool(false), "Show expanded issue metadata in text output."),
		key("quiet", tomlBool(false), "Use terse text output."),
		key("locations", tomlBool(false), "Include line and column locations in text output."),
		key("branchErrors", tomlString("best"), `Choose how many oneOf/anyOf/allOf branch errors to show: "best" or "all".`),
		key("issueHints", tomlString("auto"), `Choose issue hint detail: "auto", "off", or "verbose".`),
		blank(),
		commentedTable("ignore"),
		commentedKey("file", tomlString("fixtures/*.json"), "File glob for issues to ignore."),
		commentedKey("keyword", tomlString("required"), "JSON Schema keyword to ignore."),
		commentedKey("schemaSource", tomlString("config-association"), "Limit the ignore rule to issues from this schema source."),
		commentedKey("property", tomlString("legacyName"), "Limit the ignore rule to this property."),
		commentedKey("reason", tomlString("legacy fixture kept for compatibility"), "Human-readable reason for the ignore rule."),
	}
}

func blank() initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLBlank}
}

func comment(text string) initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLComment, Text: text}
}

func table(name string) initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLTable, Name: name}
}

func arrayTable(name string) initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLArrayTable, Name: name}
}

func commentedTable(name string) initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLComment, Text: "[[" + name + "]]"}
}

func key(name string, value initTOMLValue, comment string) initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLKey, Key: name, Value: value, Comment: comment}
}

func commentedKey(name string, value initTOMLValue, comment string) initTOMLEntry {
	return initTOMLEntry{Kind: initTOMLCommentedKey, Key: name, Value: value, Comment: comment}
}

func renderInitTOMLDocument(doc initTOMLDocument, comments bool) []byte {
	var out bytes.Buffer
	for _, entry := range doc {
		switch entry.Kind {
		case initTOMLBlank:
			out.WriteByte('\n')
		case initTOMLComment:
			writeTOMLComment(&out, entry.Text)
		case initTOMLTable:
			fmt.Fprintf(&out, "[%s]\n", entry.Name)
		case initTOMLArrayTable:
			fmt.Fprintf(&out, "[[%s]]\n", entry.Name)
		case initTOMLKey:
			fmt.Fprintf(&out, "%s = %s%s\n", entry.Key, entry.Value.String(), initCommentSuffix(entry.Comment, comments))
		case initTOMLCommentedKey:
			fmt.Fprintf(&out, "# %s = %s%s\n", entry.Key, entry.Value.String(), initCommentSuffix(entry.Comment, comments))
		}
	}
	return out.Bytes()
}

func (value initTOMLValue) String() string {
	switch value.kind {
	case initTOMLBool:
		if value.boolVal {
			return "true"
		}
		return "false"
	case initTOMLInt:
		return strconv.Itoa(value.intVal)
	case initTOMLStringKind:
		return initTOMLString(value.strVal)
	case initTOMLStringArray:
		if len(value.strsVal) == 0 {
			return "[]"
		}
		encoded := make([]string, 0, len(value.strsVal))
		for _, item := range value.strsVal {
			encoded = append(encoded, initTOMLString(item))
		}
		return "[" + strings.Join(encoded, ", ") + "]"
	default:
		return ""
	}
}

func initTOMLString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func initCommentSuffix(comment string, enabled bool) string {
	if !enabled || comment == "" {
		return ""
	}
	return " # " + sanitizeTOMLComment(comment)
}

func writeTOMLComment(out *bytes.Buffer, text string) {
	if text == "" {
		out.WriteString("#\n")
		return
	}
	for _, line := range strings.Split(text, "\n") {
		out.WriteString("# ")
		out.WriteString(sanitizeTOMLComment(line))
		out.WriteByte('\n')
	}
}

func sanitizeTOMLComment(comment string) string {
	comment = strings.ReplaceAll(comment, "\r", " ")
	comment = strings.ReplaceAll(comment, "\n", " ")
	return comment
}
