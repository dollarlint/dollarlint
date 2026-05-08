package engine

import "time"

const (
	StatusValidated = "validated"
	StatusSkipped   = "skipped"
	StatusError     = "error"
)

const (
	CatalogFailureWarn  = "warn"
	CatalogFailureError = "error"
	CatalogFailureSkip  = "skip"
)

type Config struct {
	Version   int             `json:"version,omitempty" yaml:"version,omitempty" toml:"version,omitempty"`
	Discovery DiscoveryConfig `json:"discovery,omitempty" yaml:"discovery,omitempty" toml:"discovery,omitempty"`
	Schemas   SchemaConfig    `json:"schemas,omitempty" yaml:"schemas,omitempty" toml:"schemas,omitempty"`
	Ignore    []IgnoreRule    `json:"ignore,omitempty" yaml:"ignore,omitempty" toml:"ignore,omitempty"`
	Output    OutputConfig    `json:"output,omitempty" yaml:"output,omitempty" toml:"output,omitempty"`
}

type DiscoveryConfig struct {
	Include            []string `json:"include,omitempty" yaml:"include,omitempty" toml:"include,omitempty"`
	ExtendExclude      []string `json:"extendExclude,omitempty" yaml:"extendExclude,omitempty" toml:"extendExclude,omitempty"`
	UseDefaultExcludes *bool    `json:"useDefaultExcludes,omitempty" yaml:"useDefaultExcludes,omitempty" toml:"useDefaultExcludes,omitempty"`
	RespectGitIgnore   *bool    `json:"respectGitIgnore,omitempty" yaml:"respectGitIgnore,omitempty" toml:"respectGitIgnore,omitempty"`
	ForceExclude       bool     `json:"forceExclude,omitempty" yaml:"forceExclude,omitempty" toml:"forceExclude,omitempty"`
	FollowSymlinks     bool     `json:"followSymlinks,omitempty" yaml:"followSymlinks,omitempty" toml:"followSymlinks,omitempty"`
}

type SchemaConfig struct {
	Associations    []SchemaAssociation `json:"associations,omitempty" yaml:"associations,omitempty" toml:"associations,omitempty"`
	Catalogs        CatalogConfig       `json:"catalogs,omitempty" yaml:"catalogs,omitempty" toml:"catalogs,omitempty"`
	Optimizations   OptimizationConfig  `json:"optimizations,omitempty" yaml:"optimizations,omitempty" toml:"optimizations,omitempty"`
	Fetch           FetchConfig         `json:"fetch,omitempty" yaml:"fetch,omitempty" toml:"fetch,omitempty"`
	Compile         CompileConfig       `json:"compile,omitempty" yaml:"compile,omitempty" toml:"compile,omitempty"`
	RequireCoverage bool                `json:"requireCoverage,omitempty" yaml:"requireCoverage,omitempty" toml:"requireCoverage,omitempty"`
	MaxDepth        int                 `json:"maxDepth,omitempty" yaml:"maxDepth,omitempty" toml:"maxDepth,omitempty"`
	Concurrency     int                 `json:"concurrency,omitempty" yaml:"concurrency,omitempty" toml:"concurrency,omitempty"`
}

type OptimizationConfig struct {
	Enabled *bool             `json:"enabled,omitempty" yaml:"enabled,omitempty" toml:"enabled,omitempty"`
	Azure   AzureOptimization `json:"azure,omitempty" yaml:"azure,omitempty" toml:"azure,omitempty"`
}

type AzureOptimization struct {
	PruneResources *bool `json:"pruneResources,omitempty" yaml:"pruneResources,omitempty" toml:"pruneResources,omitempty"`
}

type CatalogConfig struct {
	Enabled bool            `json:"enabled,omitempty" yaml:"enabled,omitempty" toml:"enabled,omitempty"`
	Failure string          `json:"failure,omitempty" yaml:"failure,omitempty" toml:"failure,omitempty"`
	Sources []CatalogSource `json:"sources,omitempty" yaml:"sources,omitempty" toml:"sources,omitempty"`
}

type CatalogSource struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty" toml:"name,omitempty"`
	Format  string `json:"format,omitempty" yaml:"format,omitempty" toml:"format,omitempty"`
	URL     string `json:"url,omitempty" yaml:"url,omitempty" toml:"url,omitempty"`
	Path    string `json:"path,omitempty" yaml:"path,omitempty" toml:"path,omitempty"`
	Enabled *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty" toml:"enabled,omitempty"`
}

type FetchConfig struct {
	Enabled        *bool    `json:"enabled,omitempty" yaml:"enabled,omitempty" toml:"enabled,omitempty"`
	Timeout        Duration `json:"timeout,omitempty" yaml:"timeout,omitempty" toml:"timeout,omitempty"`
	Retries        *int     `json:"retries,omitempty" yaml:"retries,omitempty" toml:"retries,omitempty"`
	RetryMinWait   Duration `json:"retryMinWait,omitempty" yaml:"retryMinWait,omitempty" toml:"retryMinWait,omitempty"`
	RetryMaxWait   Duration `json:"retryMaxWait,omitempty" yaml:"retryMaxWait,omitempty" toml:"retryMaxWait,omitempty"`
	AllowedDomains []string `json:"allowedDomains,omitempty" yaml:"allowedDomains,omitempty" toml:"allowedDomains,omitempty"`
	BlockedDomains []string `json:"blockedDomains,omitempty" yaml:"blockedDomains,omitempty" toml:"blockedDomains,omitempty"`
}

type SchemaAssociation struct {
	File   string `json:"file" yaml:"file" toml:"file"`
	Schema string `json:"schema" yaml:"schema" toml:"schema"`
}

type CompileConfig struct {
	Timeout Duration `json:"timeout,omitempty" yaml:"timeout,omitempty" toml:"timeout,omitempty"`
}

type OutputConfig struct {
	ShowSkipped bool `json:"showSkipped,omitempty" yaml:"showSkipped,omitempty" toml:"showSkipped,omitempty"`
	Verbose     bool `json:"verbose,omitempty" yaml:"verbose,omitempty" toml:"verbose,omitempty"`
	Quiet       bool `json:"quiet,omitempty" yaml:"quiet,omitempty" toml:"quiet,omitempty"`
	Locations   bool `json:"locations,omitempty" yaml:"locations,omitempty" toml:"locations,omitempty"`
}

type IgnoreRule struct {
	File     string `json:"file,omitempty" yaml:"file,omitempty" toml:"file,omitempty"`
	Keyword  string `json:"keyword,omitempty" yaml:"keyword,omitempty" toml:"keyword,omitempty"`
	Property string `json:"property,omitempty" yaml:"property,omitempty" toml:"property,omitempty"`
	Reason   string `json:"reason,omitempty" yaml:"reason,omitempty" toml:"reason,omitempty"`
}

type Options struct {
	Root            string
	Config          Config
	SourceLocations bool
	StartedAt       time.Time
}

type Result struct {
	Root     string       `json:"root"`
	Summary  Summary      `json:"summary"`
	Files    []FileResult `json:"files"`
	Issues   []Issue      `json:"issues,omitempty"`
	Warnings []Warning    `json:"warnings,omitempty"`
}

type Summary struct {
	Discovered    int      `json:"discovered"`
	Validated     int      `json:"validated"`
	Skipped       int      `json:"skipped"`
	Failed        int      `json:"failed"`
	Issues        int      `json:"issues"`
	Ignored       int      `json:"ignored"`
	Warnings      int      `json:"warnings"`
	Duration      Duration `json:"duration,omitempty"`
	DurationNanos int64    `json:"durationNanos,omitempty"`
}

type FileResult struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	Format       string `json:"format"`
	Schema       string `json:"schema,omitempty"`
	SchemaSource string `json:"schemaSource,omitempty"`
	Status       string `json:"status"`
	Issues       int    `json:"issues"`
	Ignored      int    `json:"ignored"`
	Message      string `json:"message,omitempty"`
}

type Issue struct {
	File             string `json:"file"`
	RelativePath     string `json:"relativePath"`
	Schema           string `json:"schema,omitempty"`
	Keyword          string `json:"keyword,omitempty"`
	KeywordLocation  string `json:"keywordLocation,omitempty"`
	Property         string `json:"property,omitempty"`
	InstanceLocation string `json:"instanceLocation,omitempty"`
	Line             int    `json:"line,omitempty"`
	Column           int    `json:"column,omitempty"`
	Message          string `json:"message"`
	Ignored          bool   `json:"ignored,omitempty"`
	IgnoreReason     string `json:"ignoreReason,omitempty"`
}

type Warning struct {
	Kind    string `json:"kind"`
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

func (r Result) HasIssues() bool {
	return r.Summary.Issues > 0
}

func (r Result) HasWarnings() bool {
	return r.Summary.Warnings > 0
}

func NewDuration(d time.Duration) Duration {
	return Duration{Duration: d}
}
