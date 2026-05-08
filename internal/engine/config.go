package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var defaultConfigFiles = []string{
	".dollarlint.toml",
}

func DefaultConfig() Config {
	useDefaultExcludes := true
	respectGitIgnore := true
	fetchEnabled := true
	fetchCache := true
	optimizationsEnabled := true
	azureResourcePruning := true
	retries := 2
	return Config{
		Version: 1,
		Discovery: DiscoveryConfig{
			Include: []string{
				"*.json", "**/*.json",
				"*.jsonc", "**/*.jsonc",
				"*.json5", "**/*.json5",
				"*.jsonl", "**/*.jsonl",
				"*.ndjson", "**/*.ndjson",
				"*.yaml", "**/*.yaml",
				"*.yml", "**/*.yml",
				"*.toml", "**/*.toml",
			},
			UseDefaultExcludes: &useDefaultExcludes,
			RespectGitIgnore:   &respectGitIgnore,
		},
		Schemas: SchemaConfig{
			Catalogs: CatalogConfig{
				Sources: []CatalogSource{defaultSchemaStoreCatalogSource()},
			},
			Optimizations: OptimizationConfig{
				Enabled: &optimizationsEnabled,
				Azure: AzureOptimization{
					PruneResources: &azureResourcePruning,
				},
			},
			Fetch: FetchConfig{
				Enabled:      &fetchEnabled,
				Cache:        &fetchCache,
				Timeout:      NewDuration(10 * time.Second),
				Retries:      &retries,
				RetryMinWait: NewDuration(250 * time.Millisecond),
				RetryMaxWait: NewDuration(2 * time.Second),
			},
			Compile: CompileConfig{
				Timeout: NewDuration(30 * time.Second),
			},
			MaxDepth:    8,
			Concurrency: runtime.GOMAXPROCS(0),
		},
		Output: OutputConfig{
			BranchErrors: BranchErrorsBest,
		},
	}
}

func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	if c.Version == 0 {
		c.Version = defaults.Version
	}
	if len(c.Discovery.Include) == 0 {
		c.Discovery.Include = append([]string(nil), defaults.Discovery.Include...)
	}
	if c.Discovery.UseDefaultExcludes == nil {
		c.Discovery.UseDefaultExcludes = defaults.Discovery.UseDefaultExcludes
	}
	if c.Discovery.RespectGitIgnore == nil {
		c.Discovery.RespectGitIgnore = defaults.Discovery.RespectGitIgnore
	}
	if c.Schemas.MaxDepth == 0 {
		c.Schemas.MaxDepth = defaults.Schemas.MaxDepth
	}
	if c.Schemas.Fetch.Enabled == nil {
		c.Schemas.Fetch.Enabled = defaults.Schemas.Fetch.Enabled
	}
	if c.Schemas.Fetch.Cache == nil {
		c.Schemas.Fetch.Cache = defaults.Schemas.Fetch.Cache
	}
	if c.Schemas.Optimizations.Enabled == nil {
		c.Schemas.Optimizations.Enabled = defaults.Schemas.Optimizations.Enabled
	}
	if c.Schemas.Optimizations.Azure.PruneResources == nil {
		c.Schemas.Optimizations.Azure.PruneResources = defaults.Schemas.Optimizations.Azure.PruneResources
	}
	if c.Schemas.Catalogs.Failure == "" {
		c.Schemas.Catalogs.Failure = CatalogFailureWarn
	}
	if len(c.Schemas.Catalogs.Sources) == 0 {
		c.Schemas.Catalogs.Sources = []CatalogSource{defaultSchemaStoreCatalogSource()}
	}
	if c.Schemas.Fetch.Retries == nil {
		c.Schemas.Fetch.Retries = defaults.Schemas.Fetch.Retries
	}
	if c.Schemas.Fetch.RetryMinWait.Duration == 0 {
		c.Schemas.Fetch.RetryMinWait = defaults.Schemas.Fetch.RetryMinWait
	}
	if c.Schemas.Fetch.RetryMaxWait.Duration == 0 {
		c.Schemas.Fetch.RetryMaxWait = defaults.Schemas.Fetch.RetryMaxWait
	}
	if c.Schemas.Fetch.Timeout.Duration == 0 {
		c.Schemas.Fetch.Timeout = defaults.Schemas.Fetch.Timeout
	}
	if c.Schemas.Compile.Timeout.Duration == 0 {
		c.Schemas.Compile.Timeout = defaults.Schemas.Compile.Timeout
	}
	if c.Schemas.Concurrency <= 0 {
		c.Schemas.Concurrency = defaults.Schemas.Concurrency
	}
	if c.Output.BranchErrors == "" {
		c.Output.BranchErrors = defaults.Output.BranchErrors
	}
}

func defaultSchemaStoreCatalogSource() CatalogSource {
	enabled := true
	return CatalogSource{
		Name:    "schemastore",
		Format:  "schemastore",
		URL:     defaultSchemaStoreCatalogURL,
		Enabled: &enabled,
	}
}

func LoadConfig(root, explicitPath string) (Config, string, error) {
	cfg := DefaultConfig()
	path, err := resolveConfigPath(root, explicitPath)
	if err != nil {
		return cfg, "", err
	}
	if path == "" {
		return cfg, "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, "", fmt.Errorf("read config %s: %w", path, err)
	}
	loaded := Config{}
	if err := decodeConfig(path, data, &loaded); err != nil {
		return cfg, "", err
	}
	if err := validateConfigValues(loaded); err != nil {
		return cfg, "", err
	}
	loaded.ApplyDefaults()
	return loaded, path, nil
}

func validateConfigValues(cfg Config) error {
	if cfg.Schemas.MaxDepth < 0 {
		return fmt.Errorf("schemas.maxDepth must be >= 0")
	}
	if cfg.Schemas.Concurrency < 0 {
		return fmt.Errorf("schemas.concurrency must be >= 0")
	}
	if cfg.Schemas.Fetch.Retries != nil && *cfg.Schemas.Fetch.Retries < 0 {
		return fmt.Errorf("schemas.fetch.retries must be >= 0")
	}
	if cfg.Schemas.Fetch.Timeout.Duration < 0 {
		return fmt.Errorf("schemas.fetch.timeout must be >= 0")
	}
	if cfg.Schemas.Fetch.RetryMinWait.Duration < 0 {
		return fmt.Errorf("schemas.fetch.retryMinWait must be >= 0")
	}
	if cfg.Schemas.Fetch.RetryMaxWait.Duration < 0 {
		return fmt.Errorf("schemas.fetch.retryMaxWait must be >= 0")
	}
	if cfg.Schemas.Compile.Timeout.Duration < 0 {
		return fmt.Errorf("schemas.compile.timeout must be >= 0")
	}
	if cfg.Schemas.Catalogs.Failure != "" {
		if _, err := catalogFailureMode(cfg.Schemas); err != nil {
			return err
		}
	}
	if cfg.Output.BranchErrors != "" {
		if _, err := branchErrorMode(cfg.Output); err != nil {
			return err
		}
	}
	return nil
}

func resolveConfigPath(root, explicitPath string) (string, error) {
	root = configSearchRoot(root)
	if explicitPath != "" {
		path := explicitPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if !isConfigFileName(path) {
			return "", fmt.Errorf("unsupported config file %s; dollarlint config must be named .dollarlint.toml", filepath.Base(path))
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("config %s: %w", path, err)
		}
		return path, nil
	}
	for _, name := range defaultConfigFiles {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("config %s: %w", path, err)
		}
	}
	return "", nil
}

func configSearchRoot(root string) string {
	if root == "" {
		root = "."
	}
	info, err := os.Stat(root)
	if err == nil && !info.IsDir() {
		return filepath.Dir(root)
	}
	return root
}

func decodeConfig(path string, data []byte, out *Config) error {
	if !isConfigFileName(path) {
		return fmt.Errorf("unsupported config file %s; dollarlint config must be named .dollarlint.toml", filepath.Base(path))
	}
	if err := toml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	return nil
}

func isConfigFileName(path string) bool {
	return filepath.Base(path) == ".dollarlint.toml"
}

func remoteFetchEnabled(cfg SchemaConfig) bool {
	return cfg.Fetch.Enabled == nil || *cfg.Fetch.Enabled
}

func remoteFetchCacheEnabled(cfg FetchConfig) bool {
	return cfg.Cache == nil || *cfg.Cache
}

func catalogEnabled(cfg SchemaConfig) bool {
	return cfg.Catalogs.Enabled
}

func catalogFailureMode(cfg SchemaConfig) (string, error) {
	mode := cfg.Catalogs.Failure
	if mode == "" {
		mode = CatalogFailureWarn
	}
	switch mode {
	case CatalogFailureWarn, CatalogFailureError, CatalogFailureSkip:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported catalog failure policy %q; expected warn, error, or skip", mode)
	}
}

func azureResourcePruningEnabled(cfg SchemaConfig) bool {
	return (cfg.Optimizations.Enabled == nil || *cfg.Optimizations.Enabled) &&
		(cfg.Optimizations.Azure.PruneResources == nil || *cfg.Optimizations.Azure.PruneResources)
}

func branchErrorMode(cfg OutputConfig) (string, error) {
	mode := cfg.BranchErrors
	if mode == "" {
		mode = BranchErrorsBest
	}
	switch mode {
	case BranchErrorsBest, BranchErrorsAll:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported output.branchErrors %q; expected best or all", mode)
	}
}

func fetchRetries(cfg FetchConfig) int {
	if cfg.Retries == nil {
		return 0
	}
	if *cfg.Retries < 0 {
		return 0
	}
	return *cfg.Retries
}
