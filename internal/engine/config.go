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
	fetchRemote := true
	retries := 2
	return Config{
		Version: 1,
		Discovery: DiscoveryConfig{
			Include: []string{
				"*.json", "**/*.json",
				"*.yaml", "**/*.yaml",
				"*.yml", "**/*.yml",
				"*.toml", "**/*.toml",
			},
			Exclude: []string{
				".git", "**/.git/**",
				".hg", "**/.hg/**",
				".svn", "**/.svn/**",
				"node_modules", "**/node_modules/**",
				"vendor", "**/vendor/**",
				"dist", "**/dist/**",
				"build", "**/build/**",
				"coverage", "**/coverage/**",
				".next", "**/.next/**",
				".nuxt", "**/.nuxt/**",
				".turbo", "**/.turbo/**",
				".cache", "**/.cache/**",
				".venv", "**/.venv/**",
				"venv", "**/venv/**",
				"target", "**/target/**",
				"tmp", "**/tmp/**",
			},
		},
		Schema: SchemaConfig{
			Catalogs: CatalogConfig{
				Sources: []CatalogSource{defaultSchemaStoreCatalogSource()},
			},
			SchemaStore: SchemaStoreConfig{
				URL: defaultSchemaStoreCatalogURL,
			},
			Fetch: FetchConfig{
				Retries:      &retries,
				RetryMinWait: NewDuration(250 * time.Millisecond),
				RetryMaxWait: NewDuration(2 * time.Second),
			},
			MaxDepth:    8,
			FetchRemote: &fetchRemote,
			Concurrency: runtime.GOMAXPROCS(0),
		},
		Timeouts: TimeoutConfig{
			Fetch:   NewDuration(10 * time.Second),
			Compile: NewDuration(30 * time.Second),
		},
	}
}

func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()
	c.Schema = mergeSchemaConfig(c.Schema, c.Schemas)
	c.Schemas = c.Schema
	if c.Version == 0 {
		c.Version = defaults.Version
	}
	if len(c.Discovery.Include) == 0 {
		c.Discovery.Include = append([]string(nil), defaults.Discovery.Include...)
	}
	if len(c.Discovery.Exclude) == 0 {
		c.Discovery.Exclude = append([]string(nil), defaults.Discovery.Exclude...)
	}
	if c.Schema.MaxDepth == 0 {
		c.Schema.MaxDepth = defaults.Schema.MaxDepth
	}
	if c.Schema.FetchRemote == nil {
		c.Schema.FetchRemote = defaults.Schema.FetchRemote
	}
	if c.Schema.Catalogs.Failure == "" && c.Schema.SchemaStore.Failure != "" {
		c.Schema.Catalogs.Failure = c.Schema.SchemaStore.Failure
	}
	if c.Schema.Catalogs.Strict || c.Schema.SchemaStore.Strict {
		c.Schema.Catalogs.Strict = true
	}
	if c.Schema.SchemaStoreCatalogURL != "" && c.Schema.SchemaStore.URL == "" {
		c.Schema.SchemaStore.URL = c.Schema.SchemaStoreCatalogURL
	}
	if c.Schema.SchemaStore.URL == "" {
		c.Schema.SchemaStore.URL = defaults.Schema.SchemaStore.URL
	}
	if c.Schema.SchemaStoreCatalogURL == "" {
		c.Schema.SchemaStoreCatalogURL = c.Schema.SchemaStore.URL
	}
	if c.Schema.FetchSchemaStore != nil && *c.Schema.FetchSchemaStore {
		c.Schema.SchemaStore.Enabled = true
	}
	if c.Schema.SchemaStore.Enabled {
		c.Schema.Catalogs.Enabled = true
	}
	if c.Schema.SchemaStore.Failure == "" {
		c.Schema.SchemaStore.Failure = SchemaStoreFailureWarn
	}
	if c.Schema.Catalogs.Failure == "" {
		c.Schema.Catalogs.Failure = SchemaStoreFailureWarn
	}
	if len(c.Schema.Catalogs.Sources) == 0 {
		source := defaultSchemaStoreCatalogSource()
		if c.Schema.SchemaStore.URL != "" {
			source.URL = c.Schema.SchemaStore.URL
		}
		c.Schema.Catalogs.Sources = []CatalogSource{source}
	} else if c.Schema.SchemaStore.URL != "" && c.Schema.SchemaStore.URL != defaultSchemaStoreCatalogURL {
		c.Schema.Catalogs.Sources = setDefaultCatalogSourceURL(c.Schema.Catalogs.Sources, c.Schema.SchemaStore.URL)
	}
	if c.Schema.Fetch.Retries == nil {
		c.Schema.Fetch.Retries = defaults.Schema.Fetch.Retries
	}
	if c.Schema.Fetch.RetryMinWait.Duration == 0 {
		c.Schema.Fetch.RetryMinWait = defaults.Schema.Fetch.RetryMinWait
	}
	if c.Schema.Fetch.RetryMaxWait.Duration == 0 {
		c.Schema.Fetch.RetryMaxWait = defaults.Schema.Fetch.RetryMaxWait
	}
	if c.Schema.Concurrency <= 0 {
		c.Schema.Concurrency = defaults.Schema.Concurrency
	}
	if c.Timeouts.Fetch.Duration == 0 {
		c.Timeouts.Fetch = defaults.Timeouts.Fetch
	}
	if c.Timeouts.Compile.Duration == 0 {
		c.Timeouts.Compile = defaults.Timeouts.Compile
	}
	c.Schemas = c.Schema
}

func mergeSchemaConfig(legacy, current SchemaConfig) SchemaConfig {
	if len(current.Associations) > 0 {
		legacy.Associations = current.Associations
	}
	if current.Catalogs.Enabled || current.Catalogs.Failure != "" || current.Catalogs.Strict || len(current.Catalogs.Sources) > 0 {
		legacy.Catalogs = current.Catalogs
	}
	if current.SchemaStore.Enabled || current.SchemaStore.URL != "" || current.SchemaStore.Failure != "" || current.SchemaStore.Strict {
		legacy.SchemaStore = current.SchemaStore
	}
	if current.Fetch.Retries != nil || current.Fetch.RetryMinWait.Duration != 0 || current.Fetch.RetryMaxWait.Duration != 0 {
		legacy.Fetch = current.Fetch
	}
	if current.MaxDepth != 0 {
		legacy.MaxDepth = current.MaxDepth
	}
	if current.FetchRemote != nil {
		legacy.FetchRemote = current.FetchRemote
	}
	if current.FetchSchemaStore != nil {
		legacy.FetchSchemaStore = current.FetchSchemaStore
	}
	if current.AzureResourcePruning != nil {
		legacy.AzureResourcePruning = current.AzureResourcePruning
	}
	if current.SchemaStoreCatalogURL != "" {
		legacy.SchemaStoreCatalogURL = current.SchemaStoreCatalogURL
	}
	if len(current.AllowedDomains) > 0 {
		legacy.AllowedDomains = current.AllowedDomains
	}
	if len(current.BlockedDomains) > 0 {
		legacy.BlockedDomains = current.BlockedDomains
	}
	if current.Concurrency != 0 {
		legacy.Concurrency = current.Concurrency
	}
	return legacy
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

func setDefaultCatalogSourceURL(sources []CatalogSource, catalogURL string) []CatalogSource {
	for i := range sources {
		if sources[i].Name == "schemastore" || sources[i].Format == "schemastore" {
			sources[i].Name = "schemastore"
			sources[i].Format = "schemastore"
			sources[i].URL = catalogURL
			sources[i].Path = ""
			return sources
		}
	}
	source := defaultSchemaStoreCatalogSource()
	source.URL = catalogURL
	return append(sources, source)
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
	loaded.ApplyDefaults()
	return loaded, path, nil
}

func resolveConfigPath(root, explicitPath string) (string, error) {
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
	return cfg.FetchRemote == nil || *cfg.FetchRemote
}

func schemaStoreEnabled(cfg SchemaConfig) bool {
	if cfg.Catalogs.Enabled {
		return true
	}
	if cfg.SchemaStore.Enabled {
		return true
	}
	return cfg.FetchSchemaStore != nil && *cfg.FetchSchemaStore
}

func schemaStoreFailureMode(cfg SchemaConfig) (string, error) {
	if cfg.Catalogs.Strict || cfg.SchemaStore.Strict {
		return SchemaStoreFailureError, nil
	}
	mode := cfg.Catalogs.Failure
	if mode == "" {
		mode = cfg.SchemaStore.Failure
	}
	if mode == "" {
		mode = SchemaStoreFailureWarn
	}
	switch mode {
	case SchemaStoreFailureWarn, SchemaStoreFailureError, SchemaStoreFailureSkip:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported catalog failure policy %q; expected warn, error, or skip", mode)
	}
}

func azureResourcePruningEnabled(cfg SchemaConfig) bool {
	return cfg.AzureResourcePruning == nil || *cfg.AzureResourcePruning
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
