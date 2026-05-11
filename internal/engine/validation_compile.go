package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	message := schemaFailureMessage(document, phase, err)
	if !isCatalogSchemaSource(document.SchemaSource) {
		if phase == "compile" && isRemoteURI(document.Schema) {
			return documentValidation{
				schemaWarning: newSchemaSourceFailureWarning(
					"schemaRemoteSchemaUnavailable",
					schemaFailureSource(document),
					document.Schema,
					document.SchemaSource,
					phase,
					document.RelativePath,
					schemaSourceFailureDetail("remote schema", phase, document.Schema, err),
				),
				skipped:    true,
				skipReason: SkipReasonSchemaUnavailable,
				message:    schemaSourceSkippedMessage(phase),
			}
		}
		issue := Issue{
			File:         document.Path,
			RelativePath: document.RelativePath,
			Schema:       document.Schema,
			Keyword:      issueKeywordSchema,
			Message:      message,
		}
		applySchemaFailureIssueHint(document, &issue, err, cfg.Output)
		return documentValidation{issues: []Issue{issue}}
	}
	mode, modeErr := catalogFailureMode(cfg.Schemas)
	if modeErr != nil {
		return documentValidation{err: modeErr}
	}
	catalogMessage := schemaSourceFailureDetail("catalog schema", phase, document.Schema, err)
	switch mode {
	case CatalogFailureError:
		return documentValidation{err: fmt.Errorf("%s", catalogMessage)}
	case CatalogFailureSkip:
		return documentValidation{skipped: true, skipReason: SkipReasonCatalogSchemaUnavailable, message: catalogSchemaSkippedMessage(phase)}
	default:
		return documentValidation{
			schemaWarning: newSchemaSourceFailureWarning(
				"schemaCatalogSchemaUnavailable",
				document.SchemaSource,
				document.Schema,
				document.SchemaSource,
				phase,
				document.RelativePath,
				catalogMessage,
			),
			skipped:    true,
			skipReason: SkipReasonCatalogSchemaUnavailable,
			message:    catalogSchemaSkippedMessage(phase),
		}
	}
}

func catalogSchemaSkippedMessage(phase string) string {
	return fmt.Sprintf("catalog schema could not be used; skipped catalog-inferred validation after schema %s failure", phase)
}

func schemaSourceSkippedMessage(phase string) string {
	return fmt.Sprintf("remote schema could not be used; skipped validation after schema %s failure", phase)
}

func isCatalogSchemaSource(source string) bool {
	return source == "catalog" || strings.HasPrefix(source, "catalog:")
}

func schemaFailureSource(document *Document) string {
	if document != nil && document.SchemaSource != "" {
		return document.SchemaSource
	}
	return "remote-schema"
}

func schemaSourceFailureDetail(label, phase, schema string, err error) string {
	return fmt.Sprintf("%s %s failed for %s: %v", label, phase, schema, err)
}

func schemaFailureMessage(document *Document, phase string, err error) string {
	if info, ok := classifyLocalSchemaFailure(document, err); ok {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Sprintf("schema %s failed: local schema file not found at %s", phase, info.DisplayPath)
		}
		return fmt.Sprintf("schema %s failed: local schema file could not be read at %s", phase, info.DisplayPath)
	}
	schema := ""
	if document != nil {
		schema = document.Schema
	}
	return fmt.Sprintf("schema %s failed for %s: %v", phase, schema, err)
}

type schemaSourceFailureWarning struct {
	Kind         string
	Source       string
	Schema       string
	SchemaSource string
	Phase        string
	Path         string
	Detail       string
}

func newSchemaSourceFailureWarning(kind, source, schema, schemaSource, phase, path, detail string) *schemaSourceFailureWarning {
	return &schemaSourceFailureWarning{
		Kind:         kind,
		Source:       source,
		Schema:       schema,
		SchemaSource: schemaSource,
		Phase:        phase,
		Path:         path,
		Detail:       detail,
	}
}

type schemaSourceFailureWarningGroup struct {
	Kind         string
	Source       string
	Schema       string
	SchemaSource string
	Phase        string
	Detail       string
	Paths        []string
}

type schemaSourceFailureWarningGroups struct {
	indexes map[string]int
	groups  []schemaSourceFailureWarningGroup
}

func (groups *schemaSourceFailureWarningGroups) add(warning schemaSourceFailureWarning) {
	if groups.indexes == nil {
		groups.indexes = map[string]int{}
	}
	key := strings.Join([]string{
		warning.Kind,
		warning.Source,
		warning.Schema,
		warning.SchemaSource,
		warning.Phase,
		warning.Detail,
	}, "\x00")
	index, ok := groups.indexes[key]
	if !ok {
		index = len(groups.groups)
		groups.indexes[key] = index
		groups.groups = append(groups.groups, schemaSourceFailureWarningGroup{
			Kind:         warning.Kind,
			Source:       warning.Source,
			Schema:       warning.Schema,
			SchemaSource: warning.SchemaSource,
			Phase:        warning.Phase,
			Detail:       warning.Detail,
		})
	}
	if warning.Path != "" {
		groups.groups[index].Paths = append(groups.groups[index].Paths, warning.Path)
	}
}

func (groups schemaSourceFailureWarningGroups) warnings() []Warning {
	out := make([]Warning, 0, len(groups.groups))
	for _, group := range groups.groups {
		paths := append([]string(nil), group.Paths...)
		sort.Strings(paths)
		out = append(out, Warning{
			Kind:         group.Kind,
			Source:       group.Source,
			Schema:       group.Schema,
			SchemaSource: group.SchemaSource,
			Message:      schemaSourceFailureWarningMessage(group.Kind, group.Phase, group.Schema, paths),
			Hint:         schemaSourceFailureWarningHint(group.Detail, paths),
		})
	}
	return out
}

func schemaSourceFailureWarningMessage(kind, phase, schema string, paths []string) string {
	subject := "Remote schema"
	use := "validation"
	if kind == "schemaCatalogSchemaUnavailable" {
		subject = "Catalog schema"
		use = "catalog-inferred validation"
	}
	return fmt.Sprintf("%s could not be used for %s. The schema %s failed to %s, so DollarLint skipped %s for files that use it; this is not a finding in those files.", subject, affectedFilesLabel(paths), schema, phase, use)
}

func schemaSourceFailureWarningHint(detail string, paths []string) string {
	hint := ""
	if len(paths) > 0 {
		hint = "Affected files: " + limitedPathList(paths, 8) + "."
	}
	if detail != "" {
		if hint != "" {
			hint += " "
		}
		hint += "Technical details: " + detail
	}
	return hint
}

func affectedFilesLabel(paths []string) string {
	switch len(paths) {
	case 0:
		return "files that use it"
	case 1:
		return paths[0]
	default:
		return fmt.Sprintf("%d files", len(paths))
	}
}

func limitedPathList(paths []string, limit int) string {
	if len(paths) <= limit {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(paths[:limit], ", "), len(paths)-limit)
}

func applySchemaFailureIssueHint(document *Document, issue *Issue, err error, output OutputConfig) {
	if issue == nil || issue.Hint != "" {
		return
	}
	if issueHintsEnabled(output) {
		if hint, ok := schemaFailureIssueHint(document, err); ok {
			applyIssueHint(issue, hint)
			return
		}
	}
	issue.Hint = legacyHintForSchemaFailure(document, err)
}

type localSchemaFailureInfo struct {
	DisplayPath string
	GroupPath   string
	Dependency  bool
}

func schemaFailureIssueHint(document *Document, err error) (IssueHint, bool) {
	info, ok := classifyLocalSchemaFailure(document, err)
	if !ok {
		return IssueHint{}, false
	}
	if info.Dependency {
		return IssueHint{
			ID:         "schema.local-dependency-unavailable",
			Title:      "Dependency-local schema is unavailable.",
			Detail:     fmt.Sprintf("The file's $schema points at %s, but that schema file was not present in this checkout.", info.DisplayPath),
			Suggestion: "Treat this as dependency-prep unavailable first: install project dependencies before validating, or exclude files whose tool schemas are unavailable.",
			Confidence: IssueHintConfidenceHigh,
			GroupKey:   "schema:local-dependency-unavailable:" + info.GroupPath,
		}, true
	}
	return IssueHint{
		ID:         "schema.local-schema-unavailable",
		Title:      "Local schema file is unavailable.",
		Detail:     fmt.Sprintf("The file's $schema points at %s, but DollarLint could not read that local schema file.", info.DisplayPath),
		Suggestion: "Check the $schema path, generate the local schema if the project creates it, or exclude files whose local schemas are unavailable.",
		Confidence: IssueHintConfidenceHigh,
		GroupKey:   "schema:local-schema-unavailable:" + info.GroupPath,
	}, true
}

func classifyLocalSchemaFailure(document *Document, err error) (localSchemaFailureInfo, bool) {
	if !errors.Is(err, os.ErrNotExist) {
		return localSchemaFailureInfo{}, false
	}
	documentPath := ""
	if document != nil {
		documentPath = document.Path
	}
	var readErr *schemaFileReadError
	ok := errors.As(err, &readErr)
	if !ok {
		return localSchemaFailureInfo{}, false
	}
	displayPath, groupPath, dependency := displayLocalSchemaPath(readErr.Path, documentPath)
	return localSchemaFailureInfo{DisplayPath: displayPath, GroupPath: groupPath, Dependency: dependency}, true
}

func legacyHintForSchemaFailure(document *Document, err error) string {
	info, ok := classifyLocalSchemaFailure(document, err)
	if !ok {
		return ""
	}
	if info.Dependency {
		return "The referenced local schema file is missing. If a project dependency provides it, install dependencies before validating, or exclude files whose tool schemas are unavailable."
	}
	return "The referenced local schema file is missing. Check the $schema path, install the dependency that provides it, or exclude files whose local schemas are unavailable."
}

func displayLocalSchemaPath(schemaPath, documentPath string) (string, string, bool) {
	cleanPath := filepath.Clean(schemaPath)
	slashPath := filepath.ToSlash(cleanPath)
	if display, ok := trimToNodeModulesPath(slashPath); ok {
		return display, display, true
	}
	if documentPath != "" {
		if rel, err := filepath.Rel(filepath.Dir(documentPath), cleanPath); err == nil && rel != "." {
			return filepath.ToSlash(rel), slashPath, false
		}
	}
	return slashPath, slashPath, false
}

func trimToNodeModulesPath(slashPath string) (string, bool) {
	if slashPath == "node_modules" || strings.HasPrefix(slashPath, "node_modules/") {
		return slashPath, true
	}
	const segment = "/node_modules/"
	index := strings.Index(slashPath, segment)
	if index < 0 {
		return "", false
	}
	return slashPath[index+1:], true
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
