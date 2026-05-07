package dollarlint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

type compilerLoader struct {
	cache *SchemaCache
}

func (l compilerLoader) Load(url string) (any, error) {
	return l.cache.Load(url)
}

func Lint(ctx context.Context, opts Options) (Result, error) {
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
	documents := make([]*Document, 0, len(files))
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
		applySchemaAssociation(document, cfg.Schema.Associations)
		fileResult.Schema = document.Schema
		fileResult.SchemaSource = document.SchemaSource
		if document.Schema == "" {
			result.Summary.Skipped++
			result.Files = append(result.Files, fileResult)
			continue
		}
		resolved, err := resolveSchemaURI(document.Schema, document.Path)
		if err != nil {
			fileResult.Status = StatusError
			fileResult.Message = err.Error()
			result.Files = append(result.Files, fileResult)
			addIssue(&result, issueForError(file, document.Schema, err))
			continue
		}
		document.Schema = resolved
		fileResult.Schema = resolved
		fileResult.Status = StatusValidated
		fileIndexes[document.RelativePath] = len(result.Files)
		result.Files = append(result.Files, fileResult)
		documents = append(documents, document)
		schemaRoots = append(schemaRoots, resolved)
	}
	_ = cache.Prime(ctx, schemaRoots)
	for _, document := range documents {
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
	return result, nil
}

func applySchemaAssociation(document *Document, associations []SchemaAssociation) {
	if document.Schema != "" {
		return
	}
	for _, association := range associations {
		if association.File == "" || association.Schema == "" {
			continue
		}
		if matchPattern(association.File, document.RelativePath) {
			document.Schema = association.Schema
			document.SchemaSource = "config-association"
			return
		}
	}
}

func validateDocument(ctx context.Context, cache *SchemaCache, cfg Config, document *Document) []Issue {
	if err := cache.Prime(ctx, []string{document.Schema}); err != nil {
		return []Issue{{
			File:         document.Path,
			RelativePath: document.RelativePath,
			Schema:       document.Schema,
			Message:      err.Error(),
		}}
	}
	schema, err := compileSchema(ctx, cache, cfg, document.Schema)
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

func compileSchema(ctx context.Context, cache *SchemaCache, cfg Config, schemaURI string) (*jsonschema.Schema, error) {
	compileCtx, cancel := context.WithTimeout(ctx, cfg.Timeouts.Compile.Duration)
	defer cancel()
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(compilerLoader{cache: cache})
	result := make(chan struct {
		schema *jsonschema.Schema
		err    error
	}, 1)
	go func() {
		schema, err := compiler.Compile(schemaURI)
		result <- struct {
			schema *jsonschema.Schema
			err    error
		}{schema: schema, err: err}
	}()
	select {
	case <-compileCtx.Done():
		return nil, compileCtx.Err()
	case compiled := <-result:
		return compiled.schema, compiled.err
	}
}

func issuesFromValidationError(document *Document, err *jsonschema.ValidationError) []Issue {
	var issues []Issue
	collectValidationIssues(document, err, &issues)
	return issues
}

func collectValidationIssues(document *Document, err *jsonschema.ValidationError, issues *[]Issue) {
	if len(err.Causes) > 0 {
		for _, cause := range err.Causes {
			collectValidationIssues(document, cause, issues)
		}
		return
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
			issue.Message = fmt.Sprintf("missing property %q", missing)
			*issues = append(*issues, issue)
		}
	case *kind.AdditionalProperties:
		for _, property := range typed.Properties {
			issue := base
			issue.Property = property
			issue.InstanceLocation = joinPointer(issue.InstanceLocation, property)
			applyIssuePosition(document, &issue)
			issue.Message = fmt.Sprintf("additional property %q not allowed", property)
			*issues = append(*issues, issue)
		}
	default:
		*issues = append(*issues, base)
	}
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
		return
	}
	issue.Line = 1
	issue.Column = 1
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
	return err.BasicOutput().Error.String()
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

func (r Result) MarshalJSON() ([]byte, error) {
	type alias Result
	return json.Marshal(alias(r))
}
