package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
)

const defaultSchemaStoreCatalogURL = "https://www.schemastore.org/api/json/catalog.json"

var curatedDefaultSchemaStoreAssociations = []schemaStoreEntry{
	{
		Name:      "rustfmt",
		FileMatch: []string{"rustfmt.toml", ".rustfmt.toml"},
		URL:       "https://www.schemastore.org/rustfmt.json",
	},
	{
		Name:      "release-plz.toml",
		FileMatch: []string{"release-plz.toml", ".release-plz.toml"},
		URL:       "https://raw.githubusercontent.com/MarcoIeni/release-plz/main/.schema/latest.json",
	},
}

type schemaStoreCatalog struct {
	Schemas []schemaStoreEntry `json:"schemas"`

	indexed        bool
	exactPaths     map[string]schemaStoreEntry
	exactBasenames map[string]schemaStoreEntry
	pathGlobs      []schemaStorePattern
	basenameGlobs  []schemaStorePattern
}

type schemaStoreEntry struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	FileMatch []string `json:"fileMatch"`
	URL       string   `json:"url"`
}

type schemaStorePattern struct {
	pattern string
	entry   schemaStoreEntry
}

type schemaStoreMatch struct {
	entry         schemaStoreEntry
	action        string
	reason        string
	confidence    string
	matchType     string
	pattern       string
	ignorePattern string
}

func loadSchemaStoreCatalog(ctx context.Context, cache *SchemaCache, cfg Config) (*schemaStoreCatalog, *Warning, error) {
	if !catalogEnabled(cfg.Schemas) {
		return nil, nil, nil
	}
	catalog := &schemaStoreCatalog{}
	for _, source := range enabledSchemaStoreCatalogSources(cfg.Schemas) {
		loaded, warning, err := loadSchemaStoreCatalogSource(ctx, cache, cfg, source)
		if err != nil || warning != nil {
			return nil, warning, err
		}
		if loaded != nil {
			catalog.Schemas = append(catalog.Schemas, loaded.Schemas...)
		}
	}
	if len(catalog.Schemas) == 0 {
		return nil, nil, nil
	}
	catalog.buildIndex()
	return catalog, nil, nil
}

func enabledSchemaStoreCatalogSources(cfg SchemaConfig) []CatalogSource {
	sources := cfg.Catalogs.Sources
	if len(sources) == 0 {
		sources = []CatalogSource{defaultSchemaStoreCatalogSource()}
	}
	var out []CatalogSource
	for _, source := range sources {
		if source.Enabled != nil && !*source.Enabled {
			continue
		}
		if source.Format == "" {
			source.Format = "schemastore"
		}
		if source.Name == "" && source.Format == "schemastore" {
			source.Name = "schemastore"
		}
		if source.Format == "schemastore" {
			out = append(out, source)
		}
	}
	return out
}

func loadSchemaStoreCatalogSource(ctx context.Context, cache *SchemaCache, cfg Config, source CatalogSource) (*schemaStoreCatalog, *Warning, error) {
	catalogURL := source.URL
	if catalogURL == "" {
		catalogURL = source.Path
	}
	if catalogURL == "" {
		return nil, nil, nil
	}
	if !remoteFetchEnabled(cfg.Schemas) && isRemoteURI(catalogURL) {
		return schemaStoreCatalogError(cfg, source, "catalog %s requires remote schema fetching", catalogURL)
	}
	resolved, err := resolveCatalogURI(catalogURL)
	if err != nil {
		return schemaStoreCatalogError(cfg, source, "parse catalog URL %q: %v", catalogURL, err)
	}
	doc, err := cache.LoadContext(ctx, resolved)
	if err != nil {
		return schemaStoreCatalogError(cfg, source, "load catalog %s: %v", catalogURL, err)
	}
	baseURL, _ := url.Parse(resolved)
	raw, ok := doc.(map[string]any)
	if !ok {
		return schemaStoreCatalogError(cfg, source, "catalog %s is not an object", catalogURL)
	}
	catalog := &schemaStoreCatalog{}
	for _, rawEntry := range asSlice(raw["schemas"]) {
		entryObject, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		entry := schemaStoreEntry{
			Name:      asString(entryObject["name"]),
			Source:    source.Name,
			URL:       resolveCatalogEntryURL(baseURL, asString(entryObject["url"])),
			FileMatch: asStringSlice(entryObject["fileMatch"]),
		}
		if entry.URL == "" || len(entry.FileMatch) == 0 {
			continue
		}
		catalog.Schemas = append(catalog.Schemas, entry)
	}
	addCuratedDefaultSchemaStoreAssociations(catalog, source, catalogURL)
	return catalog, nil, nil
}

func addCuratedDefaultSchemaStoreAssociations(catalog *schemaStoreCatalog, source CatalogSource, catalogURL string) {
	if catalog == nil || catalogURL != defaultSchemaStoreCatalogURL {
		return
	}
	sourceName := source.Name
	if sourceName == "" {
		sourceName = "schemastore"
	}
	for _, association := range curatedDefaultSchemaStoreAssociations {
		association.Source = sourceName
		catalog.Schemas = append(catalog.Schemas, association)
	}
}

func schemaStoreCatalogError(cfg Config, source CatalogSource, format string, args ...any) (*schemaStoreCatalog, *Warning, error) {
	message := fmt.Sprintf(format, args...)
	mode, err := catalogFailureMode(cfg.Schemas)
	if err != nil {
		return nil, nil, err
	}
	switch mode {
	case CatalogFailureError:
		return nil, nil, fmt.Errorf("%s", message)
	case CatalogFailureSkip:
		return nil, nil, nil
	default:
		sourceName := source.Name
		if sourceName == "" {
			sourceName = "catalog"
		}
		return nil, &Warning{
			Kind:    "schemaCatalogUnavailable",
			Source:  sourceName,
			Message: message,
		}, nil
	}
}

func resolveCatalogEntryURL(baseURL *url.URL, raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return baseURL.ResolveReference(parsed).String()
}

func resolveCatalogURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse catalog URL %q: %w", raw, err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	file, _ := fileURL(raw)
	return file.String(), nil
}

func applySchemaStoreAssociation(document *Document, catalog *schemaStoreCatalog, catalogConfig CatalogConfig) {
	if document.Schema != "" || catalog == nil {
		return
	}
	match, ok := catalog.match(document.RelativePath, catalogConfig)
	if !ok {
		return
	}
	document.SchemaMatch = match.schemaMatch(document.RelativePath)
	if match.action != SchemaMatchActionMatched {
		return
	}
	document.Schema = match.entry.URL
	document.SchemaSource = match.entry.schemaSource()
}

func (catalog *schemaStoreCatalog) buildIndex() {
	if catalog == nil || catalog.indexed {
		return
	}
	catalog.indexed = true
	catalog.exactPaths = map[string]schemaStoreEntry{}
	catalog.exactBasenames = map[string]schemaStoreEntry{}
	for _, entry := range catalog.Schemas {
		for _, rawPattern := range entry.FileMatch {
			pattern := cleanGlob(rawPattern)
			if pattern == "" {
				continue
			}
			if hasGlobMeta(pattern) {
				if strings.Contains(pattern, "/") {
					catalog.pathGlobs = append(catalog.pathGlobs, schemaStorePattern{pattern: pattern, entry: entry})
					continue
				}
				catalog.basenameGlobs = append(catalog.basenameGlobs, schemaStorePattern{pattern: pattern, entry: entry})
				continue
			}
			if strings.Contains(pattern, "/") {
				addSchemaStoreExact(catalog.exactPaths, pattern, entry)
				continue
			}
			addSchemaStoreExact(catalog.exactBasenames, pattern, entry)
		}
	}
}

func (catalog *schemaStoreCatalog) match(rel string, catalogConfig CatalogConfig) (schemaStoreMatch, bool) {
	if catalog == nil {
		return schemaStoreMatch{}, false
	}
	matchMode := catalogConfig.Match
	if matchMode == "" {
		matchMode = CatalogMatchAuto
	}
	catalog.buildIndex()
	rel = cleanGlob(rel)
	match, ok := catalog.findMatch(rel, matchMode)
	if !ok {
		return schemaStoreMatch{}, false
	}
	if ignore, ignored := catalogIgnoreMatch(catalogConfig.Ignore, rel); ignored {
		match.action = SchemaMatchActionIgnored
		match.ignorePattern = ignore.File
		match.reason = catalogIgnoredReason(rel, match, ignore)
		return match, true
	}
	match.reason = catalogMatchReason(rel, match)
	return match, true
}

func (catalog *schemaStoreCatalog) findMatch(rel, matchMode string) (schemaStoreMatch, bool) {
	if entry, ok := catalog.exactPaths[rel]; ok {
		return newSchemaStoreMatch(entry, SchemaMatchTypeExactPath, rel, SchemaMatchConfidenceHigh, matchMode), true
	}
	for _, candidate := range catalog.pathGlobs {
		if matchPattern(candidate.pattern, rel) {
			return newSchemaStoreMatch(candidate.entry, SchemaMatchTypePathGlob, candidate.pattern, SchemaMatchConfidenceHigh, matchMode), true
		}
	}
	var lowConfidenceMatch schemaStoreMatch
	var hasLowConfidenceMatch bool
	base := path.Base(rel)
	if entry, ok := catalog.exactBasenames[base]; ok {
		confidence := SchemaMatchConfidenceMedium
		if lowConfidenceSchemaStoreBasename(base) {
			confidence = SchemaMatchConfidenceLow
		}
		match := newSchemaStoreMatch(entry, SchemaMatchTypeExactBasename, base, confidence, matchMode)
		if match.action != SchemaMatchActionSkippedLowConfidence {
			return match, true
		}
		lowConfidenceMatch = match
		hasLowConfidenceMatch = true
	}
	for _, candidate := range catalog.basenameGlobs {
		if !matchPattern(candidate.pattern, rel) {
			continue
		}
		confidence := SchemaMatchConfidenceMedium
		if lowConfidenceSchemaStoreGlob(candidate.pattern) {
			confidence = SchemaMatchConfidenceLow
		}
		match := newSchemaStoreMatch(candidate.entry, SchemaMatchTypeBasenameGlob, candidate.pattern, confidence, matchMode)
		if match.action != SchemaMatchActionSkippedLowConfidence {
			return match, true
		}
		if !hasLowConfidenceMatch {
			lowConfidenceMatch = match
			hasLowConfidenceMatch = true
		}
	}
	if hasLowConfidenceMatch {
		return lowConfidenceMatch, true
	}
	return schemaStoreMatch{}, false
}

func newSchemaStoreMatch(entry schemaStoreEntry, matchType, pattern, confidence, matchMode string) schemaStoreMatch {
	action := SchemaMatchActionMatched
	if matchMode == CatalogMatchAuto && confidence == SchemaMatchConfidenceLow {
		action = SchemaMatchActionSkippedLowConfidence
	}
	return schemaStoreMatch{
		entry:      entry,
		action:     action,
		confidence: confidence,
		matchType:  matchType,
		pattern:    pattern,
	}
}

func catalogIgnoreMatch(rules []CatalogIgnoreRule, rel string) (CatalogIgnoreRule, bool) {
	for i := len(rules) - 1; i >= 0; i-- {
		rule := rules[i]
		if rule.File == "" {
			continue
		}
		if matchPattern(rule.File, rel) {
			return rule, true
		}
	}
	return CatalogIgnoreRule{}, false
}

func catalogMatchReason(rel string, match schemaStoreMatch) string {
	base := path.Base(rel)
	switch match.action {
	case SchemaMatchActionSkippedLowConfidence:
		return fmt.Sprintf("auto catalog matching skipped low-confidence %s", catalogPatternReason(rel, base, match))
	default:
		return catalogPatternReason(rel, base, match)
	}
}

func catalogPatternReason(rel, base string, match schemaStoreMatch) string {
	switch match.matchType {
	case SchemaMatchTypeExactPath:
		return fmt.Sprintf("path %q exactly matched catalog fileMatch %q", rel, match.pattern)
	case SchemaMatchTypePathGlob:
		return fmt.Sprintf("path %q matched catalog fileMatch %q", rel, match.pattern)
	case SchemaMatchTypeExactBasename:
		return fmt.Sprintf("basename %q matched catalog fileMatch %q", base, match.pattern)
	case SchemaMatchTypeBasenameGlob:
		return fmt.Sprintf("basename %q matched catalog fileMatch %q", base, match.pattern)
	default:
		return fmt.Sprintf("path %q matched catalog fileMatch %q", rel, match.pattern)
	}
}

func catalogIgnoredReason(rel string, match schemaStoreMatch, ignore CatalogIgnoreRule) string {
	reason := fmt.Sprintf("catalog match ignored by schemas.catalogs.ignore rule %q", ignore.File)
	if ignore.Reason != "" {
		reason += ": " + ignore.Reason
	}
	reason += " (candidate: " + catalogPatternReason(rel, path.Base(rel), match) + ")"
	return reason
}

func (match schemaStoreMatch) schemaMatch(rel string) *SchemaMatch {
	out := &SchemaMatch{
		Action:        match.action,
		Reason:        match.reason,
		Confidence:    match.confidence,
		MatchType:     match.matchType,
		Pattern:       match.pattern,
		IgnorePattern: match.ignorePattern,
	}
	if match.action == SchemaMatchActionMatched || match.action == SchemaMatchActionSkippedLowConfidence {
		out.SuggestedAssociation = suggestedSchemaAssociation(rel, match.entry.URL)
	}
	if match.action == SchemaMatchActionSkippedLowConfidence {
		out.SuggestedCatalogIgnore = suggestedCatalogIgnore(rel)
	}
	return out
}

func suggestedSchemaAssociation(rel, schema string) string {
	if rel == "" || schema == "" {
		return ""
	}
	return "[[schemas.associations]]\nfile = " + tomlBasicString(rel) + "\nschema = " + tomlBasicString(schema)
}

func suggestedCatalogIgnore(rel string) string {
	if rel == "" {
		return ""
	}
	return "[[schemas.catalogs.ignore]]\nfile = " + tomlBasicString(rel) + "\nreason = \"not this catalog schema\""
}

func tomlBasicString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func addSchemaStoreExact(entries map[string]schemaStoreEntry, key string, entry schemaStoreEntry) {
	if _, exists := entries[key]; exists {
		return
	}
	entries[key] = entry
}

func lowConfidenceSchemaStoreBasename(base string) bool {
	switch strings.ToLower(base) {
	case "config.json",
		"configuration.json",
		"extensions.json",
		"launch.json",
		"manifest.json",
		"schema.json",
		"settings.json",
		"task.json",
		"tasks.json":
		return true
	default:
		return false
	}
}

func lowConfidenceSchemaStoreGlob(pattern string) bool {
	if strings.Contains(pattern, "/") {
		return false
	}
	if highConfidenceSchemaStoreBasenameGlob(pattern) {
		return false
	}
	return strings.HasPrefix(pattern, "*")
}

func highConfidenceSchemaStoreBasenameGlob(pattern string) bool {
	switch strings.ToLower(pattern) {
	case "*.rubocop.yml", "*.rubocop.yaml":
		return true
	default:
		return false
	}
}

func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[{")
}

func (entry schemaStoreEntry) schemaSource() string {
	source := "catalog"
	if entry.Source != "" {
		source += ":" + entry.Source
	}
	if entry.Name != "" {
		source += ":" + entry.Name
	}
	return source
}

func asSlice(value any) []any {
	typed, ok := value.([]any)
	if !ok {
		return nil
	}
	return typed
}

func asString(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

func asStringSlice(value any) []string {
	var out []string
	for _, item := range asSlice(value) {
		if text := asString(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}
