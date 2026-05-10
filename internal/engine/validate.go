package engine

import (
	"context"
	"time"
)

func Lint(ctx context.Context, opts Options) (Result, error) {
	start := opts.StartedAt
	if start.IsZero() {
		start = time.Now()
	}
	cfg := opts.Config
	if err := validateConfigValues(cfg); err != nil {
		return Result{}, err
	}
	cfg.ApplyDefaults()
	root := opts.Root
	if root == "" {
		root = "."
	}
	mode, err := configMode(cfg.Configs)
	if err != nil {
		return Result{}, err
	}
	wantSourceLocations := opts.SourceLocations || cfg.Output.Locations
	var result Result
	if mode == ConfigModeNearest && !opts.ExplicitConfig {
		configPath := opts.ConfigPath
		if configPath == "" {
			if resolved, resolveErr := resolveConfigPath(root, ""); resolveErr != nil {
				return Result{}, resolveErr
			} else {
				configPath = resolved
			}
		}
		configuredFiles, err := discoverConfiguredFiles(root, cfg, configPath, opts.ConfigOverlay, cfg.Output)
		if err != nil {
			return Result{}, err
		}
		result, err = lintConfiguredFileGroups(ctx, root, configuredFiles, wantSourceLocations)
		if err != nil {
			return Result{}, err
		}
	} else {
		files, err := DiscoverFiles(root, cfg.Discovery)
		if err != nil {
			return Result{}, err
		}
		result, err = lintDiscoveredFiles(ctx, root, files, cfg, wantSourceLocations)
		if err != nil {
			return Result{}, err
		}
	}
	result.Summary.Duration = NewDuration(time.Since(start))
	result.Summary.DurationNanos = result.Summary.Duration.Nanoseconds()
	return result, nil
}

func lintDiscoveredFiles(ctx context.Context, root string, files []DiscoveredFile, cfg Config, wantSourceLocations bool) (Result, error) {
	result := Result{Root: root}
	cache := NewSchemaCache(cfg)
	catalogLoad := loadSchemaStoreCatalogAsync(ctx, cache, cfg)
	parsedDocuments := parseDocuments(files, cfg, wantSourceLocations)
	loadedCatalog := <-catalogLoad
	if loadedCatalog.err != nil {
		return Result{}, loadedCatalog.err
	}
	if loadedCatalog.warning != nil {
		addWarning(&result, *loadedCatalog.warning)
	}
	schemaStoreCatalog := loadedCatalog.catalog
	catalogMatch, err := catalogMatchMode(cfg.Schemas)
	if err != nil {
		return Result{}, err
	}
	documents := make([]*Document, 0, len(files))
	validatedDocuments := make([]*Document, 0, len(files))
	fileIndexes := map[string]int{}
	for _, parsed := range parsedDocuments {
		file := parsed.file
		result.Summary.Discovered++
		fileResult := FileResult{
			Path:         file.Path,
			RelativePath: file.RelativePath,
			Status:       StatusSkipped,
		}
		document := parsed.document
		if parsed.err != nil {
			if document != nil {
				fileResult.Format = document.Format
			}
			fileResult.Status = StatusError
			fileResult.Message = parsed.err.Error()
			result.Files = append(result.Files, fileResult)
			addIssue(&result, issueForError(file, "", issueKeywordParse, parsed.err))
			continue
		}
		fileResult.Format = document.Format
		fileIndexes[document.RelativePath] = len(result.Files)
		result.Files = append(result.Files, fileResult)
		documents = append(documents, document)
		applySchemaAssociation(document, cfg.Schemas.Associations, "config-association")
		applyBuiltinSchemaAssociation(document)
		applySchemaStoreAssociation(document, schemaStoreCatalog, catalogMatch)
	}
	for _, document := range documents {
		index := fileIndexes[document.RelativePath]
		result.Files[index].Schema = document.Schema
		result.Files[index].SchemaSource = document.SchemaSource
		parseIssues := issuesForDocumentParseErrors(document)
		for _, issue := range parseIssues {
			addIssue(&result, issue)
		}
		if len(parseIssues) > 0 {
			result.Files[index].Status = StatusError
			result.Files[index].Message = parseIssues[0].Message
		}
		if document.Schema == "" {
			if cfg.Schemas.RequireCoverage {
				result.Files[index].Status = StatusError
				result.Files[index].Message = "file is not covered by an inline schema, config association, built-in association, or catalog match"
				addIssue(&result, issueForMissingSchemaCoverage(document))
			} else if len(parseIssues) == 0 {
				applySkippedFileClassification(&result.Files[index], document, SkipReasonNoSchema, "")
				result.Summary.Skipped++
			}
			continue
		}
		resolved, err := resolveSchemaURI(document.Schema, document.Path)
		if err != nil {
			result.Files[index].Status = StatusError
			result.Files[index].Message = err.Error()
			addIssue(&result, issueForError(DiscoveredFile{Path: document.Path, RelativePath: document.RelativePath}, document.Schema, issueKeywordSchema, err))
			continue
		}
		document.Schema = resolved
		result.Files[index].Schema = resolved
		if len(parseIssues) == 0 {
			result.Files[index].Status = StatusValidated
		}
		if wantSourceLocations {
			AttachSourceMap(document)
		}
		if !document.isEmptyLineDelimitedDocument() {
			validatedDocuments = append(validatedDocuments, document)
		}
	}
	for _, validation := range validateDocuments(ctx, cache, cfg, validatedDocuments) {
		document := validatedDocuments[validation.index]
		index := fileIndexes[document.RelativePath]
		if validation.err != nil {
			return Result{}, validation.err
		}
		for _, warning := range validation.warnings {
			addWarning(&result, warning)
		}
		if validation.skipped {
			if result.Files[index].Status != StatusError {
				result.Files[index].Status = StatusSkipped
				result.Files[index].Message = validation.message
				applySkippedFileClassification(&result.Files[index], document, SkipReasonCatalogSchemaUnavailable, validation.message)
				result.Summary.Skipped++
			}
			continue
		}
		issues := validation.issues
		for _, issue := range issues {
			applyIgnore(&issue, cfg.Ignore)
			addIssue(&result, issue)
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

func lintConfiguredFileGroups(ctx context.Context, root string, files []configuredFile, wantSourceLocations bool) (Result, error) {
	result := Result{Root: root}
	type fileGroup struct {
		config     Config
		configPath string
		files      []DiscoveredFile
	}
	var groups []fileGroup
	groupIndexes := map[string]int{}
	for _, file := range files {
		key := file.configPath
		if key == "" {
			key = "<default>"
		}
		index, ok := groupIndexes[key]
		if !ok {
			index = len(groups)
			groupIndexes[key] = index
			groups = append(groups, fileGroup{config: file.config, configPath: file.configPath})
		}
		groups[index].files = append(groups[index].files, file.file)
	}
	for _, group := range groups {
		partial, err := lintDiscoveredFiles(ctx, root, group.files, group.config, wantSourceLocations)
		if err != nil {
			return Result{}, err
		}
		mergeResult(&result, partial)
	}
	return result, nil
}

func mergeResult(result *Result, partial Result) {
	result.Summary.Discovered += partial.Summary.Discovered
	result.Summary.Validated += partial.Summary.Validated
	result.Summary.Skipped += partial.Summary.Skipped
	result.Summary.Failed += partial.Summary.Failed
	result.Summary.Issues.Total += partial.Summary.Issues.Total
	result.Summary.Issues.Parsing += partial.Summary.Issues.Parsing
	result.Summary.Issues.Validation += partial.Summary.Issues.Validation
	result.Summary.Issues.Schema += partial.Summary.Issues.Schema
	result.Summary.Issues.Coverage += partial.Summary.Issues.Coverage
	result.Summary.Ignored += partial.Summary.Ignored
	result.Summary.Warnings += partial.Summary.Warnings
	result.Files = append(result.Files, partial.Files...)
	result.Issues = append(result.Issues, partial.Issues...)
	result.Warnings = append(result.Warnings, partial.Warnings...)
}
