package dollarlint

import (
	"time"

	"github.com/dollarlint/dollarlint/internal/engine"
)

const (
	StatusValidated = engine.StatusValidated
	StatusSkipped   = engine.StatusSkipped
	StatusError     = engine.StatusError
)

const (
	SkipReasonNoSchema                 = engine.SkipReasonNoSchema
	SkipReasonCatalogSchemaUnavailable = engine.SkipReasonCatalogSchemaUnavailable
)

const (
	SkipClassApplicationData   = engine.SkipClassApplicationData
	SkipClassExternalCatalog   = engine.SkipClassExternalCatalog
	SkipClassLocaleData        = engine.SkipClassLocaleData
	SkipClassLockfile          = engine.SkipClassLockfile
	SkipClassRepoManagement    = engine.SkipClassRepoManagement
	SkipClassTestData          = engine.SkipClassTestData
	SkipClassUnknown           = engine.SkipClassUnknown
	SkipClassUnsupportedConfig = engine.SkipClassUnsupportedConfig
)

const (
	SkipImportanceHigh   = engine.SkipImportanceHigh
	SkipImportanceMedium = engine.SkipImportanceMedium
	SkipImportanceLow    = engine.SkipImportanceLow
)

const (
	JSONFormatVersion   = engine.JSONFormatVersion
	BundleFormatVersion = engine.BundleFormatVersion
)

const (
	CatalogFailureWarn  = engine.CatalogFailureWarn
	CatalogFailureError = engine.CatalogFailureError
	CatalogFailureSkip  = engine.CatalogFailureSkip
)

const (
	CatalogMatchAuto = engine.CatalogMatchAuto
	CatalogMatchAll  = engine.CatalogMatchAll
)

const (
	BranchErrorsBest = engine.BranchErrorsBest
	BranchErrorsAll  = engine.BranchErrorsAll
)

const (
	ConfigModeSingle  = engine.ConfigModeSingle
	ConfigModeNearest = engine.ConfigModeNearest
)

type Config = engine.Config
type ConfigsConfig = engine.ConfigsConfig
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
type ConfigOverlay = engine.ConfigOverlay
type Result = engine.Result
type Summary = engine.Summary
type IssueSummary = engine.IssueSummary
type FileResult = engine.FileResult
type Issue = engine.Issue
type Warning = engine.Warning
type BundleOutput = engine.BundleOutput
type BundleStyledOutput = engine.BundleStyledOutput
type Duration = engine.Duration

func NewDuration(d time.Duration) Duration {
	return engine.NewDuration(d)
}
