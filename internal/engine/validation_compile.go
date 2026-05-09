package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// loaderFunc adapts a closure to the jsonschema.Loader interface so we can
// capture the compile context and propagate cancellation/timeout into schema
// fetches performed during compilation.
type loaderFunc func(string) (any, error)

func (f loaderFunc) Load(url string) (any, error) { return f(url) }

// compiledSchemaEntry serializes compilations for a schema URI and caches
// successful results for the lifetime of a single Lint pass.
type compiledSchemaEntry struct {
	mu     sync.Mutex
	loaded bool
	schema *jsonschema.Schema
}

func (c *SchemaCache) compiledEntry(key string) *compiledSchemaEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.compiled[key]
	if entry == nil {
		entry = &compiledSchemaEntry{}
		c.compiled[key] = entry
	}
	return entry
}

func validateDocument(ctx context.Context, cache *SchemaCache, cfg Config, document *Document) documentValidation {
	if document.isLineDelimited() {
		return validateLineDelimitedDocument(ctx, cache, cfg, document)
	}
	if err := cache.Prime(ctx, primeableDocumentSchemaRoots(cfg, document)); err != nil {
		return validationForSchemaFailure(document, cfg, "load", err)
	}
	schema, err := compileSchema(ctx, cache, cfg, document.Schema, document.Data)
	if err != nil {
		return validationForSchemaFailure(document, cfg, "compile", err)
	}
	if err := schema.Validate(document.Data); err != nil {
		return documentValidation{issues: issuesFromSchemaError(document, err, cfg.Output)}
	}
	return documentValidation{}
}

func validateLineDelimitedDocument(ctx context.Context, cache *SchemaCache, cfg Config, document *Document) documentValidation {
	if err := cache.Prime(ctx, primeableDocumentSchemaRoots(cfg, document)); err != nil {
		return validationForSchemaFailure(document, cfg, "load", err)
	}
	schema, err := compileSchema(ctx, cache, cfg, document.Schema, nil)
	if err != nil {
		return validationForSchemaFailure(document, cfg, "compile", err)
	}
	var issues []Issue
	for _, lineDocument := range document.LineDocuments {
		instance := *document
		instance.Data = lineDocument.Data
		instance.SourceMap = lineDocument.SourceMap
		if err := schema.Validate(lineDocument.Data); err != nil {
			issues = append(issues, issuesFromSchemaError(&instance, err, cfg.Output)...)
		}
	}
	return documentValidation{issues: issues}
}

func validationForSchemaFailure(document *Document, cfg Config, phase string, err error) documentValidation {
	message := fmt.Sprintf("schema %s failed for %s: %v", phase, document.Schema, err)
	if !isCatalogSchemaSource(document.SchemaSource) {
		return documentValidation{issues: []Issue{{
			File:         document.Path,
			RelativePath: document.RelativePath,
			Schema:       document.Schema,
			Keyword:      issueKeywordSchema,
			Message:      message,
		}}}
	}
	mode, modeErr := catalogFailureMode(cfg.Schemas)
	if modeErr != nil {
		return documentValidation{err: modeErr}
	}
	catalogMessage := fmt.Sprintf("catalog schema %s failed for %s using %s: %v", phase, document.RelativePath, document.Schema, err)
	switch mode {
	case CatalogFailureError:
		return documentValidation{err: fmt.Errorf("%s", catalogMessage)}
	case CatalogFailureSkip:
		return documentValidation{skipped: true, message: fmt.Sprintf("catalog schema %s failed; skipped catalog-inferred validation", phase)}
	default:
		return documentValidation{
			warnings: []Warning{{
				Kind:    "schemaCatalogSchemaUnavailable",
				Source:  document.SchemaSource,
				Message: catalogMessage + "; skipped catalog-inferred validation",
			}},
			skipped: true,
			message: fmt.Sprintf("catalog schema %s failed; skipped catalog-inferred validation", phase),
		}
	}
}

func isCatalogSchemaSource(source string) bool {
	return source == "catalog" || strings.HasPrefix(source, "catalog:")
}

func compileSchema(ctx context.Context, cache *SchemaCache, cfg Config, schemaURI string, documentData any) (*jsonschema.Schema, error) {
	refs := collectAzureARMResourceRefs(documentData)
	if shouldPruneRefs(cfg, schemaURI, refs) {
		return compileSchemaUncached(ctx, cache, cfg, schemaURI, refs)
	}
	entry := cache.compiledEntry(schemaURI)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.loaded {
		return entry.schema, nil
	}
	schema, err := compileSchemaUncached(ctx, cache, cfg, schemaURI, nil)
	if err != nil {
		return nil, err
	}
	entry.schema = schema
	entry.loaded = true
	return schema, nil
}

func compileSchemaUncached(ctx context.Context, cache *SchemaCache, cfg Config, schemaURI string, refs []azureARMResourceRef) (*jsonschema.Schema, error) {
	compileCtx, cancel := context.WithTimeout(ctx, cfg.Schemas.Compile.Timeout.Duration)
	defer cancel()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseRegexpEngine(compileECMARegexp)
	compiler.UseLoader(loaderFunc(func(u string) (any, error) {
		return cache.LoadContext(compileCtx, u)
	}))
	if err := addPrunedAzureARMResourcesWithRefs(compileCtx, compiler, cache, cfg, schemaURI, refs); err != nil {
		return nil, err
	}
	type compileResult struct {
		schema *jsonschema.Schema
		err    error
	}
	resultCh := make(chan compileResult, 1)
	go func() {
		schema, err := compiler.Compile(schemaURI)
		resultCh <- compileResult{schema: schema, err: err}
	}()
	select {
	case <-compileCtx.Done():
		return nil, compileCtx.Err()
	case compiled := <-resultCh:
		return compiled.schema, compiled.err
	}
}
