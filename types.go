package dollarlint

import (
	"time"

	"github.com/agorischek/dollarlint/internal/engine"
)

const (
	StatusValidated = engine.StatusValidated
	StatusSkipped   = engine.StatusSkipped
	StatusError     = engine.StatusError
)

const (
	SchemaStoreFailureWarn  = engine.SchemaStoreFailureWarn
	SchemaStoreFailureError = engine.SchemaStoreFailureError
	SchemaStoreFailureSkip  = engine.SchemaStoreFailureSkip
)

type Config = engine.Config
type DiscoveryConfig = engine.DiscoveryConfig
type SchemaConfig = engine.SchemaConfig
type SchemaStoreConfig = engine.SchemaStoreConfig
type CatalogConfig = engine.CatalogConfig
type CatalogSource = engine.CatalogSource
type FetchConfig = engine.FetchConfig
type SchemaAssociation = engine.SchemaAssociation
type TimeoutConfig = engine.TimeoutConfig
type OutputConfig = engine.OutputConfig
type IgnoreRule = engine.IgnoreRule
type Options = engine.Options
type Result = engine.Result
type Summary = engine.Summary
type FileResult = engine.FileResult
type Issue = engine.Issue
type Warning = engine.Warning
type Duration = engine.Duration

func NewDuration(d time.Duration) Duration {
	return engine.NewDuration(d)
}
