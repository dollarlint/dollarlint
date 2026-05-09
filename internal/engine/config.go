package engine

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		Configs: ConfigsConfig{
			Mode: ConfigModeSingle,
		},
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
		Parsing: ParsingConfig{
			JSON: JSONParsingConfig{Mode: JSONParsingAuto},
		},
		Schemas: SchemaConfig{
			Catalogs: CatalogConfig{
				Match:   CatalogMatchAuto,
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
	if c.Configs.Mode == "" {
		c.Configs.Mode = defaults.Configs.Mode
	}
	if c.Discovery.Include == nil {
		c.Discovery.Include = append([]string(nil), defaults.Discovery.Include...)
	}
	if c.Discovery.UseDefaultExcludes == nil {
		c.Discovery.UseDefaultExcludes = defaults.Discovery.UseDefaultExcludes
	}
	if c.Discovery.RespectGitIgnore == nil {
		c.Discovery.RespectGitIgnore = defaults.Discovery.RespectGitIgnore
	}
	if c.Parsing.JSON.Mode == "" {
		c.Parsing.JSON.Mode = defaults.Parsing.JSON.Mode
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
	if c.Schemas.Catalogs.Match == "" {
		c.Schemas.Catalogs.Match = CatalogMatchAuto
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
	searchRoot := configSearchRoot(root)
	path, err := resolveConfigPath(searchRoot, explicitPath)
	if err != nil {
		return cfg, "", err
	}
	if path == "" {
		return cfg, "", nil
	}
	loaded, err := loadResolvedConfig(searchRoot, path, nil)
	if err != nil {
		return cfg, "", err
	}
	if err := validateConfigValues(loaded); err != nil {
		return cfg, "", err
	}
	loaded.ApplyDefaults()
	return loaded, path, nil
}

func validateConfigValues(cfg Config) error {
	if cfg.Extends != "" && !isConfigFileName(cfg.Extends) {
		return fmt.Errorf("extends must reference a .dollarlint.toml file")
	}
	if cfg.Configs.Mode != "" {
		if _, err := configMode(cfg.Configs); err != nil {
			return err
		}
	}
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
	if cfg.Schemas.Catalogs.Match != "" {
		if _, err := catalogMatchMode(cfg.Schemas); err != nil {
			return err
		}
	}
	if cfg.Parsing.JSON.Mode != "" {
		if _, err := jsonParsingMode(cfg.Parsing); err != nil {
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

type configPresence map[string]bool

func (p configPresence) has(path string) bool {
	return p != nil && p[path]
}

func loadResolvedConfig(root, path string, stack []string) (Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	for _, seen := range stack {
		if seen == abs {
			return Config{}, fmt.Errorf("config extends cycle includes %s", abs)
		}
	}
	loaded, presence, err := loadConfigFile(root, abs)
	if err != nil {
		return Config{}, err
	}
	if loaded.Extends == "" {
		return loaded, nil
	}
	extendsPath, err := resolveExtendsPath(abs, loaded.Extends)
	if err != nil {
		return Config{}, err
	}
	base, err := loadResolvedConfig(root, extendsPath, append(stack, abs))
	if err != nil {
		return Config{}, err
	}
	merged := mergeConfigs(base, loaded, presence)
	merged.Extends = ""
	return merged, nil
}

func loadConfigFile(root, path string) (Config, configPresence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, nil, fmt.Errorf("read config %s: %w", path, err)
	}
	loaded := Config{}
	if err := decodeConfig(path, data, &loaded); err != nil {
		return Config{}, nil, err
	}
	presence, err := decodeConfigPresence(path, data)
	if err != nil {
		return Config{}, nil, err
	}
	normalizeConfigAuthoredPaths(&loaded, root, path)
	if err := validateConfigValues(loaded); err != nil {
		return Config{}, nil, err
	}
	return loaded, presence, nil
}

func decodeConfigPresence(path string, data []byte) (configPresence, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}
	presence := configPresence{}
	collectConfigPresence("", raw, presence)
	return presence, nil
}

func collectConfigPresence(prefix string, value any, presence configPresence) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			presence[path] = true
			collectConfigPresence(path, child, presence)
		}
	case []map[string]any:
		for _, child := range typed {
			collectConfigPresence(prefix, child, presence)
		}
	case []any:
		for _, child := range typed {
			collectConfigPresence(prefix, child, presence)
		}
	}
}

func resolveExtendsPath(configPath, extends string) (string, error) {
	extends = strings.TrimSpace(extends)
	if !isConfigFileName(extends) {
		return "", fmt.Errorf("extends must reference a .dollarlint.toml file")
	}
	if filepath.IsAbs(extends) {
		return extends, nil
	}
	return filepath.Join(filepath.Dir(configPath), extends), nil
}

func mergeConfigs(parent, child Config, childPresence configPresence) Config {
	merged := parent
	if childPresence.has("version") {
		merged.Version = child.Version
	}
	if childPresence.has("configs.mode") {
		merged.Configs.Mode = child.Configs.Mode
	}
	mergeDiscoveryConfig(&merged.Discovery, child.Discovery, childPresence)
	mergeParsingConfig(&merged.Parsing, child.Parsing, childPresence)
	mergeSchemaConfig(&merged.Schemas, child.Schemas, childPresence)
	if childPresence.has("ignore") {
		merged.Ignore = append(append([]IgnoreRule(nil), parent.Ignore...), child.Ignore...)
	}
	mergeOutputConfig(&merged.Output, child.Output, childPresence)
	return merged
}

func mergeDiscoveryConfig(parent *DiscoveryConfig, child DiscoveryConfig, presence configPresence) {
	if presence.has("discovery.include") {
		parent.Include = child.Include
	}
	if presence.has("discovery.exclude") {
		parent.Exclude = append(append([]string(nil), parent.Exclude...), child.Exclude...)
	}
	if presence.has("discovery.useDefaultExcludes") {
		parent.UseDefaultExcludes = child.UseDefaultExcludes
	}
	if presence.has("discovery.respectGitIgnore") {
		parent.RespectGitIgnore = child.RespectGitIgnore
	}
	if presence.has("discovery.forceExclude") {
		parent.ForceExclude = child.ForceExclude
	}
	if presence.has("discovery.followSymlinks") {
		parent.FollowSymlinks = child.FollowSymlinks
	}
}

func mergeParsingConfig(parent *ParsingConfig, child ParsingConfig, presence configPresence) {
	if presence.has("parsing.json.mode") {
		parent.JSON.Mode = child.JSON.Mode
	}
}

func mergeSchemaConfig(parent *SchemaConfig, child SchemaConfig, presence configPresence) {
	if presence.has("schemas.associations") {
		parent.Associations = append(append([]SchemaAssociation(nil), parent.Associations...), child.Associations...)
	}
	if presence.has("schemas.catalogs.enabled") {
		parent.Catalogs.Enabled = child.Catalogs.Enabled
	}
	if presence.has("schemas.catalogs.failure") {
		parent.Catalogs.Failure = child.Catalogs.Failure
	}
	if presence.has("schemas.catalogs.match") {
		parent.Catalogs.Match = child.Catalogs.Match
	}
	if presence.has("schemas.catalogs.sources") {
		parent.Catalogs.Sources = mergeCatalogSources(parent.Catalogs.Sources, child.Catalogs.Sources)
	}
	if presence.has("schemas.optimizations.enabled") {
		parent.Optimizations.Enabled = child.Optimizations.Enabled
	}
	if presence.has("schemas.optimizations.azure.pruneResources") {
		parent.Optimizations.Azure.PruneResources = child.Optimizations.Azure.PruneResources
	}
	if presence.has("schemas.fetch.enabled") {
		parent.Fetch.Enabled = child.Fetch.Enabled
	}
	if presence.has("schemas.fetch.cache") {
		parent.Fetch.Cache = child.Fetch.Cache
	}
	if presence.has("schemas.fetch.timeout") {
		parent.Fetch.Timeout = child.Fetch.Timeout
	}
	if presence.has("schemas.fetch.retries") {
		parent.Fetch.Retries = child.Fetch.Retries
	}
	if presence.has("schemas.fetch.retryMinWait") {
		parent.Fetch.RetryMinWait = child.Fetch.RetryMinWait
	}
	if presence.has("schemas.fetch.retryMaxWait") {
		parent.Fetch.RetryMaxWait = child.Fetch.RetryMaxWait
	}
	if presence.has("schemas.fetch.allowedDomains") {
		parent.Fetch.AllowedDomains = append(append([]string(nil), parent.Fetch.AllowedDomains...), child.Fetch.AllowedDomains...)
	}
	if presence.has("schemas.fetch.blockedDomains") {
		parent.Fetch.BlockedDomains = append(append([]string(nil), parent.Fetch.BlockedDomains...), child.Fetch.BlockedDomains...)
	}
	if presence.has("schemas.compile.timeout") {
		parent.Compile.Timeout = child.Compile.Timeout
	}
	if presence.has("schemas.requireCoverage") {
		parent.RequireCoverage = child.RequireCoverage
	}
	if presence.has("schemas.maxDepth") {
		parent.MaxDepth = child.MaxDepth
	}
	if presence.has("schemas.concurrency") {
		parent.Concurrency = child.Concurrency
	}
}

func mergeCatalogSources(parent, child []CatalogSource) []CatalogSource {
	out := append([]CatalogSource(nil), parent...)
	indexes := map[string]int{}
	for i, source := range out {
		if key := catalogSourceMergeKey(source); key != "" {
			indexes[key] = i
		}
	}
	for _, source := range child {
		key := catalogSourceMergeKey(source)
		if key == "" {
			out = append(out, source)
			continue
		}
		if index, ok := indexes[key]; ok {
			out[index] = mergeCatalogSource(out[index], source)
			continue
		}
		indexes[key] = len(out)
		out = append(out, source)
	}
	return out
}

func catalogSourceMergeKey(source CatalogSource) string {
	if source.Name != "" {
		return source.Name
	}
	if source.Format == "" || source.Format == "schemastore" {
		return "schemastore"
	}
	return ""
}

func mergeCatalogSource(parent, child CatalogSource) CatalogSource {
	merged := parent
	if child.Name != "" {
		merged.Name = child.Name
	}
	if child.Format != "" {
		merged.Format = child.Format
	}
	if child.URL != "" {
		merged.URL = child.URL
		merged.Path = ""
	}
	if child.Path != "" {
		merged.Path = child.Path
		merged.URL = ""
	}
	if child.Enabled != nil {
		merged.Enabled = child.Enabled
	}
	return merged
}

func mergeOutputConfig(parent *OutputConfig, child OutputConfig, presence configPresence) {
	if presence.has("output.showSkipped") {
		parent.ShowSkipped = child.ShowSkipped
	}
	if presence.has("output.verbose") {
		parent.Verbose = child.Verbose
	}
	if presence.has("output.quiet") {
		parent.Quiet = child.Quiet
	}
	if presence.has("output.locations") {
		parent.Locations = child.Locations
	}
	if presence.has("output.branchErrors") {
		parent.BranchErrors = child.BranchErrors
	}
}

func isConfigFileName(path string) bool {
	return filepath.Base(path) == ".dollarlint.toml"
}

func normalizeConfigAuthoredPaths(cfg *Config, root, configPath string) {
	if cfg == nil || configPath == "" {
		return
	}
	configDir := filepath.Dir(configPath)
	if cfg.Discovery.Include != nil {
		cfg.Discovery.Include = configRelativeGlobs(root, configDir, cfg.Discovery.Include)
	}
	cfg.Discovery.Exclude = configRelativeGlobs(root, configDir, cfg.Discovery.Exclude)
	for i := range cfg.Schemas.Associations {
		cfg.Schemas.Associations[i].File = configRelativeGlob(root, configDir, cfg.Schemas.Associations[i].File)
		cfg.Schemas.Associations[i].Schema = configRelativeSchemaURI(configDir, cfg.Schemas.Associations[i].Schema)
	}
	for i := range cfg.Schemas.Catalogs.Sources {
		cfg.Schemas.Catalogs.Sources[i].Path = configRelativeLocalPath(configDir, cfg.Schemas.Catalogs.Sources[i].Path)
	}
	for i := range cfg.Ignore {
		cfg.Ignore[i].File = configRelativeGlob(root, configDir, cfg.Ignore[i].File)
	}
}

func configRelativeGlobs(root, configDir string, patterns []string) []string {
	if len(patterns) == 0 {
		return patterns
	}
	out := make([]string, len(patterns))
	for i, pattern := range patterns {
		out[i] = configRelativeGlob(root, configDir, pattern)
	}
	return out
}

func configRelativeGlob(root, configDir, pattern string) string {
	pattern = cleanGlob(pattern)
	if pattern == "" || filepath.IsAbs(pattern) {
		return pattern
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return pattern
	}
	configAbs, err := filepath.Abs(configDir)
	if err != nil {
		return pattern
	}
	rel, err := filepath.Rel(rootAbs, configAbs)
	if err != nil {
		return pattern
	}
	rel = cleanGlob(rel)
	if rel == "" {
		return pattern
	}
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return pattern
	}
	if strings.Contains(pattern, "/") {
		return cleanGlob(rel + "/" + pattern)
	}
	return cleanGlob(rel + "/**/" + pattern)
}

func configRelativeSchemaURI(configDir, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Path == "" || filepath.IsAbs(parsed.Path) {
		return raw
	}
	base, _ := fileURL(configDir + string(filepath.Separator))
	return base.ResolveReference(parsed).String()
}

func configRelativeLocalPath(configDir, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || filepath.IsAbs(raw) || isRemoteURI(raw) {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.IsAbs() {
		return raw
	}
	return filepath.Clean(filepath.Join(configDir, raw))
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

func configMode(cfg ConfigsConfig) (string, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = ConfigModeSingle
	}
	switch mode {
	case ConfigModeSingle, ConfigModeNearest:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported config mode %q; expected single or nearest", mode)
	}
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

func catalogMatchMode(cfg SchemaConfig) (string, error) {
	mode := cfg.Catalogs.Match
	if mode == "" {
		mode = CatalogMatchAuto
	}
	switch mode {
	case CatalogMatchAuto, CatalogMatchAll:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported catalog match mode %q; expected auto or all", mode)
	}
}

func jsonParsingMode(cfg ParsingConfig) (string, error) {
	mode := cfg.JSON.Mode
	if mode == "" {
		mode = JSONParsingAuto
	}
	switch mode {
	case JSONParsingStrict, JSONParsingJSONC, JSONParsingAuto:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported parsing.json.mode %q; expected strict, jsonc, or auto", mode)
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
