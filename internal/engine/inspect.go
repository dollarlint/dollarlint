package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func Inspect(ctx context.Context, opts Options) (InspectResult, error) {
	start := opts.StartedAt
	if start.IsZero() {
		start = time.Now()
	}
	cfg := opts.Config
	if err := validateConfigValues(cfg); err != nil {
		return InspectResult{}, err
	}
	cfg.ApplyDefaults()
	root := opts.Root
	if root == "" {
		root = "."
	}
	mode, _ := configMode(cfg.Configs)
	var result InspectResult
	if mode == ConfigModeNearest && !opts.ExplicitConfig {
		configPath := opts.ConfigPath
		if configPath == "" {
			if resolved, resolveErr := resolveConfigPath(root, ""); resolveErr != nil {
				return InspectResult{}, resolveErr
			} else {
				configPath = resolved
			}
		}
		configuredFiles, err := discoverConfiguredFiles(root, cfg, configPath, opts.ConfigOverlay, cfg.Output)
		if err != nil {
			return InspectResult{}, err
		}
		result, err = inspectConfiguredFileGroups(ctx, root, configuredFiles)
		if err != nil {
			return InspectResult{}, err
		}
	} else {
		files, err := DiscoverFiles(root, cfg.Discovery)
		if err != nil {
			return InspectResult{}, err
		}
		result, err = inspectDiscoveredFiles(ctx, root, files, cfg)
		if err != nil {
			return InspectResult{}, err
		}
	}
	result.FormatVersion = InspectFormatVersion
	result.Summary.Duration = NewDuration(time.Since(start))
	result.Summary.DurationNanos = result.Summary.Duration.Nanoseconds()
	return result, nil
}

func inspectDiscoveredFiles(ctx context.Context, root string, files []DiscoveredFile, cfg Config) (InspectResult, error) {
	result := InspectResult{
		FormatVersion: InspectFormatVersion,
		Root:          root,
		Files:         []InspectFile{},
		Warnings:      []Warning{},
	}
	cache := NewSchemaCache(cfg)
	catalogLoad := loadSchemaStoreCatalogAsync(ctx, cache, cfg)
	parsedDocuments := parseDocuments(files, cfg, false)
	loadedCatalog := <-catalogLoad
	if loadedCatalog.err != nil {
		return InspectResult{}, loadedCatalog.err
	}
	if loadedCatalog.warning != nil {
		addInspectWarning(&result, *loadedCatalog.warning)
	}
	catalogMatch, err := catalogMatchMode(cfg.Schemas)
	if err != nil {
		return InspectResult{}, err
	}
	catalogConfig := cfg.Schemas.Catalogs
	catalogConfig.Match = catalogMatch
	displayRoot := schemaDisplayRoot(root)
	for _, parsed := range parsedDocuments {
		file := parsed.file
		out := InspectFile{
			Path:              resultPath(file.RelativePath, file.Path),
			AssociationStatus: InspectAssociationStatusUnassociated,
		}
		if parsed.err != nil {
			if parsed.document != nil {
				out.Format = parsed.document.Format
			}
			out.AssociationStatus = InspectAssociationStatusError
			out.Reason = "parse failed before schema association could be determined"
			out.Message = parsed.err.Error()
			addInspectFile(&result, out)
			continue
		}
		inspectDocumentAssociation(&out, parsed.document, cfg, loadedCatalog.catalog, catalogConfig, displayRoot)
		addInspectFile(&result, out)
	}
	return result, nil
}

func inspectConfiguredFileGroups(ctx context.Context, root string, files []configuredFile) (InspectResult, error) {
	result := InspectResult{FormatVersion: InspectFormatVersion, Root: root, Files: []InspectFile{}, Warnings: []Warning{}}
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
		partial, err := inspectDiscoveredFiles(ctx, root, group.files, group.config)
		if err != nil {
			return InspectResult{}, err
		}
		mergeInspectResult(&result, partial)
	}
	return result, nil
}

func inspectDocumentAssociation(out *InspectFile, document *Document, cfg Config, catalog *schemaStoreCatalog, catalogConfig CatalogConfig, displayRoot string) {
	out.Format = document.Format
	reason := ""
	if document.Schema != "" {
		reason = schemaDeclarationReason(document.SchemaSource)
	} else if match, ok := matchSchemaAssociation(document.RelativePath, cfg.Schemas.Associations, "config-association"); ok {
		document.Schema = match.Association.Schema
		document.SchemaSource = match.Source
		reason = fmt.Sprintf("config association matched file pattern %q", match.Association.File)
	} else if match, ok := matchBuiltinSchemaAssociation(document.RelativePath); ok {
		document.Schema = match.Association.Schema
		document.SchemaSource = match.Source
		reason = fmt.Sprintf("built-in association matched file pattern %q", match.Association.File)
	} else {
		applySchemaStoreAssociation(document, catalog, catalogConfig)
		if document.SchemaMatch != nil {
			reason = document.SchemaMatch.Reason
		}
	}
	out.SchemaSource = document.SchemaSource
	out.SchemaMatch = document.SchemaMatch
	if document.SchemaMatch != nil {
		out.SuggestedAssociation = document.SchemaMatch.SuggestedAssociation
		out.SuggestedCatalogIgnore = document.SchemaMatch.SuggestedCatalogIgnore
	}
	if document.Schema == "" {
		out.AssociationStatus = InspectAssociationStatusUnassociated
		out.Reason = fallback(reason, noSchemaAssociationReason(cfg))
		return
	}
	resolved, err := resolveSchemaURI(document.Schema, document.Path)
	if err != nil {
		out.AssociationStatus = InspectAssociationStatusError
		out.Schema = document.Schema
		out.Reason = fallback(reason, "schema association was found, but its URI could not be resolved")
		out.Message = err.Error()
		return
	}
	out.AssociationStatus = InspectAssociationStatusAssociated
	out.Schema = displaySchema(resolved, displayRoot)
	out.Reason = fallback(reason, schemaDeclarationReason(document.SchemaSource))
}

func schemaDeclarationReason(source string) string {
	switch source {
	case "$schema":
		return "$schema property declared this schema"
	case "yaml-language-server":
		return "yaml-language-server directive declared this schema"
	case "taplo-directive":
		return "Taplo #:schema directive declared this schema"
	default:
		if source != "" {
			return source + " declared this schema"
		}
		return "schema declaration found in file"
	}
}

func noSchemaAssociationReason(cfg Config) string {
	if catalogEnabled(cfg.Schemas) {
		return "no inline schema, config association, built-in association, or catalog match"
	}
	return "no inline schema, config association, or built-in association; catalog matching is disabled"
}

func addInspectFile(result *InspectResult, file InspectFile) {
	result.Files = append(result.Files, file)
	result.Summary.Discovered++
	switch file.AssociationStatus {
	case InspectAssociationStatusAssociated:
		result.Summary.Associated++
	case InspectAssociationStatusError:
		result.Summary.Errors++
		result.Summary.Unassociated++
	default:
		result.Summary.Unassociated++
	}
}

func addInspectWarning(result *InspectResult, warning Warning) {
	result.Warnings = append(result.Warnings, warning)
	result.Summary.Warnings++
}

func mergeInspectResult(result *InspectResult, partial InspectResult) {
	result.Summary.Discovered += partial.Summary.Discovered
	result.Summary.Associated += partial.Summary.Associated
	result.Summary.Unassociated += partial.Summary.Unassociated
	result.Summary.Errors += partial.Summary.Errors
	result.Summary.Warnings += partial.Summary.Warnings
	result.Files = append(result.Files, partial.Files...)
	result.Warnings = append(result.Warnings, partial.Warnings...)
}

func FormatInspectJSON(result InspectResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func FormatInspectText(result InspectResult) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "dollarlint inspected %d discovered file%s: %d associated, %d without schema",
		result.Summary.Discovered,
		plural(result.Summary.Discovered),
		result.Summary.Associated,
		result.Summary.Unassociated,
	)
	if result.Summary.Errors > 0 {
		fmt.Fprintf(&builder, ", %d error%s", result.Summary.Errors, plural(result.Summary.Errors))
	}
	if result.Summary.Warnings > 0 {
		fmt.Fprintf(&builder, ", %d warning%s", result.Summary.Warnings, plural(result.Summary.Warnings))
	}
	builder.WriteString("\n")
	if len(result.Warnings) > 0 {
		writeInspectWarnings(&builder, result.Warnings)
	}
	for _, file := range result.Files {
		fmt.Fprintf(&builder, "\n%s\n", textStyleFile.Render(file.Path))
		if file.Format != "" {
			fmt.Fprintf(&builder, "  %s %s\n", textStyleMuted.Render("format:"), file.Format)
		}
		if file.Schema != "" {
			fmt.Fprintf(&builder, "  %s %s\n", textStyleMuted.Render("schema:"), file.Schema)
		} else {
			fmt.Fprintf(&builder, "  %s %s\n", textStyleMuted.Render("schema:"), "none")
		}
		if file.SchemaSource != "" {
			fmt.Fprintf(&builder, "  %s %s\n", textStyleMuted.Render("source:"), file.SchemaSource)
		}
		if file.Reason != "" {
			writeIndentedValue(&builder, "why:", file.Reason)
		}
		if file.Message != "" {
			writeIndentedValue(&builder, "message:", file.Message)
		}
		if file.SuggestedAssociation != "" {
			writeIndentedValue(&builder, "suggested association:", file.SuggestedAssociation)
		}
		if file.SuggestedCatalogIgnore != "" {
			writeIndentedValue(&builder, "suggested catalog ignore:", file.SuggestedCatalogIgnore)
		}
	}
	return builder.String()
}

func writeInspectWarnings(builder *strings.Builder, warnings []Warning) {
	builder.WriteString("\nwarnings\n")
	for _, warning := range warnings {
		kind := warning.Kind
		if warning.Source != "" {
			kind = warning.Source
		}
		fmt.Fprintf(builder, "  %s  %s\n", textStyleKeyword.Render(kind), warning.Message)
	}
}
