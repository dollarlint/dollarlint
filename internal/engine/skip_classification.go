package engine

import (
	"path"
	"strings"
)

type skipClassification struct {
	Reason     string
	Class      string
	Importance string
	Detail     string
}

func applySkippedFileClassification(file *FileResult, document *Document, reason, detail string) {
	classification := classifySkippedFile(document.RelativePath, reason, detail)
	file.SkipReason = classification.Reason
	file.SkipClass = classification.Class
	file.SkipImportance = classification.Importance
	file.SkipDetail = classification.Detail
}

func classifySkippedFile(rel, reason, detail string) skipClassification {
	if reason == "" {
		reason = SkipReasonNoSchema
	}
	if reason == SkipReasonCatalogSchemaUnavailable {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassExternalCatalog,
			Importance: SkipImportanceMedium,
			Detail:     fallback(detail, "catalog-inferred schema could not be used; this is not a finding in the file"),
		}
	}

	lower := strings.ToLower(cleanGlob(rel))
	base := path.Base(lower)
	if isLockfileSkip(base) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassLockfile,
			Importance: SkipImportanceLow,
			Detail:     fallback(detail, "dependency lockfile without schema coverage"),
		}
	}
	if isKnownUnsupportedConfigSkip(lower, base) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassUnsupportedConfig,
			Importance: SkipImportanceHigh,
			Detail:     fallback(detail, "recognized hand-authored config without built-in or catalog schema support"),
		}
	}
	if isRepoManagementConfigSkip(lower, base) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassRepoManagement,
			Importance: SkipImportanceHigh,
			Detail:     fallback(detail, "repository-management config without schema coverage"),
		}
	}
	if isLocaleDataSkip(lower) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassLocaleData,
			Importance: SkipImportanceLow,
			Detail:     fallback(detail, "locale or translation data without schema coverage"),
		}
	}
	if isTestDataSkip(lower) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassTestData,
			Importance: SkipImportanceLow,
			Detail:     fallback(detail, "test, fixture, or benchmark data without schema coverage"),
		}
	}
	if isApplicationDataSkip(lower) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassApplicationData,
			Importance: SkipImportanceLow,
			Detail:     fallback(detail, "application data without schema coverage"),
		}
	}
	if looksLikeConfigPath(lower, base) {
		return skipClassification{
			Reason:     reason,
			Class:      SkipClassUnknown,
			Importance: SkipImportanceHigh,
			Detail:     fallback(detail, "schema-less file looks like project configuration"),
		}
	}
	return skipClassification{
		Reason:     reason,
		Class:      SkipClassUnknown,
		Importance: SkipImportanceMedium,
		Detail:     fallback(detail, "schema-less JSON/YAML/TOML file"),
	}
}

func isLockfileSkip(base string) bool {
	switch base {
	case "package-lock.json", "npm-shrinkwrap.json", "composer.lock", "pipfile.lock", "poetry.lock", "uv.lock":
		return true
	default:
		return false
	}
}

func isKnownUnsupportedConfigSkip(rel, base string) bool {
	switch base {
	case ".rubocop_todo.yml", ".rubocop_todo.yaml",
		".coveralls.yml", ".coveralls.yaml",
		".clippy.toml", "deny.toml", "release.toml",
		"atmos.yaml", "atmos.yml":
		return true
	}
	return strings.HasSuffix(rel, "/.cargo/config.toml") || rel == ".cargo/config.toml"
}

func isRepoManagementConfigSkip(rel, base string) bool {
	if rel == ".github/settings.yml" || rel == ".github/settings.yaml" ||
		strings.HasSuffix(rel, "/.github/settings.yml") || strings.HasSuffix(rel, "/.github/settings.yaml") {
		return true
	}
	return base == "readme.yaml" || base == "readme.yml"
}

func isLocaleDataSkip(rel string) bool {
	return pathSegmentContains(rel, "locale") || pathSegmentContains(rel, "locales") || pathSegmentContains(rel, "i18n")
}

func isTestDataSkip(rel string) bool {
	for _, segment := range []string{"test", "tests", "spec", "specs", "fixture", "fixtures", "testdata", "benchmark", "benchmarks"} {
		if pathSegmentContains(rel, segment) {
			return true
		}
	}
	for _, part := range strings.Split(rel, "/") {
		if strings.Contains(part, "test") {
			return true
		}
	}
	return false
}

func isApplicationDataSkip(rel string) bool {
	return pathSegmentContains(rel, "data")
}

func looksLikeConfigPath(rel, base string) bool {
	if strings.HasPrefix(base, ".") {
		return true
	}
	for _, token := range []string{"config", "settings", "rc", "tool", "lint"} {
		if strings.Contains(base, token) {
			return true
		}
	}
	return strings.Contains(rel, "/.config/")
}

func pathSegmentContains(rel, segment string) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == segment {
			return true
		}
	}
	return false
}
