package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

var defaultConfigFiles = []string{
	".dollarlint.yaml",
	".dollarlint.yml",
	".dollarlint.toml",
	".dollarlint.json",
	"dollarlint.yaml",
	"dollarlint.yml",
	"dollarlint.toml",
	"dollarlint.json",
}

func DefaultConfig() Config {
	fetchRemote := true
	fetchSchemaStore := true
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
			MaxDepth:              8,
			FetchRemote:           &fetchRemote,
			FetchSchemaStore:      &fetchSchemaStore,
			SchemaStoreCatalogURL: defaultSchemaStoreCatalogURL,
			Concurrency:           runtime.GOMAXPROCS(0),
		},
		Timeouts: TimeoutConfig{
			Fetch:   NewDuration(10 * time.Second),
			Compile: NewDuration(30 * time.Second),
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
	if len(c.Discovery.Exclude) == 0 {
		c.Discovery.Exclude = append([]string(nil), defaults.Discovery.Exclude...)
	}
	if c.Schema.MaxDepth == 0 {
		c.Schema.MaxDepth = defaults.Schema.MaxDepth
	}
	if c.Schema.FetchRemote == nil {
		c.Schema.FetchRemote = defaults.Schema.FetchRemote
	}
	if c.Schema.FetchSchemaStore == nil {
		c.Schema.FetchSchemaStore = defaults.Schema.FetchSchemaStore
	}
	if c.Schema.SchemaStoreCatalogURL == "" {
		c.Schema.SchemaStoreCatalogURL = defaults.Schema.SchemaStoreCatalogURL
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
	switch ext := filepath.Ext(path); ext {
	case ".json":
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode config %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode config %s: %w", path, err)
		}
	case ".toml":
		if err := toml.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode config %s: %w", path, err)
		}
	default:
		return fmt.Errorf("unsupported config format %s", ext)
	}
	return nil
}

func remoteFetchEnabled(cfg SchemaConfig) bool {
	return cfg.FetchRemote == nil || *cfg.FetchRemote
}

func schemaStoreFetchEnabled(cfg SchemaConfig) bool {
	if !remoteFetchEnabled(cfg) || (cfg.FetchSchemaStore != nil && !*cfg.FetchSchemaStore) {
		return false
	}
	return checkRemoteDomainPolicy(cfg.SchemaStoreCatalogURL, cfg) == nil
}
