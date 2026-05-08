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
	CatalogFailureWarn  = engine.CatalogFailureWarn
	CatalogFailureError = engine.CatalogFailureError
	CatalogFailureSkip  = engine.CatalogFailureSkip
)

type Config = engine.Config
type DiscoveryConfig = engine.DiscoveryConfig
type SchemaConfig = engine.SchemaConfig
type CatalogConfig = engine.CatalogConfig
type CatalogSource = engine.CatalogSource
type OptimizationConfig = engine.OptimizationConfig
type AzureOptimization = engine.AzureOptimization
type FetchConfig = engine.FetchConfig
type SchemaAssociation = engine.SchemaAssociation
type CompileConfig = engine.CompileConfig
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
