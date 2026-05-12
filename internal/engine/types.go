package engine

import "time"

const (
	StatusValidated = "validated"
	StatusSkipped   = "skipped"
	StatusError     = "error"
)

const (
	SkipReasonNoSchema                 = "noSchema"
	SkipReasonCatalogSchemaUnavailable = "catalogSchemaUnavailable"
	SkipReasonSchemaUnavailable        = "schemaUnavailable"
)

const (
	SkipClassApplicationData   = "application-data"
	SkipClassExternalCatalog   = "external-catalog"
	SkipClassExternalSchema    = "external-schema"
	SkipClassLocaleData        = "locale-data"
	SkipClassLockfile          = "lockfile"
	SkipClassRepoManagement    = "repo-management-config"
	SkipClassTestData          = "test-data"
	SkipClassUnknown           = "unknown"
	SkipClassUnsupportedConfig = "unsupported-config"
)

const (
	SkipImportanceHigh   = "high"
	SkipImportanceMedium = "medium"
	SkipImportanceLow    = "low"
)

const (
	JSONFormatVersion = 1
	JSONResultSchema  = "https://raw.githubusercontent.com/dollarlint/dollarlint/main/schemas/dollarlint-result.schema.json"
)

const InspectFormatVersion = 1

const (
	InspectAssociationStatusAssociated   = "associated"
	InspectAssociationStatusUnassociated = "unassociated"
	InspectAssociationStatusError        = "error"
)

const (
	issueKeywordParse  = "parse"
	issueKeywordSchema = "schema"
)

const (
	CatalogFailureWarn  = "warn"
	CatalogFailureError = "error"
	CatalogFailureSkip  = "skip"
)

const (
	CatalogMatchAuto = "auto"
	CatalogMatchAll  = "all"
)

const (
	SchemaMatchActionMatched                = "matched"
	SchemaMatchActionIgnored                = "ignored"
	SchemaMatchActionSkippedLowConfidence   = "skippedLowConfidence"
	SchemaMatchActionSkippedMissingEvidence = "skippedMissingEvidence"
)

const (
	SchemaMatchConfidenceHigh   = "high"
	SchemaMatchConfidenceMedium = "medium"
	SchemaMatchConfidenceLow    = "low"
)

const (
	SchemaMatchTypeExactPath     = "exactPath"
	SchemaMatchTypePathGlob      = "pathGlob"
	SchemaMatchTypeExactBasename = "exactBasename"
	SchemaMatchTypeBasenameGlob  = "basenameGlob"
)

const (
	JSONParsingStrict = "strict"
	JSONParsingJSONC  = "jsonc"
	JSONParsingAuto   = "auto"
)

const (
	BranchErrorsBest = "best"
	BranchErrorsAll  = "all"
)

const (
	IssueHintsAuto    = "auto"
	IssueHintsOff     = "off"
	IssueHintsVerbose = "verbose"
)

const (
	IssueHintConfidenceHigh   = "high"
	IssueHintConfidenceMedium = "medium"
	IssueHintConfidenceLow    = "low"
)

const (
	ConfigModeSingle  = "single"
	ConfigModeNearest = "nearest"
)

type Config struct {
	Version   int             `json:"version,omitempty" yaml:"version,omitempty" toml:"version,omitempty"`
	Extends   string          `json:"extends,omitempty" yaml:"extends,omitempty" toml:"extends,omitempty"`
	Configs   ConfigsConfig   `json:"configs,omitempty" yaml:"configs,omitempty" toml:"configs,omitempty"`
	Discovery DiscoveryConfig `json:"discovery,omitempty" yaml:"discovery,omitempty" toml:"discovery,omitempty"`
	Parsing   ParsingConfig   `json:"parsing,omitempty" yaml:"parsing,omitempty" toml:"parsing,omitempty"`
	Schemas   SchemaConfig    `json:"schemas,omitempty" yaml:"schemas,omitempty" toml:"schemas,omitempty"`
	Ignore    []IgnoreRule    `json:"ignore,omitempty" yaml:"ignore,omitempty" toml:"ignore,omitempty"`
	Output    OutputConfig    `json:"output,omitempty" yaml:"output,omitempty" toml:"output,omitempty"`
}

type ConfigsConfig struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty" toml:"mode,omitempty"`
}

type DiscoveryConfig struct {
	Include            []string `json:"include,omitempty" yaml:"include,omitempty" toml:"include,omitempty"`
	Exclude            []string `json:"exclude,omitempty" yaml:"exclude,omitempty" toml:"exclude,omitempty"`
	UseDefaultExcludes *bool    `json:"useDefaultExcludes,omitempty" yaml:"useDefaultExcludes,omitempty" toml:"useDefaultExcludes,omitempty"`
	RespectGitIgnore   *bool    `json:"respectGitIgnore,omitempty" yaml:"respectGitIgnore,omitempty" toml:"respectGitIgnore,omitempty"`
	ForceExclude       bool     `json:"forceExclude,omitempty" yaml:"forceExclude,omitempty" toml:"forceExclude,omitempty"`
	FollowSymlinks     bool     `json:"followSymlinks,omitempty" yaml:"followSymlinks,omitempty" toml:"followSymlinks,omitempty"`
}

type ParsingConfig struct {
	JSON JSONParsingConfig `json:"json,omitempty" yaml:"json,omitempty" toml:"json,omitempty"`
}

type JSONParsingConfig struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty" toml:"mode,omitempty"`
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
	Enabled bool                `json:"enabled,omitempty" yaml:"enabled,omitempty" toml:"enabled,omitempty"`
	Failure string              `json:"failure,omitempty" yaml:"failure,omitempty" toml:"failure,omitempty"`
	Match   string              `json:"match,omitempty" yaml:"match,omitempty" toml:"match,omitempty"`
	Sources []CatalogSource     `json:"sources,omitempty" yaml:"sources,omitempty" toml:"sources,omitempty"`
	Ignore  []CatalogIgnoreRule `json:"ignore,omitempty" yaml:"ignore,omitempty" toml:"ignore,omitempty"`
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
	Cache          *bool    `json:"cache,omitempty" yaml:"cache,omitempty" toml:"cache,omitempty"`
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

type CatalogIgnoreRule struct {
	File   string `json:"file" yaml:"file" toml:"file"`
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty" toml:"reason,omitempty"`
}

type CompileConfig struct {
	Timeout Duration `json:"timeout,omitempty" yaml:"timeout,omitempty" toml:"timeout,omitempty"`
}

type OutputConfig struct {
	ShowSkipped  bool   `json:"showSkipped,omitempty" yaml:"showSkipped,omitempty" toml:"showSkipped,omitempty"`
	Verbose      bool   `json:"verbose,omitempty" yaml:"verbose,omitempty" toml:"verbose,omitempty"`
	Quiet        bool   `json:"quiet,omitempty" yaml:"quiet,omitempty" toml:"quiet,omitempty"`
	Locations    bool   `json:"locations,omitempty" yaml:"locations,omitempty" toml:"locations,omitempty"`
	BranchErrors string `json:"branchErrors,omitempty" yaml:"branchErrors,omitempty" toml:"branchErrors,omitempty"`
	IssueHints   string `json:"issueHints,omitempty" yaml:"issueHints,omitempty" toml:"issueHints,omitempty"`
}

type IgnoreRule struct {
	File         string `json:"file,omitempty" yaml:"file,omitempty" toml:"file,omitempty"`
	Keyword      string `json:"keyword,omitempty" yaml:"keyword,omitempty" toml:"keyword,omitempty"`
	SchemaSource string `json:"schemaSource,omitempty" yaml:"schemaSource,omitempty" toml:"schemaSource,omitempty"`
	Property     string `json:"property,omitempty" yaml:"property,omitempty" toml:"property,omitempty"`
	Reason       string `json:"reason,omitempty" yaml:"reason,omitempty" toml:"reason,omitempty"`
}

type Options struct {
	Root            string
	Config          Config
	ConfigPath      string
	ExplicitConfig  bool
	ConfigOverlay   ConfigOverlay
	SourceLocations bool
	StartedAt       time.Time
}

type ConfigOverlay func(*Config) error

type Result struct {
	Root     string       `json:"root"`
	Summary  Summary      `json:"summary"`
	Files    []FileResult `json:"files"`
	Issues   []Issue      `json:"issues,omitempty"`
	Warnings []Warning    `json:"warnings,omitempty"`
}

type InspectResult struct {
	FormatVersion int            `json:"formatVersion"`
	Root          string         `json:"root"`
	Summary       InspectSummary `json:"summary"`
	Files         []InspectFile  `json:"files"`
	Warnings      []Warning      `json:"warnings"`
}

type InspectSummary struct {
	Discovered    int      `json:"discovered"`
	Associated    int      `json:"associated"`
	Unassociated  int      `json:"unassociated"`
	Errors        int      `json:"errors"`
	Warnings      int      `json:"warnings"`
	Duration      Duration `json:"duration,omitempty"`
	DurationNanos int64    `json:"durationNanos,omitempty"`
}

type InspectFile struct {
	Path                   string       `json:"path"`
	Format                 string       `json:"format,omitempty"`
	Schema                 string       `json:"schema,omitempty"`
	SchemaSource           string       `json:"schemaSource,omitempty"`
	SchemaMatch            *SchemaMatch `json:"schemaMatch,omitempty"`
	SchemaGap              *SchemaGap   `json:"schemaGap,omitempty"`
	AssociationStatus      string       `json:"associationStatus"`
	Reason                 string       `json:"reason"`
	SuggestedAssociation   string       `json:"suggestedAssociation,omitempty"`
	SuggestedCatalogIgnore string       `json:"suggestedCatalogIgnore,omitempty"`
	Message                string       `json:"message,omitempty"`
}

type Summary struct {
	Discovered    int          `json:"discovered"`
	Validated     int          `json:"validated"`
	Skipped       int          `json:"skipped"`
	Failed        int          `json:"failed"`
	Issues        IssueSummary `json:"issues"`
	Ignored       int          `json:"ignored"`
	Warnings      int          `json:"warnings"`
	Duration      Duration     `json:"duration,omitempty"`
	DurationNanos int64        `json:"durationNanos,omitempty"`
}

type IssueSummary struct {
	Total      int `json:"total"`
	Parsing    int `json:"parsing"`
	Validation int `json:"validation"`
	Schema     int `json:"schema"`
	Coverage   int `json:"coverage"`
}

type FileResult struct {
	Path           string       `json:"path"`
	RelativePath   string       `json:"relativePath"`
	Format         string       `json:"format"`
	Schema         string       `json:"schema,omitempty"`
	SchemaSource   string       `json:"schemaSource,omitempty"`
	SchemaMatch    *SchemaMatch `json:"schemaMatch,omitempty"`
	SchemaGap      *SchemaGap   `json:"schemaGap,omitempty"`
	Status         string       `json:"status"`
	Issues         int          `json:"issues"`
	Ignored        int          `json:"ignored"`
	Message        string       `json:"message,omitempty"`
	SkipReason     string       `json:"skipReason,omitempty"`
	SkipClass      string       `json:"skipClass,omitempty"`
	SkipImportance string       `json:"skipImportance,omitempty"`
	SkipDetail     string       `json:"skipDetail,omitempty"`
}

type Issue struct {
	File             string       `json:"file"`
	RelativePath     string       `json:"relativePath"`
	Schema           string       `json:"schema,omitempty"`
	SchemaSource     string       `json:"schemaSource,omitempty"`
	SchemaMatch      *SchemaMatch `json:"schemaMatch,omitempty"`
	SchemaGap        *SchemaGap   `json:"schemaGap,omitempty"`
	Keyword          string       `json:"keyword,omitempty"`
	KeywordLocation  string       `json:"keywordLocation,omitempty"`
	Property         string       `json:"property,omitempty"`
	InstanceLocation string       `json:"instanceLocation,omitempty"`
	Line             int          `json:"line,omitempty"`
	Column           int          `json:"column,omitempty"`
	Message          string       `json:"message"`
	Hint             string       `json:"hint,omitempty"`
	IssueHint        *IssueHint   `json:"issueHint,omitempty"`
	Ignored          bool         `json:"ignored,omitempty"`
	IgnoreReason     string       `json:"ignoreReason,omitempty"`
}

type IssueHint struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Detail     string `json:"detail,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Source     string `json:"source,omitempty"`
	GroupKey   string `json:"groupKey,omitempty"`
}

type SchemaMatch struct {
	Action                 string `json:"action,omitempty"`
	Reason                 string `json:"reason,omitempty"`
	Confidence             string `json:"confidence,omitempty"`
	MatchType              string `json:"matchType,omitempty"`
	Pattern                string `json:"pattern,omitempty"`
	IgnorePattern          string `json:"ignorePattern,omitempty"`
	SuggestedAssociation   string `json:"suggestedAssociation,omitempty"`
	SuggestedCatalogIgnore string `json:"suggestedCatalogIgnore,omitempty"`
}

type SchemaGap struct {
	Name      string `json:"name"`
	Reason    string `json:"reason"`
	DocsURL   string `json:"docsUrl,omitempty"`
	FileMatch string `json:"fileMatch,omitempty"`
}

type Warning struct {
	Kind         string `json:"kind"`
	Source       string `json:"source,omitempty"`
	Path         string `json:"path,omitempty"`
	Schema       string `json:"schema,omitempty"`
	SchemaSource string `json:"schemaSource,omitempty"`
	Message      string `json:"message"`
	Hint         string `json:"hint,omitempty"`
}

func (r Result) HasIssues() bool {
	return r.Summary.Issues.Total > 0
}

func (r Result) HasWarnings() bool {
	return r.Summary.Warnings > 0
}

func NewDuration(d time.Duration) Duration {
	return Duration{Duration: d}
}
