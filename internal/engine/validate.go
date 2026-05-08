package engine

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"path"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// loaderFunc adapts a closure to the jsonschema.Loader interface so we can
// capture the compile context and propagate cancellation/timeout into schema
// fetches performed during compilation.
type loaderFunc func(string) (any, error)

func (f loaderFunc) Load(url string) (any, error) { return f(url) }

func Lint(ctx context.Context, opts Options) (Result, error) {
	start := time.Now()
	cfg := opts.Config
	cfg.ApplyDefaults()
	root := opts.Root
	if root == "" {
		root = "."
	}
	files, err := DiscoverFiles(root, cfg.Discovery)
	if err != nil {
		return Result{}, err
	}
	result := Result{Root: root}
	cache := NewSchemaCache(cfg)
	schemaStoreCatalog, warning, err := loadSchemaStoreCatalog(ctx, cache, cfg)
	if err != nil {
		return Result{}, err
	}
	if warning != nil {
		addWarning(&result, *warning)
	}
	documents := make([]*Document, 0, len(files))
	validatedDocuments := make([]*Document, 0, len(files))
	schemaRoots := make([]string, 0, len(files))
	fileIndexes := map[string]int{}
	for _, file := range files {
		result.Summary.Discovered++
		fileResult := FileResult{
			Path:         file.Path,
			RelativePath: file.RelativePath,
			Status:       StatusSkipped,
		}
		document, err := ParseDocument(file)
		if err != nil {
			fileResult.Status = StatusError
			fileResult.Message = err.Error()
			result.Files = append(result.Files, fileResult)
			addIssue(&result, issueForError(file, "", err))
			continue
		}
		fileResult.Format = document.Format
		fileIndexes[document.RelativePath] = len(result.Files)
		result.Files = append(result.Files, fileResult)
		documents = append(documents, document)
		applySchemaAssociation(document, cfg.Schemas.Associations, "config-association")
		applySchemaStoreAssociation(document, schemaStoreCatalog)
	}
	for _, document := range documents {
		index := fileIndexes[document.RelativePath]
		result.Files[index].Schema = document.Schema
		result.Files[index].SchemaSource = document.SchemaSource
		if document.Schema == "" {
			result.Summary.Skipped++
			continue
		}
		resolved, err := resolveSchemaURI(document.Schema, document.Path)
		if err != nil {
			result.Files[index].Status = StatusError
			result.Files[index].Message = err.Error()
			addIssue(&result, issueForError(DiscoveredFile{Path: document.Path, RelativePath: document.RelativePath}, document.Schema, err))
			continue
		}
		document.Schema = resolved
		result.Files[index].Schema = resolved
		result.Files[index].Status = StatusValidated
		if opts.SourceLocations || cfg.Output.Locations {
			AttachSourceMap(document)
		}
		validatedDocuments = append(validatedDocuments, document)
		schemaRoots = append(schemaRoots, resolved)
	}
	// Best-effort warmup of referenced schemas. Per-document Prime in
	// validateDocument surfaces actual errors, so any failure here is ignored.
	_ = cache.Prime(ctx, primeableSchemaRoots(cfg, validatedDocuments, schemaRoots))
	for _, document := range validatedDocuments {
		issues := validateDocument(ctx, cache, cfg, document)
		for _, issue := range issues {
			applyIgnore(&issue, cfg.Ignore)
			addIssue(&result, issue)
			index := fileIndexes[document.RelativePath]
			if issue.Ignored {
				result.Files[index].Ignored++
			} else {
				result.Files[index].Issues++
			}
		}
		result.Summary.Validated++
	}
	for i := range result.Files {
		if result.Files[i].Status == StatusError {
			result.Summary.Failed++
		}
	}
	result.Summary.Duration = NewDuration(time.Since(start))
	result.Summary.DurationNanos = result.Summary.Duration.Nanoseconds()
	return result, nil
}

func addWarning(result *Result, warning Warning) {
	result.Warnings = append(result.Warnings, warning)
	result.Summary.Warnings++
}

func applySchemaAssociation(document *Document, associations []SchemaAssociation, source string) {
	if document.Schema != "" {
		return
	}
	for _, association := range associations {
		if association.File == "" || association.Schema == "" {
			continue
		}
		if matchPattern(association.File, document.RelativePath) {
			document.Schema = association.Schema
			document.SchemaSource = source
			return
		}
	}
}

func validateDocument(ctx context.Context, cache *SchemaCache, cfg Config, document *Document) []Issue {
	if err := cache.Prime(ctx, primeableDocumentSchemaRoots(cfg, document)); err != nil {
		return []Issue{{
			File:         document.Path,
			RelativePath: document.RelativePath,
			Schema:       document.Schema,
			Message:      err.Error(),
		}}
	}
	schema, err := compileSchema(ctx, cache, cfg, document.Schema, document.Data)
	if err != nil {
		return []Issue{{
			File:         document.Path,
			RelativePath: document.RelativePath,
			Schema:       document.Schema,
			Message:      fmt.Sprintf("compile schema: %v", err),
		}}
	}
	if err := schema.Validate(document.Data); err != nil {
		return issuesFromSchemaError(document, err)
	}
	return nil
}

func issuesFromSchemaError(document *Document, err error) []Issue {
	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) {
		return issuesFromValidationError(document, validationErr)
	}
	return []Issue{{
		File:         document.Path,
		RelativePath: document.RelativePath,
		Schema:       document.Schema,
		Message:      err.Error(),
	}}
}

func compileSchema(ctx context.Context, cache *SchemaCache, cfg Config, schemaURI string, documentData any) (*jsonschema.Schema, error) {
	compileCtx, cancel := context.WithTimeout(ctx, cfg.Schemas.Compile.Timeout.Duration)
	defer cancel()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseRegexpEngine(compileECMARegexp)
	compiler.UseLoader(loaderFunc(func(u string) (any, error) {
		return cache.LoadContext(compileCtx, u)
	}))
	refs := collectAzureARMResourceRefs(documentData)
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

func issuesFromValidationError(document *Document, err *jsonschema.ValidationError) []Issue {
	return collectValidationIssues(document, err, nil)
}

func collectValidationIssues(document *Document, err *jsonschema.ValidationError, issues []Issue) []Issue {
	if len(err.Causes) > 0 {
		for _, cause := range err.Causes {
			issues = collectValidationIssues(document, cause, issues)
		}
		return issues
	}
	base := Issue{
		File:             document.Path,
		RelativePath:     document.RelativePath,
		Schema:           document.Schema,
		Keyword:          keywordName(err),
		KeywordLocation:  keywordLocation(err),
		Property:         propertyName(err),
		InstanceLocation: instanceLocation(err.InstanceLocation),
		Message:          validationMessage(err),
	}
	applyIssuePosition(document, &base)
	switch typed := err.ErrorKind.(type) {
	case *kind.Required:
		for _, missing := range typed.Missing {
			issue := base
			issue.Property = missing
			issue.Message = fmt.Sprintf("must have required property %q", missing)
			issues = append(issues, issue)
		}
	case *kind.AdditionalProperties:
		for _, property := range typed.Properties {
			issue := base
			issue.Property = property
			issue.InstanceLocation = joinPointer(issue.InstanceLocation, property)
			issue.Line = 0
			issue.Column = 0
			applyIssuePosition(document, &issue)
			issue.Message = fmt.Sprintf("must not have additional property %q", property)
			issues = append(issues, issue)
		}
	default:
		issues = append(issues, base)
	}
	return issues
}

func applyIssuePosition(document *Document, issue *Issue) {
	if issue.Line > 0 && issue.Column > 0 {
		return
	}
	pointer := issue.InstanceLocation
	if pointer == "" {
		pointer = "/"
	}
	if pos, ok := document.SourceMap.Position(pointer); ok {
		issue.Line = pos.Line
		issue.Column = pos.Column
	}
}

func keywordName(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return ""
	}
	keywordPath := err.ErrorKind.KeywordPath()
	if len(keywordPath) == 0 {
		return ""
	}
	return keywordPath[0]
}

func keywordLocation(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return ""
	}
	return "/" + strings.Join(err.ErrorKind.KeywordPath(), "/")
}

func propertyName(err *jsonschema.ValidationError) string {
	if name := propertyFromKind(err.ErrorKind); name != "" {
		return name
	}
	if len(err.InstanceLocation) == 0 {
		return ""
	}
	return err.InstanceLocation[len(err.InstanceLocation)-1]
}

func propertyFromKind(errorKind jsonschema.ErrorKind) string {
	switch typed := errorKind.(type) {
	case *kind.PropertyNames:
		return typed.Property
	case *kind.Dependency:
		return typed.Prop
	case *kind.DependentRequired:
		return typed.Prop
	default:
		return ""
	}
}

func instanceLocation(parts []string) string {
	if len(parts) == 0 {
		return "/"
	}
	return "/" + path.Join(parts...)
}

func validationMessage(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return "validation failed"
	}
	switch typed := err.ErrorKind.(type) {
	case *kind.Type:
		return fmt.Sprintf("expected %s, received %s", strings.Join(typed.Want, " or "), typed.Got)
	case *kind.MinProperties:
		return fmt.Sprintf("must have at least %d %s", typed.Want, countNoun(typed.Want, "property", "properties"))
	case *kind.MaxProperties:
		return fmt.Sprintf("must have at most %d %s", typed.Want, countNoun(typed.Want, "property", "properties"))
	case *kind.MinItems:
		return fmt.Sprintf("must have at least %d %s", typed.Want, countNoun(typed.Want, "item", "items"))
	case *kind.MaxItems:
		return fmt.Sprintf("must have at most %d %s", typed.Want, countNoun(typed.Want, "item", "items"))
	case *kind.MinLength:
		return fmt.Sprintf("must be at least %d characters", typed.Want)
	case *kind.MaxLength:
		return fmt.Sprintf("must be at most %d characters", typed.Want)
	case *kind.Minimum:
		return fmt.Sprintf("must be >= %s", ratString(typed.Want))
	case *kind.Maximum:
		return fmt.Sprintf("must be <= %s", ratString(typed.Want))
	case *kind.ExclusiveMinimum:
		return fmt.Sprintf("must be > %s", ratString(typed.Want))
	case *kind.ExclusiveMaximum:
		return fmt.Sprintf("must be < %s", ratString(typed.Want))
	case *kind.MultipleOf:
		return fmt.Sprintf("must be a multiple of %s", ratString(typed.Want))
	}
	return err.BasicOutput().Error.String()
}

func countNoun(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func ratString(value *big.Rat) string {
	if value == nil {
		return "0"
	}
	if value.IsInt() {
		return value.Num().String()
	}
	return strings.TrimRight(strings.TrimRight(value.FloatString(6), "0"), ".")
}

func issueForError(file DiscoveredFile, schema string, err error) Issue {
	return Issue{
		File:         file.Path,
		RelativePath: file.RelativePath,
		Schema:       schema,
		Message:      err.Error(),
	}
}

func addIssue(result *Result, issue Issue) {
	result.Issues = append(result.Issues, issue)
	if issue.Ignored {
		result.Summary.Ignored++
		return
	}
	result.Summary.Issues++
}
