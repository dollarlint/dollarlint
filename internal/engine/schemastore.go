package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const defaultSchemaStoreCatalogURL = "https://www.schemastore.org/api/json/catalog.json"
const rubySchemaBaseURL = "https://www.rubyschema.org"

const (
	catalogFormatSchemaStore = "schemastore"
	catalogFormatRubySchema  = "rubyschema"
)

const (
	catalogEvidenceRubyProject = "ruby-project"
	catalogEvidenceRails       = "rails"
	catalogEvidencePackwerk    = "packwerk"
)

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

var rubySchemaCatalogEntries = []schemaStoreEntry{
	{
		Name:             "Rails database.yml",
		FileMatch:        []string{"config/database.yml", "config/database.yaml", "**/config/database.yml", "**/config/database.yaml"},
		URL:              rubySchemaBaseURL + "/rails/database.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:             "Rails storage.yml",
		FileMatch:        []string{"config/storage.yml", "config/storage.yaml", "**/config/storage.yml", "**/config/storage.yaml"},
		URL:              rubySchemaBaseURL + "/rails/storage.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:             "Rails cable.yml",
		FileMatch:        []string{"config/cable.yml", "config/cable.yaml", "**/config/cable.yml", "**/config/cable.yaml"},
		URL:              rubySchemaBaseURL + "/rails/cable.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:             "Rails cache.yml",
		FileMatch:        []string{"config/cache.yml", "config/cache.yaml", "**/config/cache.yml", "**/config/cache.yaml"},
		URL:              rubySchemaBaseURL + "/rails/cache.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:             "Rails queue.yml",
		FileMatch:        []string{"config/queue.yml", "config/queue.yaml", "**/config/queue.yml", "**/config/queue.yaml"},
		URL:              rubySchemaBaseURL + "/rails/queue.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:             "Rails recurring.yml",
		FileMatch:        []string{"config/recurring.yml", "config/recurring.yaml", "**/config/recurring.yml", "**/config/recurring.yaml"},
		URL:              rubySchemaBaseURL + "/rails/recurring.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:             "Kamal deploy.yml",
		FileMatch:        []string{"config/deploy.yml", "config/deploy.yaml", "config/deploy*.yml", "config/deploy*.yaml", "**/config/deploy.yml", "**/config/deploy.yaml", "**/config/deploy*.yml", "**/config/deploy*.yaml"},
		URL:              rubySchemaBaseURL + "/kamal/deploy.json",
		RequiredEvidence: catalogEvidenceRubyProject,
	},
	{
		Name:      "Lefthook",
		FileMatch: []string{"lefthook.yml", "lefthook.yaml", "lefthook.json", "lefthook.toml"},
		URL:       rubySchemaBaseURL + "/lefthook.json",
	},
	{
		Name:      "RuboCop",
		FileMatch: []string{".rubocop.yml", ".rubocop.yaml", ".rubocop_todo.yml", ".rubocop_todo.yaml", "*.rubocop.yml", "*.rubocop.yaml", "*.rubocop_todo.yml", "*.rubocop_todo.yaml"},
		URL:       rubySchemaBaseURL + "/rubocop.json",
	},
	{
		Name:      "Standard",
		FileMatch: []string{".standard.yml", ".standard.yaml"},
		URL:       rubySchemaBaseURL + "/standard.json",
	},
	{
		Name:             "Packwerk package.yml",
		FileMatch:        []string{"packs/*/package.yml", "packs/*/package.yaml", "packs/**/package.yml", "packs/**/package.yaml", "components/**/package.yml", "components/**/package.yaml"},
		URL:              rubySchemaBaseURL + "/packwerk/package.json",
		RequiredEvidence: catalogEvidencePackwerk,
	},
	{
		Name:      "Sidekiq",
		FileMatch: []string{"sidekiq.yml", "sidekiq.yaml", "config/sidekiq.yml", "config/sidekiq.yaml", "**/config/sidekiq.yml", "**/config/sidekiq.yaml"},
		URL:       rubySchemaBaseURL + "/sidekiq.json",
	},
	{
		Name:      "Shoryuken",
		FileMatch: []string{"shoryuken.yml", "shoryuken.yaml", "config/shoryuken.yml", "config/shoryuken.yaml", "**/config/shoryuken.yml", "**/config/shoryuken.yaml"},
		URL:       rubySchemaBaseURL + "/shoryuken.json",
	},
	{
		Name:      "Honeybadger",
		FileMatch: []string{"honeybadger.yml", "honeybadger.yaml", "config/honeybadger.yml", "config/honeybadger.yaml", "**/config/honeybadger.yml", "**/config/honeybadger.yaml"},
		URL:       rubySchemaBaseURL + "/honeybadger.json",
	},
	{
		Name:      "RoRvsWild",
		FileMatch: []string{"rorvswild.yml", "rorvswild.yaml", "config/rorvswild.yml", "config/rorvswild.yaml", "**/config/rorvswild.yml", "**/config/rorvswild.yaml"},
		URL:       rubySchemaBaseURL + "/rorvswild.json",
	},
	{
		Name:      "Scout APM",
		FileMatch: []string{"scout_apm.yml", "scout_apm.yaml", "config/scout_apm.yml", "config/scout_apm.yaml", "**/config/scout_apm.yml", "**/config/scout_apm.yaml"},
		URL:       rubySchemaBaseURL + "/scout_apm.json",
	},
	{
		Name:      "PgHero",
		FileMatch: []string{"pghero.yml", "pghero.yaml", "config/pghero.yml", "config/pghero.yaml", "**/config/pghero.yml", "**/config/pghero.yaml"},
		URL:       rubySchemaBaseURL + "/pghero.json",
	},
	{
		Name:             "Rails I18n locale",
		FileMatch:        []string{"config/locales/*.yml", "config/locales/*.yaml", "config/locales/**/*.yml", "config/locales/**/*.yaml", "**/config/locales/*.yml", "**/config/locales/*.yaml", "**/config/locales/**/*.yml", "**/config/locales/**/*.yaml"},
		URL:              rubySchemaBaseURL + "/i18n/locale.json",
		RequiredEvidence: catalogEvidenceRails,
	},
	{
		Name:      "i18n-tasks",
		FileMatch: []string{".i18n-tasks.yml", ".i18n-tasks.yaml", "i18n-tasks.yml", "i18n-tasks.yaml"},
		URL:       rubySchemaBaseURL + "/i18n-tasks.json",
	},
	{
		Name:             "Mongoid",
		FileMatch:        []string{"mongoid.yml", "mongoid.yaml", "config/mongoid.yml", "config/mongoid.yaml", "**/config/mongoid.yml", "**/config/mongoid.yaml"},
		URL:              rubySchemaBaseURL + "/mongoid.json",
		RequiredEvidence: catalogEvidenceRubyProject,
	},
	{
		Name:             "Vite Ruby",
		FileMatch:        []string{"vite.json", "config/vite.json", "**/config/vite.json"},
		URL:              rubySchemaBaseURL + "/vite.json",
		RequiredEvidence: catalogEvidenceRubyProject,
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
	Name             string   `json:"name"`
	Source           string   `json:"source"`
	FileMatch        []string `json:"fileMatch"`
	URL              string   `json:"url"`
	RequiredEvidence string   `json:"-"`
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
	evidence      string
}

type catalogMatchContext struct {
	relativePath string
	absolutePath string
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
		sources = defaultCatalogSources()
	}
	var out []CatalogSource
	for _, source := range sources {
		if source.Enabled != nil && !*source.Enabled {
			continue
		}
		source = normalizeCatalogSource(source)
		if source.Format == catalogFormatSchemaStore || source.Format == catalogFormatRubySchema {
			out = append(out, source)
		}
	}
	return out
}

func normalizeCatalogSource(source CatalogSource) CatalogSource {
	if source.Format == "" {
		if source.Name == catalogFormatRubySchema && source.URL == "" && source.Path == "" {
			source.Format = catalogFormatRubySchema
		} else {
			source.Format = catalogFormatSchemaStore
		}
	}
	if source.Name == "" {
		source.Name = source.Format
	}
	return source
}

func loadSchemaStoreCatalogSource(ctx context.Context, cache *SchemaCache, cfg Config, source CatalogSource) (*schemaStoreCatalog, *Warning, error) {
	source = normalizeCatalogSource(source)
	if source.Format == catalogFormatRubySchema {
		return loadRubySchemaCatalogSource(source), nil, nil
	}
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

func loadRubySchemaCatalogSource(source CatalogSource) *schemaStoreCatalog {
	sourceName := source.Name
	if sourceName == "" {
		sourceName = catalogFormatRubySchema
	}
	catalog := &schemaStoreCatalog{Schemas: make([]schemaStoreEntry, 0, len(rubySchemaCatalogEntries))}
	for _, entry := range rubySchemaCatalogEntries {
		entry.Source = sourceName
		catalog.Schemas = append(catalog.Schemas, entry)
	}
	return catalog
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
	match, ok := catalog.matchDocument(document, catalogConfig)
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
	return catalog.matchWithContext(catalogMatchContext{relativePath: rel}, catalogConfig)
}

func (catalog *schemaStoreCatalog) matchDocument(document *Document, catalogConfig CatalogConfig) (schemaStoreMatch, bool) {
	if document == nil {
		return schemaStoreMatch{}, false
	}
	return catalog.matchWithContext(catalogMatchContext{
		relativePath: document.RelativePath,
		absolutePath: document.Path,
	}, catalogConfig)
}

func (catalog *schemaStoreCatalog) matchWithContext(ctx catalogMatchContext, catalogConfig CatalogConfig) (schemaStoreMatch, bool) {
	if catalog == nil {
		return schemaStoreMatch{}, false
	}
	matchMode := catalogConfig.Match
	if matchMode == "" {
		matchMode = CatalogMatchAuto
	}
	catalog.buildIndex()
	rel := cleanGlob(ctx.relativePath)
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
	match = applyCatalogEvidence(ctx, match)
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
	case SchemaMatchActionSkippedMissingEvidence:
		return fmt.Sprintf("catalog candidate skipped because %s requires %s evidence (%s)", catalogPatternReason(rel, base, match), catalogEvidenceLabel(match.entry.RequiredEvidence), catalogMissingEvidenceHint(match.entry.RequiredEvidence))
	default:
		reason := catalogPatternReason(rel, base, match)
		if match.evidence != "" {
			reason += "; " + match.evidence
		}
		return reason
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

func applyCatalogEvidence(ctx catalogMatchContext, match schemaStoreMatch) schemaStoreMatch {
	if match.entry.RequiredEvidence == "" {
		return match
	}
	ok, reason := catalogEvidenceSatisfied(ctx, match.entry.RequiredEvidence)
	if ok {
		match.evidence = reason
		return match
	}
	match.action = SchemaMatchActionSkippedMissingEvidence
	match.confidence = SchemaMatchConfidenceLow
	return match
}

func catalogEvidenceSatisfied(ctx catalogMatchContext, required string) (bool, string) {
	switch required {
	case catalogEvidenceRubyProject:
		return rubyProjectEvidence(ctx)
	case catalogEvidenceRails:
		return railsProjectEvidence(ctx)
	case catalogEvidencePackwerk:
		return packwerkEvidence(ctx)
	default:
		return true, ""
	}
}

func rubyProjectEvidence(ctx catalogMatchContext) (bool, string) {
	if found := firstEvidenceFile(ctx, "Gemfile", "Gemfile.lock", ".ruby-version"); found != "" {
		return true, fmt.Sprintf("Ruby project evidence found at %s", filepath.ToSlash(found))
	}
	if found := firstEvidenceGlob(ctx, "*.gemspec"); found != "" {
		return true, fmt.Sprintf("Ruby project evidence found at %s", filepath.ToSlash(found))
	}
	return false, ""
}

func railsProjectEvidence(ctx catalogMatchContext) (bool, string) {
	if found := firstEvidenceFile(ctx, "config/application.rb", "bin/rails"); found != "" {
		return true, fmt.Sprintf("Rails project evidence found at %s", filepath.ToSlash(found))
	}
	if found := firstEvidenceFileContaining(ctx, "rails", "Gemfile", "Gemfile.lock"); found != "" {
		return true, fmt.Sprintf("Rails project evidence found in %s", filepath.ToSlash(found))
	}
	return false, ""
}

func packwerkEvidence(ctx catalogMatchContext) (bool, string) {
	if found := firstEvidenceFile(ctx, "packwerk.yml", "packwerk.yaml"); found != "" {
		return true, fmt.Sprintf("Packwerk evidence found at %s", filepath.ToSlash(found))
	}
	if found := firstEvidenceFileContaining(ctx, "packwerk", "Gemfile", "Gemfile.lock"); found != "" {
		return true, fmt.Sprintf("Packwerk evidence found in %s", filepath.ToSlash(found))
	}
	return false, ""
}

func catalogEvidenceLabel(required string) string {
	switch required {
	case catalogEvidenceRubyProject:
		return "Ruby project"
	case catalogEvidenceRails:
		return "Rails project"
	case catalogEvidencePackwerk:
		return "Packwerk"
	default:
		return "project"
	}
}

func catalogMissingEvidenceHint(required string) string {
	switch required {
	case catalogEvidenceRubyProject:
		return "expected Gemfile, Gemfile.lock, .ruby-version, or a .gemspec nearby"
	case catalogEvidenceRails:
		return "expected config/application.rb, bin/rails, or a Gemfile/Gemfile.lock containing rails nearby"
	case catalogEvidencePackwerk:
		return "expected packwerk.yml or a Gemfile/Gemfile.lock containing packwerk nearby"
	default:
		return "expected a nearby project marker"
	}
}

func firstEvidenceFile(ctx catalogMatchContext, names ...string) string {
	for _, dir := range evidenceSearchDirs(ctx) {
		for _, name := range names {
			candidate := filepath.Join(dir, filepath.FromSlash(name))
			if evidenceFileExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func firstEvidenceGlob(ctx catalogMatchContext, pattern string) string {
	for _, dir := range evidenceSearchDirs(ctx) {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil || len(matches) == 0 {
			continue
		}
		return matches[0]
	}
	return ""
}

func firstEvidenceFileContaining(ctx catalogMatchContext, needle string, names ...string) string {
	needle = strings.ToLower(needle)
	for _, dir := range evidenceSearchDirs(ctx) {
		for _, name := range names {
			candidate := filepath.Join(dir, filepath.FromSlash(name))
			data, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(string(data)), needle) {
				return candidate
			}
		}
	}
	return ""
}

func evidenceFileExists(candidate string) bool {
	info, err := os.Stat(candidate)
	return err == nil && !info.IsDir()
}

func evidenceSearchDirs(ctx catalogMatchContext) []string {
	if ctx.absolutePath == "" {
		return nil
	}
	start := filepath.Dir(filepath.Clean(ctx.absolutePath))
	var dirs []string
	for dir, hops := start, 0; dir != "" && hops < 8; dir, hops = filepath.Dir(dir), hops+1 {
		dirs = append(dirs, dir)
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
		if evidenceFileExists(filepath.Join(dir, ".git")) {
			break
		}
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			break
		}
	}
	return dirs
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
	if match.action == SchemaMatchActionMatched || match.action == SchemaMatchActionSkippedLowConfidence || match.action == SchemaMatchActionSkippedMissingEvidence {
		out.SuggestedAssociation = suggestedSchemaAssociation(rel, match.entry.URL)
	}
	if match.action == SchemaMatchActionSkippedLowConfidence || match.action == SchemaMatchActionSkippedMissingEvidence {
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
	case "*.rubocop.yml", "*.rubocop.yaml", "*.rubocop_todo.yml", "*.rubocop_todo.yaml":
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
