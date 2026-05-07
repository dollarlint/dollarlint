package dollarlint

import "time"

const (
	StatusValidated = "validated"
	StatusSkipped   = "skipped"
	StatusError     = "error"
)

type Config struct {
	Version   int             `json:"version,omitempty" yaml:"version,omitempty" toml:"version,omitempty"`
	Discovery DiscoveryConfig `json:"discovery,omitempty" yaml:"discovery,omitempty" toml:"discovery,omitempty"`
	Schema    SchemaConfig    `json:"schema,omitempty" yaml:"schema,omitempty" toml:"schema,omitempty"`
	Timeouts  TimeoutConfig   `json:"timeouts,omitempty" yaml:"timeouts,omitempty" toml:"timeouts,omitempty"`
	Ignore    []IgnoreRule    `json:"ignore,omitempty" yaml:"ignore,omitempty" toml:"ignore,omitempty"`
	Output    OutputConfig    `json:"output,omitempty" yaml:"output,omitempty" toml:"output,omitempty"`
}

type DiscoveryConfig struct {
	Include        []string `json:"include,omitempty" yaml:"include,omitempty" toml:"include,omitempty"`
	Exclude        []string `json:"exclude,omitempty" yaml:"exclude,omitempty" toml:"exclude,omitempty"`
	FollowSymlinks bool     `json:"followSymlinks,omitempty" yaml:"followSymlinks,omitempty" toml:"followSymlinks,omitempty"`
}

type SchemaConfig struct {
	Associations []SchemaAssociation `json:"associations,omitempty" yaml:"associations,omitempty" toml:"associations,omitempty"`
	MaxDepth     int                 `json:"maxDepth,omitempty" yaml:"maxDepth,omitempty" toml:"maxDepth,omitempty"`
	FetchRemote  *bool               `json:"fetchRemote,omitempty" yaml:"fetchRemote,omitempty" toml:"fetchRemote,omitempty"`
	Concurrency  int                 `json:"concurrency,omitempty" yaml:"concurrency,omitempty" toml:"concurrency,omitempty"`
}

type SchemaAssociation struct {
	File   string `json:"file" yaml:"file" toml:"file"`
	Schema string `json:"schema" yaml:"schema" toml:"schema"`
}

type TimeoutConfig struct {
	Fetch   Duration `json:"fetch,omitempty" yaml:"fetch,omitempty" toml:"fetch,omitempty"`
	Compile Duration `json:"compile,omitempty" yaml:"compile,omitempty" toml:"compile,omitempty"`
}

type OutputConfig struct {
	JSON        bool `json:"json,omitempty" yaml:"json,omitempty" toml:"json,omitempty"`
	ShowSkipped bool `json:"showSkipped,omitempty" yaml:"showSkipped,omitempty" toml:"showSkipped,omitempty"`
}

type IgnoreRule struct {
	File     string `json:"file,omitempty" yaml:"file,omitempty" toml:"file,omitempty"`
	Keyword  string `json:"keyword,omitempty" yaml:"keyword,omitempty" toml:"keyword,omitempty"`
	Property string `json:"property,omitempty" yaml:"property,omitempty" toml:"property,omitempty"`
	Reason   string `json:"reason,omitempty" yaml:"reason,omitempty" toml:"reason,omitempty"`
}

type Options struct {
	Root   string
	Config Config
}

type Result struct {
	Root    string       `json:"root"`
	Summary Summary      `json:"summary"`
	Files   []FileResult `json:"files"`
	Issues  []Issue      `json:"issues,omitempty"`
}

type Summary struct {
	Discovered int `json:"discovered"`
	Validated  int `json:"validated"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	Issues     int `json:"issues"`
	Ignored    int `json:"ignored"`
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
	Message          string `json:"message"`
	Ignored          bool   `json:"ignored,omitempty"`
	IgnoreReason     string `json:"ignoreReason,omitempty"`
}

func (r Result) HasIssues() bool {
	return r.Summary.Issues > 0
}

func NewDuration(d time.Duration) Duration {
	return Duration{Duration: d}
}
